// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker
// +build kubelet

package listeners

// This files handles Host & Port lookup from the kubelet's pod status
// If needed for other orchestrators, we should introduce pluggable
// sources instead of adding yet another special case.

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// DockerKubeletService overrides some methods when a container is
// running in kubernetes
type DockerKubeletService struct {
	DockerService
}

var dockerListenerKubeUtil *kubelet.KubeUtil

func (s *DockerKubeletService) getPod() (*kubelet.Pod, error) {
	if dockerListenerKubeUtil == nil {
		var err error
		dockerListenerKubeUtil, err = kubelet.GetKubeUtil()
		if err != nil {
			return nil, err
		}
	}
	searchedId := docker.ContainerIDToEntityName(string(s.GetID()))
	return dockerListenerKubeUtil.GetPodForContainerID(searchedId)
}

func (s *DockerKubeletService) GetHosts() (map[string]string, error) {
	pod, err := s.getPod()
	if err != nil {
		return nil, err
	}
	return map[string]string{"pod": pod.Status.PodIP}, nil

}

func (s *DockerKubeletService) GetPorts() ([]int, error) {
	pod, err := s.getPod()
	if err != nil {
		return nil, err
	}
	searchedId := string(s.GetID())
	var searchedContainerName string
	for _, container := range pod.Status.Containers {
		if strings.HasSuffix(container.ID, searchedId) {
			searchedContainerName = container.Name
		}
	}
	if searchedContainerName == "" {
		return nil, fmt.Errorf("can't find container %s in pod %s", searchedId, pod.Metadata.Name)
	}
	var ports []int
	for _, container := range pod.Spec.Containers {
		if container.Name == searchedContainerName {
			for _, port := range container.Ports {
				ports = append(ports, port.ContainerPort)
			}
		}
	}

	return ports, nil
}
