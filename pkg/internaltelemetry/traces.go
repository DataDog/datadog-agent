// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package internaltelemetry full description in README.md
package internaltelemetry

// Traces is a collection of traces
type Traces []Trace

// Trace is a collection of spans with the same trace ID
type Trace []*Span

// Span used for installation telemetry
type Span struct {
	// Service is the name of the service that handled this span.
	Service string `json:"service"`
	// Name is the name of the operation this span represents.
	Name string `json:"name"`
	// Resource is the name of the resource this span represents.
	Resource string `json:"resource"`
	// TraceID is the ID of the trace to which this span belongs.
	TraceID uint64 `json:"trace_id"`
	// SpanID is the ID of this span.
	SpanID uint64 `json:"span_id"`
	// ParentID is the ID of the parent span.
	ParentID uint64 `json:"parent_id"`
	// Start is the start time of this span in nanoseconds since the Unix epoch.
	Start int64 `json:"start"`
	// Duration is the duration of this span in nanoseconds.
	Duration int64 `json:"duration"`
	// Error is the error status of this span.
	Error int32 `json:"error"`
	// Meta is a mapping from tag name to tag value for string-valued tags.
	Meta map[string]string `json:"meta,omitempty"`
	// Metrics is a mapping from metric name to metric value for numeric metrics.
	Metrics map[string]float64 `json:"metrics,omitempty"`
	// Type is the type of the span.
	Type string `json:"type"`
}
