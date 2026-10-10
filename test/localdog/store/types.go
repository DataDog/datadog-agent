// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package store holds the telemetry received by localdog in memory.
package store

// Log is a single log event received from the logs intake.
type Log struct {
	ID         string         `json:"id"`
	Timestamp  int64          `json:"timestamp"` // unix milliseconds
	Message    string         `json:"message"`
	Status     string         `json:"status"`
	Service    string         `json:"service"`
	Host       string         `json:"host"`
	Source     string         `json:"source"`
	Tags       []string       `json:"tags"`
	Attributes map[string]any `json:"attributes"`
	TraceID    string         `json:"trace_id,omitempty"`
	SpanID     string         `json:"span_id,omitempty"`
}

// Span is a single APM span received from the traces intake.
// IDs are decimal strings (the 64-bit form used throughout the Datadog UI).
type Span struct {
	TraceID     string             `json:"trace_id"`
	TraceIDHigh string             `json:"trace_id_high,omitempty"` // hex upper 64 bits of 128-bit trace IDs
	SpanID      string             `json:"span_id"`
	ParentID    string             `json:"parent_id"`
	Service     string             `json:"service"`
	Name        string             `json:"name"`
	Resource    string             `json:"resource"`
	Type        string             `json:"type"`
	Env         string             `json:"env"`
	Host        string             `json:"host"`
	Version     string             `json:"version"`
	Start       int64              `json:"start"`    // unix nanoseconds
	Duration    int64              `json:"duration"` // nanoseconds
	Error       int32              `json:"error"`
	Meta        map[string]string  `json:"meta"`
	Metrics     map[string]float64 `json:"metrics"`
	IsRoot      bool               `json:"is_root"`
	IsTopLevel  bool               `json:"is_top_level"`
	ReceivedAt  int64              `json:"-"`
	ingestOrder uint64
}

// Point is a single metric datapoint.
type Point struct {
	Timestamp int64   `json:"t"` // unix seconds
	Value     float64 `json:"v"`
}

// Series is a metric timeseries uniquely identified by name and tag set.
type Series struct {
	Key  string   `json:"-"`
	Name string   `json:"metric"`
	Type string   `json:"type"`
	Host string   `json:"host"`
	Tags []string `json:"tags"`
	Unit string   `json:"unit,omitempty"`
	// Interval is the flush interval in seconds of rate/count series (dogstatsd counters are
	// sent as per-second rates over this interval).
	Interval int64   `json:"interval,omitempty"`
	Points   []Point `json:"points"`
}

// Stats summarizes what localdog has received so far.
type Stats struct {
	Logs         int            `json:"logs"`
	Spans        int            `json:"spans"`
	Traces       int            `json:"traces"`
	MetricSeries int            `json:"metric_series"`
	MetricNames  int            `json:"metric_names"`
	Points       int            `json:"points"`
	Hosts        []string       `json:"hosts"`
	Services     []string       `json:"services"`
	Payloads     map[string]int `json:"payloads"`
	LastPayload  int64          `json:"last_payload"` // unix milliseconds
}
