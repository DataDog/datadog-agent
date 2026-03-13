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
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

// workloadmetaResolvable adapts workloadmeta entities (Pod/Container) to the tmplvar.Resolvable interface
// so they can be used with template variable resolution
type workloadmetaResolvable struct {
	pod       *workloadmeta.KubernetesPod
	container *workloadmeta.Container
}

var _ tmplvar.Resolvable = (*workloadmetaResolvable)(nil)

// newResolvableAdapter creates a Resolvable adapter for a pod
func newResolvableAdapter(pod *workloadmeta.KubernetesPod, container *workloadmeta.Container) tmplvar.Resolvable {
	return &workloadmetaResolvable{
		pod:       pod,
		container: container,
	}
}

func (w *workloadmetaResolvable) GetServiceID() string {
	if w.container != nil {
		return containers.BuildEntityName(string(w.container.Runtime), w.container.ID)
	}
	if w.pod != nil {
		return kubelet.PodUIDToEntityName(w.pod.EntityID.ID)
	}
	return "unknown"
}

func (w *workloadmetaResolvable) GetHosts() (map[string]string, error) {
	hosts := make(map[string]string)

	if w.container != nil {
		hosts = docker.ContainerHosts(w.container.NetworkIPs, w.container.Labels, w.container.Hostname)
	}

	if w.pod != nil && w.pod.IP != "" {
		hosts["pod"] = w.pod.IP
	}
	return hosts, nil
}

func (w *workloadmetaResolvable) GetPorts() ([]workloadmeta.ContainerPort, error) {
	if w.container == nil {
		return nil, errors.New("no container available for port resolution")
	}

	ports := make([]workloadmeta.ContainerPort, 0, len(w.container.Ports))
	for _, port := range w.container.Ports {
		ports = append(ports, workloadmeta.ContainerPort{
			Port: port.Port,
			Name: port.Name,
		})
	}

	return ports, nil
}

func (w *workloadmetaResolvable) GetPid() (int, error) {
	if w.container == nil {
		return 0, errors.New("pid not available without container")
	}

	if w.container.PID == 0 {
		return 0, fmt.Errorf("no PID available for container %s", w.container.ID)
	}

	return w.container.PID, nil
}

func (w *workloadmetaResolvable) GetHostname() (string, error) {
	if w.container == nil {
		return "", errors.New("no container available for hostname resolution")
	}
	return w.container.Hostname, nil
}

func (w *workloadmetaResolvable) GetExtraConfig(key string) (string, error) {
	// Support kube_* variables if we have pod context
	if w.pod != nil {
		switch key {
		case "namespace":
			return w.pod.Namespace, nil
		case "pod_name":
			return w.pod.Name, nil
		case "pod_uid":
			return w.pod.EntityID.ID, nil
		}
	}

	return "", fmt.Errorf("extra config key %q not supported", key)
}
