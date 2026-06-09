// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// ScrappyCollectorConfig holds configuration for the Scrappy data collector.
type ScrappyCollectorConfig struct {
	OutputPath string `json:"output_path"`
	// TickWindow controls how many seconds of data are aggregated into each
	// output tick. Default 1 (every Detect() call emits a tick). Set to 60
	// to aggregate 60 seconds of metrics into a single tick — producing ~30-50
	// ticks per episode instead of ~2000, which matches the production agent's
	// 60-second detection window.
	TickWindow int64 `json:"tick_window"`
}

// DefaultScrappyCollectorConfig returns the default config.
func DefaultScrappyCollectorConfig() ScrappyCollectorConfig {
	return ScrappyCollectorConfig{
		OutputPath: "/tmp/scrappy-collect.jsonl",
		TickWindow: 1,
	}
}

// readScrappyCollectorConfig reads config from the agent config system.
func readScrappyCollectorConfig(reader ConfigReader, prefix string) any {
	cfg := DefaultScrappyCollectorConfig()
	if key := prefix + "output_path"; reader.IsConfigured(key) {
		cfg.OutputPath = reader.GetString(key)
	}
	if key := prefix + "tick_window"; reader.IsConfigured(key) {
		cfg.TickWindow = int64(reader.GetInt(key))
	}
	return cfg
}

// ScrappyCollector implements observerdef.Detector. On every Detect() tick it
// reads the complete metric surface from StorageReader and writes it to a JSONL
// file for tokenization research and model training.
//
// The output schema is tokenizer-oriented:
//   - Log metrics use "pattern" (the normalized log signature) instead of the
//     opaque hash-based metric name.
//   - Non-log metrics use "name" (the metric name as-is).
//   - Both carry ns (namespace/source), tags, and points.
type ScrappyCollector struct {
	config              ScrappyCollectorConfig
	writer              *bufio.Writer
	file                *os.File
	mu                  sync.Mutex
	lastEmittedDataTime int64
}

// scrappyHeader is the first line written to the output file.
type scrappyHeader struct {
	Type             string `json:"type"`
	StartTS          string `json:"start_ts"`
	CollectorVersion string `json:"collector_version"`
}

// scrappyTick is one line of output — the complete metric surface at one tick.
type scrappyTick struct {
	DataTime int64               `json:"data_time"`
	Series   []scrappySeriesSnap `json:"series"`
}

// scrappySeriesSnap is one series within a tick snapshot.
// For log metrics, Name is empty and Pattern carries the normalized log signature.
// For non-log metrics, Pattern is empty and Name carries the metric name.
type scrappySeriesSnap struct {
	Namespace string         `json:"ns"`
	Name      string         `json:"name,omitempty"`
	Pattern   string         `json:"pattern,omitempty"`
	Tags      []string       `json:"tags"`
	Points    []scrappyPoint `json:"points"`
}

// scrappyPoint is a single data point.
type scrappyPoint struct {
	Timestamp int64   `json:"ts"`
	Value     float64 `json:"val"`
}

// NewScrappyCollector creates a collector that writes to the configured output path.
func NewScrappyCollector(config ScrappyCollectorConfig) *ScrappyCollector {
	f, err := os.Create(config.OutputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrappy_collector: failed to create output file %s: %v\n", config.OutputPath, err)
		return &ScrappyCollector{config: config}
	}

	w := bufio.NewWriterSize(f, 256*1024) // 256KB write buffer

	header := scrappyHeader{
		Type:             "header",
		StartTS:          time.Now().UTC().Format(time.RFC3339),
		CollectorVersion: "0.4",
	}
	if b, err := json.Marshal(header); err == nil {
		w.Write(b)
		w.WriteByte('\n')
		w.Flush()
	}

	return &ScrappyCollector{
		config: config,
		writer: w,
		file:   f,
	}
}

// Name implements observerdef.Detector.
func (s *ScrappyCollector) Name() string { return "scrappy_collector" }

// Detect implements observerdef.Detector. It reads every series in storage and
// writes the complete metric surface for this tick to the output file.
//
// When TickWindow > 1, Detect() skips calls until TickWindow seconds have
// elapsed since the last emitted tick, then reads the full window of data
// aggregated with AggregateAverage. This produces one output tick per window
// instead of one per second.
func (s *ScrappyCollector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	if s.writer == nil {
		return observerdef.DetectionResult{}
	}

	window := s.config.TickWindow
	if window < 1 {
		window = 1
	}
	if s.lastEmittedDataTime > 0 && dataTime-s.lastEmittedDataTime < window {
		return observerdef.DetectionResult{}
	}
	s.lastEmittedDataTime = dataTime

	rangeStart := dataTime - window
	allSeries := storage.ListSeries(observerdef.WorkloadSeriesFilter())

	tick := scrappyTick{
		DataTime: dataTime,
		Series:   make([]scrappySeriesSnap, 0, len(allSeries)),
	}

	for _, meta := range allSeries {
		sr := storage.GetSeriesRange(meta.Ref, rangeStart, dataTime, observerdef.AggregateAverage)
		if sr == nil || len(sr.Points) == 0 {
			continue
		}

		snap := scrappySeriesSnap{
			Namespace: meta.Namespace,
			Tags:      meta.Tags,
			Points:    make([]scrappyPoint, len(sr.Points)),
		}
		for i, p := range sr.Points {
			snap.Points[i] = scrappyPoint{Timestamp: p.Timestamp, Value: p.Value}
		}

		// Log metrics: resolve hash to Drain pattern text via stored MetricContext.
		// Non-log metrics: use name.
		if isLogMetric(meta.Name) {
			snap.Pattern = resolvePatternFromStorage(storage, meta.Ref, meta.Name)
		} else {
			snap.Name = meta.Name
		}

		tick.Series = append(tick.Series, snap)
	}

	b, err := json.Marshal(tick)
	if err != nil {
		return observerdef.DetectionResult{}
	}

	s.mu.Lock()
	s.writer.Write(b)
	s.writer.WriteByte('\n')
	s.writer.Flush()
	s.mu.Unlock()

	return observerdef.DetectionResult{}
}

// isLogMetric returns true for metrics produced by log extractors.
// log_metrics_extractor emits "log.pattern.<hash>.count"
// log_pattern_extractor emits "log.log_pattern_extractor.<hash>.count"
func isLogMetric(name string) bool {
	return strings.HasPrefix(name, "log.pattern.") || strings.HasPrefix(name, "log.log_pattern_extractor.")
}

// resolvePatternFromStorage retrieves the Drain pattern text for a log metric
// from the MetricContext stored on the series in storage.
// Falls back to the raw metric name if no context is available.
func resolvePatternFromStorage(storage observerdef.StorageReader, ref observerdef.SeriesRef, metricName string) string {
	ctx := storage.GetContext(ref)
	if ctx != nil && ctx.Pattern != "" {
		return ctx.Pattern
	}
	return metricName
}

// Close flushes and closes the output file.
func (s *ScrappyCollector) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer != nil {
		s.writer.Flush()
	}
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}
