// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package writer

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// TraceWriterTelemetry holds telemetry metrics for the trace writer
type TraceWriterTelemetry struct {
	payloads          telemetry.Counter
	bytesUncompressed telemetry.Counter
	retries           telemetry.Counter
	bytes             telemetry.Counter
	errors            telemetry.Counter
	traces            telemetry.Counter
	events            telemetry.Counter
	spans             telemetry.Counter
	dropped           telemetry.Counter
	droppedBytes      telemetry.Counter
	connectionFill    telemetry.Histogram
	queueFill         telemetry.Histogram
}

// Singleton pattern to prevent duplicate Prometheus metric registration panics.
// Many tests create multiple Agent instances in the same process, and each Agent
// creates telemetry. Since Prometheus uses a global registry, registering the same
// metric multiple times causes a panic. Using sync.Once ensures metrics are registered
// exactly once per process, allowing tests to safely create multiple agents.
// The alternative would be refactoring all tests to use mocks or reset global state.
var (
	traceWriterTelemetryInstance *TraceWriterTelemetry
	traceWriterTelemetryOnce     sync.Once
)

// NewTraceWriterTelemetry creates a new TraceWriterTelemetry instance
func NewTraceWriterTelemetry() *TraceWriterTelemetry {
	traceWriterTelemetryOnce.Do(func() {
		opts := telemetry.Options{NoDoubleUnderscoreSep: true}
		traceWriterTelemetryInstance = &TraceWriterTelemetry{
			payloads:          telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_payloads_total", []string{}, "Number of trace payloads flushed", opts),
			bytesUncompressed: telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_bytes_uncompressed_total", []string{}, "Number of uncompressed trace bytes processed", opts),
			retries:           telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_retries_total", []string{}, "Number of trace writer retries", opts),
			bytes:             telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_bytes_total", []string{}, "Number of trace bytes emitted", opts),
			errors:            telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_errors_total", []string{}, "Number of trace writer errors", opts),
			traces:            telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_traces_total", []string{}, "Number of traces processed", opts),
			events:            telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_events_total", []string{}, "Number of events processed", opts),
			spans:             telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_spans_total", []string{}, "Number of spans processed", opts),
			dropped:           telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_dropped_total", []string{}, "Number of trace payloads dropped", opts),
			droppedBytes:      telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "trace_writer_dropped_bytes_total", []string{}, "Number of bytes dropped by the trace writer", opts),
			connectionFill:    telemetry.NewHistogramWithOpts(statsTelemetrySubsystem, "trace_writer_connection_fill", []string{}, "Number of in-flight connections used by the trace writer", []float64{1, 2, 3, 4, 5, 6, 8, 10}, opts),
			queueFill:         telemetry.NewHistogramWithOpts(statsTelemetrySubsystem, "trace_writer_queue_fill_ratio", []string{}, "Queue fill ratio for the trace writer", []float64{0.25, 0.5, 0.75, 0.9, 1}, opts),
		}
	})
	return traceWriterTelemetryInstance
}
