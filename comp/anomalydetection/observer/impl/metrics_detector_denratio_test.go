// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// testDenRatioDetector returns a detector restricted to AggregateAverage so
// each test only sees one state entry per series — keeps anomaly-count
// assertions unambiguous (a duplicate Count anomaly here would otherwise mask
// real false positives).
func testDenRatioDetector() *DenRatioDetector {
	d := NewDenRatioDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// genGaussian returns n N(mean, stddev²) samples from rng.
func genGaussian(rng *rand.Rand, n int, mean, stddev float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = mean + stddev*rng.NormFloat64()
	}
	return out
}

// genBimodal returns n samples from a 50/50 mixture of N(-3, 1) and N(3, 1).
// Same marginal mean (0) and unit-ish variance as N(0,1) at the moments
// level, so a mean/variance detector won't see it; the multimodal shape is
// exactly what density-ratio is supposed to catch.
func genBimodal(rng *rand.Rand, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		if rng.Float64() < 0.5 {
			out[i] = -3 + rng.NormFloat64()
		} else {
			out[i] = 3 + rng.NormFloat64()
		}
	}
	return out
}

// feedDenRatioSeries appends values to a fresh storage with consecutive
// timestamps starting at t=1, then runs Detect once at the final timestamp.
// We add a positive offset so storage has comfortably positive values; the
// PE divergence is invariant to translation so this doesn't perturb the test.
func feedDenRatioSeries(t *testing.T, d *DenRatioDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

// TestDenRatio_Name documents the catalog identifier the detector returns.
// The catalog entry's `name` field and the detector's Name() return value
// must agree because reporters key off DetectorName.
func TestDenRatio_Name(t *testing.T) {
	d := NewDenRatioDetector()
	assert.Equal(t, "denratio", d.Name())
}

// TestDenRatio_FlatSeriesNoFire: 600 N(0,1) → 0 anomalies. PE between two
// independent samples of the same distribution is small (chi-square noise
// floor) and stays well below the 0.30 threshold.
func TestDenRatio_FlatSeriesNoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := genGaussian(rng, 600, 0, 1)

	d := testDenRatioDetector()
	result := feedDenRatioSeries(t, d, "flat", values)

	assert.Empty(t, result.Anomalies, "i.i.d. N(0,1) should not trigger any denratio anomaly")
}

// TestDenRatio_VarianceShift: 300 N(0,1) + 300 N(0,9) (same mean!) → exactly
// 1 anomaly. This is the case BOCPD/scan miss: distribution shape changes
// (variance triples) but median stays at 0. PE divergence + the MAD-aware
// gate must catch it.
func TestDenRatio_VarianceShift(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := append(genGaussian(rng, 300, 0, 1), genGaussian(rng, 300, 0, 3)...)

	d := testDenRatioDetector()
	result := feedDenRatioSeries(t, d, "variance_shift", values)

	require.Len(t, result.Anomalies, 1, "variance shift (same mean) should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "denratio", a.DetectorName)
	assert.Contains(t, a.Title, "DenRatio")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.DeviationSigma, 0.30,
		"PE at trigger must clear DivThreshold")
	// Trigger must occur after the regime change at index 300, with slack
	// for the windows to fill enough that PE clears 0.30 for 3 ticks.
	assert.Greater(t, a.Timestamp, int64(300), "anomaly must be in the post-shift regime")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestDenRatio_MeanShift: 300 N(0,1) + 300 N(3,1) → exactly 1 anomaly. Mean
// shift is the easy case (any reasonable detector should catch it); included
// as a sanity check that the gate doesn't accidentally suppress the textbook
// scenario.
func TestDenRatio_MeanShift(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := append(genGaussian(rng, 300, 0, 1), genGaussian(rng, 300, 3, 1)...)

	d := testDenRatioDetector()
	result := feedDenRatioSeries(t, d, "mean_shift", values)

	require.Len(t, result.Anomalies, 1, "mean shift should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "denratio", a.DetectorName)
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.DeviationSigma, 0.30)
	assert.Greater(t, a.Timestamp, int64(300))
}

// TestDenRatio_BimodalShift: 300 unimodal N(0,1) followed by 300 samples
// from a 50/50 mixture of N(-3,1) and N(3,1). Marginal mean is zero in both
// regimes; the change is purely in modality. Density-ratio's full-
// distribution comparison should pick it up.
func TestDenRatio_BimodalShift(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := append(genGaussian(rng, 300, 0, 1), genBimodal(rng, 300)...)

	d := testDenRatioDetector()
	result := feedDenRatioSeries(t, d, "bimodal_shift", values)

	require.Len(t, result.Anomalies, 1, "unimodal→bimodal shift should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "denratio", a.DetectorName)
	require.NotNil(t, a.DebugInfo)
	assert.Greater(t, a.Timestamp, int64(300))
}

// TestDenRatio_NoFireOnConstant: 600 identical values → 0 anomalies. The
// range-zero guard must exit early without computing histograms over a
// degenerate range.
func TestDenRatio_NoFireOnConstant(t *testing.T) {
	values := make([]float64, 600)
	for i := range values {
		values[i] = 7.0
	}

	d := testDenRatioDetector()
	result := feedDenRatioSeries(t, d, "constant", values)

	assert.Empty(t, result.Anomalies, "constant signal must not trigger denratio")
}

// TestDenRatio_RecoveryPrevents_DoubleFire: a single sustained mean shift
// must produce exactly ONE anomaly even though it persists for 300 post-shift
// ticks. The post-fire structural reset (T zeroed, R copied from T) plus the
// recovery counter together prevent re-firing on the same incident.
func TestDenRatio_RecoveryPrevents_DoubleFire(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := append(genGaussian(rng, 300, 0, 1), genGaussian(rng, 300, 5, 1)...)

	d := testDenRatioDetector()
	result := feedDenRatioSeries(t, d, "double_fire_check", values)

	require.Len(t, result.Anomalies, 1,
		"a single sustained shift must not produce repeat anomalies during the recovery+refill window")
}

// TestDenRatio_RemoveSeries verifies that RemoveSeries shrinks the per-series
// state map — the SeriesRemover contract that keeps detector-side memory in
// step with storage eviction. Each entry holds ~1.9 KB of fixed-size
// streaming state, so without this teardown the map would grow with the
// cumulative count of series ever observed.
func TestDenRatio_RemoveSeries(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	values := genGaussian(rng, 200, 0, 1)

	d := testDenRatioDetector()
	storage := newTimeSeriesStorage()
	for i, v := range values {
		storage.Add("ns", "metric", 100+v, int64(i+1), nil)
	}
	d.Detect(storage, int64(len(values)))
	require.NotEmpty(t, d.series, "detector must record per-series state during Detect")

	var refs []observer.SeriesRef
	for k := range d.series {
		refs = append(refs, k.ref)
	}
	d.RemoveSeries(refs)
	assert.Empty(t, d.series, "RemoveSeries must drop state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries must invalidate cachedSeries")
}

// TestDenRatio_Reset verifies that Reset clears all per-series state.
func TestDenRatio_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	values := genGaussian(rng, 80, 0, 1)

	d := testDenRatioDetector()
	feedDenRatioSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
	assert.Nil(t, d.cachedSeries, "reset should clear cached series")
}

// TestDenRatio_ColdStart_NoFire_BelowWarmup verifies that during the [0, 2W)
// warmup region (rings still filling) the detector emits zero anomalies even
// for sequences that would otherwise fire. This documents the warmup contract
// — first PE can be computed only after both rings are full.
func TestDenRatio_ColdStart_NoFire_BelowWarmup(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// Strong shift in the middle of the ring-warmup region. With W=60, both
	// rings are full only at index 2W=120; emitting before that is a bug.
	values := append(genGaussian(rng, 60, 0, 1), genGaussian(rng, 30, 10, 1)...)

	d := testDenRatioDetector()
	result := feedDenRatioSeries(t, d, "cold_start", values)

	assert.Empty(t, result.Anomalies,
		"detector must not emit before both rings are full (cold-start contract)")
}

// TestComputePE_Symmetric documents that the discrete PE divergence is
// symmetric in R and T to within numerical precision. This is a property of
// the α=0.5 form and matters for the histogram approximation: an asymmetric
// kernel would give different PEs depending on which window arrived first.
func TestComputePE_Symmetric(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	r := genGaussian(rng, denratioWindow, 0, 1)
	tt := genGaussian(rng, denratioWindow, 1, 2)
	// Compute lo/hi the way computePE expects them.
	lo := math.Inf(1)
	hi := math.Inf(-1)
	for _, v := range append(append([]float64{}, r...), tt...) {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	histR := make([]float64, denratioNumBins)
	histT := make([]float64, denratioNumBins)
	peRT := computePE(r, tt, histR, histT, lo, hi)
	peTR := computePE(tt, r, histR, histT, lo, hi)
	assert.InDelta(t, peRT, peTR, 1e-12, "PE(R,T) must equal PE(T,R) under the α=0.5 form")
}

// TestComputePE_ZeroOnIdentical documents that PE between two identical
// windows is exactly zero (modulo the ε floor in the denominator, which only
// affects empty bins and cancels here because diff² is zero).
func TestComputePE_ZeroOnIdentical(t *testing.T) {
	r := make([]float64, denratioWindow)
	for i := range r {
		r[i] = float64(i)
	}
	tt := make([]float64, denratioWindow)
	copy(tt, r)
	histR := make([]float64, denratioNumBins)
	histT := make([]float64, denratioNumBins)
	pe := computePE(r, tt, histR, histT, 0, float64(denratioWindow-1))
	assert.InDelta(t, 0.0, pe, 1e-12, "PE between identical windows must be 0")
}

// TestUnionRange_Basic exercises the min/max scan that drives histogram
// bounds. Both rings must contribute to the union; we put the global min in
// R and the global max in T.
func TestUnionRange_Basic(t *testing.T) {
	r := []float64{2, 3, -1, 4, 5}
	tt := []float64{1, 2, 3, 9, 0}
	lo, hi := unionRange(r, tt)
	assert.Equal(t, -1.0, lo)
	assert.Equal(t, 9.0, hi)
}
