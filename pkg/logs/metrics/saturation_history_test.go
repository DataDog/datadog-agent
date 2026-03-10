// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHistory() *SaturationHistory {
	now := time.Now()
	h := &SaturationHistory{
		states:          make(map[string]*stageSaturationState),
		events:          make([]SaturationEvent, 0, maxSaturationEvents),
		currentFill:     make(map[string]float64),
		retryTimestamps: make([]time.Time, 0, 16),
		now:             func() time.Time { return now },
	}
	h.windows[0] = newRollingWindow(5*time.Minute, now)
	h.windows[1] = newRollingWindow(30*time.Minute, now)
	h.windows[2] = newRollingWindow(2*time.Hour, now)
	return h
}

func TestNoSaturation_NilSuggestion(t *testing.T) {
	h := newTestHistory()
	h.RecordFill(ProcessorTlmName, 0.10)
	h.RecordFill(StrategyTlmName, 0.10)

	s := h.Summary()
	assert.Empty(t, s.SuggestedProfile)
	assert.Empty(t, s.RecentEvents)
}

// Strategy fill is not used for suggestions: egress is per-batch so fill% is
// always inflated. Strategy saturation is tracked via utilization ratio (TODO).
func TestStrategySaturation_NoSuggestionFromFill(t *testing.T) {
	h := newTestHistory()
	h.RecordFill(StrategyTlmName, 0.95) // even at 95%, no suggestion fires

	s := h.Summary()
	assert.Empty(t, s.SuggestedProfile)
	// Fill is still recorded in windows for display purposes.
	assert.InDelta(t, 0.95, s.MaxFill30m[StrategyTlmName], 0.01)
}

func TestProcessorSaturation_SuggestsPerformance(t *testing.T) {
	h := newTestHistory()
	h.RecordFill(ProcessorTlmName, 0.80)

	s := h.Summary()
	assert.Equal(t, "performance", s.SuggestedProfile)
}

// Retries are informational only — they update the display windows but do not
// drive profile suggestions, since retry count measures HTTP errors not backpressure.
// Transport suggestion requires sender worker utilization signal (not yet wired).
func TestRetries_DoNotSuggestProfile(t *testing.T) {
	h := newTestHistory()
	for i := 0; i < retryRateThreshold+2; i++ {
		h.RecordRetry()
	}

	s := h.Summary()
	assert.Empty(t, s.SuggestedProfile)
	// But the retry fill-equivalent should be visible in the windows for display.
	assert.Greater(t, s.MaxFill5m[SenderTlmName], 0.0)
}

// Retries don't generate suggestions, and neither does strategy fill alone.
func TestRetries_AndStrategyFill_NoSuggestion(t *testing.T) {
	h := newTestHistory()
	for i := 0; i < 30; i++ {
		h.RecordFill(StrategyTlmName, 0.90)
	}
	for i := 0; i < retryRateThreshold+2; i++ {
		h.RecordRetry()
	}

	s := h.Summary()
	assert.Empty(t, s.SuggestedProfile)
}

func TestSaturationEvent_RecordedOnRecovery(t *testing.T) {
	var now time.Time
	h := newTestHistory()
	h.now = func() time.Time { return now }

	now = time.Now()
	h.RecordFill(StrategyTlmName, 0.85) // enter saturated

	now = now.Add(saturationMinDuration + 1*time.Second)
	h.RecordFill(StrategyTlmName, 0.10) // recover

	s := h.Summary()
	require.Len(t, s.RecentEvents, 1)
	assert.Equal(t, StrategyTlmName, s.RecentEvents[0].Stage)
	assert.Equal(t, "max_throughput", s.RecentEvents[0].Suggestion)
	assert.InDelta(t, 0.85, s.RecentEvents[0].PeakFill, 0.01)
	assert.False(t, s.RecentEvents[0].Ongoing())
}

func TestShortSpike_NotRecordedAsEvent(t *testing.T) {
	var now time.Time
	h := newTestHistory()
	h.now = func() time.Time { return now }

	now = time.Now()
	h.RecordFill(ProcessorTlmName, 0.80) // enter saturated

	// Recover immediately — below minimum duration.
	now = now.Add(2 * time.Second)
	h.RecordFill(ProcessorTlmName, 0.10)

	s := h.Summary()
	assert.Empty(t, s.RecentEvents)
}

func TestRollingWindowReset(t *testing.T) {
	var now time.Time
	h := newTestHistory()
	h.now = func() time.Time { return now }

	now = time.Now()
	h.RecordFill(StrategyTlmName, 0.90)
	assert.InDelta(t, 0.90, h.Summary().MaxFill5m[StrategyTlmName], 0.01)

	// Advance past the 5-minute window.
	now = now.Add(5*time.Minute + 1*time.Second)
	h.RecordFill(StrategyTlmName, 0.10) // record something low to trigger reset

	assert.InDelta(t, 0.10, h.Summary().MaxFill5m[StrategyTlmName], 0.01)
	// 30m window should still hold the old peak.
	assert.InDelta(t, 0.90, h.Summary().MaxFill30m[StrategyTlmName], 0.01)
}

func TestEventRingBuffer_BoundedAtMax(t *testing.T) {
	var now time.Time
	h := newTestHistory()
	h.now = func() time.Time { return now }

	now = time.Now()
	for i := 0; i < maxSaturationEvents+5; i++ {
		now = now.Add(1 * time.Second)
		h.RecordFill(StrategyTlmName, 0.90) // enter
		now = now.Add(saturationMinDuration + 1*time.Second)
		h.RecordFill(StrategyTlmName, 0.10) // recover + record event
	}

	s := h.Summary()
	assert.LessOrEqual(t, len(s.RecentEvents), maxSaturationEvents)
}

func TestRecentEvents_NewestFirst(t *testing.T) {
	var now time.Time
	h := newTestHistory()
	h.now = func() time.Time { return now }

	now = time.Now()
	t0 := now

	// First event: processor
	h.RecordFill(ProcessorTlmName, 0.80)
	now = now.Add(saturationMinDuration + 1*time.Second)
	h.RecordFill(ProcessorTlmName, 0.10)

	// Second event: strategy
	now = now.Add(1 * time.Second)
	h.RecordFill(StrategyTlmName, 0.85)
	now = now.Add(saturationMinDuration + 1*time.Second)
	h.RecordFill(StrategyTlmName, 0.10)

	s := h.Summary()
	require.Len(t, s.RecentEvents, 2)
	// Newest (strategy) first.
	assert.Equal(t, StrategyTlmName, s.RecentEvents[0].Stage)
	assert.Equal(t, ProcessorTlmName, s.RecentEvents[1].Stage)
	_ = t0
}
