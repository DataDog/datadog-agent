// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package kubernetesapiserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cache "github.com/patrickmn/go-cache"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	KubeControlPaneCheck          = "kube_apiserver_controlplane.up"
	kubernetesAPIServerCheckName  = "kubernetes_apiserver"
	eventTokenKey                 = "event"
	maxEventCardinality           = 300
	defaultResyncPeriodInSecond   = 300
	defaultTimeoutEventCollection = 2000

	defaultCacheExpire = 2 * time.Minute
	defaultCachePurge  = 10 * time.Minute
)

var (
	kubeEvents = telemetry.NewCounterWithOpts(
		kubernetesAPIServerCheckName,
		"kube_events",
		[]string{"kind", "component", "type", "reason"},
		"Number of Kubernetes events received by the check.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	emittedEvents = telemetry.NewCounterWithOpts(
		kubernetesAPIServerCheckName,
		"emitted_events",
		[]string{"kind", "type"},
		"Number of events emitted by the check.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)

// KubeASConfig is the config of the API server.
type KubeASConfig struct {
	CollectEvent             bool     `yaml:"collect_events"`
	CollectOShiftQuotas      bool     `yaml:"collect_openshift_clusterquotas"`
	FilteredEventTypes       []string `yaml:"filtered_event_types"`
	EventCollectionTimeoutMs int      `yaml:"kubernetes_event_read_timeout_ms"`
	MaxEventCollection       int      `yaml:"max_events_per_run"`
	LeaderSkip               bool     `yaml:"skip_leader_election"`
	ResyncPeriodEvents       int      `yaml:"kubernetes_event_resync_period_s"`
	UseComponentStatus       bool     `yaml:"use_component_status"`
}

// EventC holds the information pertaining to which event we collected last and when we last re-synced.
type EventC struct {
	LastResVer string
	LastTime   time.Time
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	core.CheckBase
	instance        *KubeASConfig
	eventCollection EventC
	ignoredEvents   string
	ac              *apiserver.APIClient
	oshiftAPILevel  apiserver.OpenShiftAPILevel
	providerIDCache *cache.Cache
}

func (c *KubeASConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = config.Datadog.GetBool("collect_kubernetes_events")
	c.CollectOShiftQuotas = true
	c.ResyncPeriodEvents = defaultResyncPeriodInSecond
	c.UseComponentStatus = true

	return yaml.Unmarshal(data, c)
}

// NewKubeASCheck returns a new KubeASCheck
func NewKubeASCheck(base core.CheckBase, instance *KubeASConfig) *KubeASCheck {
	return &KubeASCheck{
		CheckBase:       base,
		instance:        instance,
		providerIDCache: cache.New(defaultCacheExpire, defaultCachePurge),
	}
}

// KubernetesASFactory is exported for integration testing.
func KubernetesASFactory() check.Check {
	return NewKubeASCheck(core.NewCheckBase(kubernetesAPIServerCheckName), &KubeASConfig{})
}

// Configure parses the check configuration and init the check.
func (k *KubeASCheck) Configure(config, initConfig integration.Data, source string) error {
	err := k.CommonConfigure(initConfig, config, source)
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
	k.ignoredEvents = convertFilter(k.instance.FilteredEventTypes)

	return nil
}

func convertFilter(conf []string) string {
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

// Run executes the check.
func (k *KubeASCheck) Run() error {
	sender, err := k.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	if config.Datadog.GetBool("cluster_agent.enabled") {
		log.Debug("Cluster agent is enabled. Not running Kubernetes API Server check or collecting Kubernetes Events.")
		return nil
	}
	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// The Cluster Agent will passed in the `skip_leader_election` bool.
	if !k.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !config.Datadog.GetBool("leader_election") {
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

	// Running the event collection.
	if !k.instance.CollectEvent {
		return nil
	}

	// Get the events from the API server
	events, err := k.eventCollectionCheck()
	if err != nil {
		return err
	}

	// Process the events to have a Datadog format.
	err = k.processEvents(sender, events)
	if err != nil {
		k.Warnf("Could not submit new event %s", err.Error()) //nolint:errcheck
	}
	return nil
}

func (k *KubeASCheck) eventCollectionCheck() (newEvents []*v1.Event, err error) {
	resVer, lastTime, err := k.ac.GetTokenFromConfigmap(eventTokenKey)
	if err != nil {
		return nil, err
	}

	// This is to avoid getting in a situation where we list all the events for multiple runs in a row.
	if resVer == "" && k.eventCollection.LastResVer != "" {
		log.Errorf("Resource Version stored in the ConfigMap is incorrect. Will resume collecting from %s", k.eventCollection.LastResVer)
		resVer = k.eventCollection.LastResVer
	}

	timeout := int64(k.instance.EventCollectionTimeoutMs / 1000)
	limit := int64(k.instance.MaxEventCollection)
	resync := int64(k.instance.ResyncPeriodEvents)
	newEvents, k.eventCollection.LastResVer, k.eventCollection.LastTime, err = k.ac.RunEventCollection(resVer, lastTime, timeout, limit, resync, k.ignoredEvents)

	if err != nil {
		k.Warnf("Could not collect events from the api server: %s", err.Error()) //nolint:errcheck
		return nil, err
	}

	configMapErr := k.ac.UpdateTokenInConfigmap(eventTokenKey, k.eventCollection.LastResVer, k.eventCollection.LastTime)
	if configMapErr != nil {
		k.Warnf("Could not store the LastEventToken in the ConfigMap: %s", configMapErr.Error()) //nolint:errcheck
	}
	return newEvents, nil
}

func (k *KubeASCheck) parseComponentStatus(sender aggregator.Sender, componentsStatus *v1.ComponentStatusList) error {
	for _, component := range componentsStatus.Items {
		if component.ObjectMeta.Name == "" {
			return errors.New("metadata structure has changed. Not collecting API Server's Components status")
		}
		if component.Conditions == nil || component.Name == "" {
			log.Debug("API Server component's structure is not expected")
			continue
		}

		for _, condition := range component.Conditions {
			statusCheck := metrics.ServiceCheckUnknown
			message := ""

			// We only expect the Healthy condition. May change in the future. https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#typical-status-properties
			if condition.Type != "Healthy" {
				log.Debugf("Condition %q not supported", condition.Type)
				continue
			}

			// We only expect True, False and Unknown (default).
			switch condition.Status {
			case "True":
				statusCheck = metrics.ServiceCheckOK
				message = condition.Message
			case "False":
				statusCheck = metrics.ServiceCheckCritical
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

// processEvents:
// - iterates over the Kubernetes Events
// - extracts some attributes and builds a structure ready to be submitted as a Datadog event (bundle)
// - formats the bundle and submit the Datadog event
func (k *KubeASCheck) processEvents(sender aggregator.Sender, events []*v1.Event) error {
	bundlesByObject := make(map[bundleID]*kubernetesEventBundle)

	for _, event := range events {
		if event.InvolvedObject.Kind == "" ||
			event.InvolvedObject.Name == "" ||
			event.Reason == "" ||
			event.Message == "" {
			continue
		}

		id := buildBundleID(event)

		bundle, found := bundlesByObject[id]
		if !found {
			bundle = newKubernetesEventBundler(event)
			bundlesByObject[id] = bundle
		}

		err := bundle.addEvent(event)
		if err != nil {
			k.Warnf("Error while bundling events, %s.", err.Error()) //nolint:errcheck
			continue
		}

		kubeEvents.Inc(
			event.InvolvedObject.Kind,
			event.Source.Component,
			event.Type,
			event.Reason,
		)
	}

	ctx := context.TODO()
	hostnameDetected, _ := hostname.Get(ctx)
	clusterName := clustername.GetRFC1123CompliantClusterName(ctx, hostnameDetected)

	for id, bundle := range bundlesByObject {
		datadogEv, err := bundle.formatEvents(clusterName, k.providerIDCache)
		if err != nil {
			k.Warnf("Error while formatting bundled events, %s. Not submitting", err.Error()) //nolint:errcheck
			continue
		}

		sender.Event(datadogEv)

		emittedEvents.Inc(id.kind, id.evType)
	}

	return nil
}

func (k *KubeASCheck) componentStatusCheck(sender aggregator.Sender) error {
	componentsStatus, err := k.ac.ComponentStatuses()
	if err != nil {
		return err
	}

	return k.parseComponentStatus(sender, componentsStatus)
}

func (k *KubeASCheck) controlPlaneHealthCheck(ctx context.Context, sender aggregator.Sender) error {
	ready, err := k.ac.IsAPIServerReady(ctx)

	var (
		msg    string
		status metrics.ServiceCheckStatus
	)

	if ready {
		msg = "ok"
		status = metrics.ServiceCheckOK
	} else {
		status = metrics.ServiceCheckCritical
		if err != nil {
			msg = err.Error()
		} else {
			msg = "unknown error"
		}
	}

	sender.ServiceCheck(KubeControlPaneCheck, status, "", nil, msg)

	return nil
}

type bundleID struct {
	kind   string
	uid    string
	evType string
}

// buildBundleID generates a unique ID to separate k8s events
// based on their InvolvedObject UIDs and event Types
func buildBundleID(e *v1.Event) bundleID {
	return bundleID{
		kind:   e.InvolvedObject.Kind,
		uid:    string(e.InvolvedObject.UID),
		evType: e.Type,
	}
}

func init() {
	core.RegisterCheck(kubernetesAPIServerCheckName, KubernetesASFactory)
}
