// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package telemetry holds telemetry related files
package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/workqueue"
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
		[]string{"source", "scan_type"},
		"SBOM generation duration (in seconds)",
		[]float64{10, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600},
		commonOpts,
	)

	// SBOMExportSize is the size of the archive written on disk
	SBOMExportSize = telemetry.NewHistogramWithOpts(
		subsystem,
		"export_size",
		[]string{"source", "scan_ref"},
		"Size of the archive written on disk",
		[]float64{10_000_000, 50_000_000, 100_000_000, 200_000_000, 400_000_000, 600_000_000, 800_000_000, 1_000_000_000, 1_500_000_000},
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
)

// QueueMetricProvider is a workqueue.MetricsProvider that provides metrics for the SBOM queue
type QueueMetricProvider struct{}

// Ensure QueueMetricProvider implements the workqueue.MetricsProvider interface
var _ workqueue.MetricsProvider = QueueMetricProvider{}

type gaugeWrapper struct {
	telemetry.Gauge
}

// Inc implements the workqueue.GaugeMetric interface
func (g gaugeWrapper) Inc() {
	g.Gauge.Inc()
}

// Dec implements the workqueue.GaugeMetric interface
func (g gaugeWrapper) Dec() {
	g.Gauge.Dec()
}

// Set implements the workqueue.GaugeMetric interface
func (g gaugeWrapper) Set(v float64) {
	g.Gauge.Set(v)
}

type counterWrapper struct {
	telemetry.Counter
}

// Inc implements the workqueue.CounterMetric interface
func (c counterWrapper) Inc() {
	c.Counter.Inc()
}

type histgramWrapper struct {
	telemetry.Histogram
}

// Observer implements the workqueue.HistogramMetric interface
func (l histgramWrapper) Observe(value float64) {
	l.Histogram.Observe(value)
}

// NewDepthMetric creates a new depth metric
func (QueueMetricProvider) NewDepthMetric(string) workqueue.GaugeMetric {
	return gaugeWrapper{telemetry.NewGaugeWithOpts(
		subsystem,
		"queue_depth",
		[]string{},
		"SBOM queue depth",
		commonOpts,
	)}
}

// NewAddsMetric creates a new adds metric
func (QueueMetricProvider) NewAddsMetric(string) workqueue.CounterMetric {
	return counterWrapper{telemetry.NewCounterWithOpts(
		subsystem,
		"queue_adds",
		[]string{},
		"SBOM queue adds",
		commonOpts,
	)}
}

// NewLatencyMetric creates a new latency metric
func (QueueMetricProvider) NewLatencyMetric(string) workqueue.HistogramMetric {
	return histgramWrapper{telemetry.NewHistogramWithOpts(
		subsystem,
		"queue_latency",
		[]string{},
		"SBOM queue latency in seconds",
		[]float64{1, 15, 60, 120, 600, 1200},
		commonOpts,
	)}
}

// NewWorkDurationMetric creates a new work duration metric
func (QueueMetricProvider) NewWorkDurationMetric(string) workqueue.HistogramMetric {
	return histgramWrapper{telemetry.NewHistogramWithOpts(
		subsystem,
		"queue_work_duration",
		[]string{},
		"SBOM queue latency in seconds",
		prometheus.DefBuckets,
		commonOpts,
	)}
}

// NewUnfinishedWorkSecondsMetric creates a new unfinished work seconds metric
func (QueueMetricProvider) NewUnfinishedWorkSecondsMetric(string) workqueue.SettableGaugeMetric {
	return gaugeWrapper{telemetry.NewGaugeWithOpts(
		subsystem,
		"queue_unfinished_work",
		[]string{},
		"SBOM queue unfinished work in seconds",
		commonOpts,
	)}
}

// NewLongestRunningProcessorSecondsMetric creates a new longest running processor seconds metric
func (QueueMetricProvider) NewLongestRunningProcessorSecondsMetric(string) workqueue.SettableGaugeMetric {
	return gaugeWrapper{telemetry.NewGaugeWithOpts(
		subsystem,
		"queue_longest_running_processor",
		[]string{},
		"SBOM queue longest running processor in seconds",
		commonOpts,
	)}
}

// NewRetriesMetric creates a new retries metric
func (QueueMetricProvider) NewRetriesMetric(string) workqueue.CounterMetric {
	return counterWrapper{telemetry.NewCounterWithOpts(
		subsystem,
		"queue_retries",
		[]string{},
		"SBOM queue retries",
		commonOpts,
	)}
}
