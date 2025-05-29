// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filter

import (
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
	// ToMap converts the object to a map. The map is used to evaluate the program's rules
	ToMap() map[string]any
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

// Container represents a container object.
type Container struct {
	ID          string
	Name        string
	Image       string
	Namespace   string
	Annotations map[string]string
	Owner       Filterable
}

// CreateContainer creates a Filterable Container object from a workloadmeta.Container and an owner.
func CreateContainer(container workloadmeta.Container, owner Filterable) Container {
	return Container{
		ID:          container.ID,
		Name:        container.Name,
		Image:       container.Image.Name,
		Namespace:   container.Namespace,
		Annotations: container.Annotations,
		Owner:       owner,
	}
}

var _ Filterable = &Container{}

// ToMap converts the Container object to a map.
func (c Container) ToMap() map[string]any {
	m := map[string]any{
		"id":          c.ID,
		"name":        c.Name,
		"image":       c.Image,
		"namespace":   c.Namespace,
		"annotations": c.Annotations,
	}
	if c.Owner != nil {
		m[string(c.Owner.Type())] = c.Owner.ToMap()
	}
	return m
}

// Type returns the resource type of the container.
func (c Container) Type() ResourceType {
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
	ID          string
	Name        string
	Namespace   string
	Annotations map[string]string
}

// CreatePod creates a Filterable Pod object from a workloadmeta.KubernetesPod.
func CreatePod(pod workloadmeta.KubernetesPod) Pod {
	return Pod{
		ID:          pod.ID,
		Name:        pod.Name,
		Namespace:   pod.Namespace,
		Annotations: pod.Annotations,
	}
}

var _ Filterable = &Pod{}

// ToMap converts the Pod object to a map.
func (p Pod) ToMap() map[string]any {
	return map[string]any{
		"id":          p.ID,
		"name":        p.Name,
		"namespace":   p.Namespace,
		"annotations": p.Annotations,
	}
}

// Type returns the resource type of the pod.
func (p Pod) Type() ResourceType {
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
