// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testScanMWDetector() *ScanMWDetector {
	d := NewScanMWDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

func TestScanMW_NotEnoughPoints(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 10; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}

	result := d.Detect(storage, 10)
	assert.Empty(t, result.Anomalies, "should not fire with fewer than MinPoints")
}

func TestScanMW_DetectsStepChange(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 40)

	require.NotEmpty(t, result.Anomalies, "should detect step change")
	assert.Contains(t, result.Anomalies[0].Title, "ScanMW")
	// Changepoint should be near the transition at index 20.
	assert.InDelta(t, 21, result.Anomalies[0].Timestamp, 3)
}

func TestScanMW_IncrementalAdvance(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	// First advance: stable data, not enough to fire.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	r1 := d.Detect(storage, 20)
	assert.Empty(t, r1.Anomalies, "no anomaly in stable data")

	// Second advance: add shifted data — now has 40 points total.
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 40)
	require.NotEmpty(t, r2.Anomalies, "should detect step change on second advance")

	// Third advance: no new data — should emit nothing.
	r3 := d.Detect(storage, 40)
	assert.Empty(t, r3.Anomalies, "no new data should produce no anomalies")
}

func TestScanMW_SegmentAdvancement(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	// Phase 1: baseline at 50, shift to 200.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 50; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	r1 := d.Detect(storage, 50)
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	// After fire, the segment start should have advanced.
	// Adding more data at 200 (same as post-change) should not re-fire
	// because the post-change segment is stable.
	for i := 50; i < 90; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 90)
	assert.Empty(t, r2.Anomalies, "stable post-change data should not re-fire")
}

func TestScanMW_TwoSequentialChanges(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	// Phase 1: 50 → 200
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 50; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	r1 := d.Detect(storage, 50)
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	// Phase 2: 200 → 500
	for i := 50; i < 80; i++ {
		storage.Add("ns", "metric", 500, int64(i+1), nil)
	}

	r2 := d.Detect(storage, 80)
	require.NotEmpty(t, r2.Anomalies, "should detect second changepoint after segment advancement")
}

func TestScanMW_DeterministicReplay(t *testing.T) {
	makeDetector := func() *ScanMWDetector { return testScanMWDetector() }

	storage := newTimeSeriesStorage()
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	d1 := makeDetector()
	r1 := d1.Detect(storage, 40)

	d2 := makeDetector()
	r2 := d2.Detect(storage, 40)

	require.Equal(t, len(r1.Anomalies), len(r2.Anomalies), "replay should produce same anomaly count")
	for i := range r1.Anomalies {
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp, "anomaly timestamps should match")
		assert.Equal(t, r1.Anomalies[i].Source, r2.Anomalies[i].Source, "anomaly sources should match")
	}
}

func TestScanMW_Reset(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 40; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	d.Detect(storage, 40)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
	assert.Nil(t, d.cachedSeries, "reset should clear cached series")
}
