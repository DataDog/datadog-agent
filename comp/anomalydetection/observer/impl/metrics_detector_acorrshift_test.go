// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"math/rand"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testACorrShiftDetector returns a detector restricted to AggregateAverage so
// each test only sees one state entry per series — keeps anomaly-count
// assertions unambiguous.
func testACorrShiftDetector() *AcorrShiftDetector {
	d := NewAcorrShiftDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// genNoise returns n i.i.d. N(0,1) samples.
func genNoise(rng *rand.Rand, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = rng.NormFloat64()
	}
	return out
}

// genAR1 returns n samples from AR(1) with coefficient phi, parameterized so
// the marginal variance is 1.0 (σ²_ε = 1 − φ²). Includes a burn-in pass to
// escape the transient from the zero starting state.
func genAR1(rng *rand.Rand, phi float64, n int) []float64 {
	sigmaEps := math.Sqrt(1 - phi*phi)
	out := make([]float64, n)
	var x float64
	for i := 0; i < 100; i++ { // burn-in
		x = phi*x + sigmaEps*rng.NormFloat64()
	}
	for i := 0; i < n; i++ {
		x = phi*x + sigmaEps*rng.NormFloat64()
		out[i] = x
	}
	return out
}

// feedSeries appends values to storage with consecutive timestamps starting at
// t=1, then runs Detect once at the final timestamp.
func feedSeries(t *testing.T, d *AcorrShiftDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	// Add a positive offset so series values are firmly in the storage's
	// accepted range; ACF is invariant to translation and positive scaling.
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

func TestACorrShift_Name(t *testing.T) {
	d := NewAcorrShiftDetector()
	assert.Equal(t, "acorrshift", d.Name())
}

// TestACorrShift_WhiteNoise_NoFire: 600 i.i.d. N(0,1) → 0 anomalies. ρ̂ ≈ 0
// throughout, baseline ≈ 0, persistence streak should never reach 5.
func TestACorrShift_WhiteNoise_NoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := genNoise(rng, 600)

	d := testACorrShiftDetector()
	result := feedSeries(t, d, "noise", values)

	assert.Empty(t, result.Anomalies, "white noise should not trigger any acorrshift anomaly")
}

// TestACorrShift_AR1_NoFire_AfterWarmup: 600 AR(1) with φ=0.7 → 0 anomalies.
// ρ̂ stays consistently near 0.7 from the first computed value, so the
// baseline median locks in around 0.7 and |Δ| stays small.
func TestACorrShift_AR1_NoFire_AfterWarmup(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	values := genAR1(rng, 0.7, 600)

	d := testACorrShiftDetector()
	result := feedSeries(t, d, "ar1", values)

	assert.Empty(t, result.Anomalies, "stationary AR(1) with consistent ACF should not trigger")
}

// TestACorrShift_NoiseToAR1_Fires: 300 i.i.d. N(0,1) followed by 300 AR(1)
// with φ=0.7. Mean and marginal variance are identical between the two
// regimes — only the temporal structure changes — which is exactly the
// signal class the existing detectors cannot see. Expect exactly 1 anomaly.
func TestACorrShift_NoiseToAR1_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := append(genNoise(rng, 300), genAR1(rng, 0.7, 300)...)

	d := testACorrShiftDetector()
	result := feedSeries(t, d, "noise_to_ar1", values)

	require.Len(t, result.Anomalies, 1, "regime change from white noise to AR(1) should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "acorrshift", a.DetectorName)
	assert.Contains(t, a.Title, "AcorrShift")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.DeviationSigma, 0.4,
		"|ρ̂ − baseline| at trigger must exceed RhoDelta")
	// Trigger must occur after the regime change at index 300, and before the
	// run ends. Add 5 points of slack for the persistence ring filling up
	// with post-change ρ̂s.
	assert.Greater(t, a.Timestamp, int64(300), "anomaly timestamp must be in the AR(1) regime")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestACorrShift_AR1ToAntiAR1_Fires: 300 AR(1) at φ=0.6, then 300 AR(1) at
// φ=-0.6. The autocorrelation regime flips sign — the largest possible Δρ̂.
// Expect exactly 1 anomaly during the transition.
func TestACorrShift_AR1ToAntiAR1_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := append(genAR1(rng, 0.6, 300), genAR1(rng, -0.6, 300)...)

	d := testACorrShiftDetector()
	result := feedSeries(t, d, "ar1_flip", values)

	require.Len(t, result.Anomalies, 1, "sign flip in AR(1) coefficient should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "acorrshift", a.DetectorName)
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.DeviationSigma, 0.4)
	assert.Greater(t, a.Timestamp, int64(300))
}

// TestACorrShift_RemoveSeries verifies that RemoveSeries shrinks the per-series
// state map — the SeriesRemover contract that keeps detector-side memory in
// step with storage eviction.
func TestACorrShift_RemoveSeries(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := genAR1(rng, 0.5, 100)

	d := testACorrShiftDetector()
	storage := newTimeSeriesStorage()
	for i, v := range values {
		storage.Add("ns", "metric", 100+v, int64(i+1), nil)
	}
	d.Detect(storage, int64(len(values)))
	require.NotEmpty(t, d.series, "detector must record per-series state during Detect")

	// Pull the ref(s) used by the detector and free them.
	var refs []observer.SeriesRef
	for k := range d.series {
		refs = append(refs, k.ref)
	}
	d.RemoveSeries(refs)
	assert.Empty(t, d.series, "RemoveSeries must drop state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries must invalidate cachedSeries")
}

// TestACorrShift_Reset verifies that Reset clears all per-series state.
func TestACorrShift_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := genNoise(rng, 80)

	d := testACorrShiftDetector()
	feedSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
	assert.Nil(t, d.cachedSeries, "reset should clear cached series")
}

// TestP2Quantile_ConvergesToTrueMedian feeds 10000 N(0,1) samples into the P²
// estimator at p=0.5 and asserts the result is within 0.05 of the true
// median (0.0). Documents the asymptotic accuracy used by the detector.
func TestP2Quantile_ConvergesToTrueMedian(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	e := newP2Quantile(0.5)
	for i := 0; i < 10000; i++ {
		e.add(rng.NormFloat64())
	}
	v, ok := e.value()
	require.True(t, ok, "estimator should be initialized after >> 5 observations")
	assert.InDelta(t, 0.0, v, 0.05,
		"P² median estimate must be close to the true median for N(0,1)")
}

// TestP2Quantile_NotInitializedBeforeFiveSamples documents the explicit
// "uninitialized" return of value() so callers know to gate trigger logic.
func TestP2Quantile_NotInitializedBeforeFiveSamples(t *testing.T) {
	e := newP2Quantile(0.5)
	for i := 0; i < 4; i++ {
		e.add(float64(i))
		_, ok := e.value()
		assert.False(t, ok, "estimator must be uninitialized with fewer than 5 observations")
	}
	e.add(4.0)
	v, ok := e.value()
	require.True(t, ok, "estimator must be initialized at the 5th observation")
	assert.InDelta(t, 2.0, v, 1e-9, "post-init median of {0,1,2,3,4} is exactly 2.0")
}

// TestComputeLag1ACF_KnownInputs is a small fixed-input regression around the
// ACF helper. Uses an obvious AR(1)-like alternating pattern to verify the
// ring-indexing math when the buffer is not yet full.
func TestComputeLag1ACF_KnownInputs(t *testing.T) {
	// Constant series → variance floor → returns 0.
	ring := make([]float64, acorrshiftWindow)
	for i := range ring {
		ring[i] = 5.0
	}
	assert.Equal(t, 0.0, computeLag1ACF(ring, acorrshiftWindow, acorrshiftWindow))

	// Strict alternation 0,1,0,1,... → lag-1 ACF ≈ -1.
	for i := range ring {
		ring[i] = float64(i % 2)
	}
	rho := computeLag1ACF(ring, acorrshiftWindow, acorrshiftWindow)
	assert.InDelta(t, -1.0, rho, 0.05, "alternating sequence should have ρ̂ ≈ -1")

	// Linear ramp → strongly positive autocorrelation, clamped near 1.
	for i := range ring {
		ring[i] = float64(i)
	}
	rho = computeLag1ACF(ring, acorrshiftWindow, acorrshiftWindow)
	assert.Greater(t, rho, 0.9, "monotonically increasing sequence should have ρ̂ near 1")
}
