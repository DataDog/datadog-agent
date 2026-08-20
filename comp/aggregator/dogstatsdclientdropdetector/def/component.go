// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclientdropdetector defines the DogStatsD client drop detector component.
package dogstatsdclientdropdetector

// team: agent-metric-pipelines

// ClientByteMetric identifies a DogStatsD client byte telemetry signal.
type ClientByteMetric uint8

const (
	// ClientByteMetricSent identifies bytes successfully sent by clients.
	ClientByteMetricSent ClientByteMetric = iota
	// ClientByteMetricDropped identifies total bytes dropped by clients.
	ClientByteMetricDropped
	// ClientByteMetricDroppedQueue identifies bytes dropped by client sender queues.
	ClientByteMetricDroppedQueue
	// ClientByteMetricDroppedWriter identifies bytes dropped by client writers.
	ClientByteMetricDroppedWriter
)

// Component detects sustained client-reported DogStatsD payload drops and
// maintains the corresponding Agent Health issue lifecycle.
type Component interface {
	ObserveClientBytes(metric ClientByteMetric, bytes float64)
	CompleteFinalDogStatsDSerieFlush()
}
