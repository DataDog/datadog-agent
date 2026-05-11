// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorderimpl implements the recorder component interface
package recorderimpl

import (
	"errors"
	"fmt"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
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

	if !req.Config.GetBool("anomaly_detection.recording.enabled") {
		pkglog.Debug("Recorder disabled (anomaly_detection.recording.enabled=false)")
		r.recordingDisabled = true
		return Provides{Comp: r}, nil
	}

	parquetDir := req.Config.GetString("anomaly_detection.recording.output_dir")
	if parquetDir == "" {
		return Provides{Comp: r}, errors.New("anomaly_detection.recording.output_dir not set")
	}

	flushInterval := req.Config.GetDuration("anomaly_detection.recording.flush_interval")
	if flushInterval == 0 {
		flushInterval = 60 * time.Second
	}

	retentionDuration := req.Config.GetDuration("anomaly_detection.recording.retention")
	if retentionDuration <= 0 {
		retentionDuration = 24 * time.Hour
	}

	writer, err := newMetricParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		return Provides{Comp: r}, pkglog.Errorf("Failed to create metrics parquet writer: %v", err)
	}
	r.metricParquetWriter = writer
	pkglog.Infof("Recorder metrics writer started: dir=%s", parquetDir)

	logWriter, err := newLogParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		return Provides{Comp: r}, pkglog.Errorf("Failed to create log parquet writer: %v", err)
	}
	r.logParquetWriter = logWriter
	pkglog.Infof("Recorder log writer started: dir=%s", parquetDir)

	return Provides{Comp: r}, nil
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

		// Time is stored in milliseconds, convert to seconds
		timestamp := metric.Time / 1000

		metrics = append(metrics, recorderdef.MetricData{
			Source:    metric.RunID,
			Name:      metric.MetricName,
			Value:     value,
			Timestamp: timestamp,
			Tags:      tags,
			Dropped:   metric.Dropped,
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
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
	h.inner.ObserveLog(msg)

	content := msg.GetContent()
	contentCopy := make([]byte, len(content))
	copy(contentCopy, content)

	h.recorder.logParquetWriter.WriteLog(
		h.name,
		contentCopy,
		msg.GetStatus(),
		msg.GetHostname(),
		msg.GetTags(),
		time.Now().UnixMilli(),
	)
}
