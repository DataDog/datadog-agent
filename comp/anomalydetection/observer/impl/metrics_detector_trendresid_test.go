// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// testTrendResidDetector returns a detector restricted to AggregateAverage so
// each test only sees one state entry per series — keeps anomaly-count
// assertions unambiguous (a duplicate Count anomaly here would otherwise mask
// real false positives, mirroring the pattern in the AcorrShift/DenRatio
// tests).
func testTrendResidDetector() *TrendResidDetector {
	d := NewTrendResidDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedTrendResidSeries appends values to a fresh storage with consecutive
// timestamps starting at t=1, then runs Detect once at the final timestamp.
// A positive offset puts the values comfortably above zero; the algorithm is
// invariant to translation of the y-axis (slope and residuals are unchanged).
func feedTrendResidSeries(t *testing.T, d *TrendResidDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

// genTrendResidNoise returns n i.i.d. N(0,1) samples.
func genTrendResidNoise(rng *rand.Rand, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = rng.NormFloat64()
	}
	return out
}

// TestTrendResid_Name documents the catalog identifier the detector returns.
// The catalog entry's `name` field and the detector's Name() must agree
// because reporters key off DetectorName.
func TestTrendResid_Name(t *testing.T) {
	d := NewTrendResidDetector()
	assert.Equal(t, "trendresid", d.Name())
}

// TestTrendResid_CatalogEntryRegistered guards the catalog wiring of stage 1.
// The structural teardown contract test elsewhere checks every detector has a
// SeriesRemover; this test specifically verifies the trendresid entry exists,
// is enabled by default, and its factory produces a TrendResidDetector — so
// stage-1 registration regressions surface here, not in catalog-wide failures.
func TestTrendResid_CatalogEntryRegistered(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "trendresid" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "trendresid must be registered in the default catalog")
	assert.Equal(t, componentDetector, found.kind, "trendresid must be a detector")
	assert.False(t, found.defaultEnabled, "manual corpus candidates should be --only addressable, not default-enabled")
	require.NotNil(t, found.factory, "trendresid must have a factory")

	instance := found.factory(found.defaultConfig)
	d, ok := instance.(*TrendResidDetector)
	require.True(t, ok, "factory must produce *TrendResidDetector")
	assert.Equal(t, "trendresid", d.Name())
}

// TestTrendResid_FlatNoise_NoFire: 600 i.i.d. N(0,1) → 0 anomalies. Slope is
// near zero, so the trend strength gate stays well below 0.5 even on stray
// large residuals — the additivity gate that keeps stationary series in
// ScanMW/BOCPD's territory.
func TestTrendResid_FlatNoise_NoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := genTrendResidNoise(rng, 600)

	d := testTrendResidDetector()
	result := feedTrendResidSeries(t, d, "flat_noise", values)

	assert.Empty(t, result.Anomalies, "i.i.d. N(0,1) should not trigger any trendresid anomaly")
}

// TestTrendResid_LinearTrend_NoFire: y = 1.0·t + N(0,1) for 600 ticks. Slope
// is real and the trend strength gate is satisfied throughout, but residuals
// stay near σ — well below ResidualK=3.5 — so no break is detected. (Slope
// chosen to match TrendBreakFires below; if a steady trend at the same slope
// fired, we'd suspect the break test was passing by accident.)
func TestTrendResid_LinearTrend_NoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const n = 600
	values := make([]float64, n)
	for i := 0; i < n; i++ {
		values[i] = 1.0*float64(i) + rng.NormFloat64()
	}

	d := testTrendResidDetector()
	result := feedTrendResidSeries(t, d, "linear_trend", values)

	assert.Empty(t, result.Anomalies, "steady linear trend with no break should not fire")
}

// TestTrendResid_TrendBreakFires: y = 1.0·t + N(0,1) for 300 ticks, then a
// flat plateau at y(t)=300 + N(0,1) for the next 300 ticks (smooth kink, no
// level discontinuity). Once the rolling window straddles the kink, the
// linear fit's slope drops to roughly half the pre-break value while the
// plateau stays flat — geometrically this drives the residual at the most
// recent (plateau) point to ≈ slope·W/8 ≈ 7.5σ for these constants, which
// clears ResidualK=3.5σ for several consecutive ticks. Expect exactly one
// anomaly somewhere in the post-break regime.
//
// Plan deviation: the plan specified slope=0.05 with N(0,1) noise. With
// slope-to-noise that small, the deterministic residual at the kink peaks
// around 0.37 — well below 3.5σ — so the literal plan numbers would have
// produced zero anomalies regardless of detector correctness. Slope=1.0
// preserves the plan's geometric setup (trend → smooth kink → plateau) while
// scaling the signal above the persistence threshold so the test exercises
// the trigger path the algorithm is designed to hit.
func TestTrendResid_TrendBreakFires(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const (
		preN  = 300
		postN = 300
		slope = 1.0
	)
	values := make([]float64, preN+postN)
	for i := 0; i < preN; i++ {
		values[i] = slope*float64(i) + rng.NormFloat64()
	}
	plateau := slope * float64(preN)
	for i := 0; i < postN; i++ {
		values[preN+i] = plateau + rng.NormFloat64()
	}

	d := testTrendResidDetector()
	result := feedTrendResidSeries(t, d, "trend_break", values)

	require.Len(t, result.Anomalies, 1, "trend-to-flat regime change should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "trendresid", a.DetectorName)
	assert.Contains(t, a.Title, "TrendResid")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.DeviationSigma, 3.5,
		"|residual|/σ at trigger must clear ResidualK")
	// Trigger must be in the post-break regime. Bound at preN+1 so any
	// non-causal mis-anchoring of the fit shows up here, not as a flake.
	assert.Greater(t, a.Timestamp, int64(preN), "anomaly timestamp must be after the regime change")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestTrendResid_StationaryShift_DoesNotFire: 300 N(0,1) followed by 300
// N(5,1) — a pure level shift with no preceding trend. Slope is ~0 throughout
// the pre-shift window and stays small even when straddling the boundary, so
// the trend-strength gate blocks emission. This is ScanMW's territory and the
// additivity gate must keep TrendResid out of it.
func TestTrendResid_StationaryShift_DoesNotFire(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	const preN, postN = 300, 300
	values := make([]float64, preN+postN)
	for i := 0; i < preN; i++ {
		values[i] = rng.NormFloat64()
	}
	for i := 0; i < postN; i++ {
		values[preN+i] = 5 + rng.NormFloat64()
	}

	d := testTrendResidDetector()
	result := feedTrendResidSeries(t, d, "level_shift", values)

	assert.Empty(t, result.Anomalies,
		"level shift with no preceding trend must not fire — that's ScanMW/BOCPD territory")
}

// TestTrendResid_RemoveSeries_FreesState verifies that RemoveSeries shrinks
// the per-series state map — the SeriesRemover contract that keeps detector-
// side memory in step with storage eviction.
func TestTrendResid_RemoveSeries_FreesState(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := genTrendResidNoise(rng, 100)

	d := testTrendResidDetector()
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

// TestTrendResid_Reset verifies that Reset clears all per-series state.
// Mirrors TestACorrShift_Reset; cheap insurance that the replay path keeps
// working.
func TestTrendResid_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := genTrendResidNoise(rng, 80)

	d := testTrendResidDetector()
	feedTrendResidSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset should clear all state")
	assert.Nil(t, d.cachedSeries, "Reset should clear cached series")
}
