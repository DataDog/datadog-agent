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

// testKLDivergenceDetector returns a detector configured for the
// single-aggregation tests below. Default Aggregations includes Count, but
// for these unit tests we only seed Average and asserting on a single
// aggregation keeps anomaly counts deterministic.
func testKLDivergenceDetector() *KLDivergenceDetector {
	d := NewKLDivergenceDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// addAlternating adds a sequence of values that alternate between two
// numbers. Even indices (i+offset even) get loVal, odd indices get hiVal.
// Timestamps are 1-indexed starting at startTs.
func addAlternating(t *testing.T, storage *timeSeriesStorage, name string, count int, startTs int64, loVal, hiVal float64) {
	t.Helper()
	for i := 0; i < count; i++ {
		v := loVal
		if i%2 == 1 {
			v = hiVal
		}
		storage.Add("ns", name, v, startTs+int64(i), nil)
	}
}

// TestKLDivergence_NoChange_NoFire feeds 200 alternating low-amplitude
// points with no distributional change. The rolling reference and test
// windows see equivalent samples on every scan, so neither the KL gate
// nor the deviation gate should ever pass.
func TestKLDivergence_NoChange_NoFire(t *testing.T) {
	d := testKLDivergenceDetector()
	storage := newTimeSeriesStorage()

	addAlternating(t, storage, "metric", 200, 1, 0.0, 0.1)

	result := d.Detect(storage, 200)
	assert.Empty(t, result.Anomalies, "stationary input must not trigger KL divergence")
}

// TestKLDivergence_MeanShift_Fires feeds 60 baseline points followed by
// 30 mean-shifted points. After ingest the reference window holds the
// baseline and the test window holds the post-shift values; both KL and
// the MAD-deviation gate should pass and exactly one anomaly is emitted.
func TestKLDivergence_MeanShift_Fires(t *testing.T) {
	d := testKLDivergenceDetector()
	storage := newTimeSeriesStorage()

	// 60 low-noise baseline points.
	addAlternating(t, storage, "metric", 60, 1, 0.0, 0.1)
	// 30 shifted-up points (5x baseline scale).
	addAlternating(t, storage, "metric", 30, 61, 5.0, 5.1)

	result := d.Detect(storage, 90)
	require.Len(t, result.Anomalies, 1, "mean shift must produce exactly one anomaly")

	a := result.Anomalies[0]
	assert.Contains(t, a.Title, "KLDivergence drift", "anomaly title should identify detector")
	assert.Equal(t, "kl_divergence", a.DetectorName)
	require.NotNil(t, a.Score, "score must be set")
	assert.Greater(t, *a.Score, 1.5, "score is the symmetric KL divergence and must clear the threshold")
	assert.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
	assert.InDelta(t, 0.0, a.DebugInfo.BaselineMedian, 0.1)
	assert.InDelta(t, 5.0, a.DebugInfo.CurrentValue, 0.1)
	assert.Greater(t, a.DebugInfo.DeviationSigma, 3.0, "deviation gate must be exceeded")
	assert.Equal(t, 1.5, a.DebugInfo.Threshold)
}

// TestKLDivergence_VarianceOnly_NoFire feeds a baseline that alternates
// in [-1, 1] followed by a wider test segment alternating in [-3, 3].
// Both segments have median ~ 0, so the |testMedian - refMedian| / MAD
// gate stays below threshold and no anomaly fires. This documents the
// detector's contract: the deviation gate (mirroring scanwelch) blocks
// pure variance shifts. A future change that strips the gate would also
// need to update this test — it locks the current behaviour.
func TestKLDivergence_VarianceOnly_NoFire(t *testing.T) {
	d := testKLDivergenceDetector()
	storage := newTimeSeriesStorage()

	// Reference window: 60 points alternating in [-1, 1].
	addAlternating(t, storage, "metric", 60, 1, -1.0, 1.0)
	// Test window: 30 points alternating in [-3, 3]. Same median, wider MAD.
	addAlternating(t, storage, "metric", 30, 61, -3.0, 3.0)

	result := d.Detect(storage, 90)
	assert.Empty(t, result.Anomalies, "pure variance shift must not fire — deviation gate should block it")
}

// TestKLDivergence_ConstantInput_NoFire feeds 100 identical values. The
// combined min/max collapse to a single value so the scan returns early
// without firing.
func TestKLDivergence_ConstantInput_NoFire(t *testing.T) {
	d := testKLDivergenceDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 100; i++ {
		storage.Add("ns", "metric", 7.0, int64(i+1), nil)
	}

	result := d.Detect(storage, 100)
	assert.Empty(t, result.Anomalies, "constant input has no divergence to score")
}

// TestKLDivergence_RefractoryAfterFire confirms that a detected drift is
// not re-reported on the next few advance cycles. After a fire the
// reference window is rebased to the post-shift distribution and a
// per-series refractory blocks the next TestWindow scans, so additional
// shifted points should produce no second anomaly.
func TestKLDivergence_RefractoryAfterFire(t *testing.T) {
	d := testKLDivergenceDetector()
	storage := newTimeSeriesStorage()

	addAlternating(t, storage, "metric", 60, 1, 0.0, 0.1)
	addAlternating(t, storage, "metric", 30, 61, 5.0, 5.1)

	r1 := d.Detect(storage, 90)
	require.Len(t, r1.Anomalies, 1, "first advance should fire on the shift")

	// Add 5 more points at the post-shift level — the buffer is far from
	// refilled, so no second fire is possible.
	addAlternating(t, storage, "metric", 5, 91, 5.0, 5.1)

	r2 := d.Detect(storage, 95)
	assert.Empty(t, r2.Anomalies, "should not re-fire while buffers are still refilling")
}

// TestKLDivergence_RemoveSeries verifies the SeriesRemover contract:
// after Detect populates per-series state, RemoveSeries must drop it so
// the detector's memory tracks storage's series cardinality.
func TestKLDivergence_RemoveSeries(t *testing.T) {
	d := testKLDivergenceDetector()
	storage := newTimeSeriesStorage()

	addAlternating(t, storage, "metric", 90, 1, 0.0, 0.1)
	d.Detect(storage, 90)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	// Find the SeriesRef that storage assigned.
	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})
	assert.Empty(t, d.series, "RemoveSeries must drop per-series state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries should invalidate the series cache")
}

// TestKLDivergence_DefaultsApplied confirms ensureDefaults populates a
// zero-valued detector struct so the catalog's reflective construction
// path (and any other caller that bypasses NewKLDivergenceDetector)
// still produces a usable detector.
func TestKLDivergence_DefaultsApplied(t *testing.T) {
	d := &KLDivergenceDetector{}
	storage := newTimeSeriesStorage()

	// Detect on empty storage just to drive ensureDefaults.
	_ = d.Detect(storage, 1)

	assert.Equal(t, 60, d.ReferenceWindow)
	assert.Equal(t, 30, d.TestWindow)
	assert.Equal(t, 16, d.NumBins)
	assert.Equal(t, 1.5, d.DivergenceThreshold)
	assert.Equal(t, 3.0, d.MinDeviationMAD)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestKLDivergence_Reset confirms that Reset wipes per-series state so a
// replay run starts fresh.
func TestKLDivergence_Reset(t *testing.T) {
	d := testKLDivergenceDetector()
	storage := newTimeSeriesStorage()

	addAlternating(t, storage, "metric", 90, 1, 0.0, 0.1)
	d.Detect(storage, 90)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}

// TestKLDivergence_Name pins the catalog identifier so the allowlist
// check in catalog tests stays in sync.
func TestKLDivergence_Name(t *testing.T) {
	assert.Equal(t, "kl_divergence", NewKLDivergenceDetector().Name())
}
