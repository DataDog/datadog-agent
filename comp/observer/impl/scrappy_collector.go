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

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ScrappyCollectorConfig holds configuration for the Scrappy data collector.
type ScrappyCollectorConfig struct {
	OutputPath string `json:"output_path"`
}

// DefaultScrappyCollectorConfig returns the default config.
func DefaultScrappyCollectorConfig() ScrappyCollectorConfig {
	return ScrappyCollectorConfig{
		OutputPath: "/tmp/scrappy-collect.jsonl",
	}
}

// readScrappyCollectorConfig reads config from the agent config system.
func readScrappyCollectorConfig(reader ConfigReader, prefix string) any {
	cfg := DefaultScrappyCollectorConfig()
	if key := prefix + "output_path"; reader.IsKnown(key) {
		cfg.OutputPath = reader.GetString(key)
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
	config           ScrappyCollectorConfig
	writer           *bufio.Writer
	file             *os.File
	mu               sync.Mutex
	contextProviders map[string]observerdef.ContextProvider
}

// scrappyHeader is the first line written to the output file.
type scrappyHeader struct {
	Type             string `json:"type"`
	StartTS          string `json:"start_ts"`
	CollectorVersion string `json:"collector_version"`
}

// scrappyTick is one line of output — the complete metric surface at one tick.
type scrappyTick struct {
	DataTime int64                `json:"data_time"`
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

// SetContextProviders sets the context providers used to resolve log pattern
// hashes to their pattern text. Called by the observer after component
// instantiation, before the first Detect().
func (s *ScrappyCollector) SetContextProviders(providers map[string]observerdef.ContextProvider) {
	s.contextProviders = providers
}

// Name implements observerdef.Detector.
func (s *ScrappyCollector) Name() string { return "scrappy_collector" }

// Detect implements observerdef.Detector. It reads every series in storage and
// writes the complete metric surface for this tick to the output file.
func (s *ScrappyCollector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	if s.writer == nil {
		return observerdef.DetectionResult{}
	}

	allSeries := storage.ListSeries(observerdef.WorkloadSeriesFilter())

	tick := scrappyTick{
		DataTime: dataTime,
		Series:   make([]scrappySeriesSnap, 0, len(allSeries)),
	}

	for _, meta := range allSeries {
		sr := storage.GetSeriesRange(meta.Ref, dataTime-300, dataTime, observerdef.AggregateAverage)
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

		// Log metrics: resolve hash to Drain pattern text. Non-log metrics: use name.
		if isLogMetric(meta.Name) {
			snap.Pattern = s.resolvePattern(meta.Name, meta.Tags)
			if snap.Pattern == "" {
				snap.Pattern = meta.Name // fallback to hash if resolution fails
			}
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

// resolvePattern looks up the pattern text for a log pattern metric.
func (s *ScrappyCollector) resolvePattern(metricName string, tags []string) string {
	if len(s.contextProviders) == 0 {
		return ""
	}
	// The engine adds observer_source: tags after ProcessLog runs, so the
	// context key stored by the extractor uses tags WITHOUT observer_source.
	// Strip it to match.
	cleanTags := make([]string, 0, len(tags))
	for _, t := range tags {
		if !strings.HasPrefix(t, "observer_source:") {
			cleanTags = append(cleanTags, t)
		}
	}
	contextKey := seriesKey("", metricName, cleanTags)
	for _, provider := range s.contextProviders {
		ctx, ok := provider.GetContextByKey(contextKey)
		if ok {
			return ctx.Pattern
		}
	}
	return ""
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
