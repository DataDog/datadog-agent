// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
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
	"time"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	KubeControlPaneCheck     = "kube_apiserver_controlplane.up"
	kubernetesAPIServerCheck = "kubernetes_apiserver"
)

// KubeASConfig is the config of the API server.
type KubeASConfig struct {
	Tags []string `yaml:"tags"`
}

// KubeASCheck grabs metrics and events from the API server.
type KubeASCheck struct {
	lastWarnings          []error
	instance              *KubeASConfig
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

// Warn will log a warning and add it to the warnings
func (k *KubeASCheck) warn(v ...interface{}) error {
	w := log.Warn(v)
	k.lastWarnings = append(k.lastWarnings, w)

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

	asclient, err := apiserver.GetAPIClient()

	if err != nil {
		log.Errorf("could not connect to apiserver: %s", err)
		return err
	}

	componentsStatus, err := asclient.ComponentStatuses()
	if err != nil {
		k.warn("could not retrieve the status from the control plane's components", err.Error())
	}

	err = k.parseComponentStatus(sender, componentsStatus)
	if err != nil {
		k.warn("could not collect API Server component status: ", err.Error())
	}
	sender.Commit()
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
func init() {
	core.RegisterCheck("kubernetes_apiserver", KubernetesASFactory)
}
