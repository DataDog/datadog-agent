// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorder defines middleware and writer contracts for recording observer data.
package recorder

import (
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// team: agent-anomaly-detection

// Component is the recorder middleware component.
// It wraps observer handles to record observations after forwarding them.
type Component interface {
	// GetHandle wraps the provided handle factory with recording capability.
	GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc
}

// MetricData is a recorded metric. Timestamp is in Unix seconds.
type MetricData struct {
	Source     string   // Source/namespace
	Name       string   // Metric name
	MetricType string   // Original metric type, or Unknown
	Value      float64  // Metric value
	Timestamp  int64    // Unix timestamp in seconds
	Tags       []string // Tags in "key:value" format
	Dropped    bool     // True if the live observer's channel dropped this observation
}

// LogData is a recorded log. TimestampMs is in Unix milliseconds.
type LogData struct {
	Source      string   // Source/namespace
	TimestampMs int64    // Unix timestamp in milliseconds since epoch
	Content     []byte   // Log message content (raw bytes)
	Status      string   // Log severity level (debug, info, warn, error, etc.)
	Hostname    string   // Hostname where log originated
	Tags        []string // Tags in "key:value" format
}
