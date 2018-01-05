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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s/api/v1"
	yaml "gopkg.in/yaml.v2"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	KubeControlPaneCheck         = "kube_apiserver_controlplane.up"
	kubernetesAPIServerCheckName = "kubernetes_apiserver"
)

// KubeASConfig is the config of the API server.
type KubeASConfig struct {
	Tags              []string `yaml:"tags"`
	FilteredEventType []string `yaml:"filtered_event_types"`
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	core.CheckBase
	lastWarnings            []error
	instance                *KubeASConfig
	KubeAPIServerHostname   string
	latest_latestEventToken string
}

func (c *KubeASConfig) parse(data []byte) error {
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
		k.Warn("could not retrieve the status from the control plane's components", err.Error())
	}

	err = k.parseComponentStatus(sender, componentsStatus)
	if err != nil {
		k.Warn("could not collect API Server component status: ", err.Error())
	}

	newEvents, modifiedEvents, versionToken, err := asclient.LatestEvents(k.latest_latestEventToken)
	if err != nil {
		k.Warn("could not collect events from the api server: ", err.Error())
	}

	if versionToken != k.latest_latestEventToken {
		k.latest_latestEventToken = versionToken
		evToSubmit := k.aggregateEvents(newEvents, false)

		// We send the events in 2 steps to make sure the new events are initializing the aggregation keys.
		for _, ev := range evToSubmit {
			sender.Event(ev)
		}

		if len(modifiedEvents) != 0 {
			modifiedEv := k.aggregateEvents(modifiedEvents, true)

			for _, ev := range modifiedEv {
				sender.Event(ev)
			}
		}
	}
	sender.Commit()
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

func (k *KubeASCheck) aggregateEvents(events []*v1.Event, modified bool) []metrics.Event {
	eventsByObject := make(map[string]*kubernetesEventBundle)
	filteredByType := make(map[string]int)
	var evs []metrics.Event

ITER_EVENTS:
	for _, event := range events {
		for _, action := range k.instance.FilteredEventType {
			if *event.Reason == action {
				filteredByType[action] = filteredByType[action] + 1
				continue ITER_EVENTS
			}
			bundle, found := eventsByObject[*event.InvolvedObject.Name]
			if found == false {
				bundle = newKubernetesEventBundler(*event.InvolvedObject.Name, *event.Source.Component)
				eventsByObject[*event.InvolvedObject.Name] = bundle
			}
			bundle.addEvent(event)
		}
		if len(filteredByType) > 0 {
			log.Debugf("filtered out the following events: %s", formatStringIntMap(filteredByType))
		}
	}
	for _, bundle := range eventsByObject {
		datadogEv, _ := bundle.formatEvents(k.KubeAPIServerHostname, modified)
		datadogEv.Tags = append(datadogEv.Tags, k.instance.Tags...)
		evs = append(evs, datadogEv)
	}
	return evs
}

func init() {
	core.RegisterCheck(kubernetesAPIServerCheckName, KubernetesASFactory)
}
