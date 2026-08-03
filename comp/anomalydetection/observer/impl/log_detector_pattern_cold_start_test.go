// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shortColdStartConfig() LogPatternColdStartConfig {
	return LogPatternColdStartConfig{
		HealthyHistorySeconds:     20,
		MinOccurrences:            3,
		OccurrenceWindowSeconds:   10,
		SourceHealthMaxGapSeconds: 6,
		PatternTimeToLiveSeconds:  60,
		MaxPatternsPerSource:      16,
		MaxSources:                8,
	}
}

func observeHealthySource(detector *LogPatternColdStartDetector, sourceID string, timestamps ...int64) {
	for _, timestamp := range timestamps {
		detector.ObserveLogSourceHealth(observerdef.LogSourceHealthObservation{
			SourceID:  sourceID,
			Timestamp: timestamp,
			Healthy:   true,
		})
	}
}

func errorPattern(sourceID string, timestamp int64) observerdef.LogPatternObservation {
	return observerdef.LogPatternObservation{
		SourceID:   sourceID,
		Extractor:  LogPatternExtractorName,
		MetricName: "log.log_pattern_extractor.abc.count",
		Pattern:    "request failed status <*>",
		Example:    "request failed status 503",
		Timestamp:  timestamp,
		Tags:       []string{"service:api"},
		IsError:    true,
	}
}

func TestLogPatternColdStartDetectsOneShotOnset(t *testing.T) {
	detector := NewLogPatternColdStartDetector(shortColdStartConfig())
	observeHealthySource(detector, "source-a", 0, 5, 10, 15, 20, 25, 30)
	for _, timestamp := range []int64{21, 24, 29} {
		detector.ObserveLogPattern(errorPattern("source-a", timestamp))
	}

	result := detector.Detect(nil, 30)
	require.Len(t, result.Anomalies, 1)
	anomaly := result.Anomalies[0]
	assert.Equal(t, LogPatternColdStartDetectorName, anomaly.DetectorName)
	assert.Equal(t, observerdef.AnomalyTypeLog, anomaly.Type)
	assert.Equal(t, int64(29), anomaly.Timestamp)
	assert.Equal(t, "log.log_pattern_extractor.abc.count", anomaly.Source.Name)
	assert.Contains(t, anomaly.Description, "continuously healthy for 20s")
	assert.Empty(t, detector.Detect(nil, 40).Anomalies)
}

func TestLogPatternColdStartRejectsMissingHealthContinuity(t *testing.T) {
	detector := NewLogPatternColdStartDetector(shortColdStartConfig())
	// The 10-second sampling gap exceeds the configured six-second allowance,
	// so the healthy interval restarts at t=15.
	observeHealthySource(detector, "source-a", 0, 5, 15, 20, 25, 30)
	for _, timestamp := range []int64{21, 22, 23} {
		detector.ObserveLogPattern(errorPattern("source-a", timestamp))
	}

	assert.Empty(t, detector.Detect(nil, 30).Anomalies)
}

func TestLogPatternColdStartRejectsSlowPattern(t *testing.T) {
	detector := NewLogPatternColdStartDetector(shortColdStartConfig())
	observeHealthySource(detector, "source-a", 0, 5, 10, 15, 20, 25, 30, 35)
	for _, timestamp := range []int64{21, 28, 32} {
		detector.ObserveLogPattern(errorPattern("source-a", timestamp))
	}

	assert.Empty(t, detector.Detect(nil, 35).Anomalies)
}

func TestEngineColdStartUsesSemanticExtractorBeforeMetricWarmup(t *testing.T) {
	detector := NewLogPatternColdStartDetector(shortColdStartConfig())
	extractor := NewLogPatternExtractor(LogPatternExtractorConfig{MinClusterSizeBeforeEmit: 5})
	engine := newEngine(engineConfig{
		storage:    newTimeSeriesStorageWith(StorageConfig{PointRetentionSecs: 0}),
		detectors:  []observerdef.Detector{detector},
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})
	for _, timestamp := range []int64{0, 5, 10, 15, 20, 25, 30} {
		engine.ObserveLogSourceHealth(observerdef.LogSourceHealthObservation{
			SourceID:  "source-a",
			Timestamp: timestamp,
			Healthy:   true,
		})
	}
	for _, timestamp := range []int64{21, 24, 29} {
		engine.IngestLog("logs", &logObs{
			content:     "request failed status 503",
			status:      "error",
			tags:        []string{"source:checkout", "service:api"},
			timestampMs: timestamp * 1000,
			sourceID:    "source-a",
		})
	}

	result := engine.Advance(30)
	require.Len(t, result.anomalies, 1)
	assert.Equal(t, LogPatternColdStartDetectorName, result.anomalies[0].DetectorName)
	assert.Zero(t, engine.storage.TotalSeriesCount(""), "cold start must not backfill a metric series")
}
