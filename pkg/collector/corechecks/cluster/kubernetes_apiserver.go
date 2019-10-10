// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	KubeControlPaneCheck         = "kube_apiserver_controlplane.up"
	kubernetesAPIServerCheckName = "kubernetes_apiserver"
	eventTokenKey                = "event"
	maxEventCardinality          = 300
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
}

// EventC holds the information pertaining to which event we collected last and when we last re-synced.
type EventC struct {
	LastResVer string
	LastTime   time.Time
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	core.CheckBase
	instance              *KubeASConfig
	KubeAPIServerHostname string
	eventCollection       EventC
	ignoredEvents         string
	ac                    *apiserver.APIClient
	oshiftAPILevel        apiserver.OpenShiftAPILevel
}

func (c *KubeASConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = config.Datadog.GetBool("collect_kubernetes_events")
	c.CollectOShiftQuotas = true
	c.EventCollectionTimeoutMs = config.Datadog.GetInt("kubernetes_event_collection_timeout")

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check.
func (k *KubeASCheck) Configure(config, initConfig integration.Data, source string) error {
	err := k.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	// Check connectivity to the APIServer
	err = k.instance.parse(config)
	if err != nil {
		log.Error("could not parse the config for the API server")
		return err
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
	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}
	defer sender.Commit()

	if config.Datadog.GetBool("cluster_agent.enabled") {
		log.Debug("Cluster agent is enabled. Not running Kubernetes API Server check or collecting Kubernetes Events.")
		return nil
	}
	// Check is configured as a cluster check. The Cluster Agent passed in a config to skip the leader election.
	if !k.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !config.Datadog.GetBool("leader_election") {
			k.Warn("Leader Election not enabled. Not running Kubernetes API Server check or collecting Kubernetes Events.")
			return nil
		}
		errLeader := k.runLeaderElection()
		if errLeader != nil {
			if errLeader == apiserver.ErrNotLeader {
				// Only the leader can instantiate the apiserver client.
				return nil
			}
			return err
		}
	}
	// API Server client initialisation on first run
	if k.ac == nil {
		// We start the API Server Client.
		k.ac, err = apiserver.GetAPIClient()
		if err != nil {
			k.Warnf("Could not connect to apiserver: %s", err)
			return err
		}

		// We detect OpenShift presence for quota collection
		if k.instance.CollectOShiftQuotas {
			k.oshiftAPILevel = k.ac.DetectOpenShiftAPILevel()
		}
	}

	// Running the Control Plane status check.
	componentsStatus, err := k.ac.ComponentStatuses()
	if err != nil {
		k.Warnf("Could not retrieve the status from the control plane's components %s", err.Error())
	} else {
		err = k.parseComponentStatus(sender, componentsStatus)
		if err != nil {
			k.Warnf("Could not collect API Server component status: %s", err.Error())
		}
	}

	// Running OpenShift ClusterResourceQuota collection if available
	if k.instance.CollectOShiftQuotas && k.oshiftAPILevel != apiserver.NotOpenShift {
		quotas, err := k.retrieveOShiftClusterQuotas()
		if err != nil {
			k.Warnf("Could not collect OpenShift cluster quotas: %s", err.Error())
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
		k.Warnf("Could not submit new event %s", err.Error())
	}
	return nil
}

// KubernetesASFactory is exported for integration testing.
func KubernetesASFactory() check.Check {
	return &KubeASCheck{
		CheckBase: core.NewCheckBase(kubernetesAPIServerCheckName),
		instance:  &KubeASConfig{},
		//		eventCollection: EventC{},
	}
}

func (k *KubeASCheck) runLeaderElection() error {

	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		k.Warn("Failed to instantiate the Leader Elector. Not running the Kubernetes API Server check or collecting Kubernetes Events.")
		return err
	}

	err = leaderEngine.EnsureLeaderElectionRuns()
	if err != nil {
		k.Warn("Leader Election process failed to start")
		return err
	}

	if !leaderEngine.IsLeader() {
		log.Debugf("Leader is %q. %s will not run Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader(), leaderEngine.HolderIdentity)
		return apiserver.ErrNotLeader
	}
	log.Tracef("Current leader: %q, running Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader())
	return nil
}

func (k *KubeASCheck) eventCollectionCheck() (newEvents []*v1.Event, err error) {
	resVer, lastTime, err := k.ac.GetTokenFromConfigmap(eventTokenKey)
	if err != nil {
		return nil, err
	}

	timeout := int64(k.instance.EventCollectionTimeoutMs / 1000)
	limit := int64(k.instance.MaxEventCollection)
	resync := int64(k.instance.ResyncPeriodEvents)
	newEvents, k.eventCollection.LastResVer, k.eventCollection.LastTime, err = k.ac.RunEventCollection(resVer, lastTime, timeout, limit, resync, k.ignoredEvents)

	if err != nil {
		k.Warnf("Could not collect events from the api server: %s", err.Error())
		return nil, err
	}

	configMapErr := k.ac.UpdateTokenInConfigmap(eventTokenKey, k.eventCollection.LastResVer, k.eventCollection.LastTime)
	if configMapErr != nil {
		k.Warnf("Could not store the LastEventToken in the ConfigMap: %s", configMapErr.Error())
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
		tagComp := []string{fmt.Sprintf("component:%s", component.Name)}
		for _, condition := range component.Conditions {
			status_check := metrics.ServiceCheckUnknown

			// We only expect the Healthy condition. May change in the future. https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#typical-status-properties
			if condition.Type != "Healthy" {
				log.Debugf("Condition %q not supported", condition.Type)
				continue
			}
			// We only expect True, False and Unknown (default).
			switch condition.Status {
			case "True":
				status_check = metrics.ServiceCheckOK

			case "False":
				status_check = metrics.ServiceCheckCritical
			}
			sender.ServiceCheck(KubeControlPaneCheck, status_check, k.KubeAPIServerHostname, tagComp, "")
		}
	}
	return nil
}

// processEvents:
// - iterates over the Kubernetes Events
// - extracts some attributes and builds a structure ready to be submitted as a Datadog event (bundle)
// - formats the bundle and submit the Datadog event
func (k *KubeASCheck) processEvents(sender aggregator.Sender, events []*v1.Event) error {
	eventsByObject := make(map[types.UID]*kubernetesEventBundle)

	for _, event := range events {
		bundle, found := eventsByObject[event.InvolvedObject.UID]
		if found == false {
			bundle = newKubernetesEventBundler(event.InvolvedObject.UID, event.Source.Component)
			eventsByObject[event.InvolvedObject.UID] = bundle
		}
		err := bundle.addEvent(event)
		if err != nil {
			k.Warnf("Error while bundling events, %s.", err.Error())
		}
	}
	clusterName := clustername.GetClusterName()
	for _, bundle := range eventsByObject {
		datadogEv, err := bundle.formatEvents(clusterName)
		if err != nil {
			k.Warnf("Error while formatting bundled events, %s. Not submitting", err.Error())
			continue
		}
		sender.Event(datadogEv)
	}
	return nil
}

func init() {
	core.RegisterCheck(kubernetesAPIServerCheckName, KubernetesASFactory)
}
