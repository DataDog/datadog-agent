// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricslogs provides a component that lets other internal agent
// components write metrics as structured lines to the agent's own local
// log, bypassing metric-tag cardinality limits.
package metricslogs

// team: agent-runtimes

// Component lets other internal, non-customer-facing agent components write
// metrics as structured lines to the agent's own local log — bypassing
// metric-tag cardinality limits — without shipping anything over the
// network. Nothing sent through this component leaves the host or counts
// as billable Datadog usage.
type Component interface {
	// LogMetrics writes a batch of metrics as one structured line to the
	// agent's local log, at Debug level by default. All metrics in one call
	// share a single timestamp, so callers should batch metrics that must
	// stay correlated (e.g. all per-map/per-program stats gathered within
	// one check Run()) into a single LogMetrics call.
	LogMetrics(metrics []*Metric, opts ...Option) error
}

// Level selects which local log level a LogMetrics call writes at.
type Level int

const (
	// LevelDebug logs at Debug level (the default).
	LevelDebug Level = iota
	// LevelTrace logs at Trace level, for callers that want a quieter
	// option than agent-wide Debug.
	LevelTrace
)

// LogConfig holds the resolved options for one LogMetrics call.
type LogConfig struct {
	Level Level
}

// Option customizes a single LogMetrics call.
type Option func(*LogConfig)

// WithLevel selects the local log level for one LogMetrics call. The
// default, when omitted, is LevelDebug.
func WithLevel(level Level) Option {
	return func(c *LogConfig) { c.Level = level }
}

// MetricType is the type of a metric passed to LogMetrics.
type MetricType string

const (
	// MetricTypeGauge is a gauge metric.
	MetricTypeGauge MetricType = "gauge"
	// MetricTypeCount is a count metric.
	MetricTypeCount MetricType = "count"
)

// Metric is a single metric passed to LogMetrics.
type Metric struct {
	Name  string
	Type  MetricType
	Value float64
	Tags  []string
}
