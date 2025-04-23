// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package external

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

const (
	subsystem = "autoscaling_workload_external"
)

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

	// telemetryHorizontalExternalRecommendations tracks external horizontal scaling recommendation values
	telemetryHorizontalExternalRecommendations = telemetry.NewGaugeWithOpts(
		subsystem,
		"recommended_replicas",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "recommender_host", le.JoinLeaderLabel},
		"Tracks the value of replicas recommended by an external recommender",
		commonOpts,
	)
)
