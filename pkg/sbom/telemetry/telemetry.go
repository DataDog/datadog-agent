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

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

var (
	// SBOMAttempts tracks sbom collection attempts.
	SBOMAttempts = telemetry.NewCounterWithOpts(
		subsystem,
		"attempts",
		[]string{"source", "type"},
		"Number of sbom failures by (source, type)",
		commonOpts,
	)
	// SBOMFailures tracks sbom collection attempts that fail.
	SBOMFailures = telemetry.NewCounterWithOpts(
		subsystem,
		"errors",
		[]string{"source", "type", "reason"},
		"Number of sbom failures by (source, type, reason)",
		commonOpts,
	)

	// SBOMGenerationDuration measures the time that it takes to generate SBOMs
	// in seconds.
	SBOMGenerationDuration = telemetry.NewHistogramWithOpts(
		subsystem,
		"generation_duration",
		[]string{},
		"SBOM generation duration (in seconds)",
		[]float64{10, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600},
		commonOpts,
	)

	// SBOMCacheMemSize size in memory of the cache used for SBOM collection
	SBOMCacheMemSize = telemetry.NewGaugeWithOpts(
		subsystem,
		"cache_mem_size",
		[]string{},
		"SBOM cache size in memory (in bytes)",
		commonOpts,
	)

	// SBOMCacheDiskSize size in disk of the custom cache used for SBOM collection
	SBOMCacheDiskSize = telemetry.NewGaugeWithOpts(
		subsystem,
		"cache_disk_size",
		[]string{},
		"SBOM size in disk of the custom cache (in bytes)",
		commonOpts,
	)

	// SBOM number of cache keys stored in memory
	SBOMCacheEntries = telemetry.NewGaugeWithOpts(
		subsystem,
		"cached_keys",
		[]string{},
		"Number of cache keys stored in memory",
		commonOpts,
	)

	// SBOMCachedObjectSize total size of cached objects in disk (in bytes) used for SBOM collection
	SBOMCachedObjectSize = telemetry.NewGaugeWithOpts(
		subsystem,
		"cached_objects_size",
		[]string{},
		"SBOM total size of cached objects in disk (in bytes)",
		commonOpts,
	)

	// SBOMCacheHits number of cache hits during SBOM collection
	SBOMCacheHits = telemetry.NewCounterWithOpts(
		subsystem,
		"cache_hits_total",
		[]string{},
		"SBOM total number of cache hits during SBOM collection",
		commonOpts,
	)

	// SBOMCacheMisses number of cache misses during SBOM collection
	SBOMCacheMisses = telemetry.NewCounterWithOpts(
		subsystem,
		"cache_misses_total",
		[]string{},
		"SBOM total number of cache misses during SBOM collection",
		commonOpts,
	)

	// SBOMCacheEvicts number of cache evicts during SBOM collection
	SBOMCacheEvicts = telemetry.NewCounterWithOpts(
		subsystem,
		"cache_evicts_total",
		[]string{},
		"SBOM total number of cache misses during SBOM collection",
		commonOpts,
	)
)
