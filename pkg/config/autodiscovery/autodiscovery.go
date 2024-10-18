// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscovery contains helper function that return autodiscovery
// providers from the config and from the environment where the Agent is
// running.
package autodiscovery

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	snmplistener "github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoverComponentsFromConfig returns a list of AD Providers and Listeners based on the agent configuration
func DiscoverComponentsFromConfig() ([]pkgconfigsetup.ConfigurationProviders, []pkgconfigsetup.Listeners) {
	detectedProviders := []pkgconfigsetup.ConfigurationProviders{}
	detectedListeners := []pkgconfigsetup.Listeners{}

	// Auto-add Prometheus config provider based on `prometheus_scrape.enabled`
	if pkgconfigsetup.Datadog().GetBool("prometheus_scrape.enabled") {
		var prometheusProvider pkgconfigsetup.ConfigurationProviders
		if flavor.GetFlavor() == flavor.ClusterAgent {
			prometheusProvider = pkgconfigsetup.ConfigurationProviders{Name: "prometheus_services", Polling: true}
		} else {
			prometheusProvider = pkgconfigsetup.ConfigurationProviders{Name: "prometheus_pods", Polling: true}
		}
		log.Infof("Prometheus scraping is enabled: Adding the Prometheus config provider '%s'", prometheusProvider.Name)
		detectedProviders = append(detectedProviders, prometheusProvider)
	}
	// Add database-monitoring aurora listener if the feature is enabled
	if pkgconfigsetup.Datadog().GetBool("database_monitoring.autodiscovery.aurora.enabled") {
		detectedListeners = append(detectedListeners, pkgconfigsetup.Listeners{Name: "database-monitoring-aurora"})
		log.Info("Database monitoring aurora discovery is enabled: Adding the aurora listener")
	}

	// Auto-add file-based kube service and endpoints config providers based on check config files.
	if flavor.GetFlavor() == flavor.ClusterAgent {
		advancedConfigs, _, err := providers.ReadConfigFiles(providers.WithAdvancedADOnly)
		if err != nil {
			log.Warnf("Couldn't read config files: %v", err)
		}

		svcFound, epFound := false, false
		for _, conf := range advancedConfigs {
			for _, adv := range conf.AdvancedADIdentifiers {
				if !svcFound && !adv.KubeService.IsEmpty() {
					svcFound = true
					log.Info("Configs with advanced kube service identifiers detected: Adding the 'kube service file' config provider")
					// Polling is set to false because kube_services_file is a static config provider.
					// It generates entity IDs based on the provided advanced config: kube_service://<namespace>/<name>
					detectedProviders = append(detectedProviders, pkgconfigsetup.ConfigurationProviders{Name: names.KubeServicesFileRegisterName, Polling: false})
				}

				if !epFound && !adv.KubeEndpoints.IsEmpty() {
					epFound = true
					log.Info("Configs with advanced kube endpoints identifiers detected: Adding the 'kube endpoints file' config provider")
					// Polling is set to true because kube_endpoints_file is a dynamic config provider.
					// It generates entity IDs based on the provided advanced config + the IPs found in the corresponding Endpoints object: kube_endpoint://<namespace>/<name>/<ip>
					// The generated entity IDs are subject to change, thus the continuous polling.
					detectedProviders = append(detectedProviders, pkgconfigsetup.ConfigurationProviders{Name: names.KubeEndpointsFileRegisterName, Polling: true})
				}
			}

			if svcFound && epFound {
				break
			}
		}
	}

	// Auto-activate autodiscovery without listeners: - snmp
	snmpConfig, err := snmplistener.NewListenerConfig()

	if err != nil {
		log.Errorf("Error unmarshalling snmp listener config. Error: %v", err)
	} else if len(snmpConfig.Configs) > 0 {
		detectedListeners = append(detectedListeners, pkgconfigsetup.Listeners{Name: "snmp"})
		log.Info("Configs for autodiscovery detected: Adding the snmp listener")
	}

	return detectedProviders, detectedListeners
}

// DiscoverComponentsFromEnv returns a list of AD Providers and Listeners based on environment characteristics
func DiscoverComponentsFromEnv() ([]pkgconfigsetup.ConfigurationProviders, []pkgconfigsetup.Listeners) {
	detectedProviders := []pkgconfigsetup.ConfigurationProviders{}
	detectedListeners := []pkgconfigsetup.Listeners{}

	// When using automatic discovery of providers/listeners
	// We automatically activate the environment and static config listener
	detectedListeners = append(detectedListeners, pkgconfigsetup.Listeners{Name: "environment"})
	detectedListeners = append(detectedListeners, pkgconfigsetup.Listeners{Name: "static config"})

	// Automatic handling of AD providers/listeners should only run in the core or process agent.
	if flavor.GetFlavor() != flavor.DefaultAgent && flavor.GetFlavor() != flavor.ProcessAgent {
		return detectedProviders, detectedListeners
	}

	isContainerEnv := env.IsFeaturePresent(env.Docker) ||
		env.IsFeaturePresent(env.Containerd) ||
		env.IsFeaturePresent(env.Podman) ||
		env.IsFeaturePresent(env.ECSFargate)
	isKubeEnv := env.IsFeaturePresent(env.Kubernetes)

	if isContainerEnv || isKubeEnv {
		detectedProviders = append(detectedProviders, pkgconfigsetup.ConfigurationProviders{Name: names.KubeContainer})
		log.Info("Adding KubeContainer provider from environment")
	}

	if isContainerEnv && !isKubeEnv {
		detectedListeners = append(detectedListeners, pkgconfigsetup.Listeners{Name: names.Container})
		log.Info("Adding Container listener from environment")
	}

	if isKubeEnv {
		detectedListeners = append(detectedListeners, pkgconfigsetup.Listeners{Name: "kubelet"})
		log.Info("Adding Kubelet listener from environment")
	}

	return detectedProviders, detectedListeners
}
