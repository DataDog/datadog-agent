// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker,kubelet

package docker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// configPath refers to the configuration that can be passed over a docker label or a pod annotation,
// this feature is commonly named 'ad' or 'autodicovery'.
const (
	annotationConfigPathPrefix = "ad.datadoghq.com"
	annotationConfigPathSuffix = "logs"
)

// ContainsADIdentifier returns true if the container contains an autodiscovery identifier,
// searching first in the docker labels, then in the pod specs.
func ContainsADIdentifier(c *Container) bool {
	var exists bool
	_, exists = c.container.Labels[configPath]
	if exists {
		return true
	}
	kubeutil, err := kubelet.GetKubeUtil()
	if err != nil {
		return false
	}
	entityID := c.service.GetEntityID()
	pod, err := kubeutil.GetPodForEntityID(entityID)
	if err != nil {
		return false
	}
	for _, container := range pod.Status.GetAllContainers() {
		if container.ID == entityID {
			// looks for the container name specified in the pod manifest as it's different from the name of the container
			// returns by a docker inspect which is a concatenation of the container name specified in the pod manifest and a hash
			_, exists = pod.Metadata.Annotations[annotationConfigPath(container.Name)]
			return exists
		}
	}
	return false
}

// annotationConfigPath returns the path of a logs-config passed in a pod annotation.
func annotationConfigPath(containerName string) string {
	return fmt.Sprintf("%s/%s.%s", annotationConfigPathPrefix, containerName, annotationConfigPathSuffix)
}
