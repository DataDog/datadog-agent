// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"fmt"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// goMemLimitPattern matches the GOMEMLIMIT format accepted by the Go runtime:
// a non-negative integer with an optional IEC binary suffix (B, KiB, MiB, GiB, TiB, PiB, EiB)
// or the special value "off".
var goMemLimitPattern = regexp.MustCompile(`^([0-9]+(B|KiB|MiB|GiB|TiB|PiB|EiB)?|off)$`)

// ValidateGoMemLimit returns an error if value is not a valid GOMEMLIMIT string.
func ValidateGoMemLimit(value string) error {
	if !goMemLimitPattern.MatchString(value) {
		return fmt.Errorf("invalid GOMEMLIMIT value %q: must be a non-negative integer with optional IEC suffix (B, KiB, MiB, GiB, TiB, PiB, EiB) or \"off\"", value)
	}
	return nil
}

// ScalingValues represents the scaling values (horizontal and vertical) for a target
type ScalingValues struct {
	// HorizontalError refers to an error encountered by Datadog while computing the horizontal scaling values
	HorizontalError error                    `json:"-"`
	Horizontal      *HorizontalScalingValues `json:"horizontal"`

	// VerticalError refers to an error encountered by Datadog while computing the vertical scaling values
	VerticalError error                  `json:"-"`
	Vertical      *VerticalScalingValues `json:"vertical"`

	// Error refers to a general error encountered by Datadog while computing the scaling values
	Error error `json:"-"`
}

// IsEmpty returns true if the scaling values are empty
func (s ScalingValues) IsEmpty() bool {
	return !s.HasHorizontalValues() && !s.HasVerticalValues() && s.Error == nil
}

// HasHorizontalValues returns true if the scaling values have horizontal values
func (s ScalingValues) HasHorizontalValues() bool {
	return s.Horizontal != nil || s.HorizontalError != nil
}

// HasVerticalValues returns true if the scaling values have vertical values
func (s ScalingValues) HasVerticalValues() bool {
	return s.Vertical != nil || s.VerticalError != nil
}

// HorizontalScalingValues holds the horizontal scaling values for a target
type HorizontalScalingValues struct {
	// Source is the source of the value
	Source datadoghqcommon.DatadogPodAutoscalerValueSource `json:"source"`

	// Timestamp is the time at which the data was generated
	Timestamp time.Time `json:"timestamp"`

	// Replicas is the desired number of replicas for the target
	Replicas int32 `json:"replicas"`

	// UtilizationPct holds the average resource utilization ratio computed by the local recommender.
	// Only set when Source is DatadogPodAutoscalerLocalValueSource; nil otherwise.
	UtilizationPct *float64 `json:"utilization_pct,omitempty"`
}

// ContainerRuntimeValues holds runtime configuration for a container
type ContainerRuntimeValues struct {
	// GoMemLimit is the value for the GOMEMLIMIT environment variable
	GoMemLimit string `json:"gomemlimit,omitempty"`
}

// VerticalScalingValues holds the vertical scaling values for a target
type VerticalScalingValues struct {
	// Source is the source of the value
	Source datadoghqcommon.DatadogPodAutoscalerValueSource `json:"source"`

	// Timestamp is the time at which the data was generated
	Timestamp time.Time `json:"timestamp"`

	// ResourcesHash is the hash of containerResources and runtimeValues
	ResourcesHash string `json:"resources_hash"`

	// ContainerResources holds the resources for a container
	ContainerResources []datadoghqcommon.DatadogPodAutoscalerContainerResources `json:"container_resources"`

	// RuntimeValues holds runtime configuration per container, keyed by container name
	RuntimeValues map[string]ContainerRuntimeValues `json:"runtime_values,omitempty"`
}

// DeepCopy returns a deep copy of the VerticalScalingValues.
// We can't use mohae/deepcopy here because resource.Quantity has unexported fields.
func (v *VerticalScalingValues) DeepCopy() *VerticalScalingValues {
	if v == nil {
		return nil
	}
	out := &VerticalScalingValues{
		Source:        v.Source,
		Timestamp:     v.Timestamp,
		ResourcesHash: v.ResourcesHash,
	}
	if v.ContainerResources != nil {
		out.ContainerResources = make([]datadoghqcommon.DatadogPodAutoscalerContainerResources, len(v.ContainerResources))
		for i, cr := range v.ContainerResources {
			cp := datadoghqcommon.DatadogPodAutoscalerContainerResources{Name: cr.Name}
			if cr.Requests != nil {
				cp.Requests = make(corev1.ResourceList, len(cr.Requests))
				for k, q := range cr.Requests {
					cp.Requests[k] = q.DeepCopy()
				}
			}
			if cr.Limits != nil {
				cp.Limits = make(corev1.ResourceList, len(cr.Limits))
				for k, q := range cr.Limits {
					cp.Limits[k] = q.DeepCopy()
				}
			}
			out.ContainerResources[i] = cp
		}
	}
	if v.RuntimeValues != nil {
		out.RuntimeValues = make(map[string]ContainerRuntimeValues, len(v.RuntimeValues))
		for k, rv := range v.RuntimeValues {
			out.RuntimeValues[k] = rv
		}
	}
	return out
}

// SumCPUMemoryRequests sums the CPU and memory requests of all containers
func (v *VerticalScalingValues) SumCPUMemoryRequests() (cpu, memory resource.Quantity) {
	for _, container := range v.ContainerResources {
		cpuReq := container.Requests.Cpu()
		if cpuReq != nil {
			cpu.Add(*cpuReq)
		}

		memoryReq := container.Requests.Memory()
		if memoryReq != nil {
			memory.Add(*memoryReq)
		}
	}

	return
}

// RecommenderConfiguration holds the configuration for a custom recommender
type RecommenderConfiguration struct {
	Endpoint string         `json:"endpoint"`
	Settings map[string]any `json:"settings"`
}
