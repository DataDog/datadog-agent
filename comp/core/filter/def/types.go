// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filter

// Filterable is an interface that defines a method to convert an object to a map.
type Filterable interface {
	// ToMap converts the object to a map. The map is used to evaluate the rules
	ToMap() map[string]any
	// Key returns the resource type of the object. This is the key used to identify the rules
	Key() ResourceType
}

// ResourceType defines the type of resource.
type ResourceType string

// Type string
const (
	ContainerKey ResourceType = "container"
	PodKey       ResourceType = "pod"
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
}

var _ Filterable = &Container{}

// ToMap converts the Container object to a map.
func (c Container) ToMap() map[string]any {
	return map[string]any{
		"id":          c.ID,
		"name":        c.Name,
		"image":       c.Image,
		"namespace":   c.Namespace,
		"annotations": c.Annotations,
	}
}

// Key returns the resource type of the container.
func (c Container) Key() ResourceType {
	return ContainerKey
}

// ContainerFilter defines the type of container filter.
type ContainerFilter int

// Defined Container filter Kinds
const (
	ContainerMetrics ContainerFilter = iota
	ContainerLogs
	ContainerGlobal
	ContainerACLegacy
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

// Key returns the resource type of the pod.
func (p Pod) Key() ResourceType {
	return PodKey
}

// PodFilter defines the type of pod filter.
type PodFilter int

// Defined Pod filter Kinds
const (
	PodMetrics PodFilter = iota
	PodLogs
	PodGlobal
)
