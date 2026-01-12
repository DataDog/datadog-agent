// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadfilter

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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
	ContainerType    ResourceType = "container"
	PodType          ResourceType = "pod"
	KubeServiceType  ResourceType = "kube_service"
	KubeEndpointType ResourceType = "kube_endpoint"
	ProcessType      ResourceType = "process"
)

// Map of plural to singular resource types.
// This is the expected input key for the filtering configuration.
var singularMap = map[string]ResourceType{
	"containers":     ContainerType,
	"pods":           PodType,
	"kube_services":  KubeServiceType,
	"kube_endpoints": KubeEndpointType,
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
		KubeServiceType,
		KubeEndpointType,
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

// String returns the string representation of the Result.
func (r Result) String() string {
	switch r {
	case Included:
		return "included"
	case Excluded:
		return "excluded"
	default:
		return "unknown"
	}
}

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
	// ToBytes converts the object into a byte slice.
	ToBytes() ([]byte, error)
}

// FilterIdentifier identifies a specific filter instance
type FilterIdentifier interface {
	TargetResource() ResourceType
	GetFilterName() string
}

//
// Container Definition
//

// Container represents a filterable container object.
type Container struct {
	*core.FilterContainer
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

// ToBytes converts the Container object to a byte slice.
func (c *Container) ToBytes() ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(c.FilterContainer)
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
func CreateContainerImage(reference string) *Container {
	return &Container{
		FilterContainer: &core.FilterContainer{
			Image: &core.FilterImage{
				Reference: reference,
			},
		},
	}
}

// CreateContainer creates a Filterable Container object from a name, image and an (optional) owner.
func CreateContainer(id, name, reference string, owner Filterable) *Container {
	c := &core.FilterContainer{
		Id:   id,
		Name: name,
		Image: &core.FilterImage{
			Reference: reference,
		},
	}

	setContainerOwner(c, owner)

	return &Container{
		FilterContainer: c,
	}
}

// setContainerOwner sets the owner field in the FilterContainer based on the owner type.
func setContainerOwner(c *core.FilterContainer, owner Filterable) {
	if owner == nil {
		return
	}

	switch o := owner.(type) {
	case *Pod:
		if o != nil && o.FilterPod != nil {
			c.Owner = &core.FilterContainer_Pod{
				Pod: o.FilterPod,
			}
		}
	}
}

// ContainerFilter defines the type of container filter.
type ContainerFilter string

// TargetResource returns the resource type for ContainerFilter
func (f ContainerFilter) TargetResource() ResourceType {
	return ContainerType
}

// GetFilterName returns the name for ContainerFilter
func (f ContainerFilter) GetFilterName() string {
	return string(f)
}

// Defined Container filter kinds
const (
	ContainerLegacyMetrics         ContainerFilter = "container-legacy-metrics"
	ContainerLegacyLogs            ContainerFilter = "container-legacy-logs"
	ContainerLegacyGlobal          ContainerFilter = "container-legacy-global"
	ContainerLegacyACInclude       ContainerFilter = "container-legacy-ac-include"
	ContainerLegacyACExclude       ContainerFilter = "container-legacy-ac-exclude"
	ContainerLegacySBOM            ContainerFilter = "container-legacy-sbom"
	ContainerLegacyRuntimeSecurity ContainerFilter = "container-legacy-runtime-security"
	ContainerLegacyCompliance      ContainerFilter = "container-legacy-compliance"
	ContainerADAnnotationsMetrics  ContainerFilter = "container-ad-annotations-metrics"
	ContainerADAnnotationsLogs     ContainerFilter = "container-ad-annotations-logs"
	ContainerADAnnotations         ContainerFilter = "container-ad-annotations"
	ContainerPaused                ContainerFilter = "container-paused"
	// CEL-based filters
	ContainerCELMetrics ContainerFilter = "container-cel-metrics"
	ContainerCELLogs    ContainerFilter = "container-cel-logs"
	ContainerCELSBOM    ContainerFilter = "container-cel-sbom"
	ContainerCELGlobal  ContainerFilter = "container-cel-global"
)

//
// Pod Definition
//

// Pod represents a pod object.
type Pod struct {
	*core.FilterPod
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

// ToBytes converts the Pod object to a byte slice.
func (p *Pod) ToBytes() ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(p.FilterPod)
}

// CreatePod creates a Filterable Pod object.
func CreatePod(id, name, namespace string, annotations map[string]string) *Pod {
	return &Pod{
		FilterPod: &core.FilterPod{
			Id:          id,
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

// PodFilter defines the type of pod filter.
type PodFilter string

// TargetResource returns the resource type for PodFilter
func (f PodFilter) TargetResource() ResourceType {
	return PodType
}

// GetFilterName returns the name for PodFilter
func (f PodFilter) GetFilterName() string {
	return string(f)
}

// Defined Pod filter kinds
const (
	PodLegacyMetrics        PodFilter = "pod-legacy-metrics"
	PodLegacyGlobal         PodFilter = "pod-legacy-global"
	PodADAnnotationsMetrics PodFilter = "pod-ad-annotations-metrics"
	PodADAnnotations        PodFilter = "pod-ad-annotations"
	// CEL-based filters
	PodCELMetrics PodFilter = "pod-cel-metrics"
	PodCELGlobal  PodFilter = "pod-cel-global"
)

//
// KubeService Definition
//

// KubeService represents a filterable kube service object.
type KubeService struct {
	*core.FilterKubeService
}

// CreateKubeService creates a Filterable KubeService object
func CreateKubeService(name, namespace string, annotations map[string]string) *KubeService {
	return &KubeService{
		FilterKubeService: &core.FilterKubeService{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

var _ Filterable = &KubeService{}

// Serialize converts the KubeService object to a filterable object.
func (s *KubeService) Serialize() any {
	return s.FilterKubeService
}

// Type returns the resource type of the kube service.
func (s *KubeService) Type() ResourceType {
	return KubeServiceType
}

// ToBytes converts the KubeService object to a byte slice.
func (s *KubeService) ToBytes() ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(s.FilterKubeService)
}

// KubeServiceFilter defines the type of kube service filter.
type KubeServiceFilter string

// TargetResource returns the resource type for KubeServiceFilter
func (f KubeServiceFilter) TargetResource() ResourceType {
	return KubeServiceType
}

// GetFilterName returns the name for KubeServiceFilter
func (f KubeServiceFilter) GetFilterName() string {
	return string(f)
}

// Defined KubeService filter kinds
const (
	KubeServiceLegacyMetrics        KubeServiceFilter = "service-legacy-metrics"
	KubeServiceLegacyGlobal         KubeServiceFilter = "service-legacy-global"
	KubeServiceADAnnotationsMetrics KubeServiceFilter = "service-ad-annotations-metrics"
	KubeServiceADAnnotations        KubeServiceFilter = "service-ad-annotations"
	// CEL-based filters
	KubeServiceCELMetrics KubeServiceFilter = "service-cel-metrics"
	KubeServiceCELGlobal  KubeServiceFilter = "service-cel-global"
)

//
// KubeEndpoint Definition
//

// KubeEndpoint represents a filterable kube endpoint object.
type KubeEndpoint struct {
	*core.FilterKubeEndpoint
}

// CreateKubeEndpoint creates a Filterable KubeEndpoint object
func CreateKubeEndpoint(name, namespace string, annotations map[string]string) *KubeEndpoint {
	return &KubeEndpoint{
		FilterKubeEndpoint: &core.FilterKubeEndpoint{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

var _ Filterable = &KubeEndpoint{}

// Serialize converts the KubeEndpoint object to a filterable object.
func (e *KubeEndpoint) Serialize() any {
	return e.FilterKubeEndpoint
}

// Type returns the resource type of the kube endpoint.
func (e *KubeEndpoint) Type() ResourceType {
	return KubeEndpointType
}

// ToBytes converts the KubeEndpoint object to a byte slice.
func (e *KubeEndpoint) ToBytes() ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(e.FilterKubeEndpoint)
}

// KubeEndpointFilter defines the type of kube endpoint filter.
type KubeEndpointFilter string

// TargetResource returns the resource type for KubeEndpointFilter
func (f KubeEndpointFilter) TargetResource() ResourceType {
	return KubeEndpointType
}

// GetFilterName returns the name for KubeEndpointFilter
func (f KubeEndpointFilter) GetFilterName() string {
	return string(f)
}

// Defined Endpoint filter kinds
const (
	KubeEndpointLegacyMetrics        KubeEndpointFilter = "endpoint-legacy-metrics"
	KubeEndpointLegacyGlobal         KubeEndpointFilter = "endpoint-legacy-global"
	KubeEndpointADAnnotationsMetrics KubeEndpointFilter = "endpoint-ad-annotations-metrics"
	KubeEndpointADAnnotations        KubeEndpointFilter = "endpoint-ad-annotations"
	// CEL-based filters
	KubeEndpointCELMetrics KubeEndpointFilter = "endpoint-cel-metrics"
	KubeEndpointCELGlobal  KubeEndpointFilter = "endpoint-cel-global"
)

//
// Process Definition
//

// Process represents a filterable process object.
type Process struct {
	*core.FilterProcess
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

// ToBytes converts the Process object to a byte slice.
func (p *Process) ToBytes() ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(p.FilterProcess)
}

// SetLogFile updates the log file path on an existing Process.
func (p *Process) SetLogFile(logFile string) {
	p.FilterProcess.LogFile = logFile
}

// ProcessFilter defines the type of process filter.
type ProcessFilter string

// TargetResource returns the resource type for ProcessFilter
func (f ProcessFilter) TargetResource() ResourceType {
	return ProcessType
}

// GetFilterName returns the name for ProcessFilter
func (f ProcessFilter) GetFilterName() string {
	return string(f)
}

// Defined Process filter kinds.
const (
	ProcessLegacyExclude ProcessFilter = "process-legacy-exclude"
	ProcessCELLogs       ProcessFilter = "process-cel-logs"
	ProcessCELGlobal     ProcessFilter = "process-cel-global"
)
