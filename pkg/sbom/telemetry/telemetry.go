// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package telemetry holds telemetry related files
package telemetry

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
)

const (
	// Subsystem is the subsystem name for the provided telemetry for sbom
	Subsystem = "sbom"
)

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

var (
	// SBOMAttempts tracks sbom collection attempts.
	SBOMAttempts = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		Subsystem,
		"attempts",
		[]string{"source", "type"},
		"Number of sbom failures by (source, type)",
		commonOpts,
	)
	// SBOMFailures tracks sbom collection attempts that fail.
	SBOMFailures = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		Subsystem,
		"errors",
		[]string{"source", "type", "reason"},
		"Number of sbom failures by (source, type, reason)",
		commonOpts,
	)

	// SBOMGenerationDuration measures the time that it takes to generate SBOMs
	// in seconds.
	SBOMGenerationDuration = telemetryimpl.GetCompatComponent().NewHistogramWithOpts(
		Subsystem,
		"generation_duration",
		[]string{"source", "scan_type"},
		"SBOM generation duration (in seconds)",
		[]float64{10, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600},
		commonOpts,
	)

	// SBOMExportSize is the size of the archive written on disk
	SBOMExportSize = telemetryimpl.GetCompatComponent().NewHistogramWithOpts(
		Subsystem,
		"export_size",
		[]string{"source", "scan_ref"},
		"Size of the archive written on disk",
		[]float64{10_000_000, 50_000_000, 100_000_000, 200_000_000, 400_000_000, 600_000_000, 800_000_000, 1_000_000_000, 1_500_000_000},
		commonOpts,
	)

	// SBOMComponentsFound is the number of components found in a SBOM
	SBOMComponentsFound = telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
		Subsystem,
		"components_found",
		[]string{"source", "type"},
		"Number of components found in a SBOM",
		commonOpts,
	)

	// QueueMetricsProvider is the metrics provider for the sbom scanner retry queue
	QueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()
)
