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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigsForPod returns the openmetrics configurations for a given pod if it matches the AD configuration
func ConfigsForPod(pc *types.PrometheusCheck, pod *workloadmeta.KubernetesPod, wmeta workloadmeta.Component) ([]integration.Config, error) {
	var configs []integration.Config
	namespacedName := pod.Namespace + "/" + pod.Name
	if pc.IsExcluded(pod.Annotations, namespacedName) {
		return nil, nil
	}

	instances, found := buildInstances(pc, pod.Annotations, namespacedName)
	if !found {
		return nil, nil
	}

	var podContainers []*workloadmeta.Container
	for _, orchContainer := range pod.GetAllContainers() {
		container, err := wmeta.GetContainer(orchContainer.ID)
		if err != nil {
			log.Debugf("Failed to get container with ID %q: %s", orchContainer.ID, err)
			continue
		}
		podContainers = append(podContainers, container)
	}

	// If `prometheus.io/port` annotation has been provided, let’s keep only the container that declares this port.
	portAnnotationString, hasPortAnnotation := pod.Annotations[types.PrometheusPortAnnotation]
	var containerWithPortInAnnotation string

	if hasPortAnnotation {
		portNumber, err := strconv.Atoi(portAnnotationString)
		if err != nil {
			// Don't return configs with an invalid port
			return nil, fmt.Errorf("port in annotation %q is not an integer", portAnnotationString)
		}

		// There are valid cases where there is an annotation with a port but no
		// container with that port in the spec. This can happen with Istio
		// sidecars. Scraping still works, so don't return an error. In that
		// case, containerWithPortInAnnotation will be empty.
		containerWithPortInAnnotation = findContainerWithPort(podContainers, portNumber)
	}

	for _, container := range podContainers {
		if !pc.AD.MatchContainer(container.Name) {
			log.Debugf("Container '%s' doesn't match the AD configuration 'kubernetes_container_names', ignoring it", container.Name)
			continue
		}

		if hasPortAnnotation && containerWithPortInAnnotation != "" && container.Name != containerWithPortInAnnotation {
			continue
		}

		// Build the entity name (runtime://containerID) for ADIdentifiers
		containerEntityName := containers.BuildEntityName(string(container.Runtime), container.GetID().ID)

		configs = append(configs, integration.Config{
			Name:          openmetricsCheckName,
			InitConfig:    integration.Data(openmetricsInitConfig),
			Instances:     instances,
			Provider:      names.PrometheusPods,
			Source:        "prometheus_pods:" + containerEntityName,
			ADIdentifiers: []string{containerEntityName},
		})
	}

	// TODO: Support AD matching based on label selectors

	return configs, nil
}

// findContainerWithPort returns the name of the container that exposes the given port, or empty string if none found
func findContainerWithPort(containers []*workloadmeta.Container, port int) string {
	for _, container := range containers {
		for _, containerPort := range container.Ports {
			if containerPort.Port == port {
				return container.Name
			}
		}
	}

	return ""
}
