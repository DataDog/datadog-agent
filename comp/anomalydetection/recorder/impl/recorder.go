// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorderimpl implements the recorder component interface
package recorderimpl

import (
	"fmt"
	"sync"
	"time"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the recorder component
type Requires struct {
	Config config.Component
}

// Provides defines the output of the recorder component
type Provides struct {
	Comp recorderdef.Component
}

// NewComponent creates a new recorder component
func NewComponent(req Requires) (Provides, error) {
	r := &recorderImpl{}

	// Initialize recording if both capture_metrics.enabled AND parquet_output_dir are configured.
	captureMetricsEnabled := req.Config.GetBool("observer.capture_metrics.enabled")
	if captureMetricsEnabled {
		if parquetDir := req.Config.GetString("observer.parquet_output_dir"); parquetDir != "" {
			flushInterval := req.Config.GetDuration("observer.parquet_flush_interval")
			if flushInterval == 0 {
				flushInterval = 60 * time.Second
			}

			retentionDuration := req.Config.GetDuration("observer.parquet_retention")
			if retentionDuration <= 0 {
				retentionDuration = 24 * time.Hour
			}

			// Create parquet writer
			writer, err := NewParquetWriter(parquetDir, flushInterval, retentionDuration)
			if err != nil {
				pkglog.Errorf("Failed to create parquet writer: %v", err)
			} else {
				r.parquetWriter = writer
				pkglog.Infof("Recorder started with parquet output: dir=%s flush=%v retention=%v", parquetDir, flushInterval, retentionDuration)
			}
		}
	} else {
		pkglog.Debug("Recorder parquet writer disabled (observer.capture_metrics.enabled is false)")
	}

	return Provides{Comp: r}, nil
}

// recorderImpl implements the recorder component
type recorderImpl struct {
	parquetWriter *ParquetWriter
	mu            sync.Mutex
}

// GetHandle wraps the provided HandleFunc with recording capability.
func (r *recorderImpl) GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc {
	return func(name string) observer.Handle {
		innerHandle := handleFunc(name)

		// If recording is enabled, wrap with recording handle
		if r.parquetWriter != nil {
			return &recordingHandle{
				inner:    innerHandle,
				recorder: r,
				name:     name,
			}
		}

		// No recording, pass through
		return innerHandle
	}
}

func (r *recorderImpl) WriteMetric(source string, sample observer.MetricView) error {
	if r.parquetWriter == nil {
		return fmt.Errorf("parquet writer not initialized")
	}
	r.WriteParquetMetric(source, sample)
	return nil
}

// ReadAllMetrics reads all metrics from parquet files and returns them as a slice.
// This is for batch loading scenarios where streaming via handles is not needed.
func (r *recorderImpl) ReadAllMetrics(inputDir string) ([]recorderdef.MetricData, error) {
	// Read all parquet files from the input directory
	reader, err := NewParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating parquet reader: %w", err)
	}

	pkglog.Infof("ReadAllMetrics: loading %d metrics from %s", reader.Len(), inputDir)

	// Pre-allocate slice for efficiency
	metrics := make([]recorderdef.MetricData, 0, reader.Len())

	for {
		metric := reader.Next()
		if metric == nil {
			break
		}

		// Convert FGMMetric to MetricData
		var value float64
		if metric.ValueFloat != nil {
			value = *metric.ValueFloat
		} else if metric.ValueInt != nil {
			value = float64(*metric.ValueInt)
		}

		// Convert tags map to slice
		tags := make([]string, 0, len(metric.Tags))
		for k, v := range metric.Tags {
			if v != "" {
				tags = append(tags, k+":"+v)
			} else {
				tags = append(tags, k)
			}
		}

		// Time is in milliseconds, convert to seconds
		timestamp := metric.Time / 1000

		metrics = append(metrics, recorderdef.MetricData{
			Source:    metric.RunID,
			Name:      metric.MetricName,
			Value:     value,
			Timestamp: timestamp,
			Tags:      tags,
		})
	}

	pkglog.Infof("ReadAllMetrics: loaded %d metrics", len(metrics))
	return metrics, nil
}

func (r *recorderImpl) WriteParquetMetric(source string, sample observer.MetricView) {
	timestamp := int64(sample.GetTimestamp())
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	r.parquetWriter.WriteMetric(
		source,
		sample.GetName(),
		sample.GetValue(),
		sample.GetRawTags(),
		timestamp,
	)
}

// recordingHandle wraps an observer handle to record observations.
type recordingHandle struct {
	inner    observer.Handle
	recorder *recorderImpl
	name     string
}

// ObserveMetric forwards the metric to the inner handle and records it.
func (h *recordingHandle) ObserveMetric(sample observer.MetricView) {
	// Forward to inner handle first
	h.inner.ObserveMetric(sample)

	// Record to parquet if writer is available
	if h.recorder.parquetWriter != nil {
		h.recorder.WriteParquetMetric(h.name, sample)
	}
}

// ObserveLog forwards the log to the inner handle.
// Log recording is not implemented yet but the hook is in place.
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
	h.inner.ObserveLog(msg)
	// TODO: Optionally record logs to parquet (future enhancement)
}
