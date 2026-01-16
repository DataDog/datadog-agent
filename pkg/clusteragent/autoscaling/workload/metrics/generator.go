// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	corev1 "k8s.io/api/core/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

const (
	metricPrefix = "datadog.cluster_agent.autoscaling.workload"
)

// Tag generation helper functions

// baseAutoscalerTags generates the common base tags for autoscaler metrics
func baseAutoscalerTags(namespace, targetName, autoscalerName string) []string {
	return []string{
		"namespace:" + namespace,
		"target_name:" + targetName,
		"autoscaler_name:" + autoscalerName,
		le.JoinLeaderLabel + ":" + le.JoinLeaderValue,
	}
}

// autoscalerTagsWithSource generates autoscaler tags with a source field
func autoscalerTagsWithSource(namespace, targetName, autoscalerName, source string) []string {
	tags := baseAutoscalerTags(namespace, targetName, autoscalerName)
	return append(tags, "source:"+source)
}

// autoscalerTagsWithContainer generates autoscaler tags for container-level metrics
func autoscalerTagsWithContainer(namespace, targetName, autoscalerName, source, containerName, resourceName string) []string {
	tags := autoscalerTagsWithSource(namespace, targetName, autoscalerName, source)
	return append(tags,
		"container_name:"+containerName,
		"resource_name:"+resourceName,
	)
}

// conditionTags generates tags for autoscaler condition metrics
func conditionTags(namespace, targetName, autoscalerName, conditionType string) []string {
	tags := baseAutoscalerTags(namespace, targetName, autoscalerName)
	tags = append(tags, "type:"+conditionType)
	return tags
}

// PodAutoscalerMetricsObject bundles CRD and internal model for metrics generation
type PodAutoscalerMetricsObject struct {
	CRD      *datadoghq.DatadogPodAutoscaler
	Internal *model.PodAutoscalerInternal
}

// GeneratePodAutoscalerMetrics generates structured metrics from a PodAutoscaler object
func GeneratePodAutoscalerMetrics(obj interface{}) StructuredMetrics {
	metricsObj, ok := obj.(*PodAutoscalerMetricsObject)
	if !ok {
		return nil
	}

	var metrics StructuredMetrics

	podAutoscaler := metricsObj.CRD
	internal := metricsObj.Internal

	namespace := internal.Namespace()
	name := internal.Name()

	// Get target name
	var targetName string
	if internal.Spec() != nil {
		targetName = internal.Spec().TargetRef.Name
	}

	// Get scaling values
	scalingValues := internal.MainScalingValues()

	// 1. Horizontal scaling received replicas
	if scalingValues.Horizontal != nil {
		metrics = append(metrics, StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_received_replicas",
			Type:  MetricTypeGauge,
			Value: float64(scalingValues.Horizontal.Replicas),
			Tags:  autoscalerTagsWithSource(namespace, targetName, name, string(scalingValues.Horizontal.Source)),
		})
	}

	// 3. Vertical scaling received requests and limits
	if scalingValues.Vertical != nil {
		for _, containerResources := range scalingValues.Vertical.ContainerResources {
			// Requests
			for resource, value := range containerResources.Requests {
				metrics = append(metrics, StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling_received_requests",
					Type:  MetricTypeGauge,
					Value: value.AsApproximateFloat64(),
					Tags:  autoscalerTagsWithContainer(namespace, targetName, name, string(scalingValues.Vertical.Source), containerResources.Name, string(resource)),
				})
			}

			// Limits
			for resource, value := range containerResources.Limits {
				metrics = append(metrics, StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling_received_limits",
					Type:  MetricTypeGauge,
					Value: value.AsApproximateFloat64(),
					Tags:  autoscalerTagsWithContainer(namespace, targetName, name, string(scalingValues.Vertical.Source), containerResources.Name, string(resource)),
				})
			}
		}
	}

	// 5. Autoscaler conditions
	if podAutoscaler != nil {
		for _, condition := range podAutoscaler.Status.Conditions {
			value := 0.0
			if condition.Status == corev1.ConditionTrue {
				value = 1.0
			}
			metrics = append(metrics, StructuredMetric{
				Name:  metricPrefix + ".autoscaler_conditions",
				Type:  MetricTypeGauge,
				Value: value,
				Tags:  conditionTags(namespace, targetName, name, string(condition.Type)),
			})
		}
	}

	// 6. Local fallback enabled
	// Determine if we're using local fallback by checking the source
	localFallbackValue := 0.0
	if scalingValues.Horizontal != nil && scalingValues.Horizontal.Source == datadoghqcommon.DatadogPodAutoscalerLocalValueSource {
		localFallbackValue = 1.0
	} else if scalingValues.Vertical != nil && scalingValues.Vertical.Source == datadoghqcommon.DatadogPodAutoscalerLocalValueSource {
		localFallbackValue = 1.0
	}

	metrics = append(metrics, StructuredMetric{
		Name:  metricPrefix + ".local_fallback_enabled",
		Type:  MetricTypeGauge,
		Value: localFallbackValue,
		Tags:  baseAutoscalerTags(namespace, targetName, name),
	})

	return metrics
}
