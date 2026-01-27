// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"fmt"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

// podTemplateContext adapts a KubernetesPod to the TemplateContext interface
type podTemplateContext struct {
	pod       *workloadmeta.KubernetesPod
	container *workloadmeta.OrchestratorContainer
}

// NewPodTemplateContext creates a template context for a pod and optional container
func NewPodTemplateContext(pod *workloadmeta.KubernetesPod, container *workloadmeta.OrchestratorContainer) tmplvar.TemplateContext {
	return &podTemplateContext{
		pod:       pod,
		container: container,
	}
}

func (p *podTemplateContext) GetServiceID() string {
	if p.container != nil {
		return fmt.Sprintf("pod:%s/container:%s", p.pod.Name, p.container.Name)
	}
	return fmt.Sprintf("pod:%s", p.pod.Name)
}

func (p *podTemplateContext) GetHosts() (map[string]string, error) {
	hosts := make(map[string]string)
	if p.pod.IP != "" {
		hosts["pod"] = p.pod.IP
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no IP found for pod %s", p.pod.Name)
	}
	return hosts, nil
}

func (p *podTemplateContext) GetPorts() ([]tmplvar.ContainerPort, error) {
	if p.container == nil {
		return nil, fmt.Errorf("no container available for port resolution")
	}

	// For now, we don't have port information in OrchestratorContainer
	// This would need to be enhanced if we want to support %%port%% in pod-level tags
	return nil, fmt.Errorf("port resolution not supported for pod-level tags")
}

func (p *podTemplateContext) GetPid() (int, error) {
	return 0, fmt.Errorf("pid not available for pod %s", p.pod.Name)
}

func (p *podTemplateContext) GetHostname() (string, error) {
	return p.pod.Name, nil
}

func (p *podTemplateContext) GetExtraConfig(key string) (string, error) {
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

// containerTemplateContext adapts a Container to the TemplateContext interface
type containerTemplateContext struct {
	container *workloadmeta.Container
	pod       *workloadmeta.KubernetesPod
	store     workloadmeta.Component
}

// NewContainerTemplateContext creates a template context for a container
func NewContainerTemplateContext(container *workloadmeta.Container, pod *workloadmeta.KubernetesPod, store workloadmeta.Component) tmplvar.TemplateContext {
	return &containerTemplateContext{
		container: container,
		pod:       pod,
		store:     store,
	}
}

func (c *containerTemplateContext) GetServiceID() string {
	return fmt.Sprintf("container:%s", c.container.ID)
}

func (c *containerTemplateContext) GetHosts() (map[string]string, error) {
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

func (c *containerTemplateContext) GetPorts() ([]tmplvar.ContainerPort, error) {
	ports := make([]tmplvar.ContainerPort, 0, len(c.container.Ports))
	for _, port := range c.container.Ports {
		ports = append(ports, tmplvar.ContainerPort{
			Port: int(port.Port),
			Name: port.Name,
		})
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("no ports found for container %s", c.container.ID)
	}

	return ports, nil
}

func (c *containerTemplateContext) GetPid() (int, error) {
	if c.container.PID == 0 {
		return 0, fmt.Errorf("no PID available for container %s", c.container.ID)
	}
	return c.container.PID, nil
}

func (c *containerTemplateContext) GetHostname() (string, error) {
	if c.container.Hostname != "" {
		return c.container.Hostname, nil
	}
	return c.container.Name, nil
}

func (c *containerTemplateContext) GetExtraConfig(key string) (string, error) {
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
