// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorder provides a middleware component for recording and replaying observer data.
//
// The recorder intercepts metrics flowing through observer handles, optionally
// recording them to parquet files. This enables:
// - Capturing production data for offline analysis
// - Loading recorded data for testing and debugging
// - Building reproducible test scenarios from real workloads
package recorder

import (
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// team: agent-metric-pipelines

// Component is the recorder middleware component.
// It wraps observer handles to intercept and optionally record observations.
type Component interface {
	// GetHandle wraps the provided HandleFunc with recording capability.
	// If recording is enabled via config, metrics will be written to parquet files.
	// This is called by the observer's GetHandle to create the final handle chain.
	GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc

	// Used for debug metrics, these won't go through the handle chain.
	WriteMetric(source string, sample observer.MetricView) error

	// ReadAllMetrics reads all metrics from parquet files and returns them as a slice.
	// This is for batch loading scenarios (like testbench) where streaming via handles
	// is not needed and direct access to all metrics at once is more efficient.
	ReadAllMetrics(inputDir string) ([]MetricData, error)
}

// MetricData represents a single metric read from parquet files.
// Used by ReadAllMetrics for batch loading scenarios.
type MetricData struct {
	Source    string   // Source/namespace (RunID in parquet)
	Name      string   // Metric name
	Value     float64  // Metric value
	Timestamp int64    // Unix timestamp in seconds
	Tags      []string // Tags in "key:value" format
}
