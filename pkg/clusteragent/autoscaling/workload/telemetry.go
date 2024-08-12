// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	subsystem              = "autoscaling_workload"
	aliveTelemetryInterval = 5 * time.Minute
)

var (
	autoscalingQueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()
	commonOpts                      = telemetry.Options{NoDoubleUnderscoreSep: true}

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
	// telemetryVerticalScaleReceivedRecommendationsLimits tracks the vertical scaling recommendation limits received
	telemetryVerticalScaleReceivedRecommendationsLimits = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_limits",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "container_name", "resource_name", le.JoinLeaderLabel},
		"Tracks the value of limits received by the vertical scaling controller",
		commonOpts,
	)
	// telemetryVerticalScaleReceivedRecommendationsRequests tracks the vertical scaling recommendation requests received
	telemetryVerticalScaleReceivedRecommendationsRequests = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_requests",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "container_name", "resource_name", le.JoinLeaderLabel},
		"Tracks the value of requests received by the vertical scaling recommendation",
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
)

func trackPodAutoscalerStatus(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	for _, condition := range podAutoscaler.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			autoscalingStatusConditions.Set(1.0, podAutoscaler.Namespace, podAutoscaler.Name, string(condition.Type), le.JoinLeaderValue)
			autoscalingStatusConditions.Set(0.0, podAutoscaler.Namespace, podAutoscaler.Name, string(condition.Type), le.JoinLeaderValue)
		}
	}
}

func startLocalTelemetry(ctx context.Context, sender sender.Sender, tags []string) {
	submit := func() {
		sender.Gauge("datadog.cluster_agent.autoscaling.workload.running", 1, "", tags)
		sender.Commit()
	}

	go func() {
		ticker := time.NewTicker(aliveTelemetryInterval)
		defer ticker.Stop()

		// Submit once immediately and then every ticker
		submit()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				submit()
			}
		}
	}()
}
