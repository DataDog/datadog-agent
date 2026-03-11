// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipelinesinkimpl

import (
	"sync/atomic"

	pipelinesink "github.com/DataDog/datadog-agent/comp/pipelinesink/def"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const subsystem = "pipelinesink"

var (
	tlmMetricsSent = telemetry.NewCounter(subsystem, "metrics_sent_total", nil,
		"Total number of metric samples successfully sent")
	tlmLogsSent = telemetry.NewCounter(subsystem, "logs_sent_total", nil,
		"Total number of log entries successfully sent")
	tlmMetricsDropped = telemetry.NewCounter(subsystem, "metrics_dropped_total", nil,
		"Total number of metric samples dropped (ring buffer overflow or send error)")
	tlmLogsDropped = telemetry.NewCounter(subsystem, "logs_dropped_total", nil,
		"Total number of log entries dropped (ring buffer overflow or send error)")
	tlmBytesSent = telemetry.NewCounter(subsystem, "bytes_sent_total", nil,
		"Total bytes written to the transport")
	tlmReconnects = telemetry.NewCounter(subsystem, "reconnects_total", nil,
		"Total number of transport reconnect events")
)

// counters holds atomic counters that back both the telemetry gauges and the
// Stats() call so both views stay consistent.
type counters struct {
	metricsSent    atomic.Uint64
	logsSent       atomic.Uint64
	metricsDropped atomic.Uint64
	logsDropped    atomic.Uint64
	bytesSent      atomic.Uint64
	reconnects     atomic.Uint64
}

func (c *counters) incMetricsSent(n uint64) {
	c.metricsSent.Add(n)
	tlmMetricsSent.Add(float64(n))
}

func (c *counters) incLogsSent(n uint64) {
	c.logsSent.Add(n)
	tlmLogsSent.Add(float64(n))
}

func (c *counters) incMetricsDropped(n uint64) {
	c.metricsDropped.Add(n)
	tlmMetricsDropped.Add(float64(n))
}

func (c *counters) incLogsDropped(n uint64) {
	c.logsDropped.Add(n)
	tlmLogsDropped.Add(float64(n))
}

func (c *counters) incBytesSent(n uint64) {
	c.bytesSent.Add(n)
	tlmBytesSent.Add(float64(n))
}

func (c *counters) incReconnects() {
	c.reconnects.Add(1)
	tlmReconnects.Inc()
}

func (c *counters) stats() pipelinesink.Stats {
	return pipelinesink.Stats{
		MetricsSent:    c.metricsSent.Load(),
		LogsSent:       c.logsSent.Load(),
		MetricsDropped: c.metricsDropped.Load(),
		LogsDropped:    c.logsDropped.Load(),
		BytesSent:      c.bytesSent.Load(),
		Reconnects:     c.reconnects.Load(),
	}
}
