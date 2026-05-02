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

// testMannKendallDetector returns a detector restricted to the average
// aggregate so tests don't double-count anomalies via the count aggregate
// (which mirrors the same shape for these synthetic series).
func testMannKendallDetector() *MannKendallDetector {
	d := NewMannKendallDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// TestMannKendall_RegisteredInCatalog verifies the Mann-Kendall detector is
// reachable from defaultCatalog() under its expected name and that the
// catalog factory produces a *MannKendallDetector.
//
// NOTE: the plan called for TestMannKendall_DefaultEnabledIsFalse but the
// stage-1 catalog entry shipped with defaultEnabled=true; that field is
// out of scope for stage 2 (the user instructions explicitly forbid touching
// component_catalog.go), so this test asserts only the registration
// invariant. A flip back to defaultEnabled=false (matching scanmw/scanwelch)
// can be made in a follow-up without touching this file.
func TestMannKendall_RegisteredInCatalog(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "mannkendall" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "mannkendall entry must exist in the catalog")
	require.Equal(t, componentDetector, found.kind)

	instance := found.factory(found.defaultConfig)
	_, ok := instance.(*MannKendallDetector)
	require.True(t, ok, "factory must produce *MannKendallDetector")
}

// TestMannKendall_NoFireOnFlat verifies that a constant series produces no
// anomalies — Var(S) is zero under universal ties so the detector must skip.
func TestMannKendall_NoFireOnFlat(t *testing.T) {
	d := testMannKendallDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 60; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)
	assert.Empty(t, result.Anomalies, "constant series must not fire (Var(S) == 0)")
}

// TestMannKendall_NoFireOnNoise verifies that pure i.i.d. noise rarely fires
// at ZThreshold=5. The plan budgets <=1 false positive across 200 points.
func TestMannKendall_NoFireOnNoise(t *testing.T) {
	d := testMannKendallDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 200; i++ {
		storage.Add("ns", "metric", rng.NormFloat64(), int64(i+1), nil)
	}

	result := d.Detect(storage, 200)
	assert.LessOrEqual(t, len(result.Anomalies), 1,
		"i.i.d. N(0,1) at Z>=5 should produce at most 1 false positive")
}

// TestMannKendall_FiresOnLinearDrift verifies the canonical positive case:
// a 60-point linear drift x_i = 0.5*i + N(0,1) clears both the Z gate and
// the slope-MAD gate, producing exactly one anomaly when the window fills.
func TestMannKendall_FiresOnLinearDrift(t *testing.T) {
	d := testMannKendallDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 60; i++ {
		v := 0.5*float64(i) + rng.NormFloat64()
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)
	require.Len(t, result.Anomalies, 1, "linear drift should fire exactly once when the window fills")
	a := result.Anomalies[0]
	assert.Contains(t, a.Title, "Mann-Kendall trend:")
	assert.Equal(t, "mannkendall", a.DetectorName)
	require.NotNil(t, a.Score, "anomaly must carry a score")
	assert.Greater(t, *a.Score, 5.0, "Z-derived score should comfortably exceed the threshold")
}

// TestMannKendall_NoFireOnSharpStep verifies orthogonality to the scan family:
// 30 zeros followed by 30 fives is a textbook changepoint, but the median
// pairwise slope across the window is too small in MAD-units to trigger MK,
// confirming the dual gate keeps MK in trend-only territory.
func TestMannKendall_NoFireOnSharpStep(t *testing.T) {
	d := testMannKendallDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 30; i++ {
		storage.Add("ns", "metric", 0, int64(i+1), nil)
	}
	for i := 30; i < 60; i++ {
		storage.Add("ns", "metric", 5, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)
	assert.Empty(t, result.Anomalies,
		"sharp step is changepoint territory (scanmw) — MK must not double-fire on it")
}

// TestMannKendall_RespectsCooldown verifies that after a fire, subsequent
// scoring is suppressed for CooldownPoints points and the next eligible
// scoring tick re-fires when drift persists.
func TestMannKendall_RespectsCooldown(t *testing.T) {
	d := testMannKendallDetector()
	d.CooldownPoints = 30
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(42))

	addDriftPoint := func(i int) {
		v := 0.5*float64(i) + rng.NormFloat64()
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}

	// Phase 1: window fills with drift → 1 fire on the 60th point.
	for i := 0; i < 60; i++ {
		addDriftPoint(i)
	}
	r1 := d.Detect(storage, 60)
	require.Len(t, r1.Anomalies, 1, "expected fire when window fills with drift")

	// Phase 2: 29 more drift points — cooldown should still be active
	// (CooldownPoints=30 is decremented per ingested point, leaving 1 after
	// 29 points consumed). No new anomaly.
	for i := 60; i < 89; i++ {
		addDriftPoint(i)
	}
	r2 := d.Detect(storage, 89)
	assert.Empty(t, r2.Anomalies, "cooldown must suppress firing for CooldownPoints points")

	// Phase 3: one more point pushes cooldownLeft to 0; drift persists in
	// the window so a second anomaly is allowed.
	addDriftPoint(89)
	r3 := d.Detect(storage, 90)
	require.Len(t, r3.Anomalies, 1, "after cooldown clears, persistent drift should refire")
}

// TestMannKendall_RemoveSeriesClearsState verifies the SeriesRemover contract:
// per-series window state is dropped when storage frees the ref, so the
// detector's map cannot grow unbounded with cumulative series cardinality.
func TestMannKendall_RemoveSeriesClearsState(t *testing.T) {
	d := testMannKendallDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 60; i++ {
		storage.Add("ns", "metric", 0.5*float64(i)+rng.NormFloat64(), int64(i+1), nil)
	}
	d.Detect(storage, 60)
	require.NotEmpty(t, d.series, "expected per-series state after Detect populated the window")

	// Pull the ref back out of the catalog cache and hand it to RemoveSeries.
	require.NotEmpty(t, d.cachedSeries, "expected at least one cached series after Detect")
	ref := d.cachedSeries[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})

	for k := range d.series {
		require.NotEqual(t, ref, k.ref, "RemoveSeries must purge state for the freed ref")
	}
	// And: the cached series list is invalidated so the next Detect re-lists.
	assert.Nil(t, d.cachedSeries, "RemoveSeries must invalidate the cached series list")
}

// TestMannKendall_Reset verifies Reset clears the per-series map and the
// cached series list, mirroring the contract on ScanMW/ScanWelch.
func TestMannKendall_Reset(t *testing.T) {
	d := testMannKendallDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 60; i++ {
		storage.Add("ns", "metric", float64(i), int64(i+1), nil)
	}
	d.Detect(storage, 60)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
	assert.Nil(t, d.cachedSeries, "reset should clear cached series")
}

// TestMannKendall_NoNewDataNoWork verifies the replay-skip cursor: a Detect
// call with no new points and no in-place writes returns no anomalies and
// does not double-process the existing window.
func TestMannKendall_NoNewDataNoWork(t *testing.T) {
	d := testMannKendallDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 60; i++ {
		storage.Add("ns", "metric", 0.5*float64(i)+rng.NormFloat64(), int64(i+1), nil)
	}

	r1 := d.Detect(storage, 60)
	require.Len(t, r1.Anomalies, 1)

	// Second call with no new data should be a no-op — and must not refire
	// even if cooldown is zero, because nothing new was ingested.
	r2 := d.Detect(storage, 60)
	assert.Empty(t, r2.Anomalies, "no new data must produce no anomalies")
}

// --- Pure-function unit tests for the algorithmic helpers ---

// TestMannKendallS_StrictlyMonotonic verifies S = n(n-1)/2 for a strictly
// increasing series and -n(n-1)/2 for a strictly decreasing series.
func TestMannKendallS_StrictlyMonotonic(t *testing.T) {
	asc := []float64{1, 2, 3, 4, 5}
	desc := []float64{5, 4, 3, 2, 1}
	assert.Equal(t, 10, mannKendallS(asc), "ascending series: S = n(n-1)/2")
	assert.Equal(t, -10, mannKendallS(desc), "descending series: S = -n(n-1)/2")
}

// TestMannKendallS_AllTiesIsZero verifies S is zero when every value ties.
func TestMannKendallS_AllTiesIsZero(t *testing.T) {
	v := []float64{7, 7, 7, 7, 7}
	assert.Equal(t, 0, mannKendallS(v))
}

// TestMannKendallVariance_NoTies cross-checks the analytic Var(S) formula
// against a worked example: n=5 distinct values → Var(S) = 5*4*15/18 = 50/3.
func TestMannKendallVariance_NoTies(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5}
	expected := 5.0 * 4.0 * 15.0 / 18.0
	assert.InDelta(t, expected, mannKendallVariance(v), 1e-9)
}

// TestMannKendallVariance_AllTiesIsZero verifies Var(S)=0 under universal ties,
// the early-return signal for the scoring path on constant series.
func TestMannKendallVariance_AllTiesIsZero(t *testing.T) {
	v := []float64{42, 42, 42, 42}
	assert.Equal(t, 0.0, mannKendallVariance(v))
}

// TestTheilSenSlope_ExactLinear verifies the median slope recovers the true
// slope on a clean linear series.
func TestTheilSenSlope_ExactLinear(t *testing.T) {
	pts := make([]observer.Point, 10)
	for i := range pts {
		pts[i] = observer.Point{Timestamp: int64(i + 1), Value: 2.0 * float64(i+1)}
	}
	assert.InDelta(t, 2.0, theilSenSlope(pts), 1e-9)
}
