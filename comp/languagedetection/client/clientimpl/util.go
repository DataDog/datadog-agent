// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package clientimpl holds the client to send data to the Cluster-Agent
package clientimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/util"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

// eventsToRetry wraps all the events without pods and an expiration time for cleanup
type eventsToRetry struct {
	expirationTime time.Time
	events         []workloadmeta.Event
}

type batch map[string]*podInfo

func (b batch) getOrAddPodInfo(podName, podnamespace string, ownerRef *workloadmeta.KubernetesPodOwner) *podInfo {
	if podInfo, ok := b[podName]; ok {
		return podInfo
	}
	b[podName] = &podInfo{
		namespace:         podnamespace,
		containerInfo:     util.NewContainersLanguages(),
		initContainerInfo: util.NewContainersLanguages(),
		ownerRef:          ownerRef,
	}
	return b[podName]
}

type podInfo struct {
	namespace         string
	containerInfo     util.ContainersLanguages
	initContainerInfo util.ContainersLanguages
	ownerRef          *workloadmeta.KubernetesPodOwner
}

func (p *podInfo) toProto(podName string) *pbgo.PodLanguageDetails {
	return &pbgo.PodLanguageDetails{
		Name:      podName,
		Namespace: p.namespace,
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   p.ownerRef.ID,
			Name: p.ownerRef.Name,
			Kind: p.ownerRef.Kind,
		},
		ContainerDetails:     p.containerInfo.ToProto(),
		InitContainerDetails: p.initContainerInfo.ToProto(),
	}
}

func (p *podInfo) getOrAddContainerInfo(containerName string, isInitContainer bool) util.LanguageSet {
	cInfo := p.containerInfo
	if isInitContainer {
		cInfo = p.initContainerInfo
	}

	if languageSet, ok := cInfo[containerName]; ok {
		return languageSet
	}
	cInfo[containerName] = util.NewLanguageSet()
	return cInfo[containerName]
}

func (b batch) toProto() *pbgo.ParentLanguageAnnotationRequest {
	res := &pbgo.ParentLanguageAnnotationRequest{}
	for podName, podInfo := range b {
		res.PodDetails = append(res.PodDetails, podInfo.toProto(podName))
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
	return pod.Owners != nil && len(pod.Owners) > 0
}
