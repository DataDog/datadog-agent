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

	// Check if recording is enabled
	if !req.Config.GetBool("observer.recording.enabled") {
		pkglog.Debug("Recorder disabled (observer.recording.enabled=false)")
		return Provides{Comp: r}, nil
	}

	parquetDir := req.Config.GetString("observer.recording.parquet_output_dir")
	if parquetDir == "" {
		return Provides{Comp: r}, errors.New("observer.recording.parquet_output_dir not set")
	}

	flushInterval := req.Config.GetDuration("observer.recording.parquet_flush_interval")
	if flushInterval == 0 {
		flushInterval = 60 * time.Second
	}

	retentionDuration := req.Config.GetDuration("observer.recording.parquet_retention")
	if retentionDuration <= 0 {
		retentionDuration = 24 * time.Hour
	}

	// Initialize metrics writer (always enabled when recording is on)
	writer, err := NewParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		pkglog.Errorf("Failed to create metrics parquet writer: %v", err)
	} else {
		r.parquetWriter = writer
		pkglog.Infof("Recorder metrics writer started: dir=%s", parquetDir)
	}

	// Initialize traces writer (always enabled when recording is on)
	traceWriter, err := NewTraceParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		pkglog.Errorf("Failed to create trace parquet writer: %v", err)
	} else {
		r.traceParquetWriter = traceWriter
		pkglog.Infof("Recorder trace writer started: dir=%s", parquetDir)
	}

	// Initialize profiles writer (always enabled when recording is on)
	profileWriter, err := NewProfileParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		pkglog.Errorf("Failed to create profile parquet writer: %v", err)
	} else {
		r.profileParquetWriter = profileWriter
		pkglog.Infof("Recorder profile writer started: dir=%s", parquetDir)
	}

	// Initialize logs writer (always enabled when recording is on)
	logWriter, err := NewLogParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		pkglog.Errorf("Failed to create log parquet writer: %v", err)
	} else {
		r.logParquetWriter = logWriter
		pkglog.Infof("Recorder log writer started: dir=%s", parquetDir)
	}

	// Initialize trace stats writer (always enabled when recording is on)
	traceStatsWriter, err := NewTraceStatsParquetWriter(parquetDir, flushInterval, retentionDuration)
	if err != nil {
		pkglog.Errorf("Failed to create trace stats parquet writer: %v", err)
	} else {
		r.traceStatsParquetWriter = traceStatsWriter
		pkglog.Infof("Recorder trace stats writer started: dir=%s", parquetDir)
	}

	return Provides{Comp: r}, nil
}

// recorderImpl implements the recorder component
type recorderImpl struct {
	parquetWriter           *ParquetWriter
	traceParquetWriter      *TraceParquetWriter
	profileParquetWriter    *ProfileParquetWriter
	logParquetWriter        *LogParquetWriter
	traceStatsParquetWriter *TraceStatsParquetWriter
}

// GetHandle wraps the provided HandleFunc with recording capability.
func (r *recorderImpl) GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc {
	return func(name string) observer.Handle {
		innerHandle := handleFunc(name)

		// If any recording is enabled, wrap with recording handle
		if r.parquetWriter != nil || r.traceParquetWriter != nil || r.profileParquetWriter != nil || r.logParquetWriter != nil || r.traceStatsParquetWriter != nil {
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

// ReadAllTraces reads all traces from parquet files and returns them as a slice.
// Traces are reconstructed from denormalized span rows grouped by trace ID.
func (r *recorderImpl) ReadAllTraces(inputDir string) ([]recorderdef.TraceData, error) {
	reader, err := NewTraceParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating trace parquet reader: %w", err)
	}

	pkglog.Infof("ReadAllTraces: loading traces from %s", inputDir)

	traces := reader.ReadAll()

	pkglog.Infof("ReadAllTraces: loaded %d traces", len(traces))
	return traces, nil
}

// ReadAllProfiles reads all profiles from parquet files and returns them as a slice.
func (r *recorderImpl) ReadAllProfiles(inputDir string) ([]recorderdef.ProfileData, error) {
	reader, err := NewProfileParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating profile parquet reader: %w", err)
	}

	pkglog.Infof("ReadAllProfiles: loading profiles from %s", inputDir)

	profiles := reader.ReadAll()

	pkglog.Infof("ReadAllProfiles: loaded %d profiles", len(profiles))
	return profiles, nil
}

// ReadAllTraceStats reads all APM trace stats from parquet files and returns them as a slice.
func (r *recorderImpl) ReadAllTraceStats(inputDir string) ([]recorderdef.TraceStatsData, error) {
	reader, err := NewTraceStatsParquetReader(inputDir)
	if err != nil {
		return nil, fmt.Errorf("creating trace stats parquet reader: %w", err)
	}

	pkglog.Infof("ReadAllTraceStats: loading trace stats from %s", inputDir)

	stats := reader.ReadAll()

	pkglog.Infof("ReadAllTraceStats: loaded %d stat rows", len(stats))
	return stats, nil
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
		timestamp := int64(sample.GetTimestamp())
		if timestamp == 0 {
			timestamp = time.Now().Unix()
		}
		h.recorder.parquetWriter.WriteMetric(
			h.name,
			sample.GetName(),
			sample.GetValue(),
			sample.GetRawTags(),
			timestamp,
		)
	}
}

// ObserveLog forwards the log to the inner handle and records it.
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
	h.inner.ObserveLog(msg)

	// Record to parquet if writer is available
	if h.recorder.logParquetWriter != nil {
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
}

// ObserveTraceStats forwards stats to the inner handle and records them to the
// dedicated trace-stats parquet file (not to the metrics parquet file).
func (h *recordingHandle) ObserveTraceStats(stats observer.TraceStatsView) {
	h.inner.ObserveTraceStats(stats)

	if h.recorder.traceStatsParquetWriter == nil {
		return
	}

	agentHostname := stats.GetAgentHostname()
	agentEnv := stats.GetAgentEnv()
	rows := stats.GetRows()
	for rows.Next() {
		row := rows.Row()
		h.recorder.traceStatsParquetWriter.WriteStatRow(
			h.name,
			agentHostname, agentEnv,
			row.GetClientHostname(), row.GetClientEnv(), row.GetClientVersion(), row.GetClientContainerID(),
			row.GetBucketStart(), row.GetBucketDuration(),
			row.GetService(), row.GetName(), row.GetResource(), row.GetType(),
			row.GetHTTPStatusCode(), row.GetSpanKind(), row.GetIsTraceRoot(), row.GetSynthetics(),
			row.GetHits(), row.GetErrors(), row.GetTopLevelHits(), row.GetDuration(),
			row.GetOkSummary(), row.GetErrorSummary(),
			row.GetPeerTags(),
		)
	}
}

// ObserveTrace forwards the trace to the inner handle and records it.
func (h *recordingHandle) ObserveTrace(trace observer.TraceView) {
	h.inner.ObserveTrace(trace)

	// Record to parquet if writer is available
	if h.recorder.traceParquetWriter != nil {
		traceIDHigh, traceIDLow := trace.GetTraceID()
		traceService := trace.GetService()
		traceTags := mapToTagSlice(trace.GetTags())

		// Iterate over all spans and write each one with trace context
		iter := trace.GetSpans()
		for iter.Next() {
			span := iter.Span()
			h.recorder.traceParquetWriter.WriteSpan(
				h.name,
				traceIDHigh, traceIDLow,
				trace.GetEnv(), traceService, trace.GetHostname(), trace.GetContainerID(),
				trace.GetTimestamp(), trace.GetDuration(),
				trace.GetPriority(), trace.IsError(), traceTags,
				span.GetSpanID(), span.GetParentID(),
				span.GetService(), span.GetName(), span.GetResource(), span.GetType(),
				span.GetStart(), span.GetDuration(), span.GetError(),
				mapToTagSlice(span.GetMeta()), mapToMetricSlice(span.GetMetrics()),
			)
		}
	}
}

// ObserveProfile forwards the profile to the inner handle and records it.
func (h *recordingHandle) ObserveProfile(profile observer.ProfileView) {
	h.inner.ObserveProfile(profile)

	// Record to parquet if writer is available
	if h.recorder.profileParquetWriter != nil {
		h.recorder.profileParquetWriter.WriteProfile(
			h.name,
			profile.GetProfileID(), profile.GetProfileType(),
			profile.GetService(), profile.GetEnv(), profile.GetVersion(),
			profile.GetHostname(), profile.GetContainerID(),
			profile.GetTimestamp(), profile.GetDuration(),
			profile.GetContentType(),
			profile.GetRawData(),
			mapToTagSlice(profile.GetTags()),
		)
	}
}

// mapToTagSlice converts a map to a slice of "key:value" strings.
func mapToTagSlice(m map[string]string) []string {
	if m == nil {
		return nil
	}
	result := make([]string, 0, len(m))
	for k, v := range m {
		if v != "" {
			result = append(result, k+":"+v)
		} else {
			result = append(result, k)
		}
	}
	return result
}

// mapToMetricSlice converts a float64 map to a slice of "key:value" strings.
func mapToMetricSlice(m map[string]float64) []string {
	if m == nil {
		return nil
	}
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s:%g", k, v))
	}
	return result
}
