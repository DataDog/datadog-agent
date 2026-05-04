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

// testSpectralResidualDetector returns a detector configured for the
// single-aggregation tests below. Default Aggregations includes Count;
// pinning to Average keeps anomaly counts deterministic.
func testSpectralResidualDetector() *SpectralResidualDetector {
	d := NewSpectralResidualDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// addSine feeds n points of a noise-free sine wave starting at startTs.
// Period and amplitude are caller-controlled so individual tests can
// stress different spectral structures.
func addSine(t *testing.T, storage *timeSeriesStorage, name string, count int, startTs int64, period, amplitude float64) {
	t.Helper()
	for i := 0; i < count; i++ {
		v := amplitude * math.Sin(2*math.Pi*float64(i)/period)
		storage.Add("ns", name, v, startTs+int64(i), nil)
	}
}

// TestSpectralResidual_Constant_NoFire feeds 500 identical values. On a
// constant signal the SR transform produces a saliency that is identical
// across every window position (the buffer never changes), so the
// saliency MAD collapses to the floor and (saliency - median)/sigma = 0.
// The effect-size gate also blocks because (value - medianValue) = 0.
// Either gate alone is enough to preclude a fire — neither should pass
// here.
func TestSpectralResidual_Constant_NoFire(t *testing.T) {
	d := testSpectralResidualDetector()
	storage := newTimeSeriesStorage()

	for ts := int64(1); ts <= 500; ts++ {
		storage.Add("ns", "metric", 7.0, ts, nil)
	}

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "constant input must not trigger spectral residual")
}

// TestSpectralResidual_PureSine_NoFire feeds 500 points of a clean sine
// (period 16) — a signal whose spectrum is concentrated on one bin pair.
// The saliency at the latest sample varies only modestly as the buffer
// rotates, so the MAD-z stays well below threshold (and the effect-size
// gate, taking |x - median(values)|, is bounded by the sine amplitude
// over the rolling MAD ≈ 1.5σ — far short of MinDeviationMAD=3). Pins
// the contract that periodic signals don't fire.
func TestSpectralResidual_PureSine_NoFire(t *testing.T) {
	d := testSpectralResidualDetector()
	storage := newTimeSeriesStorage()

	addSine(t, storage, "metric", 500, 1, 16.0, 1.0)

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "pure sine must not fire — concentrated spectrum, modest saliency variation")
}

// TestSpectralResidual_NoiseWithSpike_FiresAtSpike feeds Gaussian noise
// (σ=1) for 500 points with a single 10σ spike at t=350. This is the
// canonical SR scenario: a roughly flat magnitude spectrum (noise) with
// a transient that injects phase-aligned broadband energy into the
// buffer. When the spike is the most-recent ring sample (at tick 350)
// every bin's phase is locked to the spike's, the inverse-DFT-at-N-1
// constructively sums to ≈ N, and the saliency z-score blows past the
// gate. One tick later the spike sits at position N-2, the phase
// alignment unwinds (Σ N-th roots of unity = 0), and saliency at N-1
// drops back to noise level — so we expect exactly one fire near 350.
//
// Note: the SR algorithm is not designed for highly periodic baselines.
// On a pure sine the saliency at S[N-1] is dominated by the IFFT of the
// huge exp(R[4]) at the sine's bin and oscillates by ~exp(R[4]) — a
// spike *reduces* that saliency rather than increasing it (because the
// spike flattens the spectrum). The "noise + spike" pattern here is the
// regime where the algorithm earns its keep.
func TestSpectralResidual_NoiseWithSpike_FiresAtSpike(t *testing.T) {
	d := testSpectralResidualDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(42))
	const spikeAt int64 = 350
	const spikeValue = 10.0
	for ts := int64(1); ts <= 500; ts++ {
		v := rng.NormFloat64()
		if ts == spikeAt {
			v = spikeValue
		}
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	require.GreaterOrEqual(t, len(result.Anomalies), 1, "spike on Gaussian noise must fire at least once")

	// Locate the anomaly closest to the spike. There should be exactly
	// one, but the contract under test is "the fire that fires at the
	// spike must clear the threshold and identify itself correctly" —
	// stray noise-tail fires elsewhere in the run would already trip
	// TestSpectralResidual_PureSine_NoFire / the integration suites.
	var nearest *observer.Anomaly
	bestDist := int64(math.MaxInt64)
	for i := range result.Anomalies {
		a := &result.Anomalies[i]
		d := a.Timestamp - spikeAt
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			nearest = a
		}
	}
	require.NotNil(t, nearest)

	assert.Equal(t, "spectral_residual", nearest.DetectorName)
	assert.Contains(t, nearest.Title, "Spectral residual")
	require.NotNil(t, nearest.Score)
	assert.Greater(t, *nearest.Score, 5.0, "score must clear the saliency z threshold")
	assert.NotNil(t, nearest.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, nearest.DebugInfo, "DebugInfo must be populated")
	assert.Equal(t, 5.0, nearest.DebugInfo.Threshold)
	// Fire timestamp must land essentially at the spike — within one
	// tick. By design the saliency at n=N-1 collapses one tick after the
	// spike enters the ring (phase alignment lost), so the fire is
	// either at spikeAt or — if WarmupPoints landed mid-window — the
	// very next eligible tick.
	assert.GreaterOrEqual(t, nearest.Timestamp, spikeAt, "fire must come at or after the spike")
	assert.LessOrEqual(t, nearest.Timestamp, spikeAt+1, "fire must come at or within 1 tick of the spike")
}

// TestSpectralResidual_RemoveSeries verifies the SeriesRemover contract:
// after Detect populates per-series state, RemoveSeries must drop it so
// the detector's memory tracks storage's series cardinality. Without
// this the catalog teardown contract test
// (TestDefaultCatalog_DetectorTeardownContract) would be a structural
// check only, leaving real leaks possible at runtime.
func TestSpectralResidual_RemoveSeries(t *testing.T) {
	d := testSpectralResidualDetector()
	storage := newTimeSeriesStorage()

	for ts := int64(1); ts <= 100; ts++ {
		storage.Add("ns", "metric", 1.0, ts, nil)
	}
	d.Detect(storage, 100)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})
	assert.Empty(t, d.series, "RemoveSeries must drop per-series state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries should invalidate the series cache")
}

// TestSpectralResidual_Reset confirms Reset wipes per-series state.
func TestSpectralResidual_Reset(t *testing.T) {
	d := testSpectralResidualDetector()
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

// TestSpectralResidual_DefaultsApplied confirms ensureDefaults populates a
// zero-valued struct so reflective construction (and any caller that
// bypasses NewSpectralResidualDetector) still produces a usable detector.
func TestSpectralResidual_DefaultsApplied(t *testing.T) {
	d := &SpectralResidualDetector{}
	storage := newTimeSeriesStorage()

	_ = d.Detect(storage, 1)

	assert.Equal(t, 64, d.WindowN)
	assert.Equal(t, 60, d.SaliencyMADWin)
	assert.Equal(t, 60, d.ValueMADWin)
	assert.Equal(t, 3, d.AvgFilterQ)
	assert.Equal(t, 5.0, d.ZThreshold)
	assert.Equal(t, 3.0, d.MinDeviationMAD)
	assert.Equal(t, 24, d.Refractory)
	assert.Equal(t, 64+30, d.WarmupPoints)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestSpectralResidual_Name pins the catalog identifier so the catalog
// test stays in sync with the detector identifier.
func TestSpectralResidual_Name(t *testing.T) {
	assert.Equal(t, "spectral_residual", NewSpectralResidualDetector().Name())
}

// TestSpectralResidual_InterfaceContracts checks the structural promises
// that the catalog and engine both rely on: SpectralResidualDetector
// must satisfy observer.Detector AND observer.SeriesRemover (it is
// stateful and is NOT listed in statelessDetectorAllowlist).
func TestSpectralResidual_InterfaceContracts(t *testing.T) {
	d := NewSpectralResidualDetector()
	var _ observer.Detector = d
	var _ observer.SeriesRemover = d
}
