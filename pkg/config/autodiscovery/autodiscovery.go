// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoverComponentsFromConfig returns a list of AD Providers and Listeners based on the agent configuration
func DiscoverComponentsFromConfig() ([]config.ConfigurationProviders, []config.Listeners) {
	detectedProviders := []config.ConfigurationProviders{}
	detectedListeners := []config.Listeners{}

	// Auto-add Prometheus config provider based on `prometheus_scrape.enabled`
	if config.Datadog.GetBool("prometheus_scrape.enabled") {
		var prometheusProvider config.ConfigurationProviders
		if flavor.GetFlavor() == flavor.ClusterAgent {
			prometheusProvider = config.ConfigurationProviders{Name: "prometheus_services", Polling: true}
		} else {
			prometheusProvider = config.ConfigurationProviders{Name: "prometheus_pods", Polling: true}
		}
		log.Infof("Prometheus scraping is enabled: Adding the Prometheus config provider '%s'", prometheusProvider.Name)
		detectedProviders = append(detectedProviders, prometheusProvider)
	}

	return detectedProviders, detectedListeners
}

// DiscoverComponentsFromEnv returns a list of AD Providers and Listeners based on environment characteristics
func DiscoverComponentsFromEnv() ([]config.ConfigurationProviders, []config.Listeners) {
	detectedProviders := []config.ConfigurationProviders{}
	detectedListeners := []config.Listeners{}

	// When using automatic discovery of providers/listeners
	// We automatically activate the environment listener
	detectedListeners = append(detectedListeners, config.Listeners{Name: "environment"})

	// Automatic handling of AD providers/listeners should only run in Core agent.
	if flavor.GetFlavor() != flavor.DefaultAgent {
		return detectedProviders, detectedListeners
	}

	if config.IsFeaturePresent(config.Docker) {
		detectedProviders = append(detectedProviders, config.ConfigurationProviders{Name: "docker", Polling: true, PollInterval: "1s"})
		if !config.IsFeaturePresent(config.Kubernetes) {
			detectedListeners = append(detectedListeners, config.Listeners{Name: "docker"})
			log.Info("Adding Docker listener from environment")
		}
		log.Info("Adding Docker provider from environment")
	}

	if config.IsFeaturePresent(config.Kubernetes) {
		detectedProviders = append(detectedProviders, config.ConfigurationProviders{Name: "kubelet", Polling: true})
		detectedListeners = append(detectedListeners, config.Listeners{Name: "kubelet"})
		log.Info("Adding Kubelet autodiscovery provider and listener from environment")
	}

	if config.IsFeaturePresent(config.ECSFargate) {
		detectedProviders = append(detectedProviders, config.ConfigurationProviders{Name: "ecs", Polling: true})
		detectedListeners = append(detectedListeners, config.Listeners{Name: "ecs"})
		log.Info("Adding ECS Fargate autodiscovery provider and listener from environment")
	}

	return detectedProviders, detectedListeners
}
