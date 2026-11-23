// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package recorder

import "time"

// WriterConfig contains backend settings resolved before writer construction.
type WriterConfig struct {
	OutputDir     string
	FlushInterval time.Duration
	Retention     time.Duration
}

// MetricWriter receives snapshots stable after WriteMetric returns.
// WriteMetric reports acceptance. Writes and Close are concurrency-safe;
// Close is idempotent and returns final-write errors.
type MetricWriter interface {
	WriteMetric(MetricData) bool
	Close() error
}

// LogWriter receives snapshots stable after WriteLog returns.
// WriteLog reports acceptance. Writes and Close are concurrency-safe;
// Close is idempotent and returns final-write errors.
type LogWriter interface {
	WriteLog(LogData) bool
	Close() error
}

// WriterFactory creates writers for enabled recording.
type WriterFactory interface {
	NewMetricWriter(WriterConfig) (MetricWriter, error)
	NewLogWriter(WriterConfig) (LogWriter, error)
}
