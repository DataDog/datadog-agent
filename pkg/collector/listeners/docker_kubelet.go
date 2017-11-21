// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker,kubelet

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
	kubeUtil *kubelet.KubeUtil
	Hosts    map[string]string
	Ports    []int
}

// getPod wraps KubeUtil init and pod lookup for both public methods.
func (s *DockerKubeletService) getPod() (*kubelet.Pod, error) {
	if s.kubeUtil == nil {
		var err error
		s.kubeUtil, err = kubelet.GetKubeUtil()
		if err != nil {
			return nil, err
		}
	}
	searchedId := docker.ContainerIDToEntityName(string(s.GetID()))
	return s.kubeUtil.GetPodForContainerID(searchedId)
}

// GetHosts returns the container's hosts
func (s *DockerKubeletService) GetHosts() (map[string]string, error) {
	if s.Hosts != nil {
		return s.Hosts, nil
	}

	pod, err := s.getPod()
	if err != nil {
		return nil, err
	}

	s.Hosts = map[string]string{"pod": pod.Status.PodIP}
	return s.Hosts, nil
}

// GetPorts returns the container's ports
func (s *DockerKubeletService) GetPorts() ([]int, error) {
	if s.Ports != nil {
		return s.Ports, nil
	}

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

	s.Ports = ports
	return ports, nil
}
