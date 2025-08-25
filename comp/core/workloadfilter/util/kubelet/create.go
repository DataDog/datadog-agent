// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package kubelet contains utility functions for creating filterable objects.
package kubelet

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	typedef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def/proto"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// CreateContainer creates a Filterable Container object from a kubelet.ContainerStatus and an owner.
func CreateContainer(cStatus kubelet.ContainerStatus, owner workloadfilter.Filterable) *workloadfilter.Container {
	c := &typedef.FilterContainer{
		Id:    cStatus.ID,
		Name:  cStatus.Name,
		Image: cStatus.Image,
	}

	switch o := owner.(type) {
	case *workloadfilter.Pod:
		if o != nil && o.FilterPod != nil {
			c.Owner = &typedef.FilterContainer_Pod{
				Pod: o.FilterPod,
			}
		}
	}

	return &workloadfilter.Container{
		FilterContainer: c,
		Owner:           owner,
	}
}

// CreatePod creates a Filterable Pod object from a kubelet.Pod.
func CreatePod(pod *kubelet.Pod) *workloadfilter.Pod {
	if pod == nil {
		return nil
	}

	p := &typedef.FilterPod{
		Id:        pod.Metadata.UID,
		Name:      pod.Metadata.Name,
		Namespace: pod.Metadata.Namespace,
	}

	if pod.Metadata.Annotations != nil {
		p.Annotations = pod.Metadata.Annotations
	}

	return &workloadfilter.Pod{
		FilterPod: p,
	}
}
