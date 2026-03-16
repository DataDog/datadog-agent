// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// --- Metrics Extractor ---

// // This contains what can identify a pattern
// type PatternKeyInfo struct {
// 	ClusterID int64
// }

type LogPatternExtractor struct {
	PatternClusterer *patterns.PatternClusterer
	// PatternKeys
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)

func NewLogPatternExtractor() *LogPatternExtractor {
	return &LogPatternExtractor{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
	}
}

func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) []observerdef.MetricOutput {
	message := string(log.GetContent())
	// Extract pattern
	clusterResult := e.PatternClusterer.Process(message)
	if clusterResult == nil {
		return nil
	}

	// TODO: Create a pattern key

	// Emit metric for the pattern
	return []observerdef.MetricOutput{{
		Name:  fmt.Sprintf("log.%s.%d.count", e.Name(), clusterResult.Cluster.ID),
		Value: 1,
		Tags:  log.GetTags(),
	}}
}

// --- Anomaly Detector ---

type LogPatternDetector struct {
	MetricsPrefix string
	// The duration of the window to compute rates
	WindowDurationMs int64
}

var _ observerdef.Detector = (*LogPatternDetector)(nil)

func NewLogPatternDetector() *LogPatternDetector {
	// TODO: Method to get prefix to avoid boilerplate
	return &LogPatternDetector{
		MetricsPrefix:    "_virtual.log.log_pattern_extractor",
		WindowDurationMs: 60 * 1000,
	}
}

func (d *LogPatternDetector) Name() string {
	return "log_pattern_detector"
}

func (d *LogPatternDetector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	// fmt.Printf("[cc] Detecting log patterns at %d\n", dataTime)
	telemetry := make([]observerdef.ObserverTelemetry, 0)

	// TODO
	windowStart := dataTime - d.WindowDurationMs

	// Get all series produced by the extractor
	seriesKeys := storage.ListSeries(observerdef.SeriesFilter{
		NamePattern: d.MetricsPrefix,
	})
	for _, seriesKey := range seriesKeys {
		count := storage.PointCountSince(seriesKey, windowStart)
		// if count == 0 {
		// 	continue
		// }
		rate := float64(count) / float64(d.WindowDurationMs)
		// fmt.Printf("[cc] Series %s has rate %f\n", seriesKey.Name, rate)
		// TODO: Check telemetry
		telemetry = append(telemetry, observerdef.ObserverTelemetry{
			Metric: &metricObs{
				name:      seriesKey.Name,
				value:     rate,
				tags:      seriesKey.Tags,
				timestamp: dataTime,
			},
		})
	}

	// fmt.Printf("[cc] Found %d series\n", len(seriesKeys))

	return observerdef.DetectionResult{
		Telemetry: telemetry,
	}
}
