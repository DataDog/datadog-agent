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

// testDirBFDetector returns a detector restricted to AggregateAverage so each
// test only sees one state entry per series — keeps anomaly-count assertions
// unambiguous (a duplicate Count anomaly would otherwise mask real false
// positives).
func testDirBFDetector() *DIRBFDetector {
	d := NewDIRBFDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedDirBFSeries appends values to a fresh storage with consecutive
// timestamps starting at t=1, then runs Detect once at the final timestamp.
// We add a positive offset so storage has comfortably positive values; the
// quantile-bin-based BF is invariant to translation so this doesn't perturb
// the test.
func feedDirBFSeries(t *testing.T, d *DIRBFDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

// genUniform returns n samples uniformly distributed in [lo, hi).
func genUniform(rng *rand.Rand, n int, lo, hi float64) []float64 {
	out := make([]float64, n)
	span := hi - lo
	for i := range out {
		out[i] = lo + rng.Float64()*span
	}
	return out
}

// TestDirBF_Name documents the catalog identifier the detector returns.
// The catalog entry's `name` field and the detector's Name() return value
// must agree because reporters key off DetectorName.
func TestDirBF_Name(t *testing.T) {
	d := NewDIRBFDetector()
	assert.Equal(t, "dirbf", d.Name())
}

// TestDirBF_NoFireOnConstant: 800 identical points → no anomalies and bin
// edges all collapse to the same value (the constant). Pure-constant input
// is the structural-degenerate case for any quantile-binned detector; the
// algorithm must coast through it without firing or panicking.
func TestDirBF_NoFireOnConstant(t *testing.T) {
	values := make([]float64, 800)
	for i := range values {
		values[i] = 5.0
	}

	d := testDirBFDetector()
	result := feedDirBFSeries(t, d, "constant", values)

	assert.Empty(t, result.Anomalies, "constant input must not fire dirbf")

	// Internal-state assertion: edges must have been computed (warmup ran)
	// and must all be equal to the constant value (the deciles of a degenerate
	// distribution all coincide).
	require.Len(t, d.series, 1, "exactly one (ref, agg) state entry for one series and one aggregation")
	for _, st := range d.series {
		require.True(t, st.edgesReady, "warmup must complete after WarmupPoints points")
		first := st.binEdges[0]
		for i := 1; i < dirbfNumBins-1; i++ {
			assert.Equal(t, first, st.binEdges[i],
				"bin edges must collapse to a single value for constant input")
		}
	}
}

// TestDirBF_NoFireOnPureGaussian: 1500 N(0,1) with a fixed seed produces no
// anomalies. With Lambda=5.0 the noise-floor 99.9th percentile (~3.0 for
// these window sizes per the candidate description) leaves substantial
// headroom; the persistence-of-3 gate makes spurious fires astronomically
// unlikely. NOTE: the original plan called for 800 points, but with
// WarmupPoints=400 + RefSize=400 + RecentSize=40 the BF is not even computed
// until ~tick 840, so 800 trivially passes without exercising the algorithm.
// 1500 gives 660 ticks of post-fill computation on a single distribution.
func TestDirBF_NoFireOnPureGaussian(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := genGaussian(rng, 1500, 0, 1)

	d := testDirBFDetector()
	result := feedDirBFSeries(t, d, "homogeneous", values)

	assert.Empty(t, result.Anomalies, "homogeneous N(0,1) must not trigger dirbf")
}

// TestDirBF_FiresOnDistributionShift: 1000 N(0,1) followed by 200 U(2,5)
// produces exactly one anomaly. Pre-shift the recent window matches the ref
// distribution; once the recent window fills with U(2,5) values it is
// concentrated at the top bin while ref remains broadly distributed across
// all 10 bins, so logBF blows past the threshold. The post-fire inAlert flag
// plus the cooldown (600s) suppresses re-fires while the recent window
// remains pinned to U(2,5). NOTE: the original plan said 600+200=800 points;
// with WarmupPoints=400 the ref window is not full until ~tick 840, so 1000
// pre-shift is the minimum to actually test the algorithm.
func TestDirBF_FiresOnDistributionShift(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pre := genGaussian(rng, 1000, 0, 1)
	post := genUniform(rng, 200, 2, 5)
	values := append(pre, post...)

	d := testDirBFDetector()
	result := feedDirBFSeries(t, d, "shift", values)

	require.Len(t, result.Anomalies, 1, "a single distribution shift must produce exactly one fire")
	a := result.Anomalies[0]
	assert.Equal(t, "dirbf", a.DetectorName)
	assert.Contains(t, a.Title, "DirichletBF")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.CurrentValue, d.Lambda,
		"recorded logBF must clear the Lambda threshold")
	require.NotNil(t, a.Score)
	assert.GreaterOrEqual(t, *a.Score, d.Lambda, "Score must reflect the trigger logBF")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
	// Trigger must come after the regime change at index 1000.
	assert.Greater(t, a.Timestamp, int64(1000), "anomaly must be in the post-shift regime")
	// SamplingIntervalSec is computed from the recent timestamp ring; with
	// 1-second-spaced points it must be 1.
	assert.Equal(t, int64(1), a.SamplingIntervalSec,
		"median sampling interval must reflect the recent-window timestamps")
}

// TestDirBF_FiresOnBimodalCollapse: a bimodal warmup followed by a unimodal
// collapse to a value between the modes. After warmup the bin edges are
// concentrated near each mode (the deciles of a 50/50 mixture of N(±3, 0.1)
// fall around ±3); collapsing onto N(0, 0.1) routes every recent point into
// the *single* central bin between the two clusters of edges. The recent
// histogram is then a one-bin spike against a broadly-distributed ref → BF
// blows past the threshold. NOTE: original plan called for 500 + 200 = 700
// points; bumped so the BF is actually computable (need ≥840 pre-shift).
func TestDirBF_FiresOnBimodalCollapse(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	bimodal := make([]float64, 1000)
	for i := range bimodal {
		if rng.Float64() < 0.5 {
			bimodal[i] = -3 + 0.1*rng.NormFloat64()
		} else {
			bimodal[i] = 3 + 0.1*rng.NormFloat64()
		}
	}
	collapse := genGaussian(rng, 400, 0, 0.1)
	values := append(bimodal, collapse...)

	d := testDirBFDetector()
	result := feedDirBFSeries(t, d, "bimodal_collapse", values)

	require.NotEmpty(t, result.Anomalies, "bimodal→unimodal collapse must produce at least one fire")
	a := result.Anomalies[0]
	// First anomaly must be in the post-shift regime.
	assert.Greater(t, a.Timestamp, int64(1000),
		"first anomaly must come after the bimodal→unimodal transition at index 1000")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.CurrentValue, d.Lambda)
}

// TestDirBF_CooldownSuppresses: feeding a shift, then a stretch of pre-shift
// values, then a second shift — all within the cooldown window — must yield
// exactly one anomaly. The mechanism is the union of inAlert + cooldown:
// during the recovery stretch logBF stays high (because ref still has the
// post-shift contamination) so inAlert holds; even if it cleared, the
// cooldown clamp (600s) gates the second fire because the second shift falls
// well within that window.
func TestDirBF_CooldownSuppresses(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// Pre-fill: 840 N(0,1) is the minimum to fully populate ref+recent.
	preFill := genGaussian(rng, 840, 0, 1)
	// First shift: 100 U(2,5) — fires.
	shift1 := genUniform(rng, 100, 2, 5)
	// Brief return to baseline: 100 N(0,1).
	gap := genGaussian(rng, 100, 0, 1)
	// Second shift: 100 U(2,5). Cooldown gate must suppress this fire.
	shift2 := genUniform(rng, 100, 2, 5)

	values := append(preFill, shift1...)
	values = append(values, gap...)
	values = append(values, shift2...)

	d := testDirBFDetector()
	result := feedDirBFSeries(t, d, "cooldown", values)

	require.Len(t, result.Anomalies, 1,
		"two shifts within the cooldown window must produce exactly one fire")
}

// TestDirBF_RemoveSeries_FreesState verifies that RemoveSeries shrinks the
// per-series state map — the SeriesRemover contract that keeps detector
// memory in step with storage eviction.
func TestDirBF_RemoveSeries_FreesState(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	values := genGaussian(rng, 500, 0, 1)

	d := testDirBFDetector()
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

// TestDirBF_LogMarginalDir_KnownValue exercises dirbfLogMarginal against a
// hand-computable case: K=2, α=1, counts=[5,5], n=10.
//
//	out = lgamma(2) − lgamma(12) + 2·(lgamma(6) − lgamma(1))
//	    = 0 − ln(11!)         + 2·(ln(5!) − 0)
//	    = − 17.502307…        + 2·4.787491…
//	    ≈ −7.927324…
//
// (The implementation plan's example arithmetic listed −8.527 — that's an
// off-by-one in the lgamma values; the verifiable closed-form value under
// the plan's stated formula is −7.9273.)
func TestDirBF_LogMarginalDir_KnownValue(t *testing.T) {
	got := dirbfLogMarginal([]int{5, 5})
	want := -7.92732419229
	assert.InDelta(t, want, got, 1e-6,
		"closed-form Dir(α=1)-multinomial marginal for K=2, counts=[5,5]")

	// Sanity: equal-count partitions of the same totals must be more probable
	// under M0 (shared prior) than under any unbalanced split, so logBF on
	// identical counts must be < 0 (M0 wins).
	combined := dirbfLogMarginal([]int{10, 10})
	separate := 2 * dirbfLogMarginal([]int{5, 5})
	logBF := separate - combined
	assert.Less(t, logBF, 0.0,
		"identical histograms must produce negative logBF (M0 wins)")
}

// TestDirBF_Bin_BoundaryRules pins down the half-open binning convention so
// future refactors don't silently flip the comparison. Bin i covers
// [edges[i-1], edges[i]); the leftmost bin is (-inf, edges[0]).
func TestDirBF_Bin_BoundaryRules(t *testing.T) {
	edges := []float64{-1, 0, 1}
	cases := []struct {
		x    float64
		want int
	}{
		{-2, 0}, // (-inf, -1)
		{-1, 1}, // exactly on edge → strictly-less-than test routes to next bin
		{-0.5, 1},
		{0, 2},
		{0.5, 2},
		{1, 3},
		{2, 3},
		{math.Inf(1), 3},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, dirbfBin(tc.x, edges),
			"dirbfBin(%v) must route to bin %d", tc.x, tc.want)
	}
}

// TestDirBF_CatalogEntryRegistered confirms the stage-1 catalog wiring is
// intact: the catalog must contain a "dirbf" entry registered as a detector
// whose factory produces a SeriesRemover-implementing detector. Lives next
// to the detector implementation rather than in component_catalog_test.go
// because it's a contract co-test for this detector.
func TestDirBF_CatalogEntryRegistered(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "dirbf" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "catalog must register a 'dirbf' entry")
	assert.Equal(t, componentDetector, found.kind, "dirbf must be registered as a detector")
	inst := found.factory(nil)
	det, ok := inst.(observer.Detector)
	require.True(t, ok, "dirbf factory must produce an observer.Detector")
	assert.Equal(t, "dirbf", det.Name())
	_, isRemover := inst.(manualSeriesRemover)
	assert.True(t, isRemover, "dirbf detector must implement manualSeriesRemover")
}
