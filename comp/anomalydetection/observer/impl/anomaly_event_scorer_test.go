// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func ptr(f float64) *float64 { return &f }

func makeAnomaly(sourceKey string, timestamp int64, score *float64) observerdef.Anomaly {
	return observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: sourceKey},
		DetectorName: "test_detector",
		Timestamp:    timestamp,
		Title:        "test",
		Score:        score,
	}
}

// --- Scoring correctness ---

func TestSameSingleSignalNoisyOR(t *testing.T) {
	// Two anomalies from the same signal: noisy-OR within signal → 0.98, capped at singleSignalMaxScore (0.45).
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})
	s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.8)))
	evt := s.ProcessAnomaly(makeAnomaly("s1", 101, ptr(0.9)))

	assert.Equal(t, 1, evt.Breakdown.SignalCount)
	assert.Equal(t, 1, evt.Breakdown.EffectiveSignalCount)
	assert.True(t, evt.Breakdown.SingleSignalCapApplied)
	assert.InDelta(t, singleSignalMaxScore, evt.Score, 1e-9)
	assert.Equal(t, observerdef.AnomalySeverityMedium, evt.Severity) // 0.45 >= mediumSeverityThreshold (0.40)
}

func TestCrossSignalNoisyOR(t *testing.T) {
	// Two signals each at 0.8 → noisy-OR = 0.96, capped at twoSignalMaxScore (0.65).
	// 0.65 < highSeverityThreshold (0.80) → medium.
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})
	s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.8)))
	evt := s.ProcessAnomaly(makeAnomaly("s2", 101, ptr(0.8)))

	assert.Equal(t, 2, evt.Breakdown.SignalCount)
	assert.Equal(t, 2, evt.Breakdown.EffectiveSignalCount)
	assert.True(t, evt.Breakdown.TwoSignalCapApplied)
	assert.InDelta(t, twoSignalMaxScore, evt.Score, 1e-9)
	assert.Equal(t, observerdef.AnomalySeverityMedium, evt.Severity) // 0.65 < high threshold 0.80
}

func TestThreeSignals_ThreeOrMoreCap(t *testing.T) {
	// Three signals at 0.5 each → noisy-OR = 0.875 → capped at threeOrMoreSignalMaxScore (0.82).
	// 0.82 >= highSeverityThreshold (0.80) → high.
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})
	s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.5)))
	s.ProcessAnomaly(makeAnomaly("s2", 100, ptr(0.5)))
	evt := s.ProcessAnomaly(makeAnomaly("s3", 100, ptr(0.5)))

	assert.Equal(t, 3, evt.Breakdown.SignalCount)
	assert.Equal(t, 3, evt.Breakdown.EffectiveSignalCount)
	assert.False(t, evt.Breakdown.SingleSignalCapApplied)
	assert.False(t, evt.Breakdown.TwoSignalCapApplied)
	assert.True(t, evt.Breakdown.ThreeOrMoreSignalCapApplied)
	assert.InDelta(t, threeOrMoreSignalMaxScore, evt.Score, 1e-9)
	assert.Equal(t, observerdef.AnomalySeverityHigh, evt.Severity)
}

func TestManySignals_TopNLimiting(t *testing.T) {
	// 10 signals: only top-3 are used, score capped at threeOrMoreSignalMaxScore.
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 200})
	for i := 0; i < 10; i++ {
		s.ProcessAnomaly(makeAnomaly(fmt.Sprintf("s%d", i), int64(100+i), ptr(0.5)))
	}
	evt := s.Events()[len(s.Events())-1]

	assert.Equal(t, 10, evt.Breakdown.SignalCount, "10 total signals in window")
	assert.Equal(t, maxScoringSignals, evt.Breakdown.EffectiveSignalCount, "only top-3 used")
	assert.True(t, evt.Breakdown.ThreeOrMoreSignalCapApplied)
	assert.InDelta(t, threeOrMoreSignalMaxScore, evt.Score, 1e-9)
}

func TestSingleSignalHighCap(t *testing.T) {
	// Single signal with score 1.0 → capped at singleSignalMaxScore (0.45) → medium.
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})
	evt := s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(1.0)))

	assert.True(t, evt.Breakdown.SingleSignalCapApplied)
	assert.InDelta(t, singleSignalMaxScore, evt.Score, 1e-9)
	assert.Equal(t, observerdef.AnomalySeverityMedium, evt.Severity) // 0.45 >= medium threshold (0.40)
}

// --- Missing score fallback ---

func TestMissingScoreFallback(t *testing.T) {
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})
	evt := s.ProcessAnomaly(makeAnomaly("s1", 100, nil)) // no score

	assert.Equal(t, 1, evt.Breakdown.MissingScoreCount)
	// defaultMissingAnomalyScore = 0.5 > singleSignalMaxScore (0.45) → cap applies.
	assert.InDelta(t, singleSignalMaxScore, evt.Score, 1e-9, "missing score 0.5 is capped at singleSignalMaxScore")
	assert.True(t, evt.Breakdown.SingleSignalCapApplied)
}

func TestMissingScoreCountAccumulates(t *testing.T) {
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})
	s.ProcessAnomaly(makeAnomaly("s1", 100, nil))
	evt := s.ProcessAnomaly(makeAnomaly("s2", 101, nil))
	assert.Equal(t, 2, evt.Breakdown.MissingScoreCount)
}

// --- Sliding window eviction by time ---

func TestSlidingWindowEvictionByTime(t *testing.T) {
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 10, maxItems: 100})
	s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.9))) // will be evicted
	s.ProcessAnomaly(makeAnomaly("s2", 105, ptr(0.9))) // will be evicted

	// This anomaly is at t=115; window is 10s → cutoff is 105.
	// Both earlier anomalies (t=100, t=105) are below 115-10=105 → t=100 is evicted, t=105 exactly at cutoff is not.
	evt := s.ProcessAnomaly(makeAnomaly("s3", 115, ptr(0.2)))

	// s1 (t=100) evicted (100 < 115-10=105), s2 (t=105) stays (105 >= 105), s3 stays.
	assert.Equal(t, 2, len(evt.RecentAnomalies), "s1 should be evicted")
}

func TestSlidingWindowMaxItemCap(t *testing.T) {
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 9999, maxItems: 3})
	s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.5)))
	s.ProcessAnomaly(makeAnomaly("s2", 101, ptr(0.5)))
	s.ProcessAnomaly(makeAnomaly("s3", 102, ptr(0.5)))
	evt := s.ProcessAnomaly(makeAnomaly("s4", 103, ptr(0.5)))

	// Oldest item trimmed: should keep last 3 (s2, s3, s4).
	assert.Equal(t, 3, len(evt.RecentAnomalies))
}

// --- Severity threshold mapping ---

func TestSeverityThresholds(t *testing.T) {
	cases := []struct {
		score    float64
		expected observerdef.AnomalySeverity
	}{
		{0.0, observerdef.AnomalySeverityLow},
		{0.39, observerdef.AnomalySeverityLow},
		{0.40, observerdef.AnomalySeverityMedium},
		{0.79, observerdef.AnomalySeverityMedium},
		{0.80, observerdef.AnomalySeverityHigh},
		{1.0, observerdef.AnomalySeverityHigh},
	}
	for _, tc := range cases {
		got := severityFromScore(tc.score)
		assert.Equal(t, tc.expected, got, "score=%v", tc.score)
	}
}

// --- Severity change detection by scope ---

func TestSeverityChangeDetection(t *testing.T) {
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})

	// First anomaly: single signal, score 0.1 → low (below medium threshold 0.40).
	evt1 := s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.1)))
	assert.Equal(t, observerdef.AnomalySeverityLow, evt1.Severity)
	assert.False(t, evt1.SeverityChanged, "no previous → not changed")
	assert.Equal(t, "same", evt1.SeverityDirection)

	// Second anomaly from a different signal → two signals → event score = 0.91,
	// capped at twoSignalMaxScore (0.65) → medium.
	evt2 := s.ProcessAnomaly(makeAnomaly("s2", 101, ptr(0.9)))
	assert.Equal(t, observerdef.AnomalySeverityMedium, evt2.Severity)
	assert.True(t, evt2.SeverityChanged)
	assert.Equal(t, "up", evt2.SeverityDirection)
	assert.Equal(t, observerdef.AnomalySeverityLow, evt2.PreviousSeverity)
}

func TestSeverityDirection_Down(t *testing.T) {
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 1, maxItems: 100})

	// First: three-signal → high (threeOrMoreSignalMaxScore 0.82 >= highSeverityThreshold 0.80).
	s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.9)))
	s.ProcessAnomaly(makeAnomaly("s2", 100, ptr(0.9)))
	s.ProcessAnomaly(makeAnomaly("s3", 100, ptr(0.9)))

	// Next: only one low-score anomaly in a new window (old ones evicted).
	evt := s.ProcessAnomaly(makeAnomaly("s1", 200, ptr(0.1)))
	require.Equal(t, observerdef.AnomalySeverityHigh, evt.PreviousSeverity)
	assert.Equal(t, observerdef.AnomalySeverityLow, evt.Severity)
	assert.True(t, evt.SeverityChanged)
	assert.Equal(t, "down", evt.SeverityDirection)
}

// --- Reset ---

func TestScorerReset(t *testing.T) {
	s := newAnomalyEventScorer(anomalyEventScorerConfig{windowSeconds: 300, maxItems: 100})
	s.ProcessAnomaly(makeAnomaly("s1", 100, ptr(0.9)))
	s.ProcessAnomaly(makeAnomaly("s2", 101, ptr(0.9)))

	s.Reset()

	assert.Empty(t, s.Events())
	assert.Empty(t, s.window)
	assert.Empty(t, s.previousSeverity)

	// After reset, first event should have no previous severity.
	evt := s.ProcessAnomaly(makeAnomaly("s1", 200, ptr(0.5)))
	assert.Equal(t, "same", evt.SeverityDirection)
	assert.False(t, evt.SeverityChanged)
}

// --- eventID stability ---

func TestEventIDStability(t *testing.T) {
	a := makeAnomaly("s1", 100, ptr(0.5))
	id1 := eventID(a)
	id2 := eventID(a)
	assert.Equal(t, id1, id2, "event ID should be deterministic")
	assert.Len(t, id1, 16)
}
