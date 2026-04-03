// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync/atomic"

	flightrecorder "github.com/DataDog/datadog-agent/comp/flightrecorder/def"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const subsystem = "flightrecorder"

var (
	tlmMetricsSent = telemetry.NewCounter(subsystem, "metrics_sent_total", nil,
		"Total number of metric samples successfully sent")
	tlmLogsSent = telemetry.NewCounter(subsystem, "logs_sent_total", nil,
		"Total number of log entries successfully sent")
	tlmTraceStatsSent = telemetry.NewCounter(subsystem, "trace_stats_sent_total", nil,
		"Total number of trace stats entries successfully sent")
	tlmMetricsDropped = telemetry.NewCounter(subsystem, "metrics_dropped_total", []string{"reason"},
		"Total number of metric samples dropped")
	tlmLogsDropped = telemetry.NewCounter(subsystem, "logs_dropped_total", []string{"reason"},
		"Total number of log entries dropped")
	tlmTraceStatsDropped = telemetry.NewCounter(subsystem, "trace_stats_dropped_total", []string{"reason"},
		"Total number of trace stats entries dropped")
	tlmBytesSent = telemetry.NewCounter(subsystem, "bytes_sent_total", nil,
		"Total bytes written to the transport")
	tlmReconnects = telemetry.NewCounter(subsystem, "reconnects_total", nil,
		"Total number of transport reconnect events")
	tlmBatchSize = telemetry.NewGauge(subsystem, "batch_size", []string{"type"},
		"Number of items in the last flushed batch")
	tlmFlushCycles = telemetry.NewCounter(subsystem, "flush_cycles_total", nil,
		"Total number of flush loop iterations that had data to send")
	tlmSendDuration = telemetry.NewGauge(subsystem, "send_duration_ns", nil,
		"Duration of the last transport.Send() call in nanoseconds")
	tlmReconnectReason = telemetry.NewCounter(subsystem, "reconnect_reason_total", []string{"reason"},
		"Reason for transport reconnect (write_timeout, conn_refused, etc)")
)

// counters holds atomic counters that back both the telemetry gauges and the
// Stats() call so both views stay consistent.
type counters struct {
	metricsSent      atomic.Uint64
	logsSent         atomic.Uint64
	traceStatsSent   atomic.Uint64
	metricsDropped   atomic.Uint64
	logsDropped      atomic.Uint64
	traceStatsDropped atomic.Uint64
	bytesSent        atomic.Uint64
	reconnects       atomic.Uint64
}

func (c *counters) incMetricsSent(n uint64) {
	c.metricsSent.Add(n)
	tlmMetricsSent.Add(float64(n))
}

func (c *counters) incLogsSent(n uint64) {
	c.logsSent.Add(n)
	tlmLogsSent.Add(float64(n))
}

func (c *counters) incTraceStatsSent(n uint64) {
	c.traceStatsSent.Add(n)
	tlmTraceStatsSent.Add(float64(n))
}

// Drop reason tags for telemetry.
const (
	dropReasonOverflow  = "ring_overflow"
	dropReasonTransport = "transport_error"
)

func (c *counters) incMetricsDroppedOverflow(n uint64) {
	c.metricsDropped.Add(n)
	tlmMetricsDropped.Add(float64(n), dropReasonOverflow)
}

func (c *counters) incMetricsDroppedTransport(n uint64) {
	c.metricsDropped.Add(n)
	tlmMetricsDropped.Add(float64(n), dropReasonTransport)
}

func (c *counters) incLogsDroppedOverflow(n uint64) {
	c.logsDropped.Add(n)
	tlmLogsDropped.Add(float64(n), dropReasonOverflow)
}

func (c *counters) incLogsDroppedTransport(n uint64) {
	c.logsDropped.Add(n)
	tlmLogsDropped.Add(float64(n), dropReasonTransport)
}

func (c *counters) incTraceStatsDroppedOverflow(n uint64) {
	c.traceStatsDropped.Add(n)
	tlmTraceStatsDropped.Add(float64(n), dropReasonOverflow)
}

func (c *counters) incTraceStatsDroppedTransport(n uint64) {
	c.traceStatsDropped.Add(n)
	tlmTraceStatsDropped.Add(float64(n), dropReasonTransport)
}

func (c *counters) setBatchSize(typ string, n int) {
	tlmBatchSize.Set(float64(n), typ)
}

func (c *counters) incBytesSent(n uint64) {
	c.bytesSent.Add(n)
	tlmBytesSent.Add(float64(n))
}

func (c *counters) incReconnects() {
	c.reconnects.Add(1)
	tlmReconnects.Inc()
}

func (c *counters) incFlushCycles() {
	tlmFlushCycles.Inc()
}

func (c *counters) setSendDuration(ns int64) {
	tlmSendDuration.Set(float64(ns))
}

func (c *counters) incReconnectReason(reason string) {
	tlmReconnectReason.Inc(reason)
}

func (c *counters) stats() flightrecorder.Stats {
	return flightrecorder.Stats{
		MetricsSent:       c.metricsSent.Load(),
		LogsSent:          c.logsSent.Load(),
		TraceStatsSent:    c.traceStatsSent.Load(),
		MetricsDropped:    c.metricsDropped.Load(),
		LogsDropped:       c.logsDropped.Load(),
		TraceStatsDropped: c.traceStatsDropped.Load(),
		BytesSent:         c.bytesSent.Load(),
		Reconnects:        c.reconnects.Load(),
	}
}
