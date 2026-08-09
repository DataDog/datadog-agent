// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package metrics provides structured metric generation for DatadogPodAutoscaler objects.
package metrics

import (
	"strconv"
	"strings"

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

// baseAutoscalerTags generates the common base tags for autoscaler metrics, including
// per-object tags from annotations (ad.datadoghq.com/tags) and UST labels.
func baseAutoscalerTags(internal *model.PodAutoscalerInternal) []string {
	namespace := internal.Namespace()
	name := internal.Name()

	var targetName, targetKind string
	if internal.Spec() != nil {
		targetName = internal.Spec().TargetRef.Name
		targetKind = strings.ToLower(internal.Spec().TargetRef.Kind)
	}

	tags := []string{
		"namespace:" + namespace, // keep "namespace" for backward compatibility, even if it's redundant with "kube_namespace"
		"kube_namespace:" + namespace,
		"target_name:" + targetName,
		"target_kind:" + targetKind,
		"autoscaler_name:" + name, // keep it for backward compatibility, even if it's redundant with "name"
		"name:" + name,
		le.IsLeaderLabel + ":" + le.JoinLeaderValue,
	}
	tags = append(tags, keyTagsFromObjectMetadata(internal)...)
	// Cap to prevent slice aliasing when callers append to this slice multiple times
	// (e.g. horizontalTags and conditionTags loop both append to baseTags)
	return tags[:len(tags):len(tags)]
}

func resourceTags(containerName, resourceName string) []string {
	return []string{
		"resource_name:" + resourceName,
		"kube_container_name:" + containerName,
	}
}

// conditionTags generates tags for autoscaler condition metrics
func conditionTags(baseTags []string, conditionType string) []string {
	return append(baseTags, "type:"+conditionType)
}

// objectiveTags generates tags for autoscaler objective (target) metrics.
// index is the 0-based position of the objective in spec.objectives[]; it guarantees a unique
// tag-set per objective (so multiple objectives never collapse into the same timeseries), which
// matters most for objectives that share the other tags — e.g. multiple custom query objectives,
// or two container-resource objectives targeting the same container and resource.
// resourceName and containerName are omitted from the tag set when empty
// (e.g. custom query objectives have no resource, pod resource objectives have no container).
func objectiveTags(baseTags []string, objectiveType, valueType, resourceName, containerName string, index int) []string {
	tags := append(baseTags, "objective_type:"+objectiveType, "value_type:"+valueType, "objective_index:"+strconv.Itoa(index))
	if resourceName != "" {
		tags = append(tags, "resource_name:"+resourceName)
	}
	if containerName != "" {
		tags = append(tags, "kube_container_name:"+containerName)
	}
	return tags
}

// objectiveValue extracts the numeric value and the corresponding value_type tag from an
// objective value. Utilization is returned as a raw percentage (0-100). AbsoluteValue is
// converted to the unit native to the resource: millicores for CPU, bytes for memory, and an
// approximate float otherwise (e.g. for custom query objectives that have no resource).
// Returns ok=false when the value pointer for the declared type is nil.
func objectiveValue(v datadoghqcommon.DatadogPodAutoscalerObjectiveValue, resourceName corev1.ResourceName) (value float64, valueType string, ok bool) {
	switch v.Type {
	case datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType:
		if v.Utilization != nil {
			return float64(*v.Utilization), "utilization", true
		}
	case datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType:
		if v.AbsoluteValue != nil {
			switch resourceName {
			case corev1.ResourceCPU:
				return float64(v.AbsoluteValue.MilliValue()), "absolute_value", true
			case corev1.ResourceMemory:
				return float64(v.AbsoluteValue.Value()), "absolute_value", true
			default:
				return v.AbsoluteValue.AsApproximateFloat64(), "absolute_value", true
			}
		}
	}
	return 0, "", false
}

// GeneratePodAutoscalerMetrics generates structured metrics from a PodAutoscaler object
func GeneratePodAutoscalerMetrics(internal *model.PodAutoscalerInternal) metricsstore.StructuredMetrics {
	if internal == nil {
		return nil
	}

	var metrics metricsstore.StructuredMetrics

	// Get scaling values
	scalingValues := internal.MainScalingValues()

	// Precompute base tags shared across all metrics for this autoscaler
	baseTags := baseAutoscalerTags(internal)
	var baseWithHorizontalSourceTags []string
	if scalingValues.Horizontal != nil {
		baseWithHorizontalSourceTags = append(baseWithHorizontalSourceTags, "source:"+string(scalingValues.Horizontal.Source))
	}
	baseWithHorizontalSourceTags = append(baseWithHorizontalSourceTags, baseTags...)
	// Cap to prevent slice aliasing in the container resources loop below
	baseWithHorizontalSourceTags = baseWithHorizontalSourceTags[:len(baseWithHorizontalSourceTags):len(baseWithHorizontalSourceTags)]

	var baseWithVerticalSourceTags []string
	if scalingValues.Vertical != nil {
		baseWithVerticalSourceTags = append(baseWithVerticalSourceTags, "source:"+string(scalingValues.Vertical.Source))
	}
	baseWithVerticalSourceTags = append(baseWithVerticalSourceTags, baseTags...)
	// Cap to prevent slice aliasing in the container resources loop below
	baseWithVerticalSourceTags = baseWithVerticalSourceTags[:len(baseWithVerticalSourceTags):len(baseWithVerticalSourceTags)]
	// TODO: containerExtraTags will be used in the future when we add container-level metrics, to include tags from "ad.datadoghq.com/<container-name>.tags" annotations
	/* containerExtraTags, err := parseContainerAnnotationTags(internal.Annotations())
	if err != nil {
		log.Debugf("failed to parse container annotation tags on DatadogPodAutoscaler %s/%s: %s",
			namespace, name, err)
	}
	*/

	// 1. Received recommendations version
	if version := internal.MainScalingValuesVersion(); version > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".received_recommendations_version",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(version),
			Tags:  baseTags,
		})
	}

	// 2. Local fallback enabled
	// Check the active horizontal source (not main), matching the old behaviour that only tracked horizontal fallback
	localFallbackValue := 0.0
	if activeHorizontal := internal.ScalingValues().Horizontal; activeHorizontal != nil && activeHorizontal.Source == datadoghqcommon.DatadogPodAutoscalerLocalValueSource {
		localFallbackValue = 1.0
	}

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".local.fallback_enabled",
		Type:  metricsstore.MetricTypeGauge,
		Value: localFallbackValue,
		Tags:  baseTags,
	})

	// 3. Horizontal scaling received replicas
	if scalingValues.Horizontal != nil {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_received_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(scalingValues.Horizontal.Replicas),
			Tags:  baseWithHorizontalSourceTags,
		})
	}

	// 4. Vertical scaling received requests and limits
	if scalingValues.Vertical != nil {
		for _, containerResources := range scalingValues.Vertical.ContainerResources {
			// Requests
			for resource, value := range containerResources.Requests {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling_received_requests",
					Type:  metricsstore.MetricTypeGauge,
					Value: value.AsApproximateFloat64(),
					Tags:  append(baseWithVerticalSourceTags, resourceTags(containerResources.Name, string(resource))...),
				})
			}

			// Limits
			for resource, value := range containerResources.Limits {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling_received_limits",
					Type:  metricsstore.MetricTypeGauge,
					Value: value.AsApproximateFloat64(),
					Tags:  append(baseWithVerticalSourceTags, resourceTags(containerResources.Name, string(resource))...),
				})
			}
		}
	}

	// 5. Horizontal scaling last action metrics
	lastHorizontalActions := internal.HorizontalLastActions()
	actionSource := ""
	if sv := internal.ScalingValues(); sv.Horizontal != nil {
		actionSource = string(sv.Horizontal.Source)
	}
	horizontalTags := append(baseTags, "source:"+actionSource)
	// Cap the slice to its length so each status append allocates independently
	horizontalTags = horizontalTags[:len(horizontalTags):len(horizontalTags)]

	if len(lastHorizontalActions) > 0 {
		lastAction := lastHorizontalActions[len(lastHorizontalActions)-1]
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_applied_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(lastAction.ToReplicas),
			Tags:  horizontalTags,
		})
	}

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".horizontal_scaling_actions",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.HorizontalActionErrorCount()),
		Tags:  append(horizontalTags, "status:error"),
	})

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".horizontal_scaling_actions",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.HorizontalActionSuccessCount()),
		Tags:  append(horizontalTags, "status:ok"),
	})

	// 6. Vertical scaling last action metrics
	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_rollout_triggered",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.VerticalActionErrorCount()),
		Tags:  append(baseWithVerticalSourceTags, "status:error"),
	})

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_rollout_triggered",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.VerticalActionSuccessCount()),
		Tags:  append(baseWithVerticalSourceTags, "status:ok"),
	})

	// 7. In-place vertical scaling action metrics
	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.patch",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlacePatchSuccessCount()),
		Tags:  append(baseWithVerticalSourceTags, "status:ok"),
	})
	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.patch",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlacePatchErrorCount()),
		Tags:  append(baseWithVerticalSourceTags, "status:error"),
	})

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.eviction",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlaceEvictionSuccessCount()),
		Tags:  append(baseWithVerticalSourceTags, "status:ok"),
	})
	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.eviction",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlaceEvictionErrorCount()),
		Tags:  append(baseWithVerticalSourceTags, "status:error"),
	})

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.rollout_fallback",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlaceRolloutFallbackCount()),
		Tags:  baseWithVerticalSourceTags,
	})

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.pdb_blocked",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlacePDBBlockedCount()),
		Tags:  baseWithVerticalSourceTags,
	})

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.disruption_throttled",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlaceDisruptionThrottledCount()),
		Tags:  baseWithVerticalSourceTags,
	})

	metrics = append(metrics, metricsstore.StructuredMetric{
		Name:  metricPrefix + ".vertical_inplace.resize_completed",
		Type:  metricsstore.MetricTypeMonotonicCount,
		Value: float64(internal.InPlaceResizeCompletedCount()),
		Tags:  baseWithVerticalSourceTags,
	})

	// 8. Vertical scaled/evicted replica gauges
	if scaledReplicas := internal.ScaledReplicas(); scaledReplicas != nil {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".status.vertical.scaled_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(*scaledReplicas),
			Tags:  baseTags,
		})
	}
	if evictedReplicas := internal.EvictedReplicas(); evictedReplicas != nil {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".status.vertical.evicted_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(*evictedReplicas),
			Tags:  baseTags,
		})
	}

	// 9. Local recommender horizontal metrics
	if fallbackHorizontal := internal.FallbackScalingValues().Horizontal; fallbackHorizontal != nil {
		localSourceTags := append(baseTags, "source:"+string(fallbackHorizontal.Source))
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".local.horizontal_scaling_recommended_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(fallbackHorizontal.Replicas),
			Tags:  localSourceTags,
		})
		if fallbackHorizontal.UtilizationPct != nil {
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".local.horizontal_utilization_pct",
				Type:  metricsstore.MetricTypeGauge,
				Value: *fallbackHorizontal.UtilizationPct,
				Tags:  localSourceTags,
			})
		}
	}

	// 10. Horizontal scaling constraints
	if spec := internal.Spec(); spec != nil && spec.Constraints != nil {
		if spec.Constraints.MaxReplicas != nil {
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".horizontal_scaling.constraints.max_replicas",
				Type:  metricsstore.MetricTypeGauge,
				Value: float64(*spec.Constraints.MaxReplicas),
				Tags:  baseTags,
			})
		}
		if spec.Constraints.MinReplicas != nil {
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".horizontal_scaling.constraints.min_replicas",
				Type:  metricsstore.MetricTypeGauge,
				Value: float64(*spec.Constraints.MinReplicas),
				Tags:  baseTags,
			})
		}

		// 11. Vertical scaling container constraints (per container, CPU in millicores, memory in bytes)
		// Mirror the resolveMinMaxBounds fallback from controller_vertical_helpers.go:
		// prefer top-level MinAllowed/MaxAllowed; fall back to deprecated Requests field.
		for _, container := range spec.Constraints.Containers {
			containerTags := append(baseTags, "kube_container_name:"+container.Name)

			effectiveMin := container.MinAllowed
			if len(effectiveMin) == 0 && container.Requests != nil {
				effectiveMin = container.Requests.MinAllowed
			}
			effectiveMax := container.MaxAllowed
			if len(effectiveMax) == 0 && container.Requests != nil {
				effectiveMax = container.Requests.MaxAllowed
			}

			if cpuMin, ok := effectiveMin[corev1.ResourceCPU]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.cpu.request_min",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(cpuMin.MilliValue()),
					Tags:  containerTags,
				})
			}
			if memMin, ok := effectiveMin[corev1.ResourceMemory]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.memory.request_min",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(memMin.Value()),
					Tags:  containerTags,
				})
			}
			if cpuMax, ok := effectiveMax[corev1.ResourceCPU]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.cpu.request_max",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(cpuMax.MilliValue()),
					Tags:  containerTags,
				})
			}
			if memMax, ok := effectiveMax[corev1.ResourceMemory]; ok {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling.constraints.container.memory.request_max",
					Type:  metricsstore.MetricTypeGauge,
					Value: float64(memMax.Value()),
					Tags:  containerTags,
				})
			}
		}
	}

	// 13. Autoscaling objectives (target values from spec)
	if spec := internal.Spec(); spec != nil {
		for idx, objective := range spec.Objectives {
			switch objective.Type {
			case datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType:
				if objective.PodResource == nil {
					continue
				}
				if value, valueType, ok := objectiveValue(objective.PodResource.Value, objective.PodResource.Name); ok {
					metrics = append(metrics, metricsstore.StructuredMetric{
						Name:  metricPrefix + ".objective.target",
						Type:  metricsstore.MetricTypeGauge,
						Value: value,
						Tags:  objectiveTags(baseTags, "pod_resource", valueType, strings.ToLower(objective.PodResource.Name.String()), "", idx),
					})
				}
			case datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType:
				if objective.ContainerResource == nil {
					continue
				}
				if value, valueType, ok := objectiveValue(objective.ContainerResource.Value, objective.ContainerResource.Name); ok {
					metrics = append(metrics, metricsstore.StructuredMetric{
						Name:  metricPrefix + ".objective.target",
						Type:  metricsstore.MetricTypeGauge,
						Value: value,
						Tags:  objectiveTags(baseTags, "container_resource", valueType, strings.ToLower(objective.ContainerResource.Name.String()), objective.ContainerResource.Container, idx),
					})
				}
			case datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType:
				if objective.CustomQuery == nil {
					continue
				}
				if value, valueType, ok := objectiveValue(objective.CustomQuery.Value, ""); ok {
					metrics = append(metrics, metricsstore.StructuredMetric{
						Name:  metricPrefix + ".objective.target",
						Type:  metricsstore.MetricTypeGauge,
						Value: value,
						Tags:  objectiveTags(baseTags, "custom_query", valueType, "", "", idx),
					})
				}
			}
		}
	}

	// 14. Status metrics and autoscaler conditions (from upstream CR)
	if podAutoscaler := internal.UpstreamCR(); podAutoscaler != nil {
		// 14a. Horizontal desired replicas from status
		if horizontal := podAutoscaler.Status.Horizontal; horizontal != nil && horizontal.Target != nil {
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".status.desired.replicas",
				Type:  metricsstore.MetricTypeGauge,
				Value: float64(horizontal.Target.Replicas),
				Tags:  baseTags,
			})
		}

		// 14b. Vertical desired resources from status (per container, CPU in millicores, memory in bytes)
		if vertical := podAutoscaler.Status.Vertical; vertical != nil && vertical.Target != nil {
			for _, container := range vertical.Target.DesiredResources {
				containerTags := append(baseTags, "kube_container_name:"+container.Name)
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

		// 14c. Autoscaler conditions
		for _, condition := range podAutoscaler.Status.Conditions {
			value := 0.0
			if condition.Status == corev1.ConditionTrue {
				value = 1.0
			}
			metrics = append(metrics, metricsstore.StructuredMetric{
				Name:  metricPrefix + ".autoscaler_conditions",
				Type:  metricsstore.MetricTypeGauge,
				Value: value,
				Tags:  conditionTags(baseTags, string(condition.Type)),
			})
		}
	}

	log.Tracef("GeneratePodAutoscalerMetrics: generated %d metrics for autoscaler %s/%s", len(metrics), internal.Namespace(), internal.Name())
	return metrics
}
