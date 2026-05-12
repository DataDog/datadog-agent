// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package noopimpl provides a no-op implementation of the recorder component.
package noopimpl

import (
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

// Requires defines the dependencies for the recorder component
type Requires struct{}

// Provides defines the output of the recorder component
type Provides struct {
	Comp recorderdef.Component
}

// NewNoopComponent creates a new recorder component
func NewNoopComponent(_ Requires) (Provides, error) {
	return Provides{
		Comp: &recorderImplNoop{},
	}, nil
}

type recorderImplNoop struct{}

// GetHandle wraps the provided HandleFunc with recording capability.
// If recording is enabled via config, metrics will be written to parquet files.
// This is called by the observer's GetHandle to create the final handle chain.
func (*recorderImplNoop) GetHandle(_ observer.HandleFunc) observer.HandleFunc {
	return func(_ string) observer.Handle { return &noopHandle{} }
}

// ReadAllMetrics reads all metrics from parquet files and returns them as a slice.
// This is for batch loading scenarios (like testbench) where streaming via handles
// is not needed and direct access to all metrics at once is more efficient.
func (*recorderImplNoop) ReadAllMetrics(_ string) ([]recorderdef.MetricData, error) {
	return []recorderdef.MetricData{}, nil
}

// ReadAllTraces reads all traces from parquet files and returns them as a slice.
// Traces are stored as denormalized spans (one row per span) for efficient querying.
func (*recorderImplNoop) ReadAllTraces(_ string) ([]recorderdef.TraceData, error) {
	return []recorderdef.TraceData{}, nil
}

// ReadAllProfiles reads all profile metadata from parquet files and returns them as a slice.
// Profile binary data is stored in separate files referenced by BinaryPath.
func (*recorderImplNoop) ReadAllProfiles(_ string) ([]recorderdef.ProfileData, error) {
	return []recorderdef.ProfileData{}, nil
}

// ReadAllLogs reads all logs from parquet files and returns them as a slice.
func (*recorderImplNoop) ReadAllLogs(_ string) ([]recorderdef.LogData, error) {
	return []recorderdef.LogData{}, nil
}

// ReadAllTraceStats reads all APM trace stats from parquet files and returns them as a slice.
// Each element corresponds to one aggregated stat group (ClientGroupedStats).
func (*recorderImplNoop) ReadAllTraceStats(_ string) ([]recorderdef.TraceStatsData, error) {
	return []recorderdef.TraceStatsData{}, nil
}

type noopHandle struct{}

// ObserveMetric observes a DogStatsD metric sample.
func (*noopHandle) ObserveMetric(_ observer.MetricView) {}

// ObserveLog observes a log message.
func (*noopHandle) ObserveLog(_ observer.LogView) {}
