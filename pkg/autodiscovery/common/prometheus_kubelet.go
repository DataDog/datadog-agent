// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigsForPod returns the openmetrics configurations for a given pod if it matches the AD configuration
func (pc *PrometheusCheck) ConfigsForPod(pod *kubelet.Pod) []integration.Config {
	var configs []integration.Config
	namespacedName := fmt.Sprintf("%s/%s", pod.Metadata.Namespace, pod.Metadata.Name)
	if pc.isExcluded(pod.Metadata.Annotations, namespacedName) {
		return configs
	}

	instances, found := pc.buildInstances(pod.Metadata.Annotations, namespacedName)
	if found {
		for _, container := range pod.Status.GetAllContainers() {
			if !pc.AD.matchContainer(container.Name) {
				log.Debugf("Container '%s' doesn't match the AD configuration 'kubernetes_container_names', ignoring it", container.Name)
				continue
			}
			configs = append(configs, integration.Config{
				Name:          openmetricsCheckName,
				InitConfig:    integration.Data(openmetricsInitConfig),
				Instances:     instances,
				Provider:      names.PrometheusPods,
				Source:        "prometheus_pods:" + container.ID,
				ADIdentifiers: []string{container.ID},
			})
		}
		return configs
	}

	// TODO: Support AD matching based on label selectors

	return configs
}
