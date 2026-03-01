// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package metrics provides structured metric generation for DatadogPodAutoscaler objects.
package metrics

import (
	corev1 "k8s.io/api/core/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/metricsstore"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
		"kube_container_name:"+containerName,
		"resource_name:"+resourceName,
	)
}

// autoscalerTagsWithContainerName generates autoscaler tags with a kube_container_name field (no source/resource_name)
func autoscalerTagsWithContainerName(namespace, targetName, autoscalerName, containerName string) []string {
	tags := baseAutoscalerTags(namespace, targetName, autoscalerName)
	return append(tags, "kube_container_name:"+containerName)
}

// conditionTags generates tags for autoscaler condition metrics
func conditionTags(namespace, targetName, autoscalerName, conditionType string) []string {
	tags := baseAutoscalerTags(namespace, targetName, autoscalerName)
	tags = append(tags, "type:"+conditionType)
	return tags
}

// GeneratePodAutoscalerMetrics generates structured metrics from a PodAutoscaler object
func GeneratePodAutoscalerMetrics(obj interface{}) metricsstore.StructuredMetrics {
	internal, ok := obj.(*model.PodAutoscalerInternal)
	if !ok {
		log.Debugf("GeneratePodAutoscalerMetrics: unexpected type %T, expected *model.PodAutoscalerInternal", obj)
		return nil
	}

	var metrics metricsstore.StructuredMetrics

	namespace := internal.Namespace()
	name := internal.Name()

	// Get target name
	var targetName string
	if internal.Spec() != nil {
		targetName = internal.Spec().TargetRef.Name
	}

	// Get scaling values
	scalingValues := internal.MainScalingValues()

	// 1. Received recommendations version
	if version := internal.MainScalingValuesVersion(); version > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".received_recommendations_version",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(version),
			Tags:  baseAutoscalerTags(namespace, targetName, name),
		})
	}

	// 2. Horizontal scaling received replicas
	if scalingValues.Horizontal != nil {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_received_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(scalingValues.Horizontal.Replicas),
			Tags:  autoscalerTagsWithSource(namespace, targetName, name, string(scalingValues.Horizontal.Source)),
		})
	}

	// 3. Vertical scaling received requests and limits
	if scalingValues.Vertical != nil {
		for _, containerResources := range scalingValues.Vertical.ContainerResources {
			// Requests
			for resource, value := range containerResources.Requests {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling_received_requests",
					Type:  metricsstore.MetricTypeGauge,
					Value: value.AsApproximateFloat64(),
					Tags:  autoscalerTagsWithContainer(namespace, targetName, name, string(scalingValues.Vertical.Source), containerResources.Name, string(resource)),
				})
			}

			// Limits
			for resource, value := range containerResources.Limits {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling_received_limits",
					Type:  metricsstore.MetricTypeGauge,
					Value: value.AsApproximateFloat64(),
					Tags:  autoscalerTagsWithContainer(namespace, targetName, name, string(scalingValues.Vertical.Source), containerResources.Name, string(resource)),
				})
			}
		}
	}

	// 4. Horizontal scaling last action metrics
	lastHorizontalActions := internal.HorizontalLastActions()
	actionSource := ""
	if sv := internal.ScalingValues(); sv.Horizontal != nil {
		actionSource = string(sv.Horizontal.Source)
	}

	if len(lastHorizontalActions) > 0 {
		lastAction := lastHorizontalActions[len(lastHorizontalActions)-1]
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_applied_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(lastAction.ToReplicas),
			Tags:  autoscalerTagsWithSource(namespace, targetName, name, actionSource),
		})
	}

	horizontalTags := autoscalerTagsWithSource(namespace, targetName, name, actionSource)
	// Cap the slice to its length so each status append allocates independently
	horizontalTags = horizontalTags[:len(horizontalTags):len(horizontalTags)]
	if internal.HorizontalActionErrorCount() > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_actions",
			Type:  metricsstore.MetricTypeMonotonicCount,
			Value: float64(internal.HorizontalActionErrorCount()),
			Tags:  append(horizontalTags, "status:error"),
		})
	}
	if internal.HorizontalActionSuccessCount() > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_actions",
			Type:  metricsstore.MetricTypeMonotonicCount,
			Value: float64(internal.HorizontalActionSuccessCount()),
			Tags:  append(horizontalTags, "status:ok"),
		})
	}

	// 5. Vertical scaling last action metrics
	verticalTags := baseAutoscalerTags(namespace, targetName, name)
	if internal.VerticalActionErrorCount() > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".vertical_rollout_triggered",
			Type:  metricsstore.MetricTypeMonotonicCount,
			Value: float64(internal.VerticalActionErrorCount()),
			Tags:  append(verticalTags, "status:error"),
		})
	}
	if internal.VerticalActionSuccessCount() > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".vertical_rollout_triggered",
			Type:  metricsstore.MetricTypeMonotonicCount,
			Value: float64(internal.VerticalActionSuccessCount()),
			Tags:  append(verticalTags, "status:ok"),
		})
	}

	// 6. Autoscaler conditions (from upstream CR)
	if podAutoscaler := internal.UpstreamCR(); podAutoscaler != nil {
		for _, condition := range podAutoscaler.Status.Conditions {
			value := 0.0
			if condition.Status == corev1.ConditionTrue {
				value = 1.0
			}
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".autoscaler_conditions",
				Type:  metricsstore.MetricTypeGauge,
				Value: value,
				Tags:  conditionTags(namespace, targetName, name, string(condition.Type)),
			})
		}
	}

	// 7. Local fallback enabled
	// Check the active horizontal source (not main), matching the old behaviour that only tracked horizontal fallback
	localFallbackValue := 0.0
	if activeHorizontal := internal.ScalingValues().Horizontal; activeHorizontal != nil && activeHorizontal.Source == datadoghqcommon.DatadogPodAutoscalerLocalValueSource {
		localFallbackValue = 1.0
	}

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".local_fallback_enabled",
		Type:  metricsstore.MetricTypeGauge,
		Value: localFallbackValue,
		Tags:  baseAutoscalerTags(namespace, targetName, name),
	})

	// 8. Horizontal scaling constraints
	if spec := internal.Spec(); spec != nil && spec.Constraints != nil {
		if spec.Constraints.MaxReplicas != nil {
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".horizontal_scaling.constraints.max_replicas",
				Type:  metricsstore.MetricTypeGauge,
				Value: float64(*spec.Constraints.MaxReplicas),
				Tags:  baseAutoscalerTags(namespace, targetName, name),
			})
		}
		if spec.Constraints.MinReplicas != nil {
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".horizontal_scaling.constraints.min_replicas",
				Type:  metricsstore.MetricTypeGauge,
				Value: float64(*spec.Constraints.MinReplicas),
				Tags:  baseAutoscalerTags(namespace, targetName, name),
			})
		}

		// 9. Vertical scaling container constraints (per container, CPU in millicores, memory in bytes)
		for _, container := range spec.Constraints.Containers {
			containerTags := autoscalerTagsWithContainerName(namespace, targetName, name, container.Name)
			if cpuMin, ok := container.MinAllowed[corev1.ResourceCPU]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.cpu.request_min",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(cpuMin.MilliValue()),
					Tags:  containerTags,
				})
			}
			if memMin, ok := container.MinAllowed[corev1.ResourceMemory]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.memory.request_min",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(memMin.Value()),
					Tags:  containerTags,
				})
			}
			if cpuMax, ok := container.MaxAllowed[corev1.ResourceCPU]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.cpu.request_max",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(cpuMax.MilliValue()),
					Tags:  containerTags,
				})
			}
			if memMax, ok := container.MaxAllowed[corev1.ResourceMemory]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.memory.request_max",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(memMax.Value()),
					Tags:  containerTags,
				})
			}
		}
	}

	// 10. Status metrics from upstream CR
	if podAutoscaler := internal.UpstreamCR(); podAutoscaler != nil {
		// 10a. Horizontal desired replicas from status
		if horizontal := podAutoscaler.Status.Horizontal; horizontal != nil && horizontal.Target != nil {
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".status.desired.replicas",
				Type:  metricsstore.MetricTypeGauge,
				Value: float64(horizontal.Target.Replicas),
				Tags:  baseAutoscalerTags(namespace, targetName, name),
			})
		}

		// 10b. Vertical desired resources (per container, CPU in millicores, memory in bytes)
		if vertical := podAutoscaler.Status.Vertical; vertical != nil && vertical.Target != nil {
			for _, container := range vertical.Target.DesiredResources {
				containerTags := autoscalerTagsWithContainerName(namespace, targetName, name, container.Name)
				if cpuReq, ok := container.Requests[corev1.ResourceCPU]; ok {
					metrics = append(metrics, metricsstore.StructuredMetric{
						Name:  metricPrefix + ".status.vertical.desired.container.cpu.request",
						Type:  metricsstore.MetricTypeGauge,
						Value: float64(cpuReq.MilliValue()),
						Tags:  containerTags,
					})
				}
				if memReq, ok := container.Requests[corev1.ResourceMemory]; ok {
					metrics = append(metrics, metricsstore.StructuredMetric{
						Name:  metricPrefix + ".status.vertical.desired.container.memory.request",
						Type:  metricsstore.MetricTypeGauge,
						Value: float64(memReq.Value()),
						Tags:  containerTags,
					})
				}
				if cpuLim, ok := container.Limits[corev1.ResourceCPU]; ok {
					metrics = append(metrics, metricsstore.StructuredMetric{
						Name:  metricPrefix + ".status.vertical.desired.container.cpu.limit",
						Type:  metricsstore.MetricTypeGauge,
						Value: float64(cpuLim.MilliValue()),
						Tags:  containerTags,
					})
				}
				if memLim, ok := container.Limits[corev1.ResourceMemory]; ok {
					metrics = append(metrics, metricsstore.StructuredMetric{
						Name:  metricPrefix + ".status.vertical.desired.container.memory.limit",
						Type:  metricsstore.MetricTypeGauge,
						Value: float64(memLim.Value()),
						Tags:  containerTags,
					})
				}
			}
		}
	}

	return metrics
}
