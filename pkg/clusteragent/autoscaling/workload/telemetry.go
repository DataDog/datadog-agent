// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
)

const (
	subsystem = "workload_autoscaling"
)

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

var (
	// rolloutTriggered tracks the number of patch requests sent by the patcher to the kubernetes api server
	rolloutTriggered = telemetry.NewCounterWithOpts(
		subsystem,
		"rollout_triggered",
		[]string{"owner_kind", "owner_name", "namespace", "status"},
		"Tracks the number of patch requests sent by the patcher to the kubernetes api server",
		commonOpts,
	)

	// telemetryHorizontalScaleAttempts tracks the number of horizontal scaling attempts
	telemetryHorizontalScaleAttempts = telemetry.NewCounterWithOpts(
		subsystem,
		"horizontal_scaling_attempts",
		[]string{"namespace", "target_name", "autoscaler_name"},
		"Tracks the number of horizontal scaling events triggered",
		commonOpts,
	)
	// telemetryHorizontalScaleErrors tracks the number of horizontal scaling errors
	telemetryHorizontalScaleErrors = telemetry.NewCounterWithOpts(
		subsystem,
		"horizontal_scaling_errors",
		[]string{"namespace", "target_name", "autoscaler_name"},
		"Tracks the number of horizontal scaling events triggered",
		commonOpts,
	)

	// telemetryHorizontalScaleReceivedRecommendations tracks the horizontal scaling recommendation values received
	telemetryHorizontalScaleReceivedRecommendations = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_received_replicas",
		[]string{"namespace", "target_name", "autoscaler_name"},
		"Tracks the value of replicas applied by the horizontal scaling recommendation",
		commonOpts,
	)
	// telemetryHorizontalScaleAppliedRecommendations tracks the horizontal scaling recommendation values applied
	telemetryHorizontalScaleAppliedRecommendations = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_applied_replicas",
		[]string{"namespace", "target_name", "autoscaler_name"},
		"Tracks the value of replicas applied by the horizontal scaling recommendation",
		commonOpts,
	)

	// telemetryVerticalScaleAttempts tracks the number of vertical scaling attempts
	telemetryVerticalScaleAttempts = telemetry.NewCounterWithOpts(
		subsystem,
		"vertical_scaling_attempts",
		[]string{"namespace", "target_name", "autoscaler_name", "source"},
		"Tracks the number of vertical scaling events triggered",
		commonOpts,
	)
	// telemetryVerticalScaleErrors tracks the number of vertical scaling errors
	telemetryVerticalScaleErrors = telemetry.NewCounterWithOpts(
		subsystem,
		"vertical_scaling_errors",
		[]string{"namespace", "target_name", "autoscaler_name", "source"},
		"Tracks the number of vertical scaling events triggered",
		commonOpts,
	)

	// telemetryVerticalScaleReceivedRecommendationsLimits tracks the vertical scaling recommendation limits received
	telemetryVerticalScaleReceivedRecommendationsLimits = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_limits",
		[]string{"namespace", "target_name", "autoscaler_name", "resource_name"},
		"Tracks the value of limits received by the vertical scaling controller",
		commonOpts,
	)
	// telemetryVerticalScaleReceivedRecommendationsRequests tracks the vertical scaling recommendation requests received
	telemetryVerticalScaleReceivedRecommendationsRequests = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_requests",
		[]string{"namespace", "target_name", "autoscaler_name", "resource_name"},
		"Tracks the value of requests received by the vertical scaling recommendation",
		commonOpts,
	)
	// telemetryVerticalScaleAppliedRecommendationsLimits tracks the vertical scaling recommendation limits applied
	telemetryVerticalScaleAppliedRecommendationsLimits = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_applied_limits",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "resource_name"},
		"Tracks the value of limits applied by the vertical scaling controller",
		commonOpts,
	)
	// telemetryVerticalScaleAppliedRecommendationsRequests tracks the vertical scaling recommendation requests applied
	telemetryVerticalScaleAppliedRecommendationsRequests = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_applied_requests",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "resource_name"},
		"Tracks the value of requests applied by the vertical scaling controller",
		commonOpts,
	)

	// autoscalingStatusConditions tracks the changes in autoscaler conditions
	autoscalingStatusConditions = telemetry.NewGaugeWithOpts(
		subsystem,
		"autoscaler_conditions",
		[]string{"namespace", "autoscaler_name", "type", "reason", "message"},
		"Tracks the changes in autoscaler conditions",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	autoscalingQueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()
)
