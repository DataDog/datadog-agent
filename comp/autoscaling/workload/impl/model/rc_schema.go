// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

// ReccomendationError is an error encountered while computing a recommendation on Datadog side
type ReccomendationError kubeAutoscaling.Error

// Error returns the error message
func (e *ReccomendationError) Error() string {
	return e.Message
}

// AutoscalingSettingsList holds a list of AutoscalingSettings
type AutoscalingSettingsList struct {
	// Settings is a list of .Spec
	Settings []AutoscalingSettings `json:"settings"`
}

// AutoscalingSettings is the .Spec of a PodAutoscaler retrieved through remote config
type AutoscalingSettings struct {
	// Namespace is the namespace of the PodAutoscaler
	Namespace string `json:"namespace"`

	// Name is the name of the PodAutoscaler
	Name string `json:"name"`

	// Spec is the full spec of the PodAutoscaler
	Spec *datadoghq.DatadogPodAutoscalerSpec `json:"spec"`
}

// ScalingValues represents the scaling values (horizontal and vertical) for a target
type ScalingValues struct {
	// HorizontalError refers to an error encountered by Datadog while computing the horizontal scaling values
	HorizontalError error
	Horizontal      *HorizontalScalingValues

	// VerticalError refers to an error encountered by Datadog while computing the vertical scaling values
	VerticalError error
	Vertical      *VerticalScalingValues

	// Error refers to a general error encountered by Datadog while computing the scaling values
	Error error
}

// HorizontalScalingValues holds the horizontal scaling values for a target
type HorizontalScalingValues struct {
	// Source is the source of the value
	Source datadoghq.DatadogPodAutoscalerValueSource

	// Timestamp is the time at which the data was generated
	Timestamp time.Time

	// Replicas is the desired number of replicas for the target
	Replicas int32
}

// VerticalScalingValues holds the vertical scaling values for a target
type VerticalScalingValues struct {
	// Source is the source of the value
	Source datadoghq.DatadogPodAutoscalerValueSource

	// Timestamp is the time at which the data was generated
	Timestamp time.Time

	// ResourcesHash is the hash of containerResources
	ResourcesHash string

	// ContainerResources holds the resources for a container
	ContainerResources []datadoghq.DatadogPodAutoscalerContainerResources
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
