// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filter

import (
	typedef "github.com/DataDog/datadog-agent/comp/core/filter/def/proto"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// Result is an enumeration that represents the possible results of a filter evaluation.
type Result int

// filterResult represents the result of a filter evaluation.
const (
	Included Result = iota
	Excluded
	Unknown
)

// Filterable is an interface that defines a method to convert an object to a map.
type Filterable interface {
	// Serialize converts the object into a filterable object.
	Serialize() any
	// Type returns the resource type of the object.
	Type() ResourceType
}

// ResourceType defines the type of resource.
type ResourceType string

// Type string
const (
	ContainerType ResourceType = "container"
	PodType       ResourceType = "pod"
)

//
// Container Definition
//

// Container represents a filterable container object.
type Container struct {
	*typedef.FilterContainer
	Owner Filterable
}

// CreateContainer creates a Filterable Container object from a workloadmeta.Container and an owner.
func CreateContainer(container workloadmeta.Container, owner Filterable) *Container {
	c := &typedef.FilterContainer{
		Id:    container.ID,
		Name:  container.Name,
		Image: container.Image.RawName,
	}

	if owner != nil {
		switch o := owner.(type) {
		case *Pod:
			c.Owner = &typedef.FilterContainer_Pod{
				Pod: o.FilterPod,
			}
		}
	}

	return &Container{
		FilterContainer: c,
		Owner:           owner,
	}
}

var _ Filterable = &Container{}

// Serialize converts the Container object to a map.
func (c *Container) Serialize() any {
	return c.FilterContainer
}

// Type returns the resource type of the container.
func (c *Container) Type() ResourceType {
	return ContainerType
}

// ContainerFilter defines the type of container filter.
type ContainerFilter int

// Defined Container filter Kinds
const (
	ContainerMetrics ContainerFilter = iota
	ContainerLogs
	ContainerGlobal
	ContainerACLegacyInclude
	ContainerACLegacyExclude
	ContainerADAnnotations
	ContainerPaused
	ContainerSBOM
)

//
// Pod Definition
//

// Pod represents a pod object.
type Pod struct {
	*typedef.FilterPod
}

// CreatePod creates a Filterable Pod object from a workloadmeta.KubernetesPod.
func CreatePod(pod workloadmeta.KubernetesPod) *Pod {
	return &Pod{
		FilterPod: &typedef.FilterPod{
			Id:          pod.ID,
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: pod.Annotations,
		},
	}
}

var _ Filterable = &Pod{}

// Serialize converts the Pod object to a map.
func (p *Pod) Serialize() any {
	return p.FilterPod
}

// Type returns the resource type of the pod.
func (p *Pod) Type() ResourceType {
	return PodType
}

// PodFilter defines the type of pod filter.
type PodFilter int

// Defined Pod filter Kinds
const (
	PodMetrics PodFilter = iota
	PodLogs
	PodGlobal
)
