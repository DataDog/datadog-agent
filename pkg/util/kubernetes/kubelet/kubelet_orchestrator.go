// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet,orchestrator

package kubelet

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
)

// from https://github.com/kubernetes/kubernetes/blob/abe6321296123aaba8e83978f7d17951ab1b64fd/pkg/util/node/node.go#L43
const nodeUnreachablePodReason = "NodeLost"

// KubeUtilInterface defines the interface for kubelet api
// and includes extra functions for the orchestrator build flag
type KubeUtilInterface interface {
	GetNodeInfo() (string, string, error)
	GetNodename() (string, error)
	GetLocalPodList() ([]*Pod, error)
	ForceGetLocalPodList() ([]*Pod, error)
	GetPodForContainerID(containerID string) (*Pod, error)
	GetStatusForContainerID(pod *Pod, containerID string) (ContainerStatus, error)
	GetPodFromUID(podUID string) (*Pod, error)
	GetPodForEntityID(entityID string) (*Pod, error)
	QueryKubelet(path string) ([]byte, int, error)
	GetKubeletAPIEndpoint() string
	GetRawConnectionInfo() map[string]string
	GetRawMetrics() ([]byte, error)
	ListContainers() ([]*containers.Container, error)
	IsAgentHostNetwork() (bool, error)
	UpdateContainerMetrics(ctrList []*containers.Container) error
	GetRawLocalPodList() ([]*v1.Pod, error)
}

// GetRawLocalPodList returns the unfiltered pod list from the kubelet
func (ku *KubeUtil) GetRawLocalPodList() ([]*v1.Pod, error) {
	data, code, err := ku.QueryKubelet(kubeletPodPath)

	if err != nil {
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.kubeletAPIEndpoint, kubeletPodPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletAPIEndpoint, kubeletPodPath, string(data))
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
	pods := make([]*v1.Pod, 0, len(podList.Items))
	for _, p := range podList.Items {
		pods = append(pods, &p)
	}

	return pods, nil
}

// ComputeStatus is mostly copied from kubernetes to match what users see in kubectl
// in case of issues, check for changes upstream: https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L685
func ComputeStatus(p *v1.Pod) string {
	reason := string(p.Status.Phase)
	if p.Status.Reason != "" {
		reason = p.Status.Reason
	}

	initializing := false
	for i := range p.Status.InitContainerStatuses {
		container := p.Status.InitContainerStatuses[i]
		switch {
		case container.State.Terminated != nil && container.State.Terminated.ExitCode == 0:
			continue
		case container.State.Terminated != nil:
			// initialization is failed
			if len(container.State.Terminated.Reason) == 0 {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Init:Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("Init:ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else {
				reason = "Init:" + container.State.Terminated.Reason
			}
			initializing = true
		case container.State.Waiting != nil && len(container.State.Waiting.Reason) > 0 && container.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + container.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, len(p.Spec.InitContainers))
			initializing = true
		}
		break
	}
	if !initializing {
		hasRunning := false
		for i := len(p.Status.ContainerStatuses) - 1; i >= 0; i-- {
			container := p.Status.ContainerStatuses[i]

			if container.State.Waiting != nil && container.State.Waiting.Reason != "" {
				reason = container.State.Waiting.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason != "" {
				reason = container.State.Terminated.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason == "" {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else if container.Ready && container.State.Running != nil {
				hasRunning = true
			}
		}

		// change pod status back to "Running" if there is at least one container still reporting as "Running" status
		if reason == "Completed" && hasRunning {
			reason = "Running"
		}
	}

	if p.DeletionTimestamp != nil && p.Status.Reason == nodeUnreachablePodReason {
		reason = "Unknown"
	} else if p.DeletionTimestamp != nil {
		reason = "Terminating"
	}

	return reason
}

// GetConditionMessage loops through the pod conditions, and reports the message of the first one
// (in the normal state transition order) that's doesn't pass
func GetConditionMessage(p *v1.Pod) string {
	messageMap := make(map[v1.PodConditionType]string)

	// from https://github.com/kubernetes/kubernetes/blob/ddd6d668f6a55cd3a8a2c2f268734e83524e5a7b/staging/src/k8s.io/api/core/v1/types.go#L2439-L2449
	// update if new ones appear
	chronologicalConditions := []v1.PodConditionType{
		v1.PodScheduled,
		v1.PodInitialized,
		v1.ContainersReady,
		v1.PodReady,
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range p.Status.Conditions {
		if c.Status == v1.ConditionFalse && c.Message != "" {
			messageMap[c.Type] = c.Message
		}
	}

	// return the message of the first one that failed
	for _, c := range chronologicalConditions {
		if m := messageMap[c]; m != "" {
			return m
		}
	}
	return ""
}
