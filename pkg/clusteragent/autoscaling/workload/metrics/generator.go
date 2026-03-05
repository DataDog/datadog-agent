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
		"namespace:" + namespace, // keep "namespace" for backward compatibility, even if it's redundant with "kube_namespace"
		"kube_namespace:" + namespace,
		"target_name:" + targetName,
		"autoscaler_name:" + autoscalerName, // keep it for backward compatibility, even if it's redundant with "name"
		"name:" + autoscalerName,
		le.JoinLeaderLabel + ":" + le.JoinLeaderValue,
	}
}

func resourceTags(resournceName string) []string {
	return []string{
		"resource_name:" + resournceName,
	}
}

// conditionTags generates tags for autoscaler condition metrics
func conditionTags(baseTags []string, conditionType string) []string {
	return append(baseTags, "type:"+conditionType)
}

// GeneratePodAutoscalerMetrics generates structured metrics from a PodAutoscaler object
func GeneratePodAutoscalerMetrics(obj any) metricsstore.StructuredMetrics {
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

	// Precompute base tags shared across all metrics for this autoscaler
	baseTags := baseAutoscalerTags(namespace, targetName, name)
	var baseWithHorizontalSourceTags []string
	if scalingValues.Horizontal != nil {
		baseWithHorizontalSourceTags = append(baseWithHorizontalSourceTags, "source:"+string(scalingValues.Horizontal.Source))
	}
	baseWithHorizontalSourceTags = append(baseWithHorizontalSourceTags, baseTags...)
	// Cap to prevent slice aliasing in the container resources loop below
	baseWithHorizontalSourceTags = baseWithHorizontalSourceTags[:len(baseWithHorizontalSourceTags):len(baseWithHorizontalSourceTags)] // Cap to prevent slice aliasing in the horizontal metrics below

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

	// 2. Horizontal scaling received replicas
	if scalingValues.Horizontal != nil {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_received_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(scalingValues.Horizontal.Replicas),
			Tags:  baseWithHorizontalSourceTags,
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
					Tags:  append(baseWithVerticalSourceTags, resourceTags(string(resource))...),
				})
			}

			// Limits
			for resource, value := range containerResources.Limits {
				metrics = append(metrics, metricsstore.StructuredMetric{
					Name:  metricPrefix + ".vertical_scaling_received_limits",
					Type:  metricsstore.MetricTypeGauge,
					Value: value.AsApproximateFloat64(),
					Tags:  append(baseWithVerticalSourceTags, resourceTags(string(resource))...),
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
	horizontalTags := append(baseTags, "source:"+actionSource)

	if len(lastHorizontalActions) > 0 {
		lastAction := lastHorizontalActions[len(lastHorizontalActions)-1]
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".horizontal_scaling_applied_replicas",
			Type:  metricsstore.MetricTypeGauge,
			Value: float64(lastAction.ToReplicas),
			Tags:  horizontalTags,
		})
	}

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
	if internal.VerticalActionErrorCount() > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".vertical_rollout_triggered",
			Type:  metricsstore.MetricTypeMonotonicCount,
			Value: float64(internal.VerticalActionErrorCount()),
			Tags:  append(baseWithVerticalSourceTags, "status:error"),
		})
	}
	if internal.VerticalActionSuccessCount() > 0 {
		metrics = append(metrics, metricsstore.StructuredMetric{
			Name:  metricPrefix + ".vertical_rollout_triggered",
			Type:  metricsstore.MetricTypeMonotonicCount,
			Value: float64(internal.VerticalActionSuccessCount()),
			Tags:  append(baseWithVerticalSourceTags, "status:ok"),
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
				Tags:  conditionTags(baseTags, string(condition.Type)),
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
		Tags:  baseTags,
	})

	return metrics
}
