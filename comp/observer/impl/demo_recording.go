// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// MetricEntry represents a single metric entry for JSON recording.
type MetricEntry struct {
	Timestamp int64    `json:"timestamp"`
	Name      string   `json:"name"`
	Value     float64  `json:"value"`
	Tags      []string `json:"tags,omitempty"`
	Source    string   `json:"source,omitempty"`
}

// RecordingConfig configures the demo recording.
type RecordingConfig struct {
	OutputDir string        // Directory to write recorded files
	TimeScale float64       // Time scale for demo (1.0 = realtime)
	Duration  time.Duration // How long to record (0 = full demo duration)
}

// RecordingHandle wraps an observer.Handle to record metrics and logs to files.
type RecordingHandle struct {
	inner        observer.Handle
	source       string
	metricWriter *json.Encoder
	metricFile   *os.File
	logWriter    *json.Encoder
	logFile      *os.File
	mu           sync.Mutex
}

// NewRecordingHandle creates a handle that records to the specified output directory.
func NewRecordingHandle(inner observer.Handle, source string, outputDir string) (*RecordingHandle, error) {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Create metrics file
	metricsPath := filepath.Join(outputDir, "metrics.json")
	metricFile, err := os.Create(metricsPath)
	if err != nil {
		return nil, fmt.Errorf("creating metrics file: %w", err)
	}

	// Create logs file (JSONL using recorder's LogData format)
	logsPath := filepath.Join(outputDir, "logs.json")
	logFile, err := os.Create(logsPath)
	if err != nil {
		metricFile.Close()
		return nil, fmt.Errorf("creating log file: %w", err)
	}

	return &RecordingHandle{
		inner:        inner,
		source:       source,
		metricWriter: json.NewEncoder(metricFile),
		metricFile:   metricFile,
		logWriter:    json.NewEncoder(logFile),
		logFile:      logFile,
	}, nil
}

// ObserveMetric forwards the metric to the inner handle and records it.
func (h *RecordingHandle) ObserveMetric(sample observer.MetricView) {
	// Forward to inner handle
	h.inner.ObserveMetric(sample)

	// Record to file
	h.mu.Lock()
	defer h.mu.Unlock()

	entry := MetricEntry{
		Timestamp: int64(sample.GetTimestamp()),
		Name:      sample.GetName(),
		Value:     sample.GetValue(),
		Tags:      sample.GetRawTags(),
		Source:    h.source,
	}

	_ = h.metricWriter.Encode(entry)
}

// ObserveLog forwards the log to the inner handle and records it.
func (h *RecordingHandle) ObserveLog(msg observer.LogView) {
	// Forward to inner handle
	h.inner.ObserveLog(msg)

	// Record to file using recorder's LogData format
	h.mu.Lock()
	defer h.mu.Unlock()

	entry := recorderdef.LogData{
		Timestamp: msg.GetTimestamp(),
		Content:   string(msg.GetContent()),
		Status:    msg.GetStatus(),
		Hostname:  msg.GetHostname(),
		Source:    h.source,
		Tags:      msg.GetTags(),
	}

	_ = h.logWriter.Encode(entry)
}

// Close closes the recording handle and flushes all data.
func (h *RecordingHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var errs []error

	if h.metricFile != nil {
		if err := h.metricFile.Sync(); err != nil {
			errs = append(errs, err)
		}
		if err := h.metricFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if h.logFile != nil {
		if err := h.logFile.Sync(); err != nil {
			errs = append(errs, err)
		}
		if err := h.logFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing recording handle: %v", errs)
	}
	return nil
}

// RunDemoWithRecording runs the demo scenario and records metrics + logs to the output directory.
func RunDemoWithRecording(config RecordingConfig) error {
	if config.TimeScale <= 0 {
		config.TimeScale = 0.1 // Default: 10x speed
	}

	fmt.Printf("Recording demo to: %s\n", config.OutputDir)
	fmt.Printf("TimeScale: %.2f\n", config.TimeScale)

	// Create observer components
	correlator := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 1,
		MinClusterSize:   2,
		WindowSeconds:    60,
	})

	storage := newTimeSeriesStorage()

	obs := &observerImpl{
		logProcessors: []observer.LogProcessor{
			&LogTimeSeriesAnalysis{
				MaxEvalBytes: 4096,
				ExcludeFields: map[string]struct{}{
					"timestamp": {}, "ts": {}, "time": {},
					"pid": {}, "ppid": {}, "uid": {}, "gid": {},
				},
			},
		},
		tsAnalyses: []observer.TimeSeriesAnalysis{
			NewCUSUMDetector(),
		},
		anomalyProcessors: []observer.AnomalyProcessor{
			correlator,
		},
		reporters:        []observer.Reporter{},
		storage:          storage,
		obsCh:            make(chan observation, 1000),
		rawAnomalyWindow: 120,
	}
	go obs.run()

	// Get inner handle
	innerHandle := obs.GetHandle("demo")

	// Wrap with recording handle
	recordingHandle, err := NewRecordingHandle(innerHandle, "demo", config.OutputDir)
	if err != nil {
		return fmt.Errorf("creating recording handle: %w", err)
	}
	defer recordingHandle.Close()

	// Create and run demo generator with recording handle
	generator := NewDataGenerator(recordingHandle, GeneratorConfig{
		TimeScale:     config.TimeScale,
		BaselineNoise: 0.1,
	})

	// Calculate duration
	duration := config.Duration
	if duration == 0 {
		duration = time.Duration(float64(phaseTotalDuration)*float64(time.Second)*config.TimeScale) + time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	fmt.Printf("Running demo for %.1fs...\n", duration.Seconds())
	generator.Run(ctx)

	// Small buffer to let final events flush
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("Recording complete. Files written to: %s\n", config.OutputDir)
	return nil
}
