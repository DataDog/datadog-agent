// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

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

// HorizontalScalingValues holds the horizontal scaling values for a target
type HorizontalScalingValues struct {
	// Source is the source of the value
	Source datadoghqcommon.DatadogPodAutoscalerValueSource `json:"source"`

	// Timestamp is the time at which the data was generated
	Timestamp time.Time `json:"timestamp"`

	// Replicas is the desired number of replicas for the target
	Replicas int32 `json:"replicas"`
}

// VerticalScalingValues holds the vertical scaling values for a target
type VerticalScalingValues struct {
	// Source is the source of the value
	Source datadoghqcommon.DatadogPodAutoscalerValueSource `json:"source"`

	// Timestamp is the time at which the data was generated
	Timestamp time.Time `json:"timestamp"`

	// ResourcesHash is the hash of containerResources
	ResourcesHash string `json:"resources_hash"`

	// ContainerResources holds the resources for a container
	ContainerResources []datadoghqcommon.DatadogPodAutoscalerContainerResources `json:"container_resources"`
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
