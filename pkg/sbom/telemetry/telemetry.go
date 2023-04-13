// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	subsystem = "sbom"
)

var (
	// SBOMAttempts tracks sbom collection attempts.
	SBOMAttempts = telemetry.NewCounterWithOpts(
		subsystem,
		"sbom_attempts",
		[]string{"source", "type"},
		"Number of sbom failures by (source, type)",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	// SBOMFailures tracks sbom collection attempts that fail.
	SBOMFailures = telemetry.NewCounterWithOpts(
		subsystem,
		"sbom_errors",
		[]string{"source", "type", "reason"},
		"Number of sbom failures by (source, type, reason)",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	// SBOMGenerationDuration measures the time that it takes to generate SBOMs
	// in seconds.
	SBOMGenerationDuration = telemetry.NewHistogramWithOpts(
		subsystem,
		"sbom_generation_duration",
		[]string{},
		"SBOM generation duration (in seconds)",
		[]float64{10, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600},
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	// SBOMCacheMemSize size in memory of the cache used for SBOM collection
	SBOMCacheMemSize = telemetry.NewGaugeWithOpts(
		subsystem,
		"sbom_cache_mem_size",
		[]string{},
		"SBOM cache size in memory (in bytes)",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	// SBOMCacheDiskSize size in disk of the cache used for SBOM collection
	SBOMCacheDiskSize = telemetry.NewGaugeWithOpts(
		subsystem,
		"sbom_cache_disk_size",
		[]string{},
		"SBOM cache size in disk (in bytes)",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)
