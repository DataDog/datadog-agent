// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package client holds the client to send data to the Cluster-Agent
package client

import (
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type containerInfo struct {
	languages map[string]*languagesSet
}

func newContainerInfo() *containerInfo {
	return &containerInfo{
		languages: make(map[string]*languagesSet),
	}
}

func (c *containerInfo) toProto() []*pbgo.ContainerLanguageDetails {
	res := make([]*pbgo.ContainerLanguageDetails, 0, len(c.languages))
	for containerName, languageSet := range c.languages {
		res = append(res, &pbgo.ContainerLanguageDetails{
			ContainerName: containerName,
			Languages:     languageSet.toProto(),
		})
	}
	return res
}

type languagesSet struct {
	languages map[string]struct{}
}

func newLanguageSet() *languagesSet {
	return &languagesSet{
		languages: make(map[string]struct{}),
	}
}

func (l *languagesSet) add(language string) {
	l.languages[language] = struct{}{}
}

func (l *languagesSet) merge(lang *languagesSet) {
	for language, _ := range lang.languages {
		l.add(language)
	}
}

func (l *languagesSet) toProto() []*pbgo.Language {
	res := make([]*pbgo.Language, 0, len(l.languages))
	for lang := range l.languages {
		res = append(res, &pbgo.Language{
			Name: lang,
		})
	}
	return res
}

type podInfo struct {
	namespace         string
	containerInfo     *containerInfo
	initContainerInfo *containerInfo
	ownerRef          *workloadmeta.KubernetesPodOwner
}

func (p *podInfo) merge(other *podInfo) {
	for k, otherLangSet := range other.containerInfo.languages {
		langSet, ok := p.containerInfo.languages[k]
		if !ok {
			p.containerInfo.languages[k] = otherLangSet
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

func (p *podInfo) getOrAddcontainerInfo(containerName string, isInitContainer bool) *languagesSet {
	cInfo := p.containerInfo
	if isInitContainer {
		cInfo = p.initContainerInfo
	}

	if languagesSet, ok := cInfo.languages[containerName]; ok {
		return languagesSet
	}
	cInfo.languages[containerName] = newLanguageSet()
	return cInfo.languages[containerName]
}

type batch struct {
	podInfo map[string]*podInfo
}

func newBatch() *batch { return &batch{make(map[string]*podInfo)} }

func (b *batch) getOrAddPodInfo(podName, podNamespace string, ownerRef *workloadmeta.KubernetesPodOwner) *podInfo {
	if podInfo, ok := b.podInfo[podName]; ok {
		return podInfo
	}
	b.podInfo[podName] = &podInfo{
		namespace:         podNamespace,
		containerInfo:     newContainerInfo(),
		initContainerInfo: newContainerInfo(),
		ownerRef:          ownerRef,
	}
	return b.podInfo[podName]
}

func (b *batch) merge(other *batch) {
	for k, v := range other.podInfo {
		podInfo, ok := b.podInfo[k]
		if !ok {
			b.podInfo[k] = v
			continue
		}
		podInfo.merge(v)
	}
}

func (b *batch) toProto() *pbgo.ParentLanguageAnnotationRequest {
	res := &pbgo.ParentLanguageAnnotationRequest{}
	for podName, language := range b.podInfo {
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
