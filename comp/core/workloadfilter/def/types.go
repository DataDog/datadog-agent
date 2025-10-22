// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadfilter

import (
	"fmt"
	"strings"

	typedef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def/proto"
)

// RuleBundle defines rules that apply to specific products
type RuleBundle struct {
	Products []Product                 `yaml:"products" json:"products" mapstructure:"products"`
	Rules    map[ResourceType][]string `yaml:"rules" json:"rules" mapstructure:"rules"`
}

// Product represents the different agent products that can use workload filters
type Product string

// Type string
const (
	ProductMetrics Product = "metrics"
	ProductLogs    Product = "logs"
	ProductSBOM    Product = "sbom"
	ProductGlobal  Product = "global"
)

// GetAllProducts returns a slice of all defined products
func GetAllProducts() []Product {
	return []Product{
		ProductMetrics,
		ProductLogs,
		ProductSBOM,
		ProductGlobal,
	}
}

// ResourceType defines the type of resource.
type ResourceType string

// Type string
const (
	ContainerType ResourceType = "container"
	PodType       ResourceType = "pod"
	ServiceType   ResourceType = "kube_service"
	EndpointType  ResourceType = "kube_endpoint"
	ProcessType   ResourceType = "process"
)

// Map of plural to singular resource types.
// This is the expected input key for the filtering configuration.
var singularMap = map[string]ResourceType{
	"containers":     ContainerType,
	"pods":           PodType,
	"kube_services":  ServiceType,
	"kube_endpoints": EndpointType,
	"processes":      ProcessType,
}

// ToSingular converts a plural resource type to its singular form.
func (rt ResourceType) ToSingular() ResourceType {
	if plural, ok := singularMap[string(rt)]; ok {
		return plural
	}
	return rt
}

// GetAllResourceTypes returns all defined resource types.
func GetAllResourceTypes() []ResourceType {
	return []ResourceType{
		ContainerType,
		PodType,
		ServiceType,
		EndpointType,
		ProcessType,
	}
}

// Rules defines the rules for filtering different resource types.
type Rules struct {
	Containers    []string `yaml:"containers" json:"containers"`
	Processes     []string `yaml:"processes" json:"processes"`
	Pods          []string `yaml:"pods" json:"pods"`
	KubeServices  []string `yaml:"kube_services" json:"kube_services"`
	KubeEndpoints []string `yaml:"kube_endpoints" json:"kube_endpoints"`
}

// String returns a string representation of the Rules struct, only showing non-empty fields.
func (r Rules) String() string {
	var parts []string

	if len(r.Containers) > 0 {
		parts = append(parts, fmt.Sprintf("containers:%v", r.Containers))
	}
	if len(r.Processes) > 0 {
		parts = append(parts, fmt.Sprintf("processes:%v", r.Processes))
	}
	if len(r.Pods) > 0 {
		parts = append(parts, fmt.Sprintf("pods:%v", r.Pods))
	}
	if len(r.KubeServices) > 0 {
		parts = append(parts, fmt.Sprintf("kube_services:%v", r.KubeServices))
	}
	if len(r.KubeEndpoints) > 0 {
		parts = append(parts, fmt.Sprintf("kube_endpoints:%v", r.KubeEndpoints))
	}

	if len(parts) == 0 {
		return ""
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

// Scope defines the scope of the filters.
type Scope string

// Predefined scopes for the filters.
const (
	GlobalFilter  Scope = "GlobalFilter"
	MetricsFilter Scope = "MetricsFilter"
	LogsFilter    Scope = "LogsFilter"
)

// FilterBundle represents a bundle of filters for a given resource type.
type FilterBundle interface {
	// IsExcluded checks if the given object is excluded by the filter bundle.
	IsExcluded(obj Filterable) bool
	// GetResult returns the result of the filter evaluation.
	GetResult(obj Filterable) Result
	// GetErrors returns any errors during initialization of the filters.
	GetErrors() []error
}

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
	// GetAnnotations returns the annotations of the object.
	GetAnnotations() map[string]string
	// GetName returns the name of the object.
	GetName() string
}

//
// Container Definition
//

// Container represents a filterable container object.
type Container struct {
	*typedef.FilterContainer
	Owner Filterable
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

// GetAnnotations returns the annotations of the container.
func (c *Container) GetAnnotations() map[string]string {
	// The container object itself does not have annotations.
	// Annotations are stored in the parent pod object.
	if c.FilterContainer.GetPod() != nil {
		return c.FilterContainer.GetPod().GetAnnotations()
	}
	return nil
}

// CreateContainerImage creates a Filterable Container Image object.
// This is used only for container image filtering
func CreateContainerImage(name string) *Container {
	return &Container{
		FilterContainer: &typedef.FilterContainer{
			Image: name,
		},
	}
}

// CreateContainer creates a Filterable Container object from a name, image and an (optional) owner.
func CreateContainer(id, name, img string, owner Filterable) *Container {
	c := &typedef.FilterContainer{
		Id:    id,
		Name:  name,
		Image: img,
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
	// CEL-based filters
	ContainerCELMetrics
	ContainerCELLogs
	ContainerCELSBOM
	ContainerCELGlobal
)

//
// Pod Definition
//

// Pod represents a pod object.
type Pod struct {
	*typedef.FilterPod
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

// CreatePod creates a Filterable Pod object.
func CreatePod(id, name, namespace string, annotations map[string]string) *Pod {
	return &Pod{
		FilterPod: &typedef.FilterPod{
			Id:          id,
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

// PodFilter defines the type of pod filter.
type PodFilter int

// Defined Pod filter kinds
const (
	LegacyPodMetrics PodFilter = iota
	LegacyPodGlobal
	PodADAnnotationsMetrics
	PodADAnnotations
	// CEL-based filters
	PodCELMetrics
	PodCELGlobal
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
	ServiceADAnnotationsMetrics
	ServiceADAnnotations
	// CEL-based filters
	ServiceCELMetrics
	ServiceCELGlobal
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
	EndpointADAnnotationsMetrics
	EndpointADAnnotations
	// CEL-based filters
	EndpointCELMetrics
	EndpointCELGlobal
)

//
// Process Definition
//

// Process represents a filterable process object.
type Process struct {
	*typedef.FilterProcess
}

var _ Filterable = &Process{}

// GetAnnotations returns the annotations of the process.
func (p *Process) GetAnnotations() map[string]string {
	return nil
}

// Serialize converts the Process object to a filterable object.
func (p *Process) Serialize() any {
	return p.FilterProcess
}

// Type returns the resource type of the process.
func (p *Process) Type() ResourceType {
	return ProcessType
}

// ProcessFilter defines the type of process filter.
type ProcessFilter int

// Defined Process filter kinds.
const (
	LegacyProcessExcludeList ProcessFilter = iota
)
