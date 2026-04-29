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
// reads the complete metric surface from StorageReader (including log-derived
// pattern metrics from the LogMetricsExtractor) and writes it to a JSONL file.
// This captures the exact data Scrappy will consume in production, for use in
// tokenization research and model training.
type ScrappyCollector struct {
	config ScrappyCollectorConfig
	writer *bufio.Writer
	file   *os.File
	mu     sync.Mutex
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
type scrappySeriesSnap struct {
	Namespace string          `json:"ns"`
	Name      string          `json:"name"`
	Tags      []string        `json:"tags"`
	Points    []scrappyPoint  `json:"points"`
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
		CollectorVersion: "0.1",
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
		sr := storage.GetSeriesRange(meta.Ref, dataTime-300, dataTime, observerdef.AggregateNone)
		if sr == nil || len(sr.Points) == 0 {
			continue
		}

		snap := scrappySeriesSnap{
			Namespace: meta.Namespace,
			Name:      meta.Name,
			Tags:      meta.Tags,
			Points:    make([]scrappyPoint, len(sr.Points)),
		}
		for i, p := range sr.Points {
			snap.Points[i] = scrappyPoint{Timestamp: p.Timestamp, Value: p.Value}
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
