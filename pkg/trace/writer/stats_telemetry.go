// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package writer

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const statsTelemetrySubsystem = "trace_agent"

// StatsWriterTelemetry holds telemetry metrics for the stats writer
type StatsWriterTelemetry struct {
	clientPayloads telemetry.Counter
	payloads       telemetry.Counter
	statsBuckets   telemetry.Counter
	statsEntries   telemetry.Counter
	errors         telemetry.Counter
	retries        telemetry.Counter
	splits         telemetry.Counter
	bytes          telemetry.Counter
	dropped        telemetry.Counter
	droppedBytes   telemetry.Counter
	connectionFill telemetry.Histogram
	queueFill      telemetry.Histogram
}

// NewStatsWriterTelemetry creates a new StatsWriterTelemetry instance
func NewStatsWriterTelemetry() *StatsWriterTelemetry {
	opts := telemetry.Options{NoDoubleUnderscoreSep: true}
	return &StatsWriterTelemetry{
		clientPayloads: telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_client_payloads_total", []string{}, "Number of client payloads processed by the stats writer", opts),
		payloads:       telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_payloads_total", []string{}, "Number of stats payloads flushed", opts),
		statsBuckets:   telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_stats_buckets_total", []string{}, "Number of stats buckets processed", opts),
		statsEntries:   telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_stats_entries_total", []string{}, "Number of stats entries processed", opts),
		errors:         telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_errors_total", []string{}, "Number of stats writer flush errors", opts),
		retries:        telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_retries_total", []string{}, "Number of stats writer retries", opts),
		splits:         telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_splits_total", []string{}, "Number of stats payload splits", opts),
		bytes:          telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_bytes_total", []string{}, "Number of bytes emitted by the stats writer", opts),
		dropped:        telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_dropped_total", []string{}, "Number of stats payloads dropped", opts),
		droppedBytes:   telemetry.NewCounterWithOpts(statsTelemetrySubsystem, "stats_writer_dropped_bytes_total", []string{}, "Number of bytes dropped by the stats writer", opts),
		connectionFill: telemetry.NewHistogramWithOpts(statsTelemetrySubsystem, "stats_writer_connection_fill", []string{}, "Number of in-flight connections used by the stats writer", []float64{1, 2, 3, 4, 5, 6, 8, 10}, opts),
		queueFill:      telemetry.NewHistogramWithOpts(statsTelemetrySubsystem, "stats_writer_queue_fill_ratio", []string{}, "Queue fill ratio for the stats writer", []float64{0.25, 0.5, 0.75, 0.9, 1}, opts),
	}
}
