// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadmeta contains utility functions for creating filterable objects.
package workloadmeta

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	typedef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def/proto"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// CreateContainer creates a Filterable Container object from a workloadmeta.Container and an owner.
func CreateContainer(container *workloadmeta.Container, owner workloadfilter.Filterable) *workloadfilter.Container {
	if container == nil {
		return nil
	}

	c := &typedef.FilterContainer{
		Id:    container.ID,
		Name:  container.Name,
		Image: container.Image.RawName,
	}

	setContainerOwner(c, owner)

	return &workloadfilter.Container{
		FilterContainer: c,
		Owner:           owner,
	}
}

// CreateContainerFromOrch creates a Filterable Container object from a workloadmeta.OrchestratorContainer and an owner.
func CreateContainerFromOrch(container *workloadmeta.OrchestratorContainer, owner workloadfilter.Filterable) *workloadfilter.Container {
	if container == nil {
		return nil
	}

	c := &typedef.FilterContainer{
		Id:    container.ID,
		Name:  container.Name,
		Image: container.Image.RawName,
	}

	setContainerOwner(c, owner)

	return &workloadfilter.Container{
		FilterContainer: c,
		Owner:           owner,
	}
}

// setContainerOwner sets the owner field in the FilterContainer based on the owner type.
func setContainerOwner(c *typedef.FilterContainer, owner workloadfilter.Filterable) {
	if owner == nil {
		return
	}

	switch o := owner.(type) {
	case *workloadfilter.Pod:
		if o != nil && o.FilterPod != nil {
			c.Owner = &typedef.FilterContainer_Pod{
				Pod: o.FilterPod,
			}
		}
	}
}

// CreatePod creates a Filterable Pod object from a workloadmeta.KubernetesPod.
func CreatePod(pod *workloadmeta.KubernetesPod) *workloadfilter.Pod {
	if pod == nil {
		return nil
	}

	return &workloadfilter.Pod{
		FilterPod: &typedef.FilterPod{
			Id:          pod.ID,
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: pod.Annotations,
		},
	}
}
