// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"errors"
	"fmt"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

// resolvablePodAdapter adapts a KubernetesPod to the tmplvar.Resolvable interface
// so it can be used with template variable resolution
type resolvablePodAdapter struct {
	pod       *workloadmeta.KubernetesPod
	container *workloadmeta.OrchestratorContainer
}

var _ tmplvar.Resolvable = (*resolvablePodAdapter)(nil)

// newResolvablePodAdapter creates a Service adapter for a pod
func newResolvablePodAdapter(pod *workloadmeta.KubernetesPod, container *workloadmeta.OrchestratorContainer) tmplvar.Resolvable {
	return &resolvablePodAdapter{
		pod:       pod,
		container: container,
	}
}

func (p *resolvablePodAdapter) GetServiceID() string {
	return kubelet.PodUIDToEntityName(p.pod.EntityID.ID)
}

func (p *resolvablePodAdapter) GetHosts() (map[string]string, error) {
	hosts := make(map[string]string)
	if p.pod.IP != "" {
		hosts["pod"] = p.pod.IP
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no IP found for pod %s", p.pod.Name)
	}
	return hosts, nil
}

func (p *resolvablePodAdapter) GetPorts() ([]tmplvar.ContainerPort, error) {
	if p.container == nil {
		return nil, errors.New("no container available for port resolution")
	}

	// For now, we don't have port information in OrchestratorContainer
	// This would need to be enhanced if we want to support %%port%% in pod-level tags
	return nil, errors.New("port resolution not supported for pod-level tags")
}

func (p *resolvablePodAdapter) GetPid() (int, error) {
	return 0, fmt.Errorf("pid not available for pod %s", p.pod.Name)
}

func (p *resolvablePodAdapter) GetHostname() (string, error) {
	return p.pod.Name, nil
}

func (p *resolvablePodAdapter) GetExtraConfig(key string) (string, error) {
	switch key {
	case "namespace":
		return p.pod.Namespace, nil
	case "pod_name":
		return p.pod.Name, nil
	case "pod_uid":
		return p.pod.EntityID.ID, nil
	default:
		return "", fmt.Errorf("extra config key %q not supported for pod", key)
	}
}

// resolvableContainerAdapter adapts a Container to the tmplvar.Resolvable interface
type resolvableContainerAdapter struct {
	container *workloadmeta.Container
	pod       *workloadmeta.KubernetesPod
	store     workloadmeta.Component
}

var _ tmplvar.Resolvable = (*resolvableContainerAdapter)(nil)

// newResolvableContainerAdapter creates a Service adapter for a container
func newResolvableContainerAdapter(container *workloadmeta.Container, pod *workloadmeta.KubernetesPod, store workloadmeta.Component) tmplvar.Resolvable {
	return &resolvableContainerAdapter{
		container: container,
		pod:       pod,
		store:     store,
	}
}

func (c *resolvableContainerAdapter) GetServiceID() string {
	return containers.BuildEntityName(string(c.container.Runtime), c.container.ID)
}

func (c *resolvableContainerAdapter) GetHosts() (map[string]string, error) {
	hosts := make(map[string]string)

	// Add container's network IPs
	for netName, netConfig := range c.container.NetworkIPs {
		hosts[netName] = netConfig
	}

	// If we have a pod, also add pod IP
	if c.pod != nil && c.pod.IP != "" {
		hosts["pod"] = c.pod.IP
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("no IP found for container %s", c.container.ID)
	}

	return hosts, nil
}

func (c *resolvableContainerAdapter) GetPorts() ([]tmplvar.ContainerPort, error) {
	ports := make([]tmplvar.ContainerPort, 0, len(c.container.Ports))
	for _, port := range c.container.Ports {
		ports = append(ports, tmplvar.ContainerPort{
			Port: port.Port,
			Name: port.Name,
		})
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("no ports found for container %s", c.container.ID)
	}

	return ports, nil
}

func (c *resolvableContainerAdapter) GetPid() (int, error) {
	if c.container.PID == 0 {
		return 0, fmt.Errorf("no PID available for container %s", c.container.ID)
	}
	return c.container.PID, nil
}

func (c *resolvableContainerAdapter) GetHostname() (string, error) {
	if c.container.Hostname != "" {
		return c.container.Hostname, nil
	}
	return c.container.Name, nil
}

func (c *resolvableContainerAdapter) GetExtraConfig(key string) (string, error) {
	// If we have pod context, support kube_* variables
	if c.pod != nil {
		switch key {
		case "namespace":
			return c.pod.Namespace, nil
		case "pod_name":
			return c.pod.Name, nil
		case "pod_uid":
			return c.pod.EntityID.ID, nil
		}
	}

	return "", fmt.Errorf("extra config key %q not supported for container", key)
}
