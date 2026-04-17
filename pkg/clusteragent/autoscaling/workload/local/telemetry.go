// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	telemetrydef "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

const (
	subsystem = "autoscaling_workload_local" //nolint:unused
)

var (
	commonOpts = telemetrydef.Options{NoDoubleUnderscoreSep: true} //nolint:unused

	// telemetryHorizontalLocalRecommendations tracks the local horizontal scaling recommendation values
	telemetryHorizontalLocalRecommendations = telemetryimpl.GetCompatComponent().NewGaugeWithOpts( //nolint:unused
		subsystem,
		"horizontal_scaling_recommended_replicas",
		[]string{"namespace", "target_name", "autoscaler_name", "source", le.JoinLeaderLabel},
		"Tracks the value of replicas recommended by the local recommender",
		commonOpts,
	)

	telemetryHorizontalLocalUtilizationPct = telemetryimpl.GetCompatComponent().NewGaugeWithOpts( //nolint:unused
		subsystem,
		"horizontal_utilization_pct",
		[]string{"namespace", "target_name", "autoscaler_name", "source", le.JoinLeaderLabel},
		"Tracks the utilization value reported by the local recommender",
		commonOpts,
	)
)
