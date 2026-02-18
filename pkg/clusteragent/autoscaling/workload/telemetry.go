// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	corev1 "k8s.io/api/core/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
)

const (
	subsystem = "autoscaling_workload"
)

var (
	autoscalingQueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()
	commonOpts                      = telemetry.Options{NoDoubleUnderscoreSep: true}

	// telemetryReceivedRecommendationsVersion tracks the version of the received recommendations by the config retriever
	telemetryReceivedRecommendationsVersion = telemetry.NewGaugeWithOpts(
		subsystem,
		"received_recommendations_version",
		[]string{"namespace", "target_name", "autoscaler_name", le.JoinLeaderLabel},
		"Tracks the version of the received recommendations by the config retriever",
		commonOpts,
	)
	// telemetryHorizontalScaleActions tracks the number of horizontal scaling attempts
	telemetryHorizontalScaleActions = telemetry.NewCounterWithOpts(
		subsystem,
		"horizontal_scaling_actions",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "status", le.JoinLeaderLabel},
		"Tracks the number of horizontal scale events done",
		commonOpts,
	)
	// telemetryHorizontalScaleReceivedRecommendations tracks the horizontal scaling recommendation values received
	telemetryHorizontalScaleReceivedRecommendations = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_received_replicas",
		[]string{"namespace", "target_name", "autoscaler_name", "source", le.JoinLeaderLabel},
		"Tracks the value of replicas applied by the horizontal scaling recommendation",
		commonOpts,
	)
	// telemetryHorizontalScaleAppliedRecommendations tracks the horizontal scaling recommendation values applied
	telemetryHorizontalScaleAppliedRecommendations = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_applied_replicas",
		[]string{"namespace", "target_name", "autoscaler_name", "source", le.JoinLeaderLabel},
		"Tracks the value of replicas applied by the horizontal scaling recommendation",
		commonOpts,
	)

	// telemetryVerticalRolloutTriggered tracks the number of patch requests sent by the patcher to the kubernetes api server
	telemetryVerticalRolloutTriggered = telemetry.NewCounterWithOpts(
		subsystem,
		"vertical_rollout_triggered",
		[]string{"namespace", "target_name", "autoscaler_name", "status", le.JoinLeaderLabel},
		"Tracks the number of patch requests sent by the patcher to the kubernetes api server",
		commonOpts,
	)
	// telemetryVerticalScaleReceivedRecommendationsRequests tracks the vertical scaling recommendation requests received
	telemetryVerticalScaleReceivedRecommendationsRequests = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_requests",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "container_name", "resource_name", le.JoinLeaderLabel},
		"Tracks the value of requests received by the config retriever",
		commonOpts,
	)
	// telemetryVerticalScaleReceivedRecommendationsLimits tracks the vertical scaling recommendation limits received
	telemetryVerticalScaleReceivedRecommendationsLimits = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_limits",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "container_name", "resource_name", le.JoinLeaderLabel},
		"Tracks the value of limits received by the config retriever",
		commonOpts,
	)

	// autoscalingStatusConditions tracks the changes in autoscaler conditions
	autoscalingStatusConditions = telemetry.NewGaugeWithOpts(
		subsystem,
		"autoscaler_conditions",
		[]string{"namespace", "autoscaler_name", "type", le.JoinLeaderLabel},
		"Tracks the changes in autoscaler conditions",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	// telemetryLocalFallbackEnabled tracks whether local fallback recommendations are being used
	telemetryLocalFallbackEnabled = telemetry.NewGaugeWithOpts(
		subsystem,
		"local_fallback_enabled",
		[]string{"namespace", "target_name", "autoscaler_name", le.JoinLeaderLabel},
		"Tracks whether local fallback recommendations are being used",
		commonOpts,
	)

	// telemetryMetricsForDeletion contains all gauge metrics that need to be cleaned up when deleting pod autoscaler telemetry
	telemetryMetricsForDeletion = []telemetry.Gauge{
		telemetryReceivedRecommendationsVersion,
		telemetryHorizontalScaleAppliedRecommendations,
		telemetryHorizontalScaleReceivedRecommendations,
		telemetryVerticalScaleReceivedRecommendationsLimits,
		telemetryVerticalScaleReceivedRecommendationsRequests,
		autoscalingStatusConditions,
	}
)

func trackPodAutoscalerReceivedValues(podAutoscaler model.PodAutoscalerInternal, version uint64) {
	// Emit telemetry for received values
	// Target name cannot normally be empty, but we handle it just in case
	var targetName string
	if podAutoscaler.Spec() != nil {
		targetName = podAutoscaler.Spec().TargetRef.Name
	}

	scalingValues := podAutoscaler.MainScalingValues()

	// Track received recommendations version
	telemetryReceivedRecommendationsVersion.Set(
		float64(version),
		podAutoscaler.Namespace(),
		targetName,
		podAutoscaler.Name(),
		le.JoinLeaderValue,
	)

	// Horizontal value
	if podAutoscaler.MainScalingValues().Horizontal != nil {
		telemetryHorizontalScaleReceivedRecommendations.Set(
			float64(scalingValues.Horizontal.Replicas),
			podAutoscaler.Namespace(),
			targetName,
			podAutoscaler.Name(),
			string(scalingValues.Horizontal.Source),
			le.JoinLeaderValue,
		)
	}

	// Vertical values
	if scalingValues.Vertical != nil {
		for _, containerResources := range scalingValues.Vertical.ContainerResources {
			for resource, value := range containerResources.Requests {
				telemetryVerticalScaleReceivedRecommendationsRequests.Set(
					value.AsApproximateFloat64(),
					podAutoscaler.Namespace(),
					targetName,
					podAutoscaler.Name(),
					string(scalingValues.Vertical.Source),
					containerResources.Name,
					string(resource),
					le.JoinLeaderValue,
				)
			}

			for resource, value := range containerResources.Limits {
				telemetryVerticalScaleReceivedRecommendationsLimits.Set(
					value.AsApproximateFloat64(),
					podAutoscaler.Namespace(),
					targetName,
					podAutoscaler.Name(),
					string(scalingValues.Vertical.Source),
					containerResources.Name,
					string(resource),
					le.JoinLeaderValue,
				)
			}
		}
	}
}

func trackPodAutoscalerStatus(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	for _, condition := range podAutoscaler.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			autoscalingStatusConditions.Set(1.0, podAutoscaler.Namespace, podAutoscaler.Name, string(condition.Type), le.JoinLeaderValue)
		} else {
			autoscalingStatusConditions.Set(0.0, podAutoscaler.Namespace, podAutoscaler.Name, string(condition.Type), le.JoinLeaderValue)
		}
	}
}

func deletePodAutoscalerTelemetry(ns, autoscalerName string) {
	log.Debugf("Deleting pod autoscaler telemetry for %s/%s", ns, autoscalerName)
	tags := map[string]string{
		"namespace":        ns,
		"autoscaler_name":  autoscalerName,
		le.JoinLeaderLabel: le.JoinLeaderValue,
	}

	for _, metric := range telemetryMetricsForDeletion {
		metric.DeletePartialMatch(tags)
	}
}

func trackLocalFallbackEnabled(currentSource datadoghqcommon.DatadogPodAutoscalerValueSource, podAutoscalerInternal model.PodAutoscalerInternal) {
	var value float64
	if currentSource == datadoghqcommon.DatadogPodAutoscalerLocalValueSource {
		value = 1
	} else {
		value = 0
	}
	telemetryLocalFallbackEnabled.Set(value, podAutoscalerInternal.Namespace(), podAutoscalerInternal.Spec().TargetRef.Name, podAutoscalerInternal.Name(), le.JoinLeaderValue)
}

func setHorizontalScaleAppliedRecommendations(toReplicas float64, ns, targetName, autoscalerName, source string) {
	// Clear previous values to prevent gauge from reporting old values for different sources
	unsetHorizontalScaleAppliedRecommendations(ns, targetName, autoscalerName)

	telemetryHorizontalScaleAppliedRecommendations.Set(
		toReplicas,
		ns,
		targetName,
		autoscalerName,
		source,
		le.JoinLeaderValue,
	)
}

func unsetHorizontalScaleAppliedRecommendations(ns, targetName, autoscalerName string) {
	tags := map[string]string{
		"namespace":        ns,
		"target_name":      targetName,
		"autoscaler_name":  autoscalerName,
		le.JoinLeaderLabel: le.JoinLeaderValue,
	}

	telemetryHorizontalScaleAppliedRecommendations.DeletePartialMatch(tags)
}
