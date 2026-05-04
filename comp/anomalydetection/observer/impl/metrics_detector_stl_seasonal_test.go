// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"math/rand"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSTLDetector returns a detector configured for the single-aggregation
// tests below. Default Aggregations includes Count; pinning to Average keeps
// anomaly counts deterministic across the seasonal/nonseasonal scenarios.
func testSTLDetector() *STLSeasonalDetector {
	d := NewSTLSeasonalDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// TestSTL_Constant_NoFire feeds 500 identical values. Period auto-detection
// returns 0 (constant series → zero variance → no candidate lag), so the
// detector falls into the nonseasonal path with a residual gate that sees
// near-zero standardised residuals. No anomaly should fire.
func TestSTL_Constant_NoFire(t *testing.T) {
	d := testSTLDetector()
	storage := newTimeSeriesStorage()

	for ts := int64(1); ts <= 500; ts++ {
		storage.Add("ns", "metric", 7.0, ts, nil)
	}

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "constant input must not trigger STL seasonal")
}

// TestSTL_PureSinusoid_NoFire feeds 500 noise-free sinusoid points. ACF
// detects period 24; per-phase median seeding collapses every residual to
// zero, so no anomaly should fire. This pins the contract that a perfectly
// periodic baseline is recognised as "normal" — the regression class
// (703_shopify, exp-0023..28) the detector is designed to fix.
func TestSTL_PureSinusoid_NoFire(t *testing.T) {
	d := testSTLDetector()
	storage := newTimeSeriesStorage()

	const period = 24.0
	for ts := int64(1); ts <= 500; ts++ {
		v := 5.0 * math.Sin(2*math.Pi*float64(ts)/period)
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "noise-free sinusoid must not fire — STL should track the cycle")

	// Sanity: the state must show the period was actually detected; without
	// this the test would pass trivially via the nonseasonal fallback even
	// if ACF was broken.
	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	state := d.series[stlStateKey{ref: metas[0].Ref, agg: observer.AggregateAverage}]
	require.NotNil(t, state)
	assert.Equal(t, 24, state.period, "ACF should detect period 24 for a 24-point sinusoid")
}

// TestSTL_SinusoidWithSpike_FiresOnce feeds 500 sinusoid points punctuated
// by a 2-point boost at t=300,301. The first spike point arms the
// consecutive counter; the second confirms it and fires. The σ_value gate
// passes because the spike magnitude (20) dwarfs the sinusoid's MAD.
// Refractory suppresses any follow-on fire.
func TestSTL_SinusoidWithSpike_FiresOnce(t *testing.T) {
	d := testSTLDetector()
	storage := newTimeSeriesStorage()

	const (
		period     = 24.0
		spikeStart = int64(300)
		spikeLen   = 2
		spikeBoost = 20.0
	)

	for ts := int64(1); ts <= 500; ts++ {
		v := 5.0 * math.Sin(2*math.Pi*float64(ts)/period)
		if ts >= spikeStart && ts < spikeStart+spikeLen {
			v += spikeBoost
		}
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	require.Len(t, result.Anomalies, 1, "exactly one fire expected for a 2-point spike on a clean sinusoid")

	a := result.Anomalies[0]
	assert.Equal(t, "stl_seasonal", a.DetectorName)
	assert.Contains(t, a.Title, "STL seasonal")
	require.NotNil(t, a.Score)
	assert.Greater(t, *a.Score, 4.5, "score should clear the |z| threshold")
	assert.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
	assert.Equal(t, 4.5, a.DebugInfo.Threshold)
	// Fire timestamp lands on the second spike point (M=2 confirmation).
	assert.Equal(t, spikeStart+int64(spikeLen)-1, a.Timestamp)
}

// TestSTL_NonseasonalSpike_FiresOnce drives the nonseasonal fallback path:
// 300 points of i.i.d. Gaussian noise (no autocorrelation peak above
// MinACF) followed by a 2-point spike. The detector must declare the
// series nonseasonal and fall back to a robust-z gate on raw residuals
// (s_p ≡ 0). Exactly one anomaly should fire.
func TestSTL_NonseasonalSpike_FiresOnce(t *testing.T) {
	d := testSTLDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(42))
	const (
		spikeStart = int64(310)
		spikeLen   = 2
		spikeValue = 30.0
	)
	for ts := int64(1); ts <= 500; ts++ {
		v := 10.0 + 0.5*rng.NormFloat64()
		if ts >= spikeStart && ts < spikeStart+spikeLen {
			v = spikeValue
		}
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	require.Len(t, result.Anomalies, 1, "exactly one fire expected for a 2-point spike on Gaussian noise")
	a := result.Anomalies[0]
	assert.Equal(t, "stl_seasonal", a.DetectorName)
	assert.Equal(t, spikeStart+int64(spikeLen)-1, a.Timestamp)

	// Confirm the fallback path was actually taken — without this guard,
	// a future change to ACF that spuriously detects a period would still
	// pass this test trivially.
	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	state := d.series[stlStateKey{ref: metas[0].Ref, agg: observer.AggregateAverage}]
	require.NotNil(t, state)
	assert.Equal(t, 0, state.period, "i.i.d. Gaussian noise must classify as nonseasonal")
}

// TestSTL_PhaseRollingMedianResistsAnomaly verifies the Hampel-style
// rejection: recurring spikes at the same phase across multiple cycles
// must NOT poison seasonalIdx[p]. ConfirmM is dialled to 1 so single-point
// spikes can fire (and exercise the Hampel branch); the test then runs
// ~200 additional sinusoid cycles and asserts no extra fires beyond the
// spike events themselves.
//
// Without Hampel rejection the spike values would accumulate in
// phaseRings[p], pulling seasonalIdx[p] toward the spike magnitude and
// causing every subsequent normal point at phase p to look like a large
// negative deviation.
func TestSTL_PhaseRollingMedianResistsAnomaly(t *testing.T) {
	d := testSTLDetector()
	d.ConfirmM = 1 // single-point spikes fire, exercising fire-time Hampel.
	storage := newTimeSeriesStorage()

	const period = 24.0
	// Spikes spaced exactly one period apart so they all hit the same
	// logical phase. The first one (at t=300) lands while resWin is still
	// filling and so does NOT fire — but it does push the spike value into
	// phaseRings[p] (no Hampel rejection because !fire). The rolling
	// median over 6 samples absorbs that single outlier (5 of 6 still 0).
	spikeTimestamps := []int64{300, 324, 348, 372}
	spikeSet := map[int64]bool{}
	for _, ts := range spikeTimestamps {
		spikeSet[ts] = true
	}

	for ts := int64(1); ts <= 700; ts++ {
		v := 5.0 * math.Sin(2*math.Pi*float64(ts)/period)
		if spikeSet[ts] {
			v += 20.0
		}
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 700)

	// Every fire must coincide with a spike timestamp; no spurious fires
	// from a poisoned seasonalIdx may slip through after t=372.
	for _, a := range result.Anomalies {
		assert.Contains(t, spikeTimestamps, a.Timestamp,
			"unexpected fire at ts=%d (poisoned seasonalIdx?)", a.Timestamp)
	}

	// Sanity: the post-spike normal cycles cover phase 11 many times
	// (t=396, 420, 444, 468, 492, 516, 540, 564, 588, 612, 636, 660, 684).
	// All of those should be free of anomalies if the rolling median
	// resisted the spikes.
	postSpikeFires := 0
	for _, a := range result.Anomalies {
		if a.Timestamp > spikeTimestamps[len(spikeTimestamps)-1] {
			postSpikeFires++
		}
	}
	assert.Equal(t, 0, postSpikeFires,
		"no fires expected after the spike sequence — a poisoned seasonalIdx would cause every phase-aligned normal point to fire")
}

// TestSTL_RemoveSeries verifies the SeriesRemover contract end to end.
// Two series feed Detect; RemoveSeries on one ref must drop the
// corresponding entries for BOTH aggregations and leave the other
// series's state intact.
func TestSTL_RemoveSeries(t *testing.T) {
	d := NewSTLSeasonalDetector() // keep both default aggregations.
	storage := newTimeSeriesStorage()

	for ts := int64(1); ts <= 100; ts++ {
		storage.Add("ns", "metric_a", 1.0, ts, nil)
		storage.Add("ns", "metric_b", 2.0, ts, nil)
	}
	d.Detect(storage, 100)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 2)
	var refA, refB observer.SeriesRef
	for _, m := range metas {
		switch m.Name {
		case "metric_a":
			refA = m.Ref
		case "metric_b":
			refB = m.Ref
		}
	}

	// Both aggregations of refA must have state before removal.
	for _, agg := range d.Aggregations {
		_, ok := d.series[stlStateKey{ref: refA, agg: agg}]
		require.True(t, ok, "expected pre-removal state for refA agg=%v", agg)
	}

	d.RemoveSeries([]observer.SeriesRef{refA})

	for _, agg := range d.Aggregations {
		_, gone := d.series[stlStateKey{ref: refA, agg: agg}]
		assert.False(t, gone, "RemoveSeries must drop refA agg=%v", agg)
		_, kept := d.series[stlStateKey{ref: refB, agg: agg}]
		assert.True(t, kept, "RemoveSeries must NOT drop refB agg=%v", agg)
	}
	assert.Nil(t, d.cachedSeries, "RemoveSeries should invalidate the series cache")
}

// TestSTL_Reset confirms that Reset wipes per-series state.
func TestSTL_Reset(t *testing.T) {
	d := testSTLDetector()
	storage := newTimeSeriesStorage()

	for ts := int64(1); ts <= 100; ts++ {
		storage.Add("ns", "metric", 1.0, ts, nil)
	}
	d.Detect(storage, 100)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}

// TestSTL_DefaultsApplied confirms ensureDefaults populates a zero-valued
// struct so reflective construction (and any caller that bypasses
// NewSTLSeasonalDetector) still produces a usable detector.
func TestSTL_DefaultsApplied(t *testing.T) {
	d := &STLSeasonalDetector{}
	storage := newTimeSeriesStorage()

	_ = d.Detect(storage, 1)

	assert.Equal(t, 240, d.WarmupPoints)
	assert.Equal(t, 0.3, d.MinACF)
	assert.Equal(t, 4, d.MinPeriod)
	assert.Equal(t, 120, d.MaxPeriod)
	assert.Equal(t, 6, d.PerPhaseHistory)
	assert.Equal(t, 60, d.ResidualWindow)
	assert.Equal(t, 4.5, d.ZThreshold)
	assert.Equal(t, 2, d.ConfirmM)
	assert.Equal(t, 3.0, d.MinDeviationMAD)
	assert.Equal(t, 20, d.Refractory)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestSTL_Name pins the catalog identifier.
func TestSTL_Name(t *testing.T) {
	assert.Equal(t, "stl_seasonal", NewSTLSeasonalDetector().Name())
}

// TestSTL_InterfaceContracts checks the structural promises that the
// catalog and engine both rely on: STLSeasonalDetector must satisfy
// observer.Detector AND manualSeriesRemover (it is stateful and is NOT
// listed in statelessDetectorAllowlist).
func TestSTL_InterfaceContracts(t *testing.T) {
	d := NewSTLSeasonalDetector()
	var _ observer.Detector = d
	var _ manualSeriesRemover = d
}

// TestSTL_AutoDetectPeriod_PicksDominantLag is a unit test on the ACF
// helper. A pure cosine of period 12 should be recovered within the
// configured search range; flat data should return 0.
func TestSTL_AutoDetectPeriod_PicksDominantLag(t *testing.T) {
	const n = 240
	buf := make([]float64, n)
	for i := 0; i < n; i++ {
		buf[i] = math.Cos(2 * math.Pi * float64(i) / 12.0)
	}
	got := autoDetectPeriod(buf, stlMinPeriod, stlMaxPeriod, stlMinACF)
	assert.Equal(t, 12, got)

	// Constant input → no candidate.
	flat := make([]float64, n)
	for i := range flat {
		flat[i] = 3.14
	}
	assert.Equal(t, 0, autoDetectPeriod(flat, stlMinPeriod, stlMaxPeriod, stlMinACF))
}
