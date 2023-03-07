// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet
// +build kubelet

package utils

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
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
		var portFromAnnotation int
		var containerPorts map[string]map[int]struct{}
		if portFromAnnotationString, portFromAnnotationFound := pod.Metadata.Annotations[types.PrometheusPortAnnotation]; portFromAnnotationFound {
			var err error
			if portFromAnnotation, err = strconv.Atoi(portFromAnnotationString); err != nil {
				portFromAnnotationFound = false
				portFromAnnotation = 0
			} else {
				containerPorts = make(map[string]map[int]struct{})
				for _, containerSpec := range pod.Spec.Containers {
					containerPorts[containerSpec.Name] = make(map[int]struct{})
					for _, port := range containerSpec.Ports {
						containerPorts[containerSpec.Name][port.ContainerPort] = struct{}{}
					}
				}
			}
		}
		for _, containerStatus := range pod.Status.GetAllContainers() {
			if !pc.AD.MatchContainer(containerStatus.Name) {
				log.Debugf("Container '%s' doesn't match the AD configuration 'kubernetes_container_names', ignoring it", containerStatus.Name)
				continue
			}
			if portFromAnnotation != 0 {
				if ctr, found := containerPorts[containerStatus.Name]; found {
					if _, found := ctr[portFromAnnotation]; !found {
						continue
					}
				}
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
