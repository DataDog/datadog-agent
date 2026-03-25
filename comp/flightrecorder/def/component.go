// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// team: agent-runtimes

// Package flightrecorder provides the Component interface for the flight recorder.
package flightrecorder

// Component is the flight recorder that forwards signal data to the Rust sidecar.
type Component interface {
	// Stats returns current operational counters (for health endpoints / debugging).
	Stats() Stats
}

// Stats contains runtime counters for the flight recorder.
type Stats struct {
	MetricsSent     uint64
	LogsSent        uint64
	TraceStatsSent  uint64
	MetricsDropped  uint64
	LogsDropped     uint64
	TraceStatsDropped uint64
	BytesSent       uint64
	Reconnects      uint64
}
