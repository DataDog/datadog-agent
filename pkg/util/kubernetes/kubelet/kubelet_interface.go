// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet,!orchestrator

package kubelet

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// KubeUtilInterface defines the interface for kubelet api
// see `kubelet_orchestrator` for orchestrator-only interface
type KubeUtilInterface interface {
	GetNodeInfo(ctx context.Context) (string, string, error)
	GetNodename(ctx context.Context) (string, error)
	GetLocalPodList(ctx context.Context) ([]*Pod, error)
	ForceGetLocalPodList(ctx context.Context) ([]*Pod, error)
	GetPodForContainerID(ctx context.Context, containerID string) (*Pod, error)
	GetStatusForContainerID(pod *Pod, containerID string) (ContainerStatus, error)
	GetSpecForContainerName(pod *Pod, containerName string) (ContainerSpec, error)
	GetPodFromUID(ctx context.Context, podUID string) (*Pod, error)
	GetPodForEntityID(ctx context.Context, entityID string) (*Pod, error)
	QueryKubelet(ctx context.Context, path string) ([]byte, int, error)
	GetKubeletAPIEndpoint() string
	GetRawConnectionInfo() map[string]string
	GetRawMetrics(ctx context.Context) ([]byte, error)
	IsAgentHostNetwork(ctx context.Context) (bool, error)
	ListContainers(ctx context.Context) ([]*containers.Container, error)
	UpdateContainerMetrics(ctrList []*containers.Container) error
}
