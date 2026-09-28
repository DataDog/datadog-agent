// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetection_recorder

// Package recorderimpl implements the recorder component interface.
package recorderimpl

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the recorder component.
type Requires struct {
	Config config.Component
}

// Provides defines the output of the recorder component.
type Provides struct {
	Comp option.Option[recorderdef.Component]
}

// NewComponent creates a new recorder component.
func NewComponent(req Requires) (Provides, error) {
	if !req.Config.GetBool("anomaly_detection.recording.enabled") {
		logging.Debug("recorder disabled (anomaly_detection.recording.enabled=false)")
		return Provides{Comp: option.None[recorderdef.Component]()}, nil
	}

	r := &recorderImpl{}
	parquetDir := req.Config.GetString("anomaly_detection.recording.output_dir")
	if parquetDir == "" {
		return Provides{}, errors.New("anomaly_detection.recording.output_dir not set")
	}

	flushInterval := time.Duration(req.Config.GetInt("anomaly_detection.recording.flush_interval")) * time.Second
	if flushInterval == 0 {
		flushInterval = 60 * time.Second
	}

	retentionDuration := req.Config.GetDuration("anomaly_detection.recording.retention")
	if retentionDuration <= 0 {
		retentionDuration = 24 * time.Hour
	}

	writer, err := newMetricParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		return Provides{}, fmt.Errorf("creating metrics parquet writer: %w", err)
	}
	r.metricParquetWriter = writer
	logging.Infof("recorder metrics writer started: dir=%s", parquetDir)

	logWriter, err := newLogParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		return Provides{}, fmt.Errorf("creating log parquet writer: %w", err)
	}
	r.logParquetWriter = logWriter
	logging.Infof("recorder log writer started: dir=%s", parquetDir)

	return Provides{Comp: option.New[recorderdef.Component](r)}, nil
}

type recorderImpl struct {
	metricParquetWriter *metricParquetWriter
	logParquetWriter    *logParquetWriter
}

// GetHandle wraps the provided HandleFunc with recording capability.
func (r *recorderImpl) GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc {
	return func(name string) observer.Handle {
		innerHandle := handleFunc(name)
		logging.Infof("recorder: getting handle for %s", name)
		return &recordingHandle{inner: innerHandle, recorder: r, name: name}
	}
}

// ReadAllMetrics reads all metrics from parquet files and returns them as a slice.
func (r *recorderImpl) ReadAllMetrics(inputDir string) ([]recorderdef.MetricData, error) {
	reader, err := newParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating parquet reader: %w", err)
	}

	logging.Infof("ReadAllMetrics: loading %d metrics from %s", reader.Len(), inputDir)
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
			Source:    metric.RunID,
			Name:      metric.MetricName,
			Value:     value,
			Timestamp: metric.Time / 1000,
			Tags:      tags,
			Dropped:   metric.Dropped,
		})
	}
	logging.Infof("ReadAllMetrics: loaded %d metrics", len(metrics))
	return metrics, nil
}

// ReadAllLogs reads all logs from parquet files and returns them as a slice.
func (r *recorderImpl) ReadAllLogs(inputDir string) ([]recorderdef.LogData, error) {
	reader, err := NewLogParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating log parquet reader: %w", err)
	}
	logging.Infof("ReadAllLogs: loading logs from %s", inputDir)
	logs := reader.ReadAll()
	logging.Infof("ReadAllLogs: loaded %d logs", len(logs))
	return logs, nil
}

// metricDropObserver reports whether a specific call was dropped by the live channel.
type metricDropObserver interface {
	ObserveMetricAndReportDrop(sample observer.MetricView) bool
}

type recordingHandle struct {
	inner    observer.Handle
	recorder *recorderImpl
	name     string
}

// ObserveMetric forwards the metric, then records the observation and drop result.
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
	tags := append([]string(nil), sample.GetTags().UnsafeToReadOnlySliceString()...)
	if host := sample.GetHost(); host != "" && !slices.Contains(tags, "host:"+host) {
		tags = append(tags, "host:"+host)
	}
	h.recorder.metricParquetWriter.WriteMetric(
		h.name,
		sample.GetName(),
		sample.GetValue(),
		tags,
		timestamp,
		dropped,
	)
}

// ObserveLog forwards the log, then records its contents and ingestion timestamp.
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
	h.inner.ObserveLog(msg)

	timestampMs := msg.GetTimestampUnixMilli()
	if timestampMs == 0 {
		timestampMs = time.Now().UnixMilli()
	}
	h.recorder.logParquetWriter.WriteLog(
		h.name,
		[]byte(msg.GetContent()),
		msg.GetStatus(),
		msg.GetHostname(),
		msg.Tags(),
		timestampMs,
	)
}
