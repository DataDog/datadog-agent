// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package clientimpl holds the client to send data to the Cluster-Agent
package clientimpl

import (
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

// eventsToRetry wraps all the events without pods and an expiration time for cleanup
type eventsToRetry struct {
	expirationTime time.Time
	events         []workloadmeta.Event
}

type batch map[string]*podInfo

func (b batch) getOrAddPodInfo(pod *workloadmeta.KubernetesPod) *podInfo {
	if podInfo, ok := b[pod.Name]; ok {
		// Refresh the containers list from the current pod state.
		// This is needed because the first process event for a pod may arrive
		// before the kubelet has fully populated the pod's container list in
		// workloadmeta, resulting in an empty containers map that would cause
		// hasCompleteLanguageInfo() to always return false.
		podInfo.containers = getContainersFromPod(pod)
		return podInfo
	}
	containers := getContainersFromPod(pod)
	b[pod.Name] = &podInfo{
		namespace:     pod.Namespace,
		containerInfo: make(languagemodels.ContainersLanguages),
		ownerRef:      &pod.Owners[0],
		containers:    containers,
	}
	return b[pod.Name]
}

type podInfo struct {
	namespace     string
	containerInfo languagemodels.ContainersLanguages
	ownerRef      *workloadmeta.KubernetesPodOwner
	// Record all of the containers in the pod
	containers map[languagemodels.Container]struct{}
}

func (p *podInfo) toProto(podName string) *pbgo.PodLanguageDetails {
	containersDetails, initContainersDetails := p.containerInfo.ToProto()
	return &pbgo.PodLanguageDetails{
		Name:      podName,
		Namespace: p.namespace,
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   p.ownerRef.ID,
			Name: p.ownerRef.Name,
			Kind: p.ownerRef.Kind,
		},
		ContainerDetails:     containersDetails,
		InitContainerDetails: initContainersDetails,
	}
}

func (p *podInfo) getOrAddContainerInfo(containerName string, isInitContainer bool) languagemodels.LanguageSet {
	cInfo := p.containerInfo

	container := languagemodels.Container{
		Name: containerName,
		Init: isInitContainer,
	}
	if languageSet, ok := cInfo[container]; ok {
		return languageSet
	}

	cInfo[container] = make(languagemodels.LanguageSet)
	return cInfo[container]
}

// hasCompleteLanguageInfo returns true if the pod has language information for all containers.
//
// A pod has language information if it meets the following conditions: (1) One process event for
// each container in the pod has been received, and (2) the process check successfully detected at
// least one supported language in at least one container in the pod.
//
// We don't consider init containers here because they are short-lived.
func (p *podInfo) hasCompleteLanguageInfo() bool {
	// Preserve not sending podInfo if all containers have no language detected
	atLeastOneContainerLanguageDetected := false
	for container := range p.containers {
		// Haven't received a process event from this container
		if cInfo, ok := p.containerInfo[container]; !ok {
			return false
		} else if len(cInfo) > 0 {
			atLeastOneContainerLanguageDetected = true
		}
	}
	return atLeastOneContainerLanguageDetected
}

func (b batch) toProto() *pbgo.ParentLanguageAnnotationRequest {
	res := &pbgo.ParentLanguageAnnotationRequest{}
	for podName, podInfo := range b {
		if podInfo.hasCompleteLanguageInfo() {
			res.PodDetails = append(res.PodDetails, podInfo.toProto(podName))
		}
	}

	// No pods with complete language information
	if len(res.PodDetails) == 0 {
		return nil
	}
	return res
}

// getContainerInfoFromPod returns the name of the container, if it is an init container and if it is found
func getContainerInfoFromPod(cid string, pod *workloadmeta.KubernetesPod) (string, bool, bool) {
	for _, container := range pod.Containers {
		if container.ID == cid {
			return container.Name, false, true
		}
	}
	for _, container := range pod.InitContainers {
		if container.ID == cid {
			return container.Name, true, true
		}
	}
	return "", false, false
}

func podHasOwner(pod *workloadmeta.KubernetesPod) bool {
	return len(pod.Owners) > 0
}

// getContainersFromPod returns the containers from a pod
func getContainersFromPod(pod *workloadmeta.KubernetesPod) (containers map[languagemodels.Container]struct{}) {
	containers = make(map[languagemodels.Container]struct{})
	for _, container := range pod.Containers {
		c := *languagemodels.NewContainer(container.Name)
		containers[c] = struct{}{}
	}
	return
}
