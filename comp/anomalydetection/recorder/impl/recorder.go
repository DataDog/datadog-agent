// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorderimpl implements the recorder component interface
package recorderimpl

import (
	"context"
	"errors"
	"fmt"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the recorder component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
}

// Provides defines the output of the recorder component
type Provides struct {
	Comp recorderdef.Component
}

// NewComponent creates a new recorder component. The returned component is always
// safe to use: misconfiguration is logged and silently disables recording rather
// than failing agent startup, since the recorder is an opt-in observability
// feature that should not bring down the agent it ships in.
func NewComponent(req Requires) Provides {
	r := &recorderImpl{}

	if !req.Config.GetBool("anomaly_detection.recording.enabled") {
		pkglog.Debug("Recorder disabled (anomaly_detection.recording.enabled=false)")
		r.recordingDisabled = true
		return Provides{Comp: r}
	}

	parquetDir := req.Config.GetString("anomaly_detection.recording.output_dir")
	if parquetDir == "" {
		pkglog.Warn("Recorder enabled but anomaly_detection.recording.output_dir is empty; disabling recorder")
		r.recordingDisabled = true
		return Provides{Comp: r}
	}

	flushInterval := req.Config.GetDuration("anomaly_detection.recording.flush_interval")
	if flushInterval <= 0 {
		flushInterval = 60 * time.Second
	}

	retentionDuration := req.Config.GetDuration("anomaly_detection.recording.retention")
	if retentionDuration <= 0 {
		retentionDuration = 24 * time.Hour
	}

	metricWriter, err := newMetricParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		pkglog.Warnf("Failed to create metrics parquet writer, disabling recorder: %v", err)
		r.recordingDisabled = true
		return Provides{Comp: r}
	}

	logWriter, err := newLogParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		// Tear down the metric writer goroutines we just started so they don't leak.
		if closeErr := metricWriter.Close(); closeErr != nil {
			pkglog.Warnf("Failed to close metric writer during recorder init rollback: %v", closeErr)
		}
		pkglog.Warnf("Failed to create log parquet writer, disabling recorder: %v", err)
		r.recordingDisabled = true
		return Provides{Comp: r}
	}

	r.metricParquetWriter = metricWriter
	r.logParquetWriter = logWriter
	pkglog.Infof("Recorder started: dir=%s flush=%v retention=%v", parquetDir, flushInterval, retentionDuration)

	// Register a shutdown hook so the writer goroutines stop and any pending
	// in-memory batch (up to flushInterval of observations) is flushed to disk.
	// Without this, every graceful agent shutdown leaks four goroutines and
	// loses the unflushed batch.
	if req.Lifecycle != nil {
		req.Lifecycle.Append(compdef.Hook{
			OnStop: func(_ context.Context) error {
				return errors.Join(
					r.metricParquetWriter.Close(),
					r.logParquetWriter.Close(),
				)
			},
		})
	}

	return Provides{Comp: r}
}

// recorderImpl implements the recorder component
type recorderImpl struct {
	recordingDisabled   bool
	metricParquetWriter *metricParquetWriter
	logParquetWriter    *logParquetWriter
}

// GetHandle wraps the provided HandleFunc with recording capability.
func (r *recorderImpl) GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc {
	return func(name string) observer.Handle {
		innerHandle := handleFunc(name)

		if r.recordingDisabled {
			return innerHandle
		}
		pkglog.Debugf("recorder: getting handle for %s", name)

		return &recordingHandle{
			inner:    innerHandle,
			recorder: r,
			name:     name,
		}
	}
}

// ReadAllMetrics reads all metrics from parquet files and returns them as a slice.
func (r *recorderImpl) ReadAllMetrics(inputDir string) ([]recorderdef.MetricData, error) {
	reader, err := newParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating parquet reader: %w", err)
	}

	pkglog.Infof("ReadAllMetrics: loading %d metrics from %s", reader.Len(), inputDir)

	metrics := make([]recorderdef.MetricData, 0, reader.Len())

	for {
		metric := reader.Next()
		if metric == nil {
			break
		}

		var value float64
		if metric.ValueFloat != nil {
			value = *metric.ValueFloat
		} else if metric.ValueInt != nil {
			value = float64(*metric.ValueInt)
		}

		tags := make([]string, 0, len(metric.Tags))
		for k, v := range metric.Tags {
			if v != "" {
				tags = append(tags, k+":"+v)
			} else {
				tags = append(tags, k)
			}
		}

		metrics = append(metrics, recorderdef.MetricData{
			Source:      metric.RunID,
			Name:        metric.MetricName,
			Value:       value,
			TimestampMs: metric.Time, // parquet schema stores milliseconds
			Tags:        tags,
			Dropped:     metric.Dropped,
		})
	}

	pkglog.Infof("ReadAllMetrics: loaded %d metrics", len(metrics))
	return metrics, nil
}

// ReadAllLogs reads all logs from parquet files and returns them as a slice.
func (r *recorderImpl) ReadAllLogs(inputDir string) ([]recorderdef.LogData, error) {
	reader, err := NewLogParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating log parquet reader: %w", err)
	}

	pkglog.Infof("ReadAllLogs: loading logs from %s", inputDir)

	logs := reader.ReadAll()

	pkglog.Infof("ReadAllLogs: loaded %d logs", len(logs))
	return logs, nil
}

// metricDropObserver is an optional interface that handles can implement to
// return whether a specific ObserveMetric call was dropped by the live channel.
type metricDropObserver interface {
	ObserveMetricAndReportDrop(sample observer.MetricView) bool
}

// recordingHandle wraps an observer handle to record observations.
type recordingHandle struct {
	inner    observer.Handle
	recorder *recorderImpl
	name     string
}

// ObserveMetric forwards the metric to the inner handle and records it.
func (h *recordingHandle) ObserveMetric(sample observer.MetricView) {
	dropped := false
	if dr, ok := h.inner.(metricDropObserver); ok {
		dropped = dr.ObserveMetricAndReportDrop(sample)
	} else {
		h.inner.ObserveMetric(sample)
	}

	timestamp := sample.GetTimestampUnix()
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	h.recorder.metricParquetWriter.WriteMetric(
		h.name,
		sample.GetName(),
		sample.GetValue(),
		sample.GetRawTags(),
		timestamp,
		dropped,
	)
}

// ObserveLog forwards the log to the inner handle and records it.
// Uses the LogView's own timestamp (event time) rather than wall-clock now
// so that delayed, buffered, or replayed logs are recorded with their
// original time. Wall-clock falls back only when the message reports no
// timestamp (event time == 0), which would otherwise sort to the epoch.
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
	h.inner.ObserveLog(msg)

	content := msg.GetContent()
	contentCopy := make([]byte, len(content))
	copy(contentCopy, content)

	timestampMs := msg.GetTimestampUnixMilli()
	if timestampMs == 0 {
		timestampMs = time.Now().UnixMilli()
	}
	h.recorder.logParquetWriter.WriteLog(
		h.name,
		contentCopy,
		msg.GetStatus(),
		msg.GetHostname(),
		msg.GetTags(),
		timestampMs,
	)
}
