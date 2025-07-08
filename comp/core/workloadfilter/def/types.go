// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadfilter

import (
	typedef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def/proto"
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

// Filterable is an interface for objects that can be filtered.
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
	ServiceType   ResourceType = "service"
	EndpointType  ResourceType = "endpoint"
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
func CreateContainer(container *workloadmeta.Container, owner Filterable) *Container {
	if container == nil {
		return nil
	}

	c := &typedef.FilterContainer{
		Id:    container.ID,
		Name:  container.Name,
		Image: container.Image.RawName,
	}

	setContainerOwner(c, owner)

	return &Container{
		FilterContainer: c,
		Owner:           owner,
	}
}

// CreateContainerFromOrch creates a Filterable Container object from a workloadmeta.OrchestratorContainer and an owner.
func CreateContainerFromOrch(container *workloadmeta.OrchestratorContainer, owner Filterable) *Container {
	if container == nil {
		return nil
	}

	c := &typedef.FilterContainer{
		Id:    container.ID,
		Name:  container.Name,
		Image: container.Image.RawName,
	}

	setContainerOwner(c, owner)

	return &Container{
		FilterContainer: c,
		Owner:           owner,
	}
}

// setContainerOwner sets the owner field in the FilterContainer based on the owner type.
func setContainerOwner(c *typedef.FilterContainer, owner Filterable) {
	if owner == nil {
		return
	}

	switch o := owner.(type) {
	case *Pod:
		if o != nil && o.FilterPod != nil {
			c.Owner = &typedef.FilterContainer_Pod{
				Pod: o.FilterPod,
			}
		}
	}
}

var _ Filterable = &Container{}

// Serialize converts the Container object to a filterable object.
func (c *Container) Serialize() any {
	return c.FilterContainer
}

// Type returns the resource type of the container.
func (c *Container) Type() ResourceType {
	return ContainerType
}

// ContainerFilter defines the type of container filter.
type ContainerFilter int

// Defined Container filter kinds
const (
	LegacyContainerMetrics ContainerFilter = iota
	LegacyContainerLogs
	LegacyContainerGlobal
	LegacyContainerACInclude
	LegacyContainerACExclude
	LegacyContainerSBOM
	ContainerADAnnotationsMetrics
	ContainerADAnnotationsLogs
	ContainerADAnnotations
	ContainerPaused
)

//
// Pod Definition
//

// Pod represents a pod object.
type Pod struct {
	*typedef.FilterPod
}

// CreatePod creates a Filterable Pod object from a workloadmeta.KubernetesPod.
func CreatePod(pod *workloadmeta.KubernetesPod) *Pod {
	if pod == nil {
		return nil
	}

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

// Serialize converts the Pod object to a filterable object.
func (p *Pod) Serialize() any {
	return p.FilterPod
}

// Type returns the resource type of the pod.
func (p *Pod) Type() ResourceType {
	return PodType
}

// PodFilter defines the type of pod filter.
type PodFilter int

// Defined Pod filter kinds
const (
	PodMetrics PodFilter = iota
	PodLogs
	PodGlobal
)

//
// Service Definition
//

// Service represents a filterable service object.
type Service struct {
	*typedef.FilterKubeService
}

// CreateService creates a Filterable Service object
func CreateService(name, namespace string, annotations map[string]string) *Service {
	return &Service{
		FilterKubeService: &typedef.FilterKubeService{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

var _ Filterable = &Service{}

// Serialize converts the Service object to a filterable object.
func (s *Service) Serialize() any {
	return s.FilterKubeService
}

// Type returns the resource type of the service.
func (s *Service) Type() ResourceType {
	return ServiceType
}

// ServiceFilter defines the type of service filter.
type ServiceFilter int

// Defined Service filter kinds
const (
	LegacyServiceMetrics ServiceFilter = iota
	LegacyServiceGlobal
)

//
// Endpoint Definition
//

// Endpoint represents a filterable endpoint object.
type Endpoint struct {
	*typedef.FilterKubeEndpoint
}

// CreateEndpoint creates a Filterable Endpoint object
func CreateEndpoint(name, namespace string, annotations map[string]string) *Endpoint {
	return &Endpoint{
		FilterKubeEndpoint: &typedef.FilterKubeEndpoint{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

var _ Filterable = &Endpoint{}

// Serialize converts the Endpoint object to a filterable object.
func (e *Endpoint) Serialize() any {
	return e.FilterKubeEndpoint
}

// Type returns the resource type of the endpoint.
func (e *Endpoint) Type() ResourceType {
	return EndpointType
}

// EndpointFilter defines the type of endpoint filter.
type EndpointFilter int

// Defined Endpoint filter kinds
const (
	LegacyEndpointMetrics EndpointFilter = iota
	LegacyEndpointGlobal
)
