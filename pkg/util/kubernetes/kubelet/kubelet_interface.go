// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet,!orchestrator

package kubelet

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// KubeUtilInterface defines the interface for kubelet api
// see `kubelet_orchestrator` for orchestrator-only interface
type KubeUtilInterface interface {
	GetNodeInfo() (string, string, error)
	GetNodename() (string, error)
	GetLocalPodList() ([]*Pod, error)
	ForceGetLocalPodList() ([]*Pod, error)
	GetPodForContainerID(containerID string) (*Pod, error)
	GetStatusForContainerID(pod *Pod, containerID string) (ContainerStatus, error)
	GetSpecForContainerName(pod *Pod, containerName string) (ContainerSpec, error)
	GetPodFromUID(podUID string) (*Pod, error)
	GetPodForEntityID(entityID string) (*Pod, error)
	QueryKubelet(path string) ([]byte, int, error)
	GetKubeletAPIEndpoint() string
	GetRawConnectionInfo() map[string]string
	GetRawMetrics() ([]byte, error)
	IsAgentHostNetwork() (bool, error)
	ListContainers() ([]*containers.Container, error)
	UpdateContainerMetrics(ctrList []*containers.Container) error
}
