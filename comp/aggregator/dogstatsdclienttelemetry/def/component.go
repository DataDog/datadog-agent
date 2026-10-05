// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclienttelemetry defines the DogStatsD client telemetry component.
package dogstatsdclienttelemetry

import "github.com/DataDog/datadog-agent/pkg/aggregator"

// team: agent-metric-pipelines

// Component observes final DogStatsD series to record client byte telemetry.
type Component interface {
	aggregator.FinalDogStatsDSerieObserver
}
