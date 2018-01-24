// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s/api/v1"
	yaml "gopkg.in/yaml.v2"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	KubeControlPaneCheck         = "kube_apiserver_controlplane.up"
	kubernetesAPIServerCheckName = "kubernetes_apiserver"
	eventTokenKey                = "event"
)

// KubeASConfig is the config of the API server.
type KubeASConfig struct {
	Tags              []string `yaml:"tags"`
	CollectEvent      bool     `yaml:"collect_events"`
	FilteredEventType []string `yaml:"filtered_event_types"`
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	core.CheckBase
	instance              *KubeASConfig
	KubeAPIServerHostname string
	latestEventToken      string
	configMapAvailable    bool
}

func (c *KubeASConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = true

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check.
func (k *KubeASCheck) Configure(config, initConfig check.ConfigData) error {
	// Check connectivity to the APIServer
	err := k.instance.parse(config)
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

	asclient, err := apiserver.GetAPIClient()

	if err != nil {
		log.Errorf("could not connect to apiserver: %s", err)
		return err
	}

	componentsStatus, err := asclient.ComponentStatuses()
	if err != nil {
		k.Warnf("Could not retrieve the status from the control plane's components %s", err.Error())
	}

	err = k.parseComponentStatus(sender, componentsStatus)
	if err != nil {
		k.Warnf("Could not collect API Server component status: %s", err.Error())
	}

	defer sender.Commit()
	if !k.instance.CollectEvent {
		return nil
	}
	if k.latestEventToken == "" {
		// Initialization: Checking if we previously stored the latestEventToken in a configMap
		token, found, err := asclient.GetTokenFromConfigmap(eventTokenKey, 60)
		switch {
		case err == collectors.ErrOutdated:
			k.configMapAvailable = found
			k.latestEventToken = token
		case err == collectors.ErrNotFound:
			k.latestEventToken = "0"
		case err == nil:
			k.configMapAvailable = found
			k.latestEventToken = token
		}
	}

	newEvents, modifiedEvents, versionToken, err := asclient.LatestEvents(k.latestEventToken)
	if err != nil {
		k.Warnf("Could not collect events from the api server: %s", err.Error())
	}

	// We check that the resversion gotten from the API Server is more recent than the one cached in the util.
	if len(newEvents)+len(modifiedEvents) == 0 {
		return nil
	}
	k.latestEventToken = versionToken
	if k.configMapAvailable {
		configMapErr := asclient.UpdateTokenInConfigmap(eventTokenKey, versionToken)
		if configMapErr != nil {
			k.Warnf("Could not store the LastEventToken in the ConfigMap: %s", configMapErr.Error())
		}
	}

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

func (k *KubeASCheck) parseComponentStatus(sender aggregator.Sender, componentsStatus *v1.ComponentStatusList) error {
	for _, component := range componentsStatus.Items {

		if component.Metadata == nil {
			return errors.New("metadata structure has changed. Not collecting API Server's Components status")
		}
		if component.Conditions == nil || component.Metadata.Name == nil {
			log.Debug("API Server component's structure is not expected")
			continue
		}
		tagComp := append(k.instance.Tags, fmt.Sprintf("component:%s", *component.Metadata.Name))
		for _, condition := range component.Conditions {
			status_check := metrics.ServiceCheckUnknown

			// We only expect the Healthy condition. May change in the future. https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#typical-status-properties
			if *condition.Type != "Healthy" {
				log.Debugf("Condition %q not supported", *condition.Type)
				continue
			}
			// We only expect True, False and Unknown (default).
			switch *condition.Status {
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
	eventsByObject := make(map[string]*kubernetesEventBundle)
	filteredByType := make(map[string]int)

	// Only process the events which actions aren't part of the FilteredEventType list in the yaml config.
ITER_EVENTS:
	for _, event := range events {
		for _, action := range k.instance.FilteredEventType {
			if *event.Reason == action {
				filteredByType[action] = filteredByType[action] + 1
				continue ITER_EVENTS
			}
		}
		bundle, found := eventsByObject[*event.InvolvedObject.Uid]
		if found == false {
			bundle = newKubernetesEventBundler(*event.InvolvedObject.Uid, *event.Source.Component)
			eventsByObject[*event.InvolvedObject.Uid] = bundle
		}
		err := bundle.addEvent(event)
		if err != nil {
			k.Warnf("Error while bundling events, %s.", err.Error())
		}

		if len(filteredByType) > 0 {
			log.Debugf("Filtered out the following events: %s", formatStringIntMap(filteredByType))
		}
	}
	for _, bundle := range eventsByObject {
		datadogEv, err := bundle.formatEvents(k.KubeAPIServerHostname, modified)
		if err != nil {
			k.Warnf("Error while formatting bundled events, %s. Not submitting", err.Error())
			continue
		}
		datadogEv.Tags = append(datadogEv.Tags, k.instance.Tags...)
		sender.Event(datadogEv)
	}
	return nil
}

func init() {
	core.RegisterCheck(kubernetesAPIServerCheckName, KubernetesASFactory)
}
