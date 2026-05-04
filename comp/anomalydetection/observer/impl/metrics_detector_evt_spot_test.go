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

// testEVTSpotDetector returns a detector configured for the
// single-aggregation tests below. Default Aggregations includes Count;
// pinning to Average keeps anomaly counts deterministic.
func testEVTSpotDetector() *EVTSpotDetector {
	d := NewEVTSpotDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// TestEVTSpot_Constant_NoFire feeds 500 identical values. The empirical
// excess set after calibration is empty (no value strictly exceeds the
// quantile threshold tInit), so the GPD never fits and zQ stays at the
// safe-large default. No anomaly should be emitted.
func TestEVTSpot_Constant_NoFire(t *testing.T) {
	d := testEVTSpotDetector()
	storage := newTimeSeriesStorage()

	addConstant(t, storage, "metric", 500, 1, 7.0)

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "constant input must not trigger EVT-SPOT")
}

// TestEVTSpot_Gaussian_NoFire feeds 1000 N(0,1) points. With QAlarm=1e-4
// and N=1000 the expected number of fires is N*q = 0.1, so we tolerate at
// most one. The RNG is seeded for determinism.
func TestEVTSpot_Gaussian_NoFire(t *testing.T) {
	d := testEVTSpotDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(1))
	for ts := int64(1); ts <= 1000; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 1000)
	assert.LessOrEqual(t, len(result.Anomalies), 1, "stationary Gaussian must not produce more than one false fire at q=1e-4")
}

// TestEVTSpot_GaussianPlusOutlier_FiresOnce feeds 500 N(0,1) points with
// a single value of 10.0 injected near t=400. The detector must fire
// exactly once with DetectorName=="evt_spot" and Timestamp=400.
func TestEVTSpot_GaussianPlusOutlier_FiresOnce(t *testing.T) {
	d := testEVTSpotDetector()
	storage := newTimeSeriesStorage()

	const outlierTs int64 = 400
	const outlierValue = 10.0

	rng := rand.New(rand.NewSource(2))
	for ts := int64(1); ts <= 500; ts++ {
		v := rng.NormFloat64()
		if ts == outlierTs {
			v = outlierValue
		}
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	require.Len(t, result.Anomalies, 1, "exactly one fire expected for a 10σ outlier on N(0,1) noise")

	a := result.Anomalies[0]
	assert.Equal(t, "evt_spot", a.DetectorName)
	assert.Contains(t, a.Title, "EVT-SPOT")
	assert.Equal(t, outlierTs, a.Timestamp, "fire timestamp must match the outlier")
	require.NotNil(t, a.Score, "score must be populated")
	assert.Greater(t, *a.Score, 0.0, "score is xPrime - zQ; must be positive on fire")
	assert.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
}

// TestEVTSpot_DriftingMean_NoFalseFire feeds 500 points whose mean drifts
// linearly from 0 to 100 plus N(0,1) noise. DSPOT subtracts the rolling
// mean before the EVT gate, so the slow drift should be stripped before
// the threshold check and no anomaly should fire.
func TestEVTSpot_DriftingMean_NoFalseFire(t *testing.T) {
	d := testEVTSpotDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(3))
	for ts := int64(1); ts <= 500; ts++ {
		drift := 100.0 * float64(ts) / 500.0
		v := drift + rng.NormFloat64()
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "DSPOT must detrend slow drift; no fire expected")
}

// TestEVTSpot_Refractory feeds three consecutive 10.0 outliers on a
// stationary Gaussian baseline. The first should fire; the next two are
// suppressed by the refractory period. The series resumes baseline noise
// afterwards so the cursor reaches the trailing points.
func TestEVTSpot_Refractory(t *testing.T) {
	d := testEVTSpotDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(4))
	for ts := int64(1); ts <= 500; ts++ {
		var v float64
		switch {
		case ts == 400 || ts == 401 || ts == 402:
			v = 10.0
		default:
			v = rng.NormFloat64()
		}
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	require.Len(t, result.Anomalies, 1, "refractory must suppress the second and third consecutive outliers")
	assert.Equal(t, int64(400), result.Anomalies[0].Timestamp, "first outlier should be the one that fires")
}

// TestEVTSpot_RemoveSeries verifies the SeriesRemover contract: after
// Detect populates per-series state, RemoveSeries must drop it so the
// detector's memory tracks storage's series cardinality.
func TestEVTSpot_RemoveSeries(t *testing.T) {
	d := testEVTSpotDetector()
	storage := newTimeSeriesStorage()

	addConstant(t, storage, "metric", 100, 1, 1.0)
	d.Detect(storage, 100)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})
	assert.Empty(t, d.series, "RemoveSeries must drop per-series state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries should invalidate the series cache")
}

// TestEVTSpot_Reset confirms that Reset wipes per-series state.
func TestEVTSpot_Reset(t *testing.T) {
	d := testEVTSpotDetector()
	storage := newTimeSeriesStorage()

	addConstant(t, storage, "metric", 100, 1, 1.0)
	d.Detect(storage, 100)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}

// TestEVTSpot_DefaultsApplied confirms ensureDefaults populates a
// zero-valued struct so reflective construction (and any caller that
// bypasses NewEVTSpotDetector) still produces a usable detector.
func TestEVTSpot_DefaultsApplied(t *testing.T) {
	d := &EVTSpotDetector{}
	storage := newTimeSeriesStorage()

	_ = d.Detect(storage, 1)

	assert.Equal(t, 200, d.CalibrationSize)
	assert.InDelta(t, 0.02, d.QInit, 1e-12)
	assert.InDelta(t, 1e-4, d.QAlarm, 1e-12)
	assert.Equal(t, 50, d.MaxExcesses)
	assert.Equal(t, 10, d.RefitEvery)
	assert.Equal(t, 30, d.DriftWindow)
	assert.InDelta(t, 3.0, d.MinDeviationMAD, 1e-12)
	assert.Equal(t, 24, d.Refractory)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestEVTSpot_Name pins the catalog identifier so the catalog test stays
// in sync with the detector's runtime identifier.
func TestEVTSpot_Name(t *testing.T) {
	assert.Equal(t, "evt_spot", NewEVTSpotDetector().Name())
}

// TestEVTSpot_InterfaceContracts checks the structural promises that the
// catalog and engine both rely on: EVTSpotDetector must satisfy
// observer.Detector AND observer.SeriesRemover (it is stateful and is
// NOT listed in statelessDetectorAllowlist).
func TestEVTSpot_InterfaceContracts(t *testing.T) {
	d := NewEVTSpotDetector()
	var _ observer.Detector = d
	var _ observer.SeriesRemover = d
}

// TestEVTSpot_FactoryAlias verifies the lower-camelcase alias used by the
// catalog factory still returns a working detector, so the stage-1
// catalog wiring keeps working after this stage's rename.
func TestEVTSpot_FactoryAlias(t *testing.T) {
	d := NewEvtSpotDetector()
	require.NotNil(t, d)
	assert.Equal(t, "evt_spot", d.Name())
}

// TestEVTSpot_GPDFit_Closed_Form verifies the Method-of-Moments closed-form
// produces sensible parameters on a known excess set. With excesses drawn
// from a known mean/variance the resulting (γ, σ) should match the MoM
// formula exactly.
func TestEVTSpot_GPDFit_Closed_Form(t *testing.T) {
	d := NewEVTSpotDetector()
	state := &evtSeriesState{
		excesses: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}
	d.refitGPD(state)

	// mean=5.5, variance(unbiased over 10)=sum((x-5.5)^2)/9 = 82.5/9 ≈ 9.1667
	// γ = 0.5*(1 - 5.5²/9.1667) ≈ 0.5*(1 - 30.25/9.1667) = 0.5*(1 - 3.30) ≈ -1.15
	// σ = 5.5*(1 - γ) ≈ 5.5 * 2.15 ≈ 11.825
	assert.InDelta(t, -1.15, state.gpdGamma, 0.05, "γ should match MoM closed-form within rounding")
	assert.InDelta(t, 11.825, state.gpdSigma, 0.1, "σ should match MoM closed-form within rounding")
}

// TestEVTSpot_RefitGPD_Insufficient verifies the refit guard: with fewer
// than 5 excesses the parameters fall back to (0, 0) so computeZQ pins
// zQ to a safe-large default.
func TestEVTSpot_RefitGPD_Insufficient(t *testing.T) {
	d := NewEVTSpotDetector()
	state := &evtSeriesState{
		excesses:    []float64{1.0, 2.0},
		totalSeen:   100,
		totalExceed: 2,
		tInit:       1.0,
	}
	d.refitGPD(state)
	assert.Equal(t, 0.0, state.gpdGamma)
	assert.Equal(t, 0.0, state.gpdSigma)
	d.computeZQ(state)
	// Safe default: tInit + 1e9 — large enough that no realistic value
	// would ever trip the gate.
	assert.Greater(t, state.zQ, 1e8, "zQ must remain in the safe-large region with no GPD fit")
}
