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

// testVarShiftDetector returns a detector restricted to AggregateAverage so
// each test only sees one state entry per series — keeps anomaly-count
// assertions unambiguous (a duplicate Count anomaly here would otherwise
// mask real false positives).
func testVarShiftDetector() *VarShiftDetector {
	d := NewVarShiftDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedVarShiftSeries appends values to a fresh storage with consecutive
// timestamps starting at t=1, then runs Detect once at the final timestamp.
// We add a positive offset so storage has comfortably positive values; the
// log-variance-ratio is invariant to translation so this doesn't perturb
// the test.
func feedVarShiftSeries(t *testing.T, d *VarShiftDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

// TestVarShift_Name documents the catalog identifier the detector returns.
// The catalog entry's `name` field and the detector's Name() return value
// must agree because reporters key off DetectorName.
func TestVarShift_Name(t *testing.T) {
	d := NewVarShiftDetector()
	assert.Equal(t, "varshift", d.Name())
}

// TestVarShift_HomoscedasticNoise_NoFire: 600 i.i.d. N(0,1) → 0 anomalies.
// The log-variance ratio between two abutting 60-sample windows of the same
// distribution has standard deviation ~sqrt(2/W) ≈ 0.18, so it almost never
// reaches the 1.6 threshold (>8σ event). Persistence-of-3 makes spurious
// triggers astronomically unlikely.
func TestVarShift_HomoscedasticNoise_NoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := genGaussian(rng, 600, 0, 1)

	d := testVarShiftDetector()
	result := feedVarShiftSeries(t, d, "homoscedastic", values)

	assert.Empty(t, result.Anomalies, "i.i.d. N(0,1) must not trigger varshift")
}

// TestVarShift_VarianceShift_Fires: 200 N(0,1) followed by 200 N(0,3) (3×
// sigma = 9× variance). Log-variance ratio jumps to ≈ ln(9) ≈ 2.2, well
// above the 1.6 threshold. Mean stays at zero so the meanGap stays under
// 0.5σ. Exactly one anomaly should fire near the boundary.
func TestVarShift_VarianceShift_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := append(genGaussian(rng, 200, 0, 1), genGaussian(rng, 200, 0, 3)...)

	d := testVarShiftDetector()
	result := feedVarShiftSeries(t, d, "variance_shift", values)

	require.Len(t, result.Anomalies, 1, "variance shift (same mean, 9× variance) should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "varshift", a.DetectorName)
	assert.Contains(t, a.Title, "VarShift")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.DeviationSigma, 1.6,
		"|logRatio| at trigger must clear LogRatioThreshold")
	// Trigger must occur after the regime change at index 200, with slack
	// for the windows to fill enough that logRatio clears 1.6 for K=3 ticks.
	assert.Greater(t, a.Timestamp, int64(200), "anomaly must be in the post-shift regime")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
	// Debug fields document the variance-shift narrative: T's stddev should
	// be larger than R's at the trigger.
	assert.Greater(t, a.DebugInfo.CurrentValue, a.DebugInfo.BaselineStddev,
		"post-shift stddev must exceed pre-shift stddev for a variance-up regime change")
}

// TestVarShift_MeanShiftDoesNotFire: 200 N(0,1) followed by 200 N(5,1).
// Variance is identical in both regimes; only the mean shifts. This is the
// additivity test: varshift must NOT fire on a pure mean shift, because
// ScanMW/BOCPD already cover that and double-firing inflates false-positive
// counts on the same incident.
func TestVarShift_MeanShiftDoesNotFire(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := append(genGaussian(rng, 200, 0, 1), genGaussian(rng, 200, 5, 1)...)

	d := testVarShiftDetector()
	result := feedVarShiftSeries(t, d, "mean_shift", values)

	assert.Empty(t, result.Anomalies,
		"pure mean shift must not fire varshift — additivity gate against ScanMW/BOCPD")
}

// TestVarShift_StatelessAcrossSeries verifies state isolation between two
// interleaved series with different behaviour. Series A is a steady-state
// N(0,1); series B has a clear variance shift. The stable A must remain
// quiet while B fires — proving per-series state keys (ref+agg) don't bleed.
func TestVarShift_StatelessAcrossSeries(t *testing.T) {
	rngA := rand.New(rand.NewSource(10))
	rngB := rand.New(rand.NewSource(11))
	stableA := genGaussian(rngA, 400, 0, 1)
	shiftB := append(genGaussian(rngB, 200, 0, 1), genGaussian(rngB, 200, 0, 3)...)

	d := testVarShiftDetector()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i := 0; i < 400; i++ {
		storage.Add("ns", "stableA", offset+stableA[i], int64(i+1), nil)
		storage.Add("ns", "shiftB", offset+shiftB[i], int64(i+1), nil)
	}
	result := d.Detect(storage, 400)

	// Group anomalies by series via SourceRef. The stable series must yield
	// zero; the shifting series must yield exactly one.
	countByName := map[string]int{}
	for _, a := range result.Anomalies {
		countByName[a.Source.Name]++
	}
	assert.Equal(t, 0, countByName["stableA"], "stable series must not fire")
	assert.Equal(t, 1, countByName["shiftB"], "shifting series must fire exactly once")
	// Both series should have allocated state entries (ref+agg keys).
	assert.Len(t, d.series, 2, "per-series state must be allocated for each ref")
}

// TestVarShift_RemoveSeries_FreesState verifies that RemoveSeries shrinks
// the per-series state map — the SeriesRemover contract that keeps detector
// memory in step with storage eviction.
func TestVarShift_RemoveSeries_FreesState(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	values := genGaussian(rng, 200, 0, 1)

	d := testVarShiftDetector()
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

// TestVarShift_Reset documents that Reset clears every per-series state and
// the cached series list — needed by replay/reanalysis call sites.
func TestVarShift_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	values := genGaussian(rng, 80, 0, 1)

	d := testVarShiftDetector()
	feedVarShiftSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset should clear all state")
	assert.Nil(t, d.cachedSeries, "Reset should clear cached series")
}

// TestVarShift_ColdStart_NoFire_BelowWarmup: a strong variance shift in the
// middle of the [0, 2W) warmup region (W=60, 2W=120) must not fire because
// neither logRatio is computable until both rings are full.
func TestVarShift_ColdStart_NoFire_BelowWarmup(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// Big variance jump well before 2W=120 ticks — no anomaly should be
	// emitted because logRatio is undefined while either ring is filling.
	values := append(genGaussian(rng, 60, 0, 1), genGaussian(rng, 30, 0, 5)...)

	d := testVarShiftDetector()
	result := feedVarShiftSeries(t, d, "cold_start", values)

	assert.Empty(t, result.Anomalies,
		"detector must not emit before both rings are full (cold-start contract)")
}

// TestVarShift_RecoveryPrevents_DoubleFire: a single sustained variance
// shift must produce exactly ONE anomaly even though it persists for 200
// post-shift ticks. The post-fire structural reset (T zeroed, R copied
// from T, sums migrated) plus the recovery counter together prevent
// re-firing on the same incident.
func TestVarShift_RecoveryPrevents_DoubleFire(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := append(genGaussian(rng, 200, 0, 1), genGaussian(rng, 400, 0, 3)...)

	d := testVarShiftDetector()
	result := feedVarShiftSeries(t, d, "double_fire_check", values)

	require.Len(t, result.Anomalies, 1,
		"a single sustained variance shift must not produce repeat anomalies during the recovery+refill window")
}

// TestVarShift_PersistentLogRatio exercises the persistence/regime helper
// directly. The function must (1) require all entries above the magnitude
// threshold and (2) require all entries to share a sign, since alternating
// signs are sampling noise rather than a regime shift.
func TestVarShift_PersistentLogRatio(t *testing.T) {
	cases := []struct {
		name      string
		history   []float64
		threshold float64
		want      bool
	}{
		{"all-positive-above", []float64{2.0, 1.7, 1.8}, 1.6, true},
		{"all-negative-above", []float64{-2.0, -1.7, -1.8}, 1.6, true},
		{"one-below", []float64{2.0, 1.5, 1.8}, 1.6, false},
		{"sign-flip", []float64{2.0, -1.7, 1.8}, 1.6, false},
		{"all-below", []float64{0.5, 0.6, 0.4}, 1.6, false},
		{"empty", nil, 1.6, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := persistentLogRatio(tc.history, tc.threshold)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestVarShift_CatalogEntryRegistered confirms the stage-1 catalog wiring
// is intact: the catalog must contain a "varshift" entry with kind
// componentDetector. Lives next to the detector implementation rather than
// in component_catalog_test.go because it's a contract co-test for this
// detector.
func TestVarShift_CatalogEntryRegistered(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "varshift" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "catalog must register a 'varshift' entry")
	assert.Equal(t, componentDetector, found.kind, "varshift must be registered as a detector")
	// The factory must produce a working detector instance.
	inst := found.factory(nil)
	det, ok := inst.(observer.Detector)
	require.True(t, ok, "varshift factory must produce an observer.Detector")
	assert.Equal(t, "varshift", det.Name())
	// And it must implement SeriesRemover so the engine can reclaim state
	// when storage evicts a series.
	_, isRemover := inst.(observer.SeriesRemover)
	assert.True(t, isRemover, "varshift detector must implement observer.SeriesRemover")
}
