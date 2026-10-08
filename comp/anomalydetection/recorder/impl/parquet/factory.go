// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

// Package parquet writes recorder observations in the Parquet v1 format.
package parquet

import recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"

// Factory constructs Parquet writers when recording is enabled.
type Factory struct{}

var (
	_ recorder.WriterFactory = Factory{}
	_ recorder.MetricWriter  = (*metricParquetWriter)(nil)
	_ recorder.LogWriter     = (*logParquetWriter)(nil)
)

// NewMetricWriter constructs a metric writer.
func (Factory) NewMetricWriter(cfg recorder.WriterConfig) (recorder.MetricWriter, error) {
	return newMetricParquetWriter(cfg.OutputDir, cfg.FlushInterval, cfg.Retention)
}

// NewLogWriter constructs a log writer.
func (Factory) NewLogWriter(cfg recorder.WriterConfig) (recorder.LogWriter, error) {
	return newLogParquetWriter(cfg.OutputDir, cfg.FlushInterval, cfg.Retention)
}
