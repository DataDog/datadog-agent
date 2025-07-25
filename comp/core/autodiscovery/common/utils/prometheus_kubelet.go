// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package utils

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigsForPod returns the openmetrics configurations for a given pod if it matches the AD configuration
func ConfigsForPod(pc *types.PrometheusCheck, pod *kubelet.Pod) []integration.Config {
	var configs []integration.Config
	namespacedName := fmt.Sprintf("%s/%s", pod.Metadata.Namespace, pod.Metadata.Name)
	if pc.IsExcluded(pod.Metadata.Annotations, namespacedName) {
		return configs
	}

	instances, found := buildInstances(pc, pod.Metadata.Annotations, namespacedName)
	if found {
		// If `prometheus.io/port` annotation has been provided, let’s keep only the container that declares this port.
		portAnnotationString, hasPortAnnotation := pod.Metadata.Annotations[types.PrometheusPortAnnotation]
		var containerWithPortInAnnotation string

		if hasPortAnnotation {
			portNumber, err := strconv.Atoi(portAnnotationString)
			if err != nil {
				log.Debugf("Port in annotation '%s' is not an integer", portAnnotationString)
				// Don't return configs with an invalid port
				return configs
			}

			containerWithPortInAnnotation = findContainerWithPort(pod.Spec.Containers, portNumber)

			// If port annotation exists but no container matches, return empty
			if containerWithPortInAnnotation == "" {
				return configs
			}
		}

		for _, containerStatus := range pod.Status.GetAllContainers() {
			if !pc.AD.MatchContainer(containerStatus.Name) {
				log.Debugf("Container '%s' doesn't match the AD configuration 'kubernetes_container_names', ignoring it", containerStatus.Name)
				continue
			}

			if hasPortAnnotation && containerStatus.Name != containerWithPortInAnnotation {
				continue
			}

			configs = append(configs, integration.Config{
				Name:          openmetricsCheckName,
				InitConfig:    integration.Data(openmetricsInitConfig),
				Instances:     instances,
				Provider:      names.PrometheusPods,
				Source:        "prometheus_pods:" + containerStatus.ID,
				ADIdentifiers: []string{containerStatus.ID},
			})
		}
		return configs
	}

	// TODO: Support AD matching based on label selectors

	return configs
}

// findContainerWithPort returns the name of the container that exposes the given port, or empty string if none found
func findContainerWithPort(containers []kubelet.ContainerSpec, targetPort int) string {
	for _, containerSpec := range containers {
		for _, port := range containerSpec.Ports {
			if port.ContainerPort == targetPort {
				return containerSpec.Name
			}
		}
	}
	return ""
}
