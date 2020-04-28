// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker,kubelet

package listeners

// This files handles Host & Port lookup from the kubelet's pod status
// If needed for other orchestrators, we should introduce pluggable
// sources instead of adding yet another special case.

import (
	"fmt"
	"sort"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// DockerKubeletService overrides some methods when a container is
// running in kubernetes
type DockerKubeletService struct {
	DockerService
	kubeUtil kubelet.KubeUtilInterface
	Hosts    map[string]string
	Ports    []ContainerPort
	sync.RWMutex
}

// Make sure DockerKubeletService implements the Service interface
var _ Service = &DockerKubeletService{}

// getPod wraps KubeUtil init and pod lookup for both public methods.
func (s *DockerKubeletService) getPod() (*kubelet.Pod, error) {
	if s.kubeUtil == nil {
		var err error
		s.kubeUtil, err = kubelet.GetKubeUtil()
		if err != nil {
			return nil, err
		}
	}
	searchedID := s.GetEntity()
	return s.kubeUtil.GetPodForContainerID(searchedID)
}

// GetHosts returns the container's hosts
func (s *DockerKubeletService) GetHosts() (map[string]string, error) {
	s.Lock()
	defer s.Unlock()

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
func (s *DockerKubeletService) GetPorts() ([]ContainerPort, error) {
	if s.Ports != nil {
		return s.Ports, nil
	}

	pod, err := s.getPod()
	if err != nil {
		return nil, err
	}
	searchedID := s.GetEntity()
	var searchedContainerName string
	for _, container := range pod.Status.Containers {
		if container.ID == searchedID {
			searchedContainerName = container.Name
		}
	}
	if searchedContainerName == "" {
		return nil, fmt.Errorf("can't find container %s in pod %s", searchedID, pod.Metadata.Name)
	}
	ports := []ContainerPort{}
	for _, container := range pod.Spec.Containers {
		if container.Name == searchedContainerName {
			for _, port := range container.Ports {
				ports = append(ports, ContainerPort{port.ContainerPort, port.Name})
			}
		}
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})
	s.Ports = ports
	return ports, nil
}

// IsReady returns if the service is ready
func (s *DockerKubeletService) IsReady() bool {
	pod, err := s.getPod()
	if err != nil {
		return false
	}

	return kubelet.IsPodReady(pod)
}

// GetCheckNames returns slice of check names defined in kubernetes annotations or docker labels
// DockerKubeletService doesn't implement this method
func (s *DockerKubeletService) GetCheckNames() []string {
	return nil
}

// HasFilter always returns false
// DockerKubeletService doesn't implement this method
func (s *DockerKubeletService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig isn't supported
func (s *DockerKubeletService) GetExtraConfig(key string) (string, error) {
	return "", ErrNotSupported
}
