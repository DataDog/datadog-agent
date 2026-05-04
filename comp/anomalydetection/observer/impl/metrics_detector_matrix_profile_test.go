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

// testMatrixProfileDetector returns a detector configured for fast unit
// tests. We shrink SubseqLen and HistorySubs from the defaults so each test
// fits inside ~1200 raw points (rather than 5000+), and use a single
// aggregation so anomaly counts are deterministic. The threshold tunables
// match the production defaults so the tests still exercise the real gate
// logic.
func testMatrixProfileDetector() *MatrixProfileDetector {
	d := NewMatrixProfileDetector()
	d.SubseqLen = 10
	d.HistorySubs = 40
	d.MinPointsToScan = 50
	d.AbsoluteFloor = 1.5
	d.ThresholdK = 3.0
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d.series = make(map[mpStateKey]*mpSeriesState)
	return d
}

// mpAddSine seeds the storage with a pure sine of the given period, and
// optionally injects single-point spikes at the listed indices. Spike
// indices use the same 0-based input space as the loop variable; storage
// timestamps are 1-indexed.
func mpAddSine(storage *timeSeriesStorage, name string, start, end int, period float64, spikes map[int]float64) {
	for i := start; i < end; i++ {
		v := math.Sin(2 * math.Pi * float64(i) / period)
		if sv, ok := spikes[i]; ok {
			v = sv
		}
		storage.Add("ns", name, v, int64(i+1), nil)
	}
}

// TestMatrixProfile_Constant_NoFire feeds 600 identical values. All subseqs
// are flat → z-normalization stores zero vectors → pairwise distances are
// all sqrt(2L), MAD is zero, the threshold equals the minDist, and the
// strict `minDist > threshold` gate keeps the detector silent.
func TestMatrixProfile_Constant_NoFire(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 600; i++ {
		storage.Add("ns", "metric", 100.0, int64(i+1), nil)
	}
	result := d.Detect(storage, 600)
	assert.Empty(t, result.Anomalies, "constant input has no shape variation to score")
}

// TestMatrixProfile_Sine_NoFire feeds a 1000-point pure sine of period 50
// (so each L=10 subseq is 1/5 of a cycle). Phases cycle every 5 subseqs, so
// every new subseq has at least one phase-matched cousin already in the
// cache once warmup completes — minDist collapses to ~0 and the absolute
// floor blocks the fire path.
func TestMatrixProfile_Sine_NoFire(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()
	mpAddSine(storage, "metric", 0, 1000, 50.0, nil)

	result := d.Detect(storage, 1000)
	assert.Empty(t, result.Anomalies, "perfectly periodic data has a near-zero matrix profile")
}

// TestMatrixProfile_Glitch_Fires injects a single-point spike at index 600
// of an otherwise-pure sine. Single-point spikes z-normalize to a
// distinctive impulse shape that has no near-match anywhere in a sine
// cache, so the matrix profile spikes and the gate fires within L points
// of the glitch onset.
func TestMatrixProfile_Glitch_Fires(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()

	const glitchIdx = 600
	const spikeValue = 100.0
	mpAddSine(storage, "metric", 0, 1000, 50.0, map[int]float64{glitchIdx: spikeValue})

	result := d.Detect(storage, 1000)
	require.NotEmpty(t, result.Anomalies, "single-point spike must fire as a shape discord")
	a := result.Anomalies[0]

	assert.Equal(t, "matrix_profile", a.DetectorName)
	assert.Contains(t, a.Title, "Matrix profile discord")
	require.NotNil(t, a.Score, "score must be populated")
	assert.Greater(t, *a.Score, d.AbsoluteFloor, "score is the minDist and must clear the absolute floor")

	require.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
	assert.Greater(t, a.DebugInfo.CurrentValue, a.DebugInfo.Threshold, "current minDist must exceed threshold")

	// The fire should happen on the subseq that consumed the glitch (or
	// the one immediately after, depending on alignment) — in any case
	// before 2L raw points have elapsed past the glitch onset (timestamps
	// are 1-indexed in the seeder).
	assert.GreaterOrEqual(t, a.Timestamp, int64(glitchIdx+1))
	assert.LessOrEqual(t, a.Timestamp, int64(glitchIdx+1+2*d.SubseqLen))
}

// TestMatrixProfile_Ramp_NoFire feeds a 600-point linear ramp. Every
// L-sample slice of a constant-slope ramp z-normalizes to the same canonical
// "rising line" shape, so pairwise z-norm distances collapse to ~0 across
// the cache and the floor blocks the gate.
func TestMatrixProfile_Ramp_NoFire(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 600; i++ {
		v := 0.1 * float64(i)
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	result := d.Detect(storage, 600)
	assert.Empty(t, result.Anomalies, "smooth ramp has identical z-normalized shape across all subseqs")
}

// TestMatrixProfile_RemoveSeries verifies the SeriesRemover contract: after
// Detect populates per-series state, RemoveSeries must drop it so the
// detector's memory tracks storage's series cardinality (the cache is
// ~38KB per (series, agg) at defaults).
func TestMatrixProfile_RemoveSeries(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()
	mpAddSine(storage, "metric", 0, 200, 50.0, nil)

	d.Detect(storage, 200)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})
	assert.Empty(t, d.series, "RemoveSeries must drop per-series state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries should invalidate the series cache")
}

// TestMatrixProfile_RefractorySuppressesNearbyGlitch confirms that a second
// glitch within the refractory window is suppressed even when its shape
// differs from the first (so the suppression is not just cache-match
// coincidence). With H=40 the refractory is 10 subseqs (= 100 raw points
// at L=10).
func TestMatrixProfile_RefractorySuppressesNearbyGlitch(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()

	// Phase 1: 700 sine points with a +100 spike at index 600. First
	// glitch must fire and arm the refractory (= H/4 = 10 subseqs).
	mpAddSine(storage, "metric", 0, 700, 50.0, map[int]float64{600: 100.0})
	r1 := d.Detect(storage, 700)
	require.Len(t, r1.Anomalies, 1, "first glitch must fire")

	// Phase 2: 200 sine points with a -100 spike at index 700 (i.e.,
	// 10 subseqs after the first glitch). Even though the opposite-sign
	// spike has a clearly distinct z-normalized shape, the refractory
	// gate must suppress it.
	mpAddSine(storage, "metric", 700, 900, 50.0, map[int]float64{700: -100.0})
	r2 := d.Detect(storage, 900)
	assert.Empty(t, r2.Anomalies, "second glitch within refractory window must be suppressed")
}

// TestMatrixProfile_RefractoryAllowsDistantGlitch confirms that once the
// refractory has decayed, a second (shape-distinct) glitch fires. We use an
// opposite-sign spike for the second glitch so the cached first glitch
// cannot match it; this isolates the refractory expiry from the
// cache-eviction path.
func TestMatrixProfile_RefractoryAllowsDistantGlitch(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()

	// Phase 1: through the first glitch (subseq 60).
	mpAddSine(storage, "metric", 0, 700, 50.0, map[int]float64{600: 100.0})
	r1 := d.Detect(storage, 700)
	require.Len(t, r1.Anomalies, 1, "first glitch must fire")

	// Phase 2: 100 normal sine points (10 subseqs) — exactly enough to
	// drain the refractory counter.
	mpAddSine(storage, "metric", 700, 800, 50.0, nil)
	r2 := d.Detect(storage, 800)
	assert.Empty(t, r2.Anomalies, "no glitch in this batch")

	// Phase 3: another 100 points with a -100 spike at index 800 (20
	// subseqs after the first glitch, past refractory). The opposite-sign
	// spike is shape-distinct from the cached +100 spike, so the
	// data-driven and floor gates pass and a second anomaly fires.
	mpAddSine(storage, "metric", 800, 900, 50.0, map[int]float64{800: -100.0})
	r3 := d.Detect(storage, 900)
	require.Len(t, r3.Anomalies, 1, "second glitch past refractory must fire")
}

// TestMatrixProfile_DetectorAndSeriesRemoverInterfaces locks in the catalog
// teardown contract: the catalog's validateDetectorTeardownContract walks
// every detector and asserts SeriesRemover is satisfied (or the entry is
// allowlisted). matrix_profile is not allowlisted, so this test acts as a
// local sanity check before the full catalog test.
func TestMatrixProfile_DetectorAndSeriesRemoverInterfaces(t *testing.T) {
	var d any = NewMatrixProfileDetector()
	_, ok := d.(observer.Detector)
	assert.True(t, ok, "MatrixProfileDetector must implement observer.Detector")
	_, ok = d.(observer.SeriesRemover)
	assert.True(t, ok, "MatrixProfileDetector must implement observer.SeriesRemover")
}

// TestMatrixProfile_Name pins the catalog identifier so the catalog
// registration stays in sync with the detector itself.
func TestMatrixProfile_Name(t *testing.T) {
	assert.Equal(t, "matrix_profile", NewMatrixProfileDetector().Name())
}

// TestMatrixProfile_DefaultsApplied confirms ensureDefaults populates a
// zero-valued detector struct, mirroring the kl_divergence and
// holt_residual contract. This protects any reflective construction path
// that bypasses NewMatrixProfileDetector.
func TestMatrixProfile_DefaultsApplied(t *testing.T) {
	d := &MatrixProfileDetector{}
	storage := newTimeSeriesStorage()
	_ = d.Detect(storage, 1)

	assert.Equal(t, mpDefaultSubseqLen, d.SubseqLen)
	assert.Equal(t, mpDefaultHistorySubs, d.HistorySubs)
	assert.Equal(t, mpDefaultThresholdK, d.ThresholdK)
	assert.Equal(t, mpDefaultAbsoluteFloor, d.AbsoluteFloor)
	assert.Equal(t, mpDefaultMinPointsToScan, d.MinPointsToScan)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestMatrixProfile_Reset confirms Reset clears all per-series state so a
// replay run starts fresh.
func TestMatrixProfile_Reset(t *testing.T) {
	d := testMatrixProfileDetector()
	storage := newTimeSeriesStorage()
	mpAddSine(storage, "metric", 0, 200, 50.0, nil)

	d.Detect(storage, 200)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}
