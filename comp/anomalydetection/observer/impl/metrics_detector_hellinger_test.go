// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHellingerDetector returns a detector tuned for unit tests with small
// data sets — short window so we can hit the warmup with ~tens of points.
func testHellingerDetector() *HellingerDetector {
	cfg := DefaultHellingerConfig()
	cfg.WindowPoints = 15
	cfg.RecoveryPoints = 5
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return NewHellingerDetectorWithConfig(cfg)
}

func TestHellinger_Name(t *testing.T) {
	d := NewHellingerDetector()
	assert.Equal(t, "hellinger", d.Name())
}

func TestHellinger_DefaultConfig(t *testing.T) {
	cfg := DefaultHellingerConfig()
	assert.Equal(t, 30, cfg.WindowPoints)
	assert.Equal(t, 32, cfg.Bins)
	assert.InDelta(t, 0.55, cfg.HellingerThreshold, 1e-9)
	assert.InDelta(t, 2.5, cfg.MinDeviationMAD, 1e-9)
	assert.Equal(t, 10, cfg.RecoveryPoints)
}

func TestHellinger_NewWithConfig_FillsZeroFields(t *testing.T) {
	// All zero → all fields should be filled from defaults.
	d := NewHellingerDetectorWithConfig(HellingerConfig{})
	defaults := DefaultHellingerConfig()
	assert.Equal(t, defaults.WindowPoints, d.config.WindowPoints)
	assert.Equal(t, defaults.Bins, d.config.Bins)
	assert.Equal(t, defaults.HellingerThreshold, d.config.HellingerThreshold)
	assert.Equal(t, defaults.MinDeviationMAD, d.config.MinDeviationMAD)
	assert.Equal(t, defaults.RecoveryPoints, d.config.RecoveryPoints)
	assert.NotEmpty(t, d.config.Aggregations)
}

func TestHellinger_NotEnoughPoints(t *testing.T) {
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	// 2*WindowPoints = 30. Add only 25 points.
	for i := 0; i < 25; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}

	result := d.Detect(storage, 25)
	assert.Empty(t, result.Anomalies, "should not fire before warmup completes")
}

func TestHellinger_StableData_NoFire(t *testing.T) {
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	// 60 points of stable data (well past warmup, with steady jitter).
	for i := 0; i < 60; i++ {
		val := 100.0 + float64(i%3-1) // values cycle 99, 100, 101
		storage.Add("ns", "metric", val, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)
	assert.Empty(t, result.Anomalies, "stable data should not trigger Hellinger")
}

func TestHellinger_DetectsStepShift(t *testing.T) {
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	// Pre window of 15 points at 50, post window of 15 points at 200.
	// At t=30 the buffer is full and pre/post are completely disjoint
	// distributions — Hellinger ≈ 1.
	for i := 0; i < 15; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 15; i < 30; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 30)

	require.NotEmpty(t, result.Anomalies, "should detect step shift")
	a := result.Anomalies[0]
	assert.Contains(t, a.Title, "Hellinger drift")
	assert.Equal(t, "hellinger", a.DetectorName)
	assert.Contains(t, a.Description, "increased")
	require.NotNil(t, a.SourceRef, "SourceRef should be populated")
	require.NotNil(t, a.Score, "Score should be populated with H value")
	assert.GreaterOrEqual(t, *a.Score, 0.55)
	assert.LessOrEqual(t, *a.Score, 1.0)
	require.NotNil(t, a.DebugInfo, "DebugInfo should be populated")
	assert.InDelta(t, 50, a.DebugInfo.BaselineMedian, 0.5)
	assert.InDelta(t, 200, a.DebugInfo.CurrentValue, 0.5)
}

func TestHellinger_DetectsDownwardShift(t *testing.T) {
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 15; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	for i := 15; i < 30; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}

	result := d.Detect(storage, 30)
	require.NotEmpty(t, result.Anomalies, "should detect downward shift")
	assert.Contains(t, result.Anomalies[0].Description, "decreased")
}

func TestHellinger_NoMedianShift_NoFire(t *testing.T) {
	// Pure variance burst with the same median — Hellinger fires on shape but
	// the MAD gate (the FP guard) blocks the alert.
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	// Pre: tight noise at 100. preMedian = 100, preMAD ≈ 0.5.
	for i := 0; i < 15; i++ {
		val := 100.0 + float64(i%3-1)*0.5
		storage.Add("ns", "metric", val, int64(i+1), nil)
	}
	// Post: 15 values arranged so the median is exactly 100 but the shape
	// is bimodal with wide spread — 7 lows, 1 center, 7 highs. Hellinger ≫
	// threshold, but |postMedian-preMedian| = 0 → MAD gate blocks.
	postVals := []float64{
		70, 70, 70, 70, 70, 70, 70,
		100,
		130, 130, 130, 130, 130, 130, 130,
	}
	for i, v := range postVals {
		storage.Add("ns", "metric", v, int64(15+i+1), nil)
	}

	result := d.Detect(storage, 30)
	for _, a := range result.Anomalies {
		// Logged for debugging if the gate ever regresses.
		t.Logf("unexpected anomaly: %+v", a)
	}
	assert.Empty(t, result.Anomalies, "MAD gate should reject pure variance bursts (no median shift)")
}

func TestHellinger_IncrementalAdvance(t *testing.T) {
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	// First advance: stable, not enough points.
	for i := 0; i < 15; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	r1 := d.Detect(storage, 15)
	assert.Empty(t, r1.Anomalies)

	// Second advance: introduce a clean shift.
	for i := 15; i < 30; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 30)
	require.NotEmpty(t, r2.Anomalies, "shift should fire on second advance")

	// Third advance: no new data → no anomaly.
	r3 := d.Detect(storage, 30)
	assert.Empty(t, r3.Anomalies, "no new data should produce no anomalies")
}

func TestHellinger_SustainedDrift_EmitsOnce(t *testing.T) {
	// A long sustained drift must emit at most one anomaly per series/agg
	// before recovery — the FP failure mode that motivated the recovery gate.
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 15; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	// 60 sustained shifted points (well over RecoveryPoints).
	for i := 15; i < 75; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 75)
	assert.Equal(t, 1, len(result.Anomalies),
		"sustained drift should emit exactly one anomaly per series/agg")
}

func TestHellinger_RecoveryAndReAlert(t *testing.T) {
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()
	ts := int64(0)

	addN := func(n int, val float64) {
		for i := 0; i < n; i++ {
			ts++
			storage.Add("ns", "m", val, ts, nil)
		}
	}

	// Warmup baseline.
	addN(15, 100)
	r1 := d.Detect(storage, ts)
	assert.Empty(t, r1.Anomalies)

	// First incident.
	addN(15, 300)
	r2 := d.Detect(storage, ts)
	require.NotEmpty(t, r2.Anomalies, "should detect first incident")

	// Recovery: enough stable points at the new level for the gate to clear,
	// then plenty more to refill the buffer with stable data so the next
	// incident can fire cleanly.
	addN(30, 300)
	r3 := d.Detect(storage, ts)
	// During recovery + refill the detector should not re-fire.
	assert.Empty(t, r3.Anomalies, "stable post-shift data should not re-fire")

	// Second incident — a new big jump.
	addN(15, 800)
	r4 := d.Detect(storage, ts)
	require.NotEmpty(t, r4.Anomalies, "should detect second incident after recovery")
}

func TestHellinger_RingBufferEviction_NoFalseFireOnLongStable(t *testing.T) {
	// Long stable run far exceeding 2*WindowPoints — the ring buffer must
	// keep evicting old values; pre and post must remain similar.
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 200; i++ {
		val := 100.0 + math.Sin(float64(i)*0.4) // small bounded oscillation
		storage.Add("ns", "metric", val, int64(i+1), nil)
	}

	result := d.Detect(storage, 200)
	assert.Empty(t, result.Anomalies, "long stable run must not yield false fires")
}

func TestHellinger_DeterministicReplay(t *testing.T) {
	makeDetector := func() *HellingerDetector { return testHellingerDetector() }

	storage := newTimeSeriesStorage()
	for i := 0; i < 15; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 15; i < 35; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	d1 := makeDetector()
	r1 := d1.Detect(storage, 35)

	d2 := makeDetector()
	r2 := d2.Detect(storage, 35)

	require.Equal(t, len(r1.Anomalies), len(r2.Anomalies), "replay should match anomaly count")
	for i := range r1.Anomalies {
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp)
		assert.Equal(t, r1.Anomalies[i].Source, r2.Anomalies[i].Source)
		require.NotNil(t, r1.Anomalies[i].Score)
		require.NotNil(t, r2.Anomalies[i].Score)
		assert.InDelta(t, *r1.Anomalies[i].Score, *r2.Anomalies[i].Score, 1e-12)
	}
}

func TestHellinger_Reset(t *testing.T) {
	d := testHellingerDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 30; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	d.Detect(storage, 30)
	assert.NotEmpty(t, d.series, "should accumulate state during detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear per-series state")
	assert.Nil(t, d.cachedSeries, "reset should clear cached series list")
	assert.Equal(t, uint64(0), d.cachedGen)
}

// ---------------------------------------------------------------------------
// Pure-function tests for the histogram-based Hellinger distance.
// ---------------------------------------------------------------------------

func TestHellingerDistance_IdenticalDistributions(t *testing.T) {
	pre := []float64{1, 2, 3, 4, 5, 1, 2, 3, 4, 5}
	post := []float64{1, 2, 3, 4, 5, 1, 2, 3, 4, 5}
	var preH, postH []float64
	h := hellingerDistance(pre, post, 16, &preH, &postH)
	assert.InDelta(t, 0.0, h, 1e-12, "identical distributions → H=0")
}

func TestHellingerDistance_DisjointDistributions(t *testing.T) {
	pre := []float64{0, 0, 0, 0, 0}
	post := []float64{100, 100, 100, 100, 100}
	var preH, postH []float64
	h := hellingerDistance(pre, post, 16, &preH, &postH)
	assert.InDelta(t, 1.0, h, 1e-9, "disjoint single-bin distributions → H=1")
}

func TestHellingerDistance_ConstantSeries(t *testing.T) {
	// All values identical → span=0 → H=0 (no defined drift).
	pre := []float64{42, 42, 42}
	post := []float64{42, 42, 42}
	var preH, postH []float64
	h := hellingerDistance(pre, post, 8, &preH, &postH)
	assert.Equal(t, 0.0, h)
}

func TestHellingerDistance_EmptyInputs(t *testing.T) {
	var preH, postH []float64
	assert.Equal(t, 0.0, hellingerDistance(nil, []float64{1}, 8, &preH, &postH))
	assert.Equal(t, 0.0, hellingerDistance([]float64{1}, nil, 8, &preH, &postH))
}

func TestHellingerDistance_RangeBounded(t *testing.T) {
	// Random-ish values: H must always land in [0, 1].
	pre := []float64{1, 3, 5, 7, 9, 11, 13, 15}
	post := []float64{2, 4, 6, 8, 10, 12, 14, 16}
	var preH, postH []float64
	h := hellingerDistance(pre, post, 16, &preH, &postH)
	assert.GreaterOrEqual(t, h, 0.0)
	assert.LessOrEqual(t, h, 1.0)
}

func TestHellingerDistance_ReusesScratchBuffers(t *testing.T) {
	// The histogram scratch slices should not grow on each call when bins
	// stays the same.
	var preH, postH []float64
	pre := []float64{1, 2, 3}
	post := []float64{4, 5, 6}
	hellingerDistance(pre, post, 8, &preH, &postH)
	cap1Pre, cap1Post := cap(preH), cap(postH)
	hellingerDistance(pre, post, 8, &preH, &postH)
	assert.Equal(t, cap1Pre, cap(preH), "preH should be reused, not regrown")
	assert.Equal(t, cap1Post, cap(postH), "postH should be reused, not regrown")
}
