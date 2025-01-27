// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package local provides local recommendations for autoscaling workloads.
package local

import (
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	corev1 "k8s.io/api/core/v1"
)

const (
	staleDataThresholdSeconds      = 180 // 3 minutes
	watermarkTolerance             = 5
	containerCPUUsageMetricName    = "container.cpu.usage"
	containerMemoryUsageMetricName = "container.memory.usage"
)

var (
	resourceToMetric = map[corev1.ResourceName]string{
		corev1.ResourceCPU:    containerCPUUsageMetricName,
		corev1.ResourceMemory: containerMemoryUsageMetricName,
	}
)

type resourceRecommenderSettings struct {
	metricName    string
	containerName string
	lowWatermark  float64
	highWatermark float64
}

func newResourceRecommenderSettings(target datadoghq.DatadogPodAutoscalerTarget) (*resourceRecommenderSettings, error) {
	if target.Type == datadoghq.DatadogPodAutoscalerContainerResourceTargetType {
		return getOptionsFromContainerResource(target.ContainerResource)
	}
	if target.Type == datadoghq.DatadogPodAutoscalerResourceTargetType {
		return getOptionsFromPodResource(target.PodResource)
	}
	return nil, fmt.Errorf("Invalid target type: %s", target.Type)
}

func getOptionsFromPodResource(target *datadoghq.DatadogPodAutoscalerResourceTarget) (*resourceRecommenderSettings, error) {
	if target == nil {
		return nil, fmt.Errorf("nil target")
	}
	if target.Value.Type != datadoghq.DatadogPodAutoscalerUtilizationTargetValueType {
		return nil, fmt.Errorf("invalid value type: %s", target.Value.Type)
	}
	metric, ok := resourceToMetric[target.Name]
	if !ok {
		return nil, fmt.Errorf("invalid resource name: %s", target.Name)
	}

	recSettings := &resourceRecommenderSettings{
		metricName:    metric,
		lowWatermark:  float64((*target.Value.Utilization - watermarkTolerance)) / 100.0,
		highWatermark: float64((*target.Value.Utilization + watermarkTolerance)) / 100.0,
	}
	return recSettings, nil
}

func getOptionsFromContainerResource(target *datadoghq.DatadogPodAutoscalerContainerResourceTarget) (*resourceRecommenderSettings, error) {
	if target == nil {
		return nil, fmt.Errorf("nil target")
	}
	if target.Value.Type != datadoghq.DatadogPodAutoscalerUtilizationTargetValueType {
		return nil, fmt.Errorf("invalid value type: %s", target.Value.Type)
	}

	metric, ok := resourceToMetric[target.Name]
	if !ok {
		return nil, fmt.Errorf("invalid resource name: %s", target.Name)
	}

	recSettings := &resourceRecommenderSettings{
		metricName:    metric,
		lowWatermark:  float64((*target.Value.Utilization - watermarkTolerance)) / 100.0,
		highWatermark: float64((*target.Value.Utilization + watermarkTolerance)) / 100.0,
		containerName: target.Container,
	}
	return recSettings, nil
}
