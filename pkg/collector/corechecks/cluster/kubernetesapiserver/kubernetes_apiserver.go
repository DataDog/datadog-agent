// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubernetesapiserver implements the Kubernetes API Server cluster
// check.
package kubernetesapiserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	// CheckName is the name of the check
	CheckName = "kubernetes_apiserver"

	KubeControlPaneCheck          = "kube_apiserver_controlplane.up"
	eventTokenKey                 = "event"
	maxEventCardinality           = 300
	defaultResyncPeriodInSecond   = 300
	defaultTimeoutEventCollection = 2000
)

var (
	kubeEvents = telemetry.NewCounterWithOpts(
		CheckName,
		"kube_events",
		[]string{"kind", "component", "type", "reason", "source"},
		"Number of Kubernetes events received by the check.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	emittedEvents = telemetry.NewCounterWithOpts(
		CheckName,
		"emitted_events",
		[]string{"kind", "type", "source", "is_bundled"},
		"Number of events emitted by the check.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)

// KubeASConfig is the config of the API server.
type KubeASConfig struct {
	CollectOShiftQuotas bool `yaml:"collect_openshift_clusterquotas"`
	LeaderSkip          bool `yaml:"skip_leader_election"`
	UseComponentStatus  bool `yaml:"use_component_status"`

	// Event collection configuration
	CollectEvent             bool `yaml:"collect_events"`
	MaxEventCollection       int  `yaml:"max_events_per_run"`
	EventCollectionTimeoutMs int  `yaml:"kubernetes_event_read_timeout_ms"`
	ResyncPeriodEvents       int  `yaml:"kubernetes_event_resync_period_s"`
	UnbundleEvents           bool `yaml:"unbundle_events"`
	BundleUnspecifiedEvents  bool `yaml:"bundle_unspecified_events"`

	// FilteredEventTypes is a slice of kubernetes field selectors that
	// works as a deny list of events to filter out.
	FilteredEventTypes []string `yaml:"filtered_event_types"`

	// CollectedEventTypes specifies which events to collect.
	// Only effective when UnbundleEvents = true
	CollectedEventTypes []collectedEventType `yaml:"collected_event_types"`
	FilteringEnabled    bool                 `yaml:"filtering_enabled"`
}

type collectedEventType struct {
	Kind    string   `yaml:"kind"`
	Source  string   `yaml:"source"`
	Reasons []string `yaml:"reasons"`
}

type eventTransformer interface {
	Transform([]*v1.Event) ([]event.Event, []error)
}

type noopEventTransformer struct{}

func (noopEventTransformer) Transform(_ []*v1.Event) ([]event.Event, []error) {
	return nil, nil
}

type eventCollection struct {
	LastResVer  string
	LastTime    time.Time
	Filter      string
	Transformer eventTransformer
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	core.CheckBase
	instance        *KubeASConfig
	eventCollection eventCollection
	ac              *apiserver.APIClient
	oshiftAPILevel  apiserver.OpenShiftAPILevel
	tagger          tagger.Component
}

func (c *KubeASConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = pkgconfigsetup.Datadog().GetBool("collect_kubernetes_events")
	c.CollectOShiftQuotas = true
	c.ResyncPeriodEvents = defaultResyncPeriodInSecond
	c.UseComponentStatus = true

	return yaml.Unmarshal(data, c)
}

// NewKubeASCheck returns a new KubeASCheck
func NewKubeASCheck(base core.CheckBase, instance *KubeASConfig, tagger tagger.Component) *KubeASCheck {
	return &KubeASCheck{
		CheckBase: base,
		instance:  instance,
		tagger:    tagger,
	}
}

// Factory creates a new check factory
func Factory(tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(
		func() check.Check {
			return newCheck(tagger)
		},
	)
}

func newCheck(tagger tagger.Component) check.Check {
	return NewKubeASCheck(core.NewCheckBase(CheckName), &KubeASConfig{}, tagger)
}

// Configure parses the check configuration and init the check.
func (k *KubeASCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := k.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	// Check connectivity to the APIServer
	err = k.instance.parse(config)
	if err != nil {
		log.Error("could not parse the config for the API server")
		return err
	}
	if k.instance.EventCollectionTimeoutMs == 0 {
		k.instance.EventCollectionTimeoutMs = defaultTimeoutEventCollection
	}

	if k.instance.MaxEventCollection == 0 {
		k.instance.MaxEventCollection = maxEventCardinality
	}

	hostnameDetected, _ := hostname.Get(context.TODO())
	clusterName := clustername.GetRFC1123CompliantClusterName(context.TODO(), hostnameDetected)

	// Automatically add events based on activated Datadog products
	if pkgconfigsetup.Datadog().GetBool("autoscaling.workload.enabled") {
		k.instance.CollectedEventTypes = append(k.instance.CollectedEventTypes, collectedEventType{
			Source: "datadog-workload-autoscaler",
		})
	}

	// When we use both bundled and unbundled transformers, we apply two filters: filtered_event_types and collected_event_types.
	// When we use only the bundled transformer, we apply filtered_event_types.
	// When we use only the unbundled transformer, we apply collected_event_types.
	if (k.instance.UnbundleEvents && k.instance.BundleUnspecifiedEvents) || !k.instance.UnbundleEvents {
		k.eventCollection.Filter = convertFilters(k.instance.FilteredEventTypes)
	}

	if k.instance.UnbundleEvents {
		k.eventCollection.Transformer = newUnbundledTransformer(clusterName, k.tagger, k.instance.CollectedEventTypes, k.instance.BundleUnspecifiedEvents, k.instance.FilteringEnabled)
	} else {
		k.eventCollection.Transformer = newBundledTransformer(clusterName, k.tagger, k.instance.CollectedEventTypes, k.instance.FilteringEnabled)
	}

	return nil
}

// Run executes the check.
func (k *KubeASCheck) Run() error {
	sender, err := k.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	if pkgconfigsetup.Datadog().GetBool("cluster_agent.enabled") {
		log.Debug("Cluster agent is enabled. Not running Kubernetes API Server check or collecting Kubernetes Events.")
		return nil
	}
	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// The Cluster Agent will passed in the `skip_leader_election` bool.
	if !k.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !pkgconfigsetup.Datadog().GetBool("leader_election") {
			return log.Error("Leader Election not enabled. Not running Kubernetes API Server check or collecting Kubernetes Events.")
		}
		leader, errLeader := cluster.RunLeaderElection()
		if errLeader != nil {
			if errLeader == apiserver.ErrNotLeader {
				// Only the leader can instantiate the apiserver client.
				log.Debugf("Not leader (leader is %q). Skipping the Kubernetes API Server check", leader)
				return nil
			}

			_ = k.Warn("Leader Election error. Not running the Kubernetes API Server check.")
			return err
		}

		log.Tracef("Current leader: %q, running the Kubernetes API Server check", leader)
	}
	// API Server client initialisation on first run
	if k.ac == nil {
		// Using GetAPIClient (no wait) as check we'll naturally retry with each check run
		k.ac, err = apiserver.GetAPIClient()
		if err != nil {
			k.Warnf("Could not connect to apiserver: %s", err) //nolint:errcheck
			return err
		}

		// We detect OpenShift presence for quota collection
		if k.instance.CollectOShiftQuotas {
			k.oshiftAPILevel = k.ac.DetectOpenShiftAPILevel()
		}
	}

	// Running the Control Plane status check.
	if k.instance.UseComponentStatus {
		err = k.componentStatusCheck(sender)
		if err != nil {
			k.Warnf("Could not collect control plane status from ComponentStatus: %s", err.Error()) //nolint:errcheck
		}
	} else {
		err = k.controlPlaneHealthCheck(context.TODO(), sender)
		if err != nil {
			k.Warnf("Could not collect control plane status from health checks: %s", err.Error()) //nolint:errcheck
		}
	}

	// Running OpenShift ClusterResourceQuota collection if available
	if k.instance.CollectOShiftQuotas && k.oshiftAPILevel != apiserver.NotOpenShift {
		quotas, err := k.retrieveOShiftClusterQuotas()
		if err != nil {
			k.Warnf("Could not collect OpenShift cluster quotas: %s", err.Error()) //nolint:errcheck
		} else {
			k.reportClusterQuotas(quotas, sender)
		}
	}

	if k.instance.CollectEvent {
		events, err := k.eventCollectionCheck()
		if err != nil {
			return err
		}

		for _, event := range events {
			sender.Event(event)
		}
	}

	return nil
}

func (k *KubeASCheck) eventCollectionCheck() ([]event.Event, error) {
	resVer, lastTime, err := k.ac.GetTokenFromConfigmap(eventTokenKey)
	if err != nil {
		return nil, err
	}

	// This is to avoid getting in a situation where we list all the events
	// for multiple runs in a row.
	if resVer == "" && k.eventCollection.LastResVer != "" {
		log.Errorf("Resource Version stored in the ConfigMap is incorrect. Will resume collecting from %s", k.eventCollection.LastResVer)
		resVer = k.eventCollection.LastResVer
	}

	timeout := int64(k.instance.EventCollectionTimeoutMs / 1000)
	limit := int64(k.instance.MaxEventCollection)
	resync := int64(k.instance.ResyncPeriodEvents)

	var kubeEvents []*v1.Event
	kubeEvents, resVer, lastTime, err = k.ac.RunEventCollection(
		resVer,
		lastTime,
		timeout,
		limit,
		resync,
		k.eventCollection.Filter,
	)
	if err != nil {
		k.Warnf("Could not collect events from the api server: %s", err.Error()) //nolint:errcheck
		return nil, err
	}

	k.eventCollection.LastResVer = resVer
	k.eventCollection.LastTime = lastTime

	err = k.ac.UpdateTokenInConfigmap(eventTokenKey, resVer, lastTime)
	if err != nil {
		k.Warnf("Could not store the LastEventToken in the ConfigMap: %s", err.Error()) //nolint:errcheck
	}

	events, errs := k.eventCollection.Transformer.Transform(kubeEvents)

	for _, err := range errs {
		k.Warnf("Error transforming events: %s", err.Error()) //nolint:errcheck
	}

	return events, nil
}

func (k *KubeASCheck) parseComponentStatus(sender sender.Sender, componentsStatus *v1.ComponentStatusList) error {
	for _, component := range componentsStatus.Items {
		if component.ObjectMeta.Name == "" {
			return errors.New("metadata structure has changed. Not collecting API Server's Components status")
		}
		if component.Conditions == nil || component.Name == "" {
			log.Debug("API Server component's structure is not expected")
			continue
		}

		for _, condition := range component.Conditions {
			statusCheck := servicecheck.ServiceCheckUnknown
			message := ""

			// We only expect the Healthy condition. May change in the future. https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#typical-status-properties
			if condition.Type != "Healthy" {
				log.Debugf("Condition %q not supported", condition.Type)
				continue
			}

			// We only expect True, False and Unknown (default).
			switch condition.Status {
			case "True":
				statusCheck = servicecheck.ServiceCheckOK
				message = condition.Message
			case "False":
				statusCheck = servicecheck.ServiceCheckCritical
				message = condition.Error
				if message == "" {
					message = condition.Message
				}
			}

			tags := []string{fmt.Sprintf("component:%s", component.Name)}
			sender.ServiceCheck(KubeControlPaneCheck, statusCheck, "", tags, message)
		}
	}
	return nil
}

func (k *KubeASCheck) componentStatusCheck(sender sender.Sender) error {
	componentsStatus, err := k.ac.ComponentStatuses()
	if err != nil {
		return err
	}

	return k.parseComponentStatus(sender, componentsStatus)
}

func (k *KubeASCheck) controlPlaneHealthCheck(ctx context.Context, sender sender.Sender) error {
	ready, err := k.ac.IsAPIServerReady(ctx)

	var (
		msg    string
		status servicecheck.ServiceCheckStatus
	)

	if ready {
		msg = "ok"
		status = servicecheck.ServiceCheckOK
	} else {
		status = servicecheck.ServiceCheckCritical
		if err != nil {
			msg = err.Error()
		} else {
			msg = "unknown error"
		}
	}

	sender.ServiceCheck(KubeControlPaneCheck, status, "", nil, msg)

	return nil
}

func convertFilters(conf []string) string {
	var formatedFilters []string
	for _, filter := range conf {
		f := strings.Split(filter, "=")
		if len(f) == 1 {
			formatedFilters = append(formatedFilters, fmt.Sprintf("reason!=%s", f[0]))
			continue
		}
		formatedFilters = append(formatedFilters, filter)
	}
	return strings.Join(formatedFilters, ",")
}
