// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet,orchestrator

package kubelet

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
)

// KubeUtilInterface defines the interface for kubelet api
// and includes extra functions for the orchestrator build flag
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
	ListContainers(ctx context.Context) ([]*containers.Container, error)
	IsAgentHostNetwork(ctx context.Context) (bool, error)
	UpdateContainerMetrics(ctrList []*containers.Container) error
	GetRawLocalPodList(ctx context.Context) ([]*v1.Pod, error)
}

// GetRawLocalPodList returns the unfiltered pod list from the kubelet
func (ku *KubeUtil) GetRawLocalPodList(ctx context.Context) ([]*v1.Pod, error) {
	data, code, err := ku.QueryKubelet(ctx, kubeletPodPath)

	if err != nil {
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.kubeletClient.kubeletURL, kubeletPodPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletClient.kubeletURL, kubeletPodPath, string(data))
	}

	podListData, err := runtime.Decode(clientsetscheme.Codecs.UniversalDecoder(v1.SchemeGroupVersion), data)
	if err != nil {
		return nil, fmt.Errorf("unable to decode the pod list: %s", err)
	}
	podList, ok := podListData.(*v1.PodList)
	if !ok {
		return nil, fmt.Errorf("pod list type assertion failed on %v", podListData)
	}

	// transform []v1.Pod in []*v1.Pod
	pods := make([]*v1.Pod, len(podList.Items))
	for i := 0; i < len(pods); i++ {
		pods[i] = &podList.Items[i]
	}

	return pods, nil
}
