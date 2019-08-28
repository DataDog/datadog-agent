// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"errors"
	"fmt"
	"time"

	yaml "gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/clustername"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	KubeControlPaneCheck         = "kube_apiserver_controlplane.up"
	kubernetesAPIServerCheckName = "kubernetes_apiserver"
	eventTokenKey                = "event"
)

// KubeASConfig is the config of the API server.
type KubeASConfig struct {
	CollectEvent             bool     `yaml:"collect_events"`
	CollectOShiftQuotas      bool     `yaml:"collect_openshift_clusterquotas"`
	FilteredEventType        []string `yaml:"filtered_event_types"`
	EventCollectionTimeoutMs int      `yaml:"kubernetes_event_read_timeout_ms"`
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	core.CheckBase
	instance              *KubeASConfig
	KubeAPIServerHostname string
	latestEventToken      string
	configMapAvailable    bool
	ac                    *apiserver.APIClient
	oshiftAPILevel        apiserver.OpenShiftAPILevel
}

func (c *KubeASConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = config.Datadog.GetBool("collect_kubernetes_events")
	c.CollectOShiftQuotas = true
	c.EventCollectionTimeoutMs = 100

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check.
func (k *KubeASCheck) Configure(config, initConfig integration.Data) error {
	err := k.CommonConfigure(config)
	if err != nil {
		return err
	}

	// Check connectivity to the APIServer
	err = k.instance.parse(config)
	if err != nil {
		log.Error("could not parse the config for the API server")
		return err
	}

	log.Debugf("Running config %s", config)
	return nil
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

	// Init of the resVersion token.
	k.eventCollectionInit()

	// Get the events from the API server
	newEvents, modifiedEvents, err := k.eventCollectionCheck()
	if err != nil {
		return err
	}

	// Process the events to have a Datadog format.
	err = k.processEvents(sender, newEvents, false)
	if err != nil {
		k.Warnf("Could not submit new event %s", err.Error())
	}
	// We send the events in 2 steps to make sure the new events are initializing the aggregation keys and as modified events have a different payload.
	if len(modifiedEvents) == 0 {
		return nil
	}
	err = k.processEvents(sender, modifiedEvents, true)
	if err != nil {
		k.Warnf("Could not submit modified event %s", err.Error())
	}
	return nil
}

// KubernetesASFactory is exported for integration testing.
func KubernetesASFactory() check.Check {
	return &KubeASCheck{
		CheckBase: core.NewCheckBase(kubernetesAPIServerCheckName),
		instance:  &KubeASConfig{},
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
func (k *KubeASCheck) eventCollectionInit() {
	if k.latestEventToken == "" {
		// Initialization: Checking if we previously stored the latestEventToken in a configMap
		tokenValue, found, err := k.ac.GetTokenFromConfigmap(eventTokenKey, 3600)
		switch {
		case err == apiserver.ErrOutdated:
			k.configMapAvailable = found
			k.latestEventToken = "0"

		case err == apiserver.ErrNotFound:
			k.latestEventToken = "0"

		case err == nil:
			k.configMapAvailable = found
			k.latestEventToken = tokenValue

		default:
			log.Warnf("Cannot handle the tokenValue: %q, querying the kube-apiserver cache for events", tokenValue)
			k.latestEventToken = "0"
		}
	}
}

func (k *KubeASCheck) eventCollectionCheck() ([]*v1.Event, []*v1.Event, error) {
	timeout := time.Duration(k.instance.EventCollectionTimeoutMs) * time.Millisecond

	newEvents, modifiedEvents, versionToken, err := k.ac.LatestEvents(k.latestEventToken, timeout)
	if err != nil {
		k.Warnf("Could not collect events from the api server: %s", err.Error())
		return nil, nil, err
	}

	if versionToken == "0" {
		// API server cache expired or no recent events to process. Resetting the Resversion token.
		_, _, versionToken, err = k.ac.LatestEvents("0", timeout)
		if err != nil {
			k.Warnf("Could not collect cached events from the api server: %s", err.Error())
			return nil, nil, err
		}

		if k.latestEventToken == versionToken {
			log.Tracef("No new events collected")
			// No new events but protobuf error was caught. Will retry at next run.
			return nil, nil, nil
		}
		// There was a protobuf error and new events were submitted. Processing them and updating the resVersion.
		k.latestEventToken = versionToken
	}

	// We check that the resversion gotten from the API Server is more recent than the one cached in the util.
	if len(newEvents)+len(modifiedEvents) == 0 {
		return nil, nil, nil
	}

	k.latestEventToken = versionToken
	if k.configMapAvailable {
		configMapErr := k.ac.UpdateTokenInConfigmap(eventTokenKey, versionToken)
		if configMapErr != nil {
			k.Warnf("Could not store the LastEventToken in the ConfigMap: %s", configMapErr.Error())
		}
	}

	return newEvents, modifiedEvents, nil
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
func (k *KubeASCheck) processEvents(sender aggregator.Sender, events []*v1.Event, modified bool) error {
	eventsByObject := make(map[types.UID]*kubernetesEventBundle)
	filteredByType := make(map[string]int)

	// Only process the events which actions aren't part of the FilteredEventType list in the yaml config.
ITER_EVENTS:
	for _, event := range events {
		for _, action := range k.instance.FilteredEventType {
			if event.Reason == action {
				filteredByType[action] = filteredByType[action] + 1
				continue ITER_EVENTS
			}
		}
		bundle, found := eventsByObject[event.InvolvedObject.UID]
		if found == false {
			bundle = newKubernetesEventBundler(event.InvolvedObject.UID, event.Source.Component)
			eventsByObject[event.InvolvedObject.UID] = bundle
		}
		err := bundle.addEvent(event)
		if err != nil {
			k.Warnf("Error while bundling events, %s.", err.Error())
		}

		if len(filteredByType) > 0 {
			log.Debugf("Filtered out the following events: %s", formatStringIntMap(filteredByType))
		}
	}

	clusterName := clustername.GetClusterName()
	for _, bundle := range eventsByObject {
		datadogEv, err := bundle.formatEvents(modified, clusterName)
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
