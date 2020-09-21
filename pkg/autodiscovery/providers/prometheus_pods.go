// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package providers

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PrometheusPodsConfigProvider implements the ConfigProvider interface for prometheus pods.
type PrometheusPodsConfigProvider struct {
	kubelet kubelet.KubeUtilInterface
	checks  []*PrometheusCheck
}

// NewPrometheusPodsConfigProvider returns a new Prometheus ConfigProvider connected to kubelet.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewPrometheusPodsConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	p := &PrometheusPodsConfigProvider{}
	err := p.setupConfigs()
	return p, err
}

// setupConfigs reads and initializes the checks from the configuration
// It defines a default openmetrics instances with default AD if the checks configuration is empty
func (p *PrometheusPodsConfigProvider) setupConfigs() error {
	checks := []*PrometheusCheck{}
	if err := config.Datadog.UnmarshalKey("prometheus_scrape.checks", &checks); err != nil {
		return err
	}

	if len(checks) == 0 {
		log.Info("The 'prometheus_scrape.checks' configuration is empty, a default openmetrics check configuration will be used")
		p.checks = []*PrometheusCheck{defaultCheck}
		return nil
	}

	for i, check := range checks {
		if err := check.init(); err != nil {
			log.Errorf("Ignoring check configuration (# %d): %v", i+1, err)
			continue
		}
		p.checks = append(p.checks, check)
	}

	return nil
}

// String returns a string representation of the PrometheusPodsConfigProvider
func (p *PrometheusPodsConfigProvider) String() string {
	return names.Prometheus
}

// Collect retrieves templates from the kubelet's podlist, builds config objects and returns them
func (p *PrometheusPodsConfigProvider) Collect() ([]integration.Config, error) {
	var err error
	if p.kubelet == nil {
		p.kubelet, err = kubelet.GetKubeUtil()
		if err != nil {
			return []integration.Config{}, err
		}
	}

	pods, err := p.kubelet.GetLocalPodList()
	if err != nil {
		return []integration.Config{}, err
	}

	return p.parsePodlist(pods), nil
}

// IsUpToDate always return false to poll new data from kubelet
func (p *PrometheusPodsConfigProvider) IsUpToDate() (bool, error) {
	return false, nil
}

// parsePodlist searches for pods that match the AD configuration
func (p *PrometheusPodsConfigProvider) parsePodlist(podlist []*kubelet.Pod) []integration.Config {
	var configs []integration.Config
	for _, pod := range podlist {
		for _, check := range p.checks {
			configs = append(configs, check.configsForPod(pod)...)
		}
	}
	return configs
}

// configsForPod returns the openmetrics configurations for a given pod if it matches the AD configuration
func (pc *PrometheusCheck) configsForPod(pod *kubelet.Pod) []integration.Config {
	var configs []integration.Config
	for k, v := range pc.AD.KubeAnnotations.Excl {
		if pod.Metadata.Annotations[k] == v {
			log.Debugf("Pod '%s' matched the exclusion annotation '%s=%s' ignoring it", pod.Metadata.Name, k, v)
			return configs
		}
	}

	for k, v := range pc.AD.KubeAnnotations.Incl {
		if pod.Metadata.Annotations[k] == v {
			log.Debugf("Pod '%s' matched the annotation '%s=%s' to schedule an openmetrics check", pod.Metadata.Name, k, v)
			instances := []integration.Data{}
			for _, instance := range pc.Instances {
				instanceValues := *instance
				if instanceValues.URL == "" {
					instanceValues.URL = buildURL(pod.Metadata.Annotations)
				}
				instanceJSON, err := json.Marshal(instanceValues)
				if err != nil {
					log.Warnf("Error processing prometheus configuration: %v", err)
					continue
				}
				instances = append(instances, instanceJSON)
			}

			for _, container := range pod.Status.GetAllContainers() {
				if !pc.AD.matchContainer(container.Name) {
					log.Debugf("Container '%s' doesn't match the AD configuration 'kubernetes_container_names', ignoring it", container.Name)
					continue
				}
				configs = append(configs, integration.Config{
					Name:          openmetricsCheckName,
					InitConfig:    integration.Data(openmetricsInitConfig),
					Instances:     instances,
					Provider:      names.Prometheus,
					Source:        "kubelet:" + container.ID,
					ADIdentifiers: []string{container.ID},
				})
			}

			return configs
		}
	}

	// TODO: Support AD matching based on label selectors

	return configs
}

func init() {
	RegisterProvider("prometheus-pods", NewPrometheusPodsConfigProvider)
}
