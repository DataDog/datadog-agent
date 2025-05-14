// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// ScalingValues represents the scaling values (horizontal and vertical) for a target
type ScalingValues struct {
	// HorizontalError refers to an error encountered by Datadog while computing the horizontal scaling values
	HorizontalError error                    `json:"horizontal_error"`
	Horizontal      *HorizontalScalingValues `json:"horizontal"`

	// VerticalError refers to an error encountered by Datadog while computing the vertical scaling values
	VerticalError error                  `json:"vertical_error"`
	Vertical      *VerticalScalingValues `json:"vertical"`

	// Error refers to a general error encountered by Datadog while computing the scaling values
	Error error `json:"error"`
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

// RecommenderConfiguration holds the configuration for a custom recommender
type RecommenderConfiguration struct {
	Endpoint string         `json:"endpoint"`
	Settings map[string]any `json:"settings"`
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

// MarshalJSON implements custom JSON marshaling for ScalingValues to handle error fields
func (sv *ScalingValues) MarshalJSON() ([]byte, error) {
	type Alias ScalingValues

	// Create a temporary struct without error fields
	temp := struct {
		*Alias
		// Override error fields to use string representation
		Error           interface{} `json:"error"`
		HorizontalError interface{} `json:"horizontal_error"`
		VerticalError   interface{} `json:"vertical_error"`
	}{
		Alias: (*Alias)(sv),
	}

	// Convert error fields to strings
	if sv.Error != nil {
		temp.Error = sv.Error.Error()
	}

	if sv.HorizontalError != nil {
		temp.HorizontalError = sv.HorizontalError.Error()
	}

	if sv.VerticalError != nil {
		temp.VerticalError = sv.VerticalError.Error()
	}

	return json.Marshal(temp)
}

// UnmarshalJSON implements custom JSON unmarshaling for ScalingValues to handle error fields
func (sv *ScalingValues) UnmarshalJSON(data []byte) error {
	// Use an alias type to avoid infinite recursion
	type Alias ScalingValues

	// Create a temporary struct with string fields for errors
	temp := struct {
		*Alias
		Error           interface{} `json:"error"`
		HorizontalError interface{} `json:"horizontal_error"`
		VerticalError   interface{} `json:"vertical_error"`
	}{
		Alias: (*Alias)(sv),
	}

	// Unmarshal into the temporary struct
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Convert error strings back to error objects
	sv.Error = convertToError(temp.Error)
	sv.HorizontalError = convertToError(temp.HorizontalError)
	sv.VerticalError = convertToError(temp.VerticalError)

	return nil
}

// Helper function to convert various types to error
func convertToError(value interface{}) error {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return errors.New(v)
	case map[string]interface{}:
		if msg, ok := v["message"]; ok {
			if msgStr, ok := msg.(string); ok {
				return errors.New(msgStr)
			}
		}
	}

	// Try to convert to string as a last resort
	return errors.New(fmt.Sprintf("%v", value))
}
