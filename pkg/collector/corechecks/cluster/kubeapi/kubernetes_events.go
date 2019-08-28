// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package kubeapi

import (
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/clustername"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	kubernetesAPIEventsCheckName = "kubernetes_api_events"
	eventTokenKey                = "event"
)

// KubeApiEventsConfig is the config of the API server.
type EventsConfig struct {
	CollectEvent             bool     `yaml:"collect_events"`
	FilteredEventType        []string `yaml:"filtered_event_types"`
	EventCollectionTimeoutMs int      `yaml:"kubernetes_event_read_timeout_ms"`
}

// KubeApiEventsCheck grabs events from the API server.
type EventsCheck struct {
	CommonCheck
	instance           *EventsConfig
	latestEventToken   string
	configMapAvailable bool
}

func (c *EventsConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = config.Datadog.GetBool("collect_kubernetes_events")
	c.EventCollectionTimeoutMs = 100

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check.
func (k *EventsCheck) Configure(config, initConfig integration.Data) error {
	err := k.ConfigureKubeApiCheck(config)
	if err != nil {
		return err
	}

	// Check connectivity to the APIServer
	err = k.instance.parse(config)
	if err != nil {
		_ = log.Error("could not parse the config for the API events check")
		return err
	}

	log.Debugf("Running config %s", config)
	return nil
}

// Run executes the check.
func (k *EventsCheck) Run() error {
	// initialize kube api check
	err := k.InitKubeApiCheck()
	if err == apiserver.ErrNotLeader {
		log.Debug("Agent is not leader, will not run the check")
		return nil
	} else if err != nil {
		return err
	}

	// Running the event collection.
	if !k.instance.CollectEvent {
		return nil
	}

	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}
	defer sender.Commit()

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
		_ = k.Warnf("Could not submit new event %s", err.Error())
	}
	// We send the events in 2 steps to make sure the new events are initializing the aggregation keys and as modified events have a different payload.
	if len(modifiedEvents) == 0 {
		return nil
	}
	err = k.processEvents(sender, modifiedEvents, true)
	if err != nil {
		_ = k.Warnf("Could not submit modified event %s", err.Error())
	}

	return nil
}

// KubernetesASFactory is exported for integration testing.
func KubernetesApiEventsFactory() check.Check {
	return &EventsCheck{
		CommonCheck: CommonCheck{
			CheckBase: core.NewCheckBase(kubernetesAPIEventsCheckName),
		},
		instance: &EventsConfig{},
	}
}

func (k *EventsCheck) eventCollectionInit() {
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
			_ = log.Warnf("Cannot handle the tokenValue: %q, querying the kube-apiserver cache for events", tokenValue)
			k.latestEventToken = "0"
		}
	}
}

func (k *EventsCheck) eventCollectionCheck() ([]*v1.Event, []*v1.Event, error) {
	timeout := time.Duration(k.instance.EventCollectionTimeoutMs) * time.Millisecond

	newEvents, modifiedEvents, versionToken, err := k.ac.LatestEvents(k.latestEventToken, timeout)
	if err != nil {
		_ = k.Warnf("Could not collect events from the api server: %s", err.Error())
		return nil, nil, err
	}

	if versionToken == "0" {
		// API server cache expired or no recent events to process. Resetting the Resversion token.
		_, _, versionToken, err = k.ac.LatestEvents("0", timeout)
		if err != nil {
			_ = k.Warnf("Could not collect cached events from the api server: %s", err.Error())
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
			_ = k.Warnf("Could not store the LastEventToken in the ConfigMap: %s", configMapErr.Error())
		}
	}

	return newEvents, modifiedEvents, nil
}

// processEvents:
// - iterates over the Kubernetes Events
// - extracts some attributes and builds a structure ready to be submitted as a Datadog event (bundle)
// - formats the bundle and submit the Datadog event
func (k *EventsCheck) processEvents(sender aggregator.Sender, events []*v1.Event, modified bool) error {
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
			_ = k.Warnf("Error while bundling events, %s.", err.Error())
		}

		if len(filteredByType) > 0 {
			log.Debugf("Filtered out the following events: %s", formatStringIntMap(filteredByType))
		}
	}

	clusterName := clustername.GetClusterName()
	for _, bundle := range eventsByObject {
		datadogEv, err := bundle.formatEvents(modified, clusterName)
		if err != nil {
			_ = k.Warnf("Error while formatting bundled events, %s. Not submitting", err.Error())
			continue
		}
		sender.Event(datadogEv)
	}
	return nil
}

func init() {
	core.RegisterCheck(kubernetesAPIEventsCheckName, KubernetesApiEventsFactory)
}
