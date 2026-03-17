// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- LogPatternExtractor ---

func TestLogPatternExtractor_Name(t *testing.T) {
	e := NewLogPatternExtractor()
	assert.Equal(t, "log_pattern_extractor", e.Name())
}

func TestLogPatternExtractor_Setup(t *testing.T) {
	e := NewLogPatternExtractor()
	assert.NoError(t, e.Setup(nil))
}

func TestLogPatternExtractor_EmitsCountMetric(t *testing.T) {
	e := NewLogPatternExtractor()
	log := &mockLogView{
		content: []byte("Connection refused by server"),
		tags:    []string{"env:prod"},
	}

	result := e.ProcessLog(log)

	require.Len(t, result, 1)
	assert.True(t, strings.HasPrefix(result[0].Name, "log.log_pattern_extractor."), "metric name should have correct prefix")
	assert.True(t, strings.HasSuffix(result[0].Name, ".count"), "metric name should end with .count")
	assert.Equal(t, float64(1), result[0].Value)
	assert.Equal(t, []string{"env:prod"}, result[0].Tags)
}

func TestLogPatternExtractor_SameMessageSameMetricName(t *testing.T) {
	e := NewLogPatternExtractor()
	log1 := &mockLogView{content: []byte("Connection refused by server"), tags: []string{"env:prod"}}
	log2 := &mockLogView{content: []byte("Connection refused by server"), tags: []string{"env:staging"}}

	r1 := e.ProcessLog(log1)
	r2 := e.ProcessLog(log2)

	require.Len(t, r1, 1)
	require.Len(t, r2, 1)
	assert.Equal(t, r1[0].Name, r2[0].Name, "identical messages should produce the same metric name")
}

func TestLogPatternExtractor_SimilarMessagesSharePattern(t *testing.T) {
	// Messages that differ only in variable parts (numbers, IPs) should cluster
	// into the same pattern and thus the same metric name.
	e := NewLogPatternExtractor()
	log1 := &mockLogView{content: []byte("Connection refused by 10.0.0.1")}
	log2 := &mockLogView{content: []byte("Connection refused by 10.0.0.2")}
	log3 := &mockLogView{content: []byte("Connection refused by 192.168.1.100")}

	r1 := e.ProcessLog(log1)
	r2 := e.ProcessLog(log2)
	r3 := e.ProcessLog(log3)

	require.Len(t, r1, 1)
	require.Len(t, r2, 1)
	require.Len(t, r3, 1)
	assert.Equal(t, r1[0].Name, r2[0].Name, "messages differing only in variable tokens should share a pattern")
	assert.Equal(t, r1[0].Name, r3[0].Name)
}

func TestLogPatternExtractor_DifferentMessagesDistinctPatterns(t *testing.T) {
	e := NewLogPatternExtractor()
	log1 := &mockLogView{content: []byte("Connection refused by server")}
	log2 := &mockLogView{content: []byte("Disk full on volume dev sda")}

	r1 := e.ProcessLog(log1)
	r2 := e.ProcessLog(log2)

	require.Len(t, r1, 1)
	require.Len(t, r2, 1)
	assert.NotEqual(t, r1[0].Name, r2[0].Name, "structurally different messages should produce distinct metric names")
}

func TestLogPatternExtractor_PatternKeysPopulated(t *testing.T) {
	e := NewLogPatternExtractor()
	assert.Empty(t, e.PatternKeys)

	e.ProcessLog(&mockLogView{content: []byte("some log message")})

	assert.Len(t, e.PatternKeys, 1)
}

// --- LogPatternDetector ---

func TestLogPatternDetector_Name(t *testing.T) {
	d := NewLogPatternDetector()
	assert.Equal(t, "log_pattern_detector", d.Name())
}

func TestLogPatternDetector_SetupResolvesExtractor(t *testing.T) {
	extractor := NewLogPatternExtractor()
	d := NewLogPatternDetector()

	err := d.Setup(func(name string) (any, error) {
		if name == "log_pattern_extractor" {
			return extractor, nil
		}
		return nil, fmt.Errorf("unknown component: %s", name)
	})

	require.NoError(t, err)
	assert.Equal(t, extractor, d.extractor)
}

func TestLogPatternDetector_SetupErrorOnMissingExtractor(t *testing.T) {
	d := NewLogPatternDetector()

	err := d.Setup(func(_ string) (any, error) {
		return nil, fmt.Errorf("component not found")
	})

	assert.Error(t, err)
}

func TestLogPatternDetector_SetupErrorOnWrongType(t *testing.T) {
	d := NewLogPatternDetector()

	err := d.Setup(func(_ string) (any, error) {
		return "not an extractor", nil
	})

	assert.Error(t, err)
}

func TestLogPatternDetector_NoAnomaliesWithInsufficientHistory(t *testing.T) {
	extractor, d, storage := newPatternPipeline(t)

	// Feed fewer than TooRecentSize steps; detector skips all items as "too recent".
	for i := 0; i < d.TooRecentSize; i++ {
		ts := patternTestTS(i)
		feedLogs(t, extractor, storage, "Connection refused by server", ts, 1)
		result := d.Detect(storage, ts)
		assert.Empty(t, result.Anomalies, "no anomaly expected when history is too short")
	}
}

func TestLogPatternDetector_NoAnomaliesForStableRate(t *testing.T) {
	extractor, d, storage := newPatternPipeline(t)

	steps := d.HistorySize
	for i := 0; i < steps; i++ {
		ts := patternTestTS(i)
		feedLogs(t, extractor, storage, "Connection refused by server", ts, 3)
		d.Detect(storage, ts)
	}

	// One final detect after building up full stable history.
	result := d.Detect(storage, patternTestTS(steps))
	assert.Empty(t, result.Anomalies, "stable rate should not produce anomalies")
}

func TestLogPatternDetector_DetectsRateSpike(t *testing.T) {
	extractor, d, storage := newPatternPipeline(t)

	// Alternating baseline (1, 3, 1, 3, ...) gives avg≈2 and stddev≈1 so that
	// the z-score is well-defined and the spike is clearly detectable.
	baseline := d.TooRecentSize + 20
	for i := 0; i < baseline; i++ {
		ts := patternTestTS(i)
		count := 1 + (i%2)*2 // alternates between 1 and 3
		feedLogs(t, extractor, storage, "Connection refused by server", ts, count)
		d.Detect(storage, ts)
	}

	spikeTs := patternTestTS(baseline)
	feedLogs(t, extractor, storage, "Connection refused by server", spikeTs, 100)

	result := d.Detect(storage, spikeTs)

	require.NotEmpty(t, result.Anomalies, "sudden rate spike should produce an anomaly")
	assert.Contains(t, result.Anomalies[0].Title, "increase")
	require.NotNil(t, result.Anomalies[0].Score, "anomaly score must not be nil")
	assert.GreaterOrEqual(t, *result.Anomalies[0].Score, 0.0)
	assert.LessOrEqual(t, *result.Anomalies[0].Score, 1.0)
}

func TestLogPatternDetector_DetectsRateDrop(t *testing.T) {
	extractor, d, storage := newPatternPipeline(t)

	// Alternating high baseline (95, 105, ...) gives avg≈100, stddev≈5.
	baseline := d.TooRecentSize + 20
	for i := 0; i < baseline; i++ {
		ts := patternTestTS(i)
		count := 95 + (i%2)*10 // alternates 95 and 105
		feedLogs(t, extractor, storage, "Connection refused by server", ts, count)
		d.Detect(storage, ts)
	}

	// Drop: no logs in the next window – the series was already created, so
	// PointCountSince returns 0 for the new window.
	dropTs := patternTestTS(baseline)
	result := d.Detect(storage, dropTs)

	require.NotEmpty(t, result.Anomalies, "sudden rate drop should produce an anomaly")
	assert.Contains(t, result.Anomalies[0].Title, "decrease")
}

func TestLogPatternDetector_RateLimiterSuppressesDuplicates(t *testing.T) {
	extractor, d, storage := newPatternPipeline(t)

	baseline := d.TooRecentSize + 20
	for i := 0; i < baseline; i++ {
		ts := patternTestTS(i)
		count := 1 + (i%2)*2
		feedLogs(t, extractor, storage, "Connection refused by server", ts, count)
		d.Detect(storage, ts)
	}

	// First spike – should produce an anomaly.
	spikeTs := patternTestTS(baseline)
	feedLogs(t, extractor, storage, "Connection refused by server", spikeTs, 100)
	result1 := d.Detect(storage, spikeTs)
	require.NotEmpty(t, result1.Anomalies, "first spike should trigger anomaly")

	// Second spike immediately after (same timestamp + 2) – rate limiter cooldown
	// is 60000 so this is well within the suppression window.
	spikeTs2 := spikeTs + 2
	feedLogs(t, extractor, storage, "Connection refused by server", spikeTs2, 100)
	result2 := d.Detect(storage, spikeTs2)
	assert.Empty(t, result2.Anomalies, "rate limiter should suppress duplicate anomaly")
}

func TestLogPatternDetector_TelemetryAlwaysEmitted(t *testing.T) {
	extractor, d, storage := newPatternPipeline(t)

	feedLogs(t, extractor, storage, "Connection refused by server", patternTestTS(0), 1)

	result := d.Detect(storage, patternTestTS(0))
	assert.NotEmpty(t, result.Telemetry, "detector should emit telemetry for each tracked series")
}

func TestLogPatternDetector_IndependentPatternsTrackedSeparately(t *testing.T) {
	extractor, d, storage := newPatternPipeline(t)

	// Build stable baselines for two structurally different patterns.
	baseline := d.TooRecentSize + 20
	for i := 0; i < baseline; i++ {
		ts := patternTestTS(i)
		count := 1 + (i%2)*2 // alternates 1 and 3
		feedLogs(t, extractor, storage, "Connection refused by server", ts, count)
		feedLogs(t, extractor, storage, "Disk full on volume dev sda", ts, count)
		d.Detect(storage, ts)
	}

	// Spike only the second pattern; keep the first one at its average rate.
	spikeTs := patternTestTS(baseline)
	feedLogs(t, extractor, storage, "Connection refused by server", spikeTs, 2)  // ≈ average
	feedLogs(t, extractor, storage, "Disk full on volume dev sda", spikeTs, 100) // spike

	result := d.Detect(storage, spikeTs)

	require.NotEmpty(t, result.Anomalies)
	found := false
	for _, a := range result.Anomalies {
		if strings.Contains(a.Description, "Disk") {
			found = true
		}
	}
	assert.True(t, found, "anomaly should reference the spiked 'Disk full' pattern")
}

// --- Integration: extractor → engine storage → detector ---

func TestLogPatternPipeline_EndToEnd(t *testing.T) {
	extractor := NewLogPatternExtractor()
	detector := NewLogPatternDetector()

	eng := mustNewEngine(t, engineConfig{
		storage:    newTimeSeriesStorage(),
		extractors: []observerdef.LogMetricsExtractor{extractor},
		detectors:  []observerdef.Detector{detector},
		scheduler:  &currentBehaviorPolicy{},
	})

	// Use a small window so the detector only counts logs from the current step.
	// With stride-10 timestamps (seconds) and window=5, each step's window [ts-5, ts]
	// covers only the current 10s bucket and not the previous one.
	detector.WindowDurationMs = 5
	detector.ZThreshold = 1.5

	source := "test-source"
	baseline := detector.TooRecentSize + 20
	for i := 0; i < baseline; i++ {
		tsSec := int64(i+1) * 10
		count := 1 + (i%2)*2 // alternates 1 and 3 for non-zero variance
		for j := 0; j < count; j++ {
			eng.IngestLog(source, &logObs{
				content:     []byte("Connection refused by server"),
				timestampMs: tsSec * 1000,
			})
		}
		eng.Advance(tsSec)
	}

	// Spike: 200 logs in one window.
	spikeSec := int64(baseline+1) * 10
	for i := 0; i < 200; i++ {
		eng.IngestLog(source, &logObs{
			content:     []byte("Connection refused by server"),
			timestampMs: spikeSec * 1000,
		})
	}

	state := eng.Advance(spikeSec)
	assert.NotEmpty(t, state.anomalies, "engine should surface anomaly after log rate spike")
}

// --- helpers ---

// patternTestTS maps step index i to a timestamp using stride 2.
// With WindowDurationMs=1, the window [ts-1, ∞) only captures logs at ts=2*(i+1),
// excluding the previous step's logs at ts=2*i. This gives clean, non-overlapping
// per-step rates for unit tests.
func patternTestTS(i int) int64 {
	return int64(i+1) * 2
}

// newPatternPipeline creates a wired extractor + detector pair with fresh storage.
// WindowDurationMs is set to 1 so that tests using patternTestTS get clean,
// non-overlapping windows.
func newPatternPipeline(t *testing.T) (*LogPatternExtractor, *LogPatternDetector, *timeSeriesStorage) {
	t.Helper()
	extractor := NewLogPatternExtractor()
	detector := NewLogPatternDetector()
	detector.WindowDurationMs = 1
	err := detector.Setup(func(name string) (any, error) {
		if name == "log_pattern_extractor" {
			return extractor, nil
		}
		return nil, fmt.Errorf("unknown component: %s", name)
	})
	require.NoError(t, err)
	return extractor, detector, newTimeSeriesStorage()
}

// feedLogs runs count identical log messages through the extractor and stores
// the resulting virtual metrics into storage at the given timestamp.
func feedLogs(t *testing.T, e *LogPatternExtractor, storage *timeSeriesStorage, message string, ts int64, count int) {
	t.Helper()
	log := &mockLogView{content: []byte(message)}
	for i := 0; i < count; i++ {
		metrics := e.ProcessLog(log)
		for _, m := range metrics {
			storage.Add("logs", fmt.Sprintf("_virtual.%s", m.Name), m.Value, ts, m.Tags)
		}
	}
}
