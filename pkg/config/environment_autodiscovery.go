// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoverAutodiscoveryComponents returns a list of AD Providers and Listeners based on environment characteristics
func DiscoverAutodiscoveryComponents() ([]ConfigurationProviders, []Listeners) {
	detectedProviders := []ConfigurationProviders{}
	detectedListeners := []Listeners{}

	// When using automatic discovery of providers/listeners
	// We automatically activate the environment listener
	detectedListeners = append(detectedListeners, Listeners{Name: "environment"})

	if IsFeaturePresent(Docker) {
		detectedProviders = append(detectedProviders, ConfigurationProviders{Name: "docker", Polling: true, PollInterval: "1s"})
		if !IsFeaturePresent(Kubernetes) {
			detectedListeners = append(detectedListeners, Listeners{Name: "docker"})
			log.Info("Adding Docker listener from environment")
		}
		log.Info("Adding Docker provider from environment")
	}

	if IsFeaturePresent(Kubernetes) {
		detectedProviders = append(detectedProviders, ConfigurationProviders{Name: "kubelet", Polling: true})
		detectedListeners = append(detectedListeners, Listeners{Name: "kubelet"})
		log.Info("Adding Kubelet autodiscovery provider and listener from environment")
	}

	if IsFeaturePresent(ECSFargate) {
		detectedProviders = append(detectedProviders, ConfigurationProviders{Name: "ecs", Polling: true})
		detectedListeners = append(detectedListeners, Listeners{Name: "ecs"})
		log.Info("Adding ECS Fargate autodiscovery provider and listener from environment")
	}

	return detectedProviders, detectedListeners
}
