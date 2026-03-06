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

	recordingEnabled := req.Config.GetBool("observer.recording.enabled")

	// --- Raw observation recording ---
	// When enabled, all incoming observations (metrics, logs, traces, profiles, trace stats)
	// are written to parquet files as they arrive via the handle middleware.
	var recordingDir string
	var flushInterval time.Duration
	var retentionDuration time.Duration

	if recordingEnabled {
		recordingDir = req.Config.GetString("observer.recording.parquet_output_dir")
		if recordingDir == "" {
			return Provides{Comp: r}, errors.New("observer.recording.parquet_output_dir not set")
		}

		flushInterval = req.Config.GetDuration("observer.recording.parquet_flush_interval")
		if flushInterval == 0 {
			flushInterval = 60 * time.Second
		}

		retentionDuration = req.Config.GetDuration("observer.recording.parquet_retention")
		if retentionDuration <= 0 {
			retentionDuration = 24 * time.Hour
		}

		writer, err := newMetricParquetWriter(recordingDir, flushInterval, retentionDuration)
		if err != nil {
			return Provides{Comp: r}, pkglog.Errorf("Failed to create metrics parquet writer: %v", err)
		}
		r.metricParquetWriter = writer

		traceWriter, err := newTraceParquetWriter(recordingDir, flushInterval, retentionDuration)
		if err != nil {
			return Provides{Comp: r}, pkglog.Errorf("Failed to create trace parquet writer: %v", err)
		}
		r.traceParquetWriter = traceWriter

		profileWriter, err := newProfileParquetWriter(recordingDir, flushInterval, retentionDuration)
		if err != nil {
			return Provides{Comp: r}, pkglog.Errorf("Failed to create profile parquet writer: %v", err)
		}
		r.profileParquetWriter = profileWriter

		logWriter, err := newLogParquetWriter(recordingDir, flushInterval, retentionDuration)
		if err != nil {
			return Provides{Comp: r}, pkglog.Errorf("Failed to create log parquet writer: %v", err)
		}
		r.logParquetWriter = logWriter

		traceStatsWriter, err := newTraceStatsParquetWriter(recordingDir, flushInterval, retentionDuration)
		if err != nil {
			return Provides{Comp: r}, pkglog.Errorf("Failed to create trace stats parquet writer: %v", err)
		}
		r.traceStatsParquetWriter = traceStatsWriter

		pkglog.Infof("Recorder started: dir=%s flush=%v retention=%v", recordingDir, flushInterval, retentionDuration)
	} else {
		r.recordingDisabled = true
		pkglog.Debug("Recorder disabled (observer.recording.enabled=false)")
	}

	// --- Results saving ---
	// Writes computed intermediate data (virtual metrics from log detectors).
	// Enabled by default when recording is on; can be enabled independently.
	// The output directory defaults to the recording directory when unset.
	resultsEnabled := recordingEnabled || req.Config.GetBool("observer.results.enabled")
	if resultsEnabled {
		resultsDir := req.Config.GetString("observer.results.output_dir")
		if resultsDir == "" {
			resultsDir = recordingDir // falls back to recording dir (may be empty if recording is off)
		}
		if resultsDir == "" {
			pkglog.Warn("observer.results.enabled is true but no output directory configured; set observer.results.output_dir or observer.recording.parquet_output_dir")
		} else {
			// Reuse recording flush settings when available; fall back to sensible defaults.
			resultsFlush := flushInterval
			if resultsFlush == 0 {
				resultsFlush = 60 * time.Second
			}
			resultsRetention := retentionDuration

			resultsWriter, err := newResultsMetricParquetWriter(resultsDir, resultsFlush, resultsRetention)
			if err != nil {
				return Provides{Comp: r}, pkglog.Errorf("Failed to create results metrics parquet writer: %v", err)
			}
			r.resultsMetricParquetWriter = resultsWriter

			resultsLogWriter, err := newResultsLogParquetWriter(resultsDir, resultsFlush, resultsRetention)
			if err != nil {
				return Provides{Comp: r}, pkglog.Errorf("Failed to create results logs parquet writer: %v", err)
			}
			r.resultsLogParquetWriter = resultsLogWriter

			correlationWriter, err := newResultsCorrelationParquetWriter(resultsDir, resultsFlush, resultsRetention)
			if err != nil {
				return Provides{Comp: r}, pkglog.Errorf("Failed to create results correlations parquet writer: %v", err)
			}
			r.resultsCorrelationParquetWriter = correlationWriter

			pkglog.Infof("Results saving enabled: results metrics + logs + correlations → %s", resultsDir)
		}
	}

	return Provides{Comp: r}, nil
}

// recorderImpl implements the recorder component
type recorderImpl struct {
	recordingDisabled               bool
	metricParquetWriter             *metricParquetWriter
	resultsMetricParquetWriter      *metricParquetWriter      // observer-resultsmetrics-*.parquet (virtual + telemetry metrics)
	resultsLogParquetWriter         *logParquetWriter         // observer-resultslogs-*.parquet (telemetry logs)
	resultsCorrelationParquetWriter *correlationParquetWriter // observer-resultscorrelations-*.parquet (correlator output)
	traceParquetWriter              *traceParquetWriter
	profileParquetWriter            *profileParquetWriter
	logParquetWriter                *logParquetWriter
	traceStatsParquetWriter         *traceStatsParquetWriter
}

// GetHandle wraps the provided HandleFunc with recording capability.
func (r *recorderImpl) GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc {
	return func(name string) observer.Handle {
		innerHandle := handleFunc(name)

		if r.recordingDisabled {
			return innerHandle
		}
		pkglog.Infof("recorder: getting handle for %s", name)

		return &recordingHandle{
			inner:    innerHandle,
			recorder: r,
			name:     name,
		}
	}
}

// ReadAllMetrics reads all metrics from parquet files and returns them as a slice.
// This is for batch loading scenarios where streaming via handles is not needed.
func (r *recorderImpl) ReadAllMetrics(inputDir string) ([]recorderdef.MetricData, error) {
	// Read all parquet files from the input directory
	reader, err := newParquetReader(inputDir)
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
	reader, err := newTraceParquetReader(inputDir)
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
	reader, err := newProfileParquetReader(inputDir)
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
	reader, err := newTraceStatsParquetReader(inputDir)
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

// EnableResultsSaving initializes the results writers for headless mode.
// It creates only the results parquet writers (metrics + logs), leaving raw observation
// writers (metrics, logs, traces, profiles) uninitialized so that loaded input data is
// never re-recorded. Safe to call multiple times; subsequent calls are no-ops.
func (r *recorderImpl) EnableResultsSaving(outputDir string) error {
	if r.resultsMetricParquetWriter != nil {
		return nil // already initialized (either via full recording or a previous call)
	}
	// Use a very long flush interval: the testbench drives flushing explicitly via
	// FlushResultsMetrics() / FlushResultsLogs(), so periodic flushing is not needed.
	metricWriter, err := newResultsMetricParquetWriter(outputDir, 365*24*time.Hour, 0)
	if err != nil {
		return fmt.Errorf("creating results metrics parquet writer: %w", err)
	}
	r.resultsMetricParquetWriter = metricWriter

	logWriter, err := newResultsLogParquetWriter(outputDir, 365*24*time.Hour, 0)
	if err != nil {
		return fmt.Errorf("creating results logs parquet writer: %w", err)
	}
	r.resultsLogParquetWriter = logWriter

	corrWriter, err := newResultsCorrelationParquetWriter(outputDir, 365*24*time.Hour, 0)
	if err != nil {
		return fmt.Errorf("creating results correlations parquet writer: %w", err)
	}
	r.resultsCorrelationParquetWriter = corrWriter

	pkglog.Infof("Results saving enabled: metrics + logs + correlations will be written to %s", outputDir)
	return nil
}

// RecordVirtualMetric writes a log-derived virtual metric to the results metrics parquet file.
func (r *recorderImpl) RecordVirtualMetric(source, name string, value float64, tags []string, timestamp int64) {
	if r.resultsMetricParquetWriter == nil {
		return
	}
	r.resultsMetricParquetWriter.WriteMetric(source, name, value, tags, timestamp)
}

// RecordTelemetryMetric writes a detector telemetry metric to the results metrics parquet file.
// Telemetry metrics share the file with virtual metrics (observer-resultsmetrics-*.parquet).
func (r *recorderImpl) RecordTelemetryMetric(source, name string, value float64, tags []string, timestamp int64) {
	if r.resultsMetricParquetWriter == nil {
		return
	}
	r.resultsMetricParquetWriter.WriteMetric(source, name, value, tags, timestamp)
}

// FlushResultsMetrics forces an immediate flush of buffered results metrics to disk.
func (r *recorderImpl) FlushResultsMetrics() {
	if r.resultsMetricParquetWriter == nil {
		return
	}
	r.resultsMetricParquetWriter.flush()
}

// RecordTelemetryLog writes a detector telemetry log to the results logs parquet file.
func (r *recorderImpl) RecordTelemetryLog(source string, content []byte, status, hostname string, tags []string, timestampMs int64) {
	if r.resultsLogParquetWriter == nil {
		return
	}
	r.resultsLogParquetWriter.WriteLog(source, content, status, hostname, tags, timestampMs)
}

// FlushResultsLogs forces an immediate flush of buffered results logs to disk.
func (r *recorderImpl) FlushResultsLogs() {
	if r.resultsLogParquetWriter == nil {
		return
	}
	r.resultsLogParquetWriter.flush()
}

// RecordCorrelation writes a correlator output row to the results correlations parquet file.
func (r *recorderImpl) RecordCorrelation(data recorderdef.CorrelationData) {
	if r.resultsCorrelationParquetWriter == nil {
		return
	}
	r.resultsCorrelationParquetWriter.WriteCorrelation(data)
}

// FlushResultsCorrelations forces an immediate flush of buffered correlation results to disk.
func (r *recorderImpl) FlushResultsCorrelations() {
	if r.resultsCorrelationParquetWriter == nil {
		return
	}
	r.resultsCorrelationParquetWriter.flush()
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
	timestamp := int64(sample.GetTimestamp())
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	h.recorder.metricParquetWriter.WriteMetric(
		h.name,
		sample.GetName(),
		sample.GetValue(),
		sample.GetRawTags(),
		timestamp,
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

// ObserveTraceStats forwards stats to the inner handle and records them to the
// dedicated trace-stats parquet file (not to the metrics parquet file).
func (h *recordingHandle) ObserveTraceStats(stats observer.TraceStatsView) {
	h.inner.ObserveTraceStats(stats)

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

// ObserveProfile forwards the profile to the inner handle and records it.
func (h *recordingHandle) ObserveProfile(profile observer.ProfileView) {
	h.inner.ObserveProfile(profile)

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
