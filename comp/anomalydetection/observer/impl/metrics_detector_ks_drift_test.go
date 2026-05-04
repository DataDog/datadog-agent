// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math/rand"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKSDriftDetector returns a detector configured for the single-aggregation
// tests below. Default Aggregations includes Count, but for these unit tests
// we only seed Average and asserting on a single aggregation keeps anomaly
// counts deterministic.
func testKSDriftDetector() *KSDriftDetector {
	d := NewKSDriftDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// addNormal seeds count points sampled from Normal(mu, sigma) into storage at
// consecutive timestamps starting at startTs. rng is required so callers can
// pin the seed and keep the test deterministic.
func addNormal(t *testing.T, storage *timeSeriesStorage, name string, count int, startTs int64, rng *rand.Rand, mu, sigma float64) {
	t.Helper()
	for i := 0; i < count; i++ {
		v := rng.NormFloat64()*sigma + mu
		storage.Add("ns", name, v, startTs+int64(i), nil)
	}
}

// TestKSDrift_Constant_NoFire feeds 200 identical points. The reference and
// test windows hold equal-valued samples so the empirical CDFs collapse to
// a single step at the same point and the KS statistic is exactly zero.
func TestKSDrift_Constant_NoFire(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 200; i++ {
		storage.Add("ns", "metric", 7.0, int64(i+1), nil)
	}

	result := d.Detect(storage, 200)
	assert.Empty(t, result.Anomalies, "constant input has no distributional drift")
}

// TestKSDrift_PureNoise_NoFire feeds 200 standard normal samples with a fixed
// seed. The 60-sample reference and 30-sample test windows are drawn from
// the same distribution, so the KS statistic stays well below the 0.55
// threshold and no anomaly fires. Asymptotic critical KS at n=60/m=30 and
// alpha=1e-4 is ~0.46 — 0.55 is deliberately above that for safety.
func TestKSDrift_PureNoise_NoFire(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(42))

	addNormal(t, storage, "metric", 200, 1, rng, 0.0, 1.0)

	result := d.Detect(storage, 200)
	assert.Empty(t, result.Anomalies, "stationary noise must not trip the KS gate")
}

// TestKSDrift_LevelShift_Fires feeds 60 N(0,1) baseline points followed by
// 30 N(5,1) shifted points. The empirical CDFs are essentially disjoint so
// the KS statistic approaches 1.0 and the MAD-deviation gate also passes.
// Exactly one anomaly is expected.
func TestKSDrift_LevelShift_Fires(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(1))

	addNormal(t, storage, "metric", 60, 1, rng, 0.0, 1.0)
	addNormal(t, storage, "metric", 30, 61, rng, 5.0, 1.0)

	result := d.Detect(storage, 90)
	require.Len(t, result.Anomalies, 1, "level shift must produce exactly one anomaly")

	a := result.Anomalies[0]
	assert.Contains(t, a.Title, "KS drift", "anomaly title should identify detector")
	assert.Equal(t, "ks_drift", a.DetectorName)
	require.NotNil(t, a.Score, "score must be set")
	assert.GreaterOrEqual(t, *a.Score, 0.55, "score is the KS statistic and must clear the threshold")
	assert.LessOrEqual(t, *a.Score, 1.0, "KS statistic is bounded by 1")
	assert.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
	assert.InDelta(t, 0.0, a.DebugInfo.BaselineMedian, 0.5)
	assert.Greater(t, a.DebugInfo.DeviationSigma, 3.0, "deviation gate must be exceeded")
	assert.Equal(t, 0.55, a.DebugInfo.Threshold)
}

// TestKSDrift_ShapeShift_Fires feeds a baseline N(0,1) followed by a wider
// N(4,3) test segment. The plan calls this case "VarianceShift" but a pure
// variance shift would be blocked by the MAD-deviation gate (mirroring
// KLDivergence's VarianceOnly_NoFire). The post-shift mean of 4 keeps the
// gate engaged with margin (devMAD ≈ 4/0.67 ≈ 6) while the 3x variance
// produces wider tails — this is exactly the case where KL's fixed bin
// grid can be drowned by the larger-range bin width but KS's continuous
// CDF still picks up the shape change reliably.
func TestKSDrift_ShapeShift_Fires(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(7))

	addNormal(t, storage, "metric", 60, 1, rng, 0.0, 1.0)
	addNormal(t, storage, "metric", 30, 61, rng, 4.0, 3.0)

	result := d.Detect(storage, 90)
	require.Len(t, result.Anomalies, 1, "shape+location shift must fire")

	a := result.Anomalies[0]
	require.NotNil(t, a.Score, "score must be set")
	assert.GreaterOrEqual(t, *a.Score, 0.55, "KS statistic must clear threshold")
}

// TestKSDrift_RefractoryBlocksRepeat confirms that a detected drift is not
// re-reported on the next few advance cycles. After a fire the reference
// window is rebased to the post-shift distribution and a per-series
// refractory blocks the next TestWindow scans, so additional shifted points
// produce no second anomaly.
func TestKSDrift_RefractoryBlocksRepeat(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(11))

	addNormal(t, storage, "metric", 60, 1, rng, 0.0, 1.0)
	addNormal(t, storage, "metric", 30, 61, rng, 5.0, 1.0)

	r1 := d.Detect(storage, 90)
	require.Len(t, r1.Anomalies, 1, "first advance should fire on the shift")

	// Add 5 more post-shift points — buffer is far from refilled, so no
	// second fire is possible.
	addNormal(t, storage, "metric", 5, 91, rng, 5.0, 1.0)

	r2 := d.Detect(storage, 95)
	assert.Empty(t, r2.Anomalies, "should not re-fire while buffers are still refilling")
}

// TestKSDrift_RemoveSeries verifies the SeriesRemover contract: after Detect
// populates per-series state, RemoveSeries must drop it so the detector's
// memory tracks storage's series cardinality.
func TestKSDrift_RemoveSeries(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(2))

	addNormal(t, storage, "metric", 90, 1, rng, 0.0, 1.0)
	d.Detect(storage, 90)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})
	assert.Empty(t, d.series, "RemoveSeries must drop per-series state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries should invalidate the series cache")
}

// TestKSDrift_WarmupNoFireUntilFull feeds fewer than ReferenceWindow+TestWindow
// points and verifies the detector withholds anomalies until both windows are
// full. This locks the warmup contract: a transient distribution change
// during the early stages of a series shouldn't fire before the detector has
// a stable reference baseline.
func TestKSDrift_WarmupNoFireUntilFull(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(3))

	// 30 baseline + 30 shifted = 60 points; less than the 90-point warmup
	// requirement (60 ref + 30 test). The test buffer is full but the ref
	// buffer only holds 30 points, so the buffers-full guard blocks the scan.
	addNormal(t, storage, "metric", 30, 1, rng, 0.0, 1.0)
	addNormal(t, storage, "metric", 30, 31, rng, 10.0, 1.0)

	result := d.Detect(storage, 60)
	assert.Empty(t, result.Anomalies, "must not fire before the reference window is full")

	// Confirm state advanced — refLen should equal TestWindow because the
	// first 30 points rolled into ref while the second 30 sat in test.
	require.Len(t, d.series, 1)
	for _, st := range d.series {
		assert.Equal(t, 30, st.refLen, "first 30 points should have rolled into the ref window")
		assert.Equal(t, 30, st.testLen, "test window should be full")
	}
}

// TestKSDrift_DefaultsApplied confirms ensureDefaults populates a zero-valued
// detector struct so the catalog's reflective construction path (and any
// other caller that bypasses NewKSDriftDetector) still produces a usable
// detector.
func TestKSDrift_DefaultsApplied(t *testing.T) {
	d := &KSDriftDetector{}
	storage := newTimeSeriesStorage()

	// Detect on empty storage just to drive ensureDefaults.
	_ = d.Detect(storage, 1)

	assert.Equal(t, 60, d.ReferenceWindow)
	assert.Equal(t, 30, d.TestWindow)
	assert.Equal(t, 0.55, d.KSThreshold)
	assert.Equal(t, 3.0, d.MinDeviationMAD)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestKSDrift_Reset confirms that Reset wipes per-series state so a replay
// run starts fresh.
func TestKSDrift_Reset(t *testing.T) {
	d := testKSDriftDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(4))

	addNormal(t, storage, "metric", 90, 1, rng, 0.0, 1.0)
	d.Detect(storage, 90)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}

// TestKSDrift_Name pins the catalog identifier so the allowlist check in
// catalog tests stays in sync.
func TestKSDrift_Name(t *testing.T) {
	assert.Equal(t, "ks_drift", NewKSDriftDetector().Name())
}
