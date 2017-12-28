// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	as_util "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s/api/v1"
	"gopkg.in/yaml.v2"
	"time"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	KubeControlPaneCheck     = "kube_apiserver_controlplane.up"
	kubernetesAPIServerCheck = "kubernetes_apiserver"
)

// KubeASConfig is the config of the API server.
type KubeASConfig struct {
	//CollectContainerSize bool               `yaml:"collect_container_size"`
	Tags []string `yaml:"tags"`
	//CollectEvent         bool               `yaml:"collect_events"`
	//FilteredEventType    []string           `yaml:"filtered_event_types"`
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	lastWarnings []error
	instance     *KubeASConfig
	//lastEventTime  time.Time
	KubeAPIServerHostname string
}

// String returns the name of the API Server check.
func (k *KubeASCheck) String() string {
	return kubernetesAPIServerCheck
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

// GetWarnings grabs the last warnings from the sender.
func (k *KubeASCheck) GetWarnings() []error {
	w := k.lastWarnings
	k.lastWarnings = []error{}
	return w
}

// ID returns the name of the check since there should be only one instance running.
func (k *KubeASCheck) ID() check.ID {
	return check.ID(k.String())
}

// Interval returns the scheduling time for the check.
func (k *KubeASCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// Run executes the check.
func (k *KubeASCheck) Run() error {
	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}

	asclient, err := as_util.GetAPIClient()

	if err != nil {
		log.Errorf("could not instantiate the cluster %q", err)
		return err
	}

	componentsStatus := asclient.GetComponents()

	k.parseComponentStatus(sender, componentsStatus)

	return nil
}

// Stop does nothing.
func (k *KubeASCheck) Stop() {}

// KubernetesASFactory is exported for integration testing.
func KubernetesASFactory() check.Check {
	return &KubeASCheck{
		instance: &KubeASConfig{},
	}
}

// GetMetricStats returns the stats from the last run of the check.
func (k *KubeASCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return nil, log.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func (k *KubeASCheck) parseComponentStatus(sender aggregator.Sender, componentsStatus *v1.ComponentStatusList) {

	for _, component := range componentsStatus.Items {

		if component.Metadata.Name == nil || component.Conditions == nil {
			log.Error("Metadata structure has changed. Not collecting API Server's Components status")
			return
		}
		tagComp := append(k.instance.Tags, fmt.Sprintf("component:%s", *component.Metadata.Name))
		// We only expect the Healthy condition. May change in the future. https://godoc.org/github.com/ericchiang/k8s/api/v1#ComponentCondition
		for _, condition := range component.Conditions {
			// We only expect True, False and Unknown.
			switch {
			case *condition.Type != "Healthy":
				log.Debugf("Condition %q not supported", *condition.Type)
				sender.ServiceCheck(KubeControlPaneCheck, metrics.ServiceCheckUnknown, k.KubeAPIServerHostname, tagComp, "The Component's condition type isn't supported")
				continue

			case *condition.Status == "True":
				sender.ServiceCheck(KubeControlPaneCheck, metrics.ServiceCheckOK, k.KubeAPIServerHostname, tagComp, "")
				continue

			case *condition.Status == "False":
				sender.ServiceCheck(KubeControlPaneCheck, metrics.ServiceCheckCritical, k.KubeAPIServerHostname, tagComp, "")
				continue

			default:
				sender.ServiceCheck(KubeControlPaneCheck, metrics.ServiceCheckUnknown, k.KubeAPIServerHostname, tagComp, "")
			}
		}
	}
	sender.Commit()
}
func init() {
	core.RegisterCheck("kubernetes_apiserver", KubernetesASFactory)
}
