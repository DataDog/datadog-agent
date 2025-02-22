// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package telemetry implements a component for all agent telemetry.
package telemetry

import (
	"net/http"
)

// team: agent-runtimes

// Component is the component type.
type Component interface {
	// Handler returns an http handler to expose the internal metrics
	Handler() http.Handler
	// Reset resets all tracked telemetry
	Reset()
	// RegisterCollector Registers a Collector with the prometheus registry
	RegisterCollector(c Collector)
	// UnregisterCollector unregisters a Collector with the prometheus registry
	UnregisterCollector(c Collector) bool
	// NewCounter creates a Counter with default options for telemetry purpose.
	NewCounter(subsystem, name string, tags []string, help string) Counter
	// NewCounterWithOpts creates a Counter with the given options for telemetry purpose.
	NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter

	// NewSimpleCounter creates a new SimpleCounter with default options.
	NewSimpleCounter(subsystem, name, help string) SimpleCounter
	// NewSimpleCounterWithOpts creates a new SimpleCounter.
	NewSimpleCounterWithOpts(subsystem, name, help string, opts Options) SimpleCounter

	// NewGauge creates a Gauge with default options for telemetry purpose.
	NewGauge(subsystem, name string, tags []string, help string) Gauge
	// NewGaugeWithOpts creates a Gauge with the given options for telemetry purpose.
	NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge

	// NewSimpleGauge creates a new SimpleGauge with default options.
	NewSimpleGauge(subsystem, name, help string) SimpleGauge
	// NewSimpleGaugeWithOpts creates a new SimpleGauge.
	NewSimpleGaugeWithOpts(subsystem, name, help string, opts Options) SimpleGauge

	// NewHistogram creates a Histogram with default options for telemetry purpose.
	NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) Histogram
	// NewHistogramWithOpts creates a Histogram with the given options for telemetry purpose.
	NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts Options) Histogram

	// NewSimpleHistogram creates a new SimpleHistogram with default options.
	NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram
	// NewSimpleHistogramWithOpts creates a new SimpleHistogram.
	NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram

	// Gather exposes metrics from the general or default telemetry registry (see options.DefaultMetric)
	Gather(defaultGather bool) ([]*MetricFamily, error)
}
