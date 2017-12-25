// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package containers

import(
	"time"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	log "github.com/cihub/seelog"
	yaml "gopkg.in/yaml.v2"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	_ "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)
// Covers the Controle pannel service check and the in memory pod metadata.

const kubernetesApiServerCheck = "kuberenetes_apiserver"

const (
	KubeControlPaneCheck string = "kubernetes_APIServer.controlePlaneUp"
)

type KubeASConfig struct {
	//CollectContainerSize bool               `yaml:"collect_container_size"`
	Tags                 []string           `yaml:"tags"`
	//CollectEvent         bool               `yaml:"collect_events"`
	//FilteredEventType    []string           `yaml:"filtered_event_types"`
}

// KubeASCheck grabs docker metrics
type KubeASCheck struct {
	lastWarnings   []error
	instance       *KubeASConfig
	//lastEventTime  time.Time
	KubeApiServerHostname string
}

func (k *KubeASCheck) String() string {
	return kubernetesApiServerCheck
}

func (c *KubeASConfig) Parse(data []byte) error {

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

// Configure parses the check configuration and init the check
func (k *KubeASCheck) Configure(config, initConfig check.ConfigData) error {
	k.instance.Parse(config)
	// Check connectivity to the APIServer
	log.Infof("Running config %s", config)
	return nil
}

// GetWarnings grabs the last warnings from the sender
func (k *KubeASCheck) GetWarnings() []error {
	w := k.lastWarnings
	k.lastWarnings = []error{}
	return w
}

// ID returns the name of the check since there should be only one instance running
func (k *KubeASCheck) ID() check.ID {
	return check.ID(k.String())
}

// Interval returns the scheduling time for the check
func (k *KubeASCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}
// Run executes the check
func (k *KubeASCheck) Run() error {
	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}
	sender.Gauge("kubernetes_api_server", 10, "", k.instance.Tags)
	sender.Commit()
	return nil
}
// Stop does nothing
func (k *KubeASCheck) Stop() {}


// DockerFactory is exported for integration testing
func KubernetesASFactory() check.Check {
	return &KubeASCheck{
		instance: &KubeASConfig{},
	}
}

// GetMetricStats returns the stats from the last run of the check
func (k *KubeASCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return nil, log.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func init() {
	core.RegisterCheck("kubernetes_apiserver", KubernetesASFactory)
}
