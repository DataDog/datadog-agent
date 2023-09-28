// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package client holds the client to send data to the Cluster-Agent
// Package client holds the client to send data to the Cluster-Agent
package client

import (
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type languageSet map[string]struct{}
type containerInfo map[string]languageSet
type batch map[string]*podInfo

func (c containerInfo) toProto() []*pbgo.ContainerLanguageDetails {
	res := make([]*pbgo.ContainerLanguageDetails, 0, len(c))
	for containerName, languageSet := range c {
		res = append(res, &pbgo.ContainerLanguageDetails{
			ContainerName: containerName,
			Languages:     languageSet.toProto(),
		})
	}
	return res
}

func (l languageSet) add(language string) {
	l[language] = struct{}{}
}

func (l languageSet) merge(lang languageSet) {
	for language := range lang {
		l.add(language)
	}
}

func (l languageSet) toProto() []*pbgo.Language {
	res := make([]*pbgo.Language, 0, len(l))
	for lang := range l {
		res = append(res, &pbgo.Language{
			Name: lang,
		})
	}
	return res
}

func (b batch) getOrAddPodInfo(podName, podnamespace string, ownerRef *workloadmeta.KubernetesPodOwner) *podInfo {
	if podInfo, ok := b[podName]; ok {
		return podInfo
	}
	b[podName] = &podInfo{
		namespace:         podnamespace,
		containerInfo:     make(containerInfo),
		initContainerInfo: make(containerInfo),
		ownerRef:          ownerRef,
	}
	return b[podName]
}

func (b batch) merge(other batch) {
	for k, v := range other {
		podInfo, ok := b[k]
		if !ok {
			b[k] = v
			continue
		}
		podInfo.merge(v)
	}
}

type podInfo struct {
	namespace         string
	containerInfo     containerInfo
	initContainerInfo containerInfo
	ownerRef          *workloadmeta.KubernetesPodOwner
}

func (p *podInfo) merge(other *podInfo) {
	for containerName, otherLangSet := range other.containerInfo {
		langSet, ok := p.containerInfo[containerName]
		if !ok {
			p.containerInfo[containerName] = otherLangSet
			continue
		}
		langSet.merge(otherLangSet)
	}
	for containerName, otherLangSet := range other.initContainerInfo {
		langSet, ok := p.initContainerInfo[containerName]
		if !ok {
			p.initContainerInfo[containerName] = otherLangSet
			continue
		}
		langSet.merge(otherLangSet)
	}
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
		ContainerDetails:     p.containerInfo.toProto(),
		InitContainerDetails: p.initContainerInfo.toProto(),
	}
}

func (p *podInfo) getOrAddcontainerInfo(containerName string, isInitContainer bool) languageSet {
	cInfo := p.containerInfo
	if isInitContainer {
		cInfo = p.initContainerInfo
	}

	if languageSet, ok := cInfo[containerName]; ok {
		return languageSet
	}
	cInfo[containerName] = make(languageSet)
	return cInfo[containerName]
}

func (b batch) toProto() *pbgo.ParentLanguageAnnotationRequest {
	res := &pbgo.ParentLanguageAnnotationRequest{}
	for podName, language := range b {
		res.PodDetails = append(res.PodDetails, language.toProto(podName))
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
