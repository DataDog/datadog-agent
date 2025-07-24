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
func ConfigsForPod(pc *types.PrometheusCheck, pod *workloadmeta.KubernetesPod, wmeta workloadmeta.Component) []integration.Config {
	var configs []integration.Config
	namespacedName := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
	if pc.IsExcluded(pod.Annotations, namespacedName) {
		return configs
	}

	instances, found := buildInstances(pc, pod.Annotations, namespacedName)
	if !found {
		return configs
	}

	// If `prometheus.io/port` annotation has been provided, let's keep only the container that declares this port.
	// TODO: if there's a port annotation, but no container uses it, this
	// generates configs for all containers. This seems counter-intuitive, but
	// it was like this before migrating to workloadmeta. Not sure if it's a
	// bug.
	portInAnnotation, portInAnnotationFound := portFromAnnotation(pod.Annotations)

	var podContainers []*workloadmeta.Container
	for _, orchContainer := range pod.GetAllContainers() {
		container, err := wmeta.GetContainer(orchContainer.ID)
		if err != nil {
			log.Debugf("Failed to get container with ID %q: %s", orchContainer.ID, err)
			continue
		}
		podContainers = append(podContainers, container)
	}

	containerUsingAnnotationPort := ""
	if portInAnnotationFound {
		containerUsingAnnotationPort = findContainerNameUsingPort(podContainers, portInAnnotation)
	}

	for _, container := range podContainers {
		if !pc.AD.MatchContainer(container.Name) {
			log.Debugf("Container '%s' doesn't match the AD configuration 'kubernetes_container_names', ignoring it", container.Name)
			continue
		}

		if containerUsingAnnotationPort != "" && container.Name != containerUsingAnnotationPort {
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

	return configs
}

func portFromAnnotation(annotations map[string]string) (int, bool) {
	portString, portFound := annotations[types.PrometheusPortAnnotation]
	if !portFound {
		return 0, false
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		log.Debugf("Failed to parse port from annotation: %s", portString)
		return 0, false
	}

	return port, true
}

// findContainerNameUsingPort returns the name of the container that uses the given
// port from a pre-fetched container map. Returns empty string if not found
func findContainerNameUsingPort(containers []*workloadmeta.Container, port int) string {
	for _, container := range containers {
		for _, containerPort := range container.Ports {
			if containerPort.Port == port {
				return container.Name
			}
		}
	}

	return ""
}
