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

// testEnergyDistDetector returns a detector restricted to AggregateAverage so
// each test only sees one state entry per series — keeps anomaly-count
// assertions unambiguous (a duplicate Count anomaly here would otherwise mask
// real false positives).
func testEnergyDistDetector() *EnergyDistDetector {
	d := NewEnergyDistDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedEnergyDistSeries appends values to a fresh storage with consecutive
// timestamps starting at t=1, then runs Detect once at the final timestamp.
// We add a positive offset so storage sees comfortably positive values; the
// energy statistic is invariant to translation so this doesn't perturb the
// test (translation cancels in every |x_i − y_j| term).
func feedEnergyDistSeries(t *testing.T, d *EnergyDistDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

// TestEnergyDist_Name documents the catalog identifier the detector returns.
// The catalog entry's `name` field and the detector's Name() return value
// must agree because reporters key off DetectorName.
func TestEnergyDist_Name(t *testing.T) {
	d := NewEnergyDistDetector()
	assert.Equal(t, "energydist", d.Name())
}

// TestEnergyDist_NoFireBeforeWarmup feeds 63 N(0,1) values and asserts the
// detector emits nothing. The warmup threshold is 64 = 2·m (both rings full),
// so the 63rd point cannot have produced an E value yet — by construction.
func TestEnergyDist_NoFireBeforeWarmup(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := genGaussian(rng, 63, 0, 1)

	d := testEnergyDistDetector()
	result := feedEnergyDistSeries(t, d, "warmup", values)

	assert.Empty(t, result.Anomalies,
		"detector must not emit before both rings are full (cold-start contract)")
}

// TestEnergyDist_FiresOnDistributionShift: 64 N(0,1) followed by 64 N(0,3)
// (variance shift only, mean stationary). E should rise above eCritical for
// the persistence ring within a few post-shift ticks while meanGap stays
// well below 0.5σ. Exactly one anomaly should fire.
func TestEnergyDist_FiresOnDistributionShift(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := append(genGaussian(rng, 64, 0, 1), genGaussian(rng, 64, 0, 3)...)

	d := testEnergyDistDetector()
	result := feedEnergyDistSeries(t, d, "variance_shift", values)

	require.Len(t, result.Anomalies, 1, "variance shift (same mean, 9× variance) must fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "energydist", a.DetectorName)
	assert.Contains(t, a.Title, "EnergyDist")
	require.NotNil(t, a.DebugInfo)
	assert.Greater(t, a.DebugInfo.CurrentValue, a.DebugInfo.Threshold,
		"E at trigger must clear the bootstrap-derived eCritical")
	// Trigger must occur after the regime change at index 64.
	assert.Greater(t, a.Timestamp, int64(64), "anomaly must be in the post-shift regime")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
	require.NotNil(t, a.Score, "Score must be populated for downstream ranking")
	assert.Greater(t, *a.Score, 1.0, "Score is E/eCritical and must exceed 1 at fire")
}

// TestEnergyDist_AdditivityGateOnMeanShift: 64 N(0,1) followed by 64 N(3,1)
// (pure mean shift, large meanGap). The mean-stationarity gate must veto
// the fire because |meanT−meanR|/σR ≫ 0.5. This is the explicit anti-co-
// firing test against scanmw/bocpd — the recurring shopify/postgresql
// regression seen across exp-0023, 0026, 0027.
func TestEnergyDist_AdditivityGateOnMeanShift(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := append(genGaussian(rng, 64, 0, 1), genGaussian(rng, 64, 3, 1)...)

	d := testEnergyDistDetector()
	result := feedEnergyDistSeries(t, d, "mean_shift", values)

	assert.Empty(t, result.Anomalies,
		"pure mean shift must not fire energydist — additivity gate against scanmw/bocpd")
}

// TestEnergyDist_NoFireOnIID: 1000 i.i.d. N(0,1) → ≤ 4 false positives. The
// bootstrap p95 × factor 2.0 should give a per-tick FP rate well under 1%
// across the ~936 post-warmup ticks; ≤ 4 is comfortable headroom. If this
// test fails, see the fallback in the design plan: drop ECritFactor to 1.5
// (more recall, more FPs) or raise to 2.5 (fewer FPs, less recall).
func TestEnergyDist_NoFireOnIID(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	values := genGaussian(rng, 1000, 0, 1)

	d := testEnergyDistDetector()
	result := feedEnergyDistSeries(t, d, "iid", values)

	assert.LessOrEqual(t, len(result.Anomalies), 4,
		"i.i.d. N(0,1) must produce at most a handful of FPs over 1000 ticks (got %d)", len(result.Anomalies))
}

// TestEnergyDist_RemoveSeries_FreesState verifies that RemoveSeries shrinks
// the per-series state map — the SeriesRemover contract that keeps detector
// memory in step with storage eviction.
func TestEnergyDist_RemoveSeries_FreesState(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	values := genGaussian(rng, 100, 0, 1)

	d := testEnergyDistDetector()
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

// TestEnergyDist_Reset documents that Reset clears every per-series state and
// the cached series list — needed by replay/reanalysis call sites.
func TestEnergyDist_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	values := genGaussian(rng, 80, 0, 1)

	d := testEnergyDistDetector()
	feedEnergyDistSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset should clear all state")
	assert.Nil(t, d.cachedSeries, "Reset should clear cached series")
}

// TestEnergyDist_RecoveryPrevents_DoubleFire: a single sustained variance
// shift must produce exactly ONE anomaly even though the post-shift regime
// persists for 200 ticks. The post-fire structural reset (T zeroed, R copied
// from T, sums migrated) plus the recovery counter together prevent
// re-firing on the same incident.
func TestEnergyDist_RecoveryPrevents_DoubleFire(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := append(genGaussian(rng, 64, 0, 1), genGaussian(rng, 200, 0, 3)...)

	d := testEnergyDistDetector()
	result := feedEnergyDistSeries(t, d, "double_fire_check", values)

	require.Len(t, result.Anomalies, 1,
		"a single sustained variance shift must not produce repeat anomalies during the recovery+refill window")
}

// TestEnergyDist_BootstrapDeterministic: the same input must yield the same
// eCritical across two independent detector instances. The RNG is seeded
// from a fixed function of pool length so bootstrap order is reproducible
// for tests.
func TestEnergyDist_BootstrapDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	// 64 = 2·m points so both rings fill exactly once; the bootstrap fires
	// on the 64th point and eCritical is set.
	values := genGaussian(rng, 64, 0, 1)

	d1 := testEnergyDistDetector()
	feedEnergyDistSeries(t, d1, "det1", values)
	d2 := testEnergyDistDetector()
	feedEnergyDistSeries(t, d2, "det1", values)

	require.Len(t, d1.series, 1)
	require.Len(t, d2.series, 1)

	var crit1, crit2 float64
	for _, st := range d1.series {
		crit1 = st.eCritical
	}
	for _, st := range d2.series {
		crit2 = st.eCritical
	}
	assert.Greater(t, crit1, 0.0, "bootstrap must produce a positive eCritical on a non-degenerate pool")
	assert.Equal(t, crit1, crit2,
		"bootstrap must be reproducible: identical input must yield identical eCritical")
}

// TestEnergyDist_StatelessAcrossSeries verifies state isolation between two
// interleaved series with different behaviour. Series A is a steady-state
// N(0,1); series B has a clear variance shift. The stable A must remain
// quiet while B fires — proving per-series state keys (ref+agg) don't bleed
// into each other through the shared bootstrap RNG seeding strategy.
func TestEnergyDist_StatelessAcrossSeries(t *testing.T) {
	rngA := rand.New(rand.NewSource(10))
	rngB := rand.New(rand.NewSource(11))
	stableA := genGaussian(rngA, 200, 0, 1)
	shiftB := append(genGaussian(rngB, 64, 0, 1), genGaussian(rngB, 136, 0, 3)...)

	d := testEnergyDistDetector()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i := 0; i < 200; i++ {
		storage.Add("ns", "stableA", offset+stableA[i], int64(i+1), nil)
		storage.Add("ns", "shiftB", offset+shiftB[i], int64(i+1), nil)
	}
	result := d.Detect(storage, 200)

	countByName := map[string]int{}
	for _, a := range result.Anomalies {
		countByName[a.Source.Name]++
	}
	assert.Equal(t, 0, countByName["stableA"], "stable series must not fire")
	assert.GreaterOrEqual(t, countByName["shiftB"], 1, "shifting series must fire at least once")
	assert.LessOrEqual(t, countByName["shiftB"], 1,
		"shifting series must fire exactly once during the recovery window")
	assert.Len(t, d.series, 2, "per-series state must be allocated for each ref")
}

// TestEnergyDist_EnergyStatisticBasics exercises the math kernel directly so
// regressions on the (2A − B − C) closed form are caught without going
// through the streaming wrapper.
func TestEnergyDist_EnergyStatisticBasics(t *testing.T) {
	t.Run("identical samples yield zero", func(t *testing.T) {
		R := []float64{1, 2, 3, 4}
		T := []float64{1, 2, 3, 4}
		// E = (1/m²)·(2A − B − C). For identical samples, A = B/2 = C/2 (as
		// unordered pair sums × 2 from the j>i factor of 2), so
		// 2A − B − C = 0 and E = 0.
		got := energyStatistic(R, T)
		assert.InDelta(t, 0.0, got, 1e-12, "identical samples must give E=0")
	})
	t.Run("disjoint samples yield positive", func(t *testing.T) {
		R := []float64{0, 0, 0, 0}
		T := []float64{10, 10, 10, 10}
		// All pairwise |R_i − T_j| = 10, so A = m²·10 = 160. Within-window
		// gaps are zero, so B = C = 0. E = (1/m²)·2A = 2·10 = 20.
		got := energyStatistic(R, T)
		assert.InDelta(t, 20.0, got, 1e-9, "constant-disjoint samples have closed-form E=20")
	})
	t.Run("mismatched lengths return zero", func(t *testing.T) {
		assert.Equal(t, 0.0, energyStatistic([]float64{1, 2, 3}, []float64{1, 2}))
	})
	t.Run("empty inputs return zero", func(t *testing.T) {
		assert.Equal(t, 0.0, energyStatistic(nil, nil))
	})
}

// TestEnergyDist_AllEnergyAboveCritical exercises the persistence helper so
// boundary cases (empty, exactly-at-threshold, mixed) are pinned down.
func TestEnergyDist_AllEnergyAboveCritical(t *testing.T) {
	cases := []struct {
		name      string
		history   []float64
		threshold float64
		want      bool
	}{
		{"all-above", []float64{1.0, 1.5, 2.0}, 0.9, true},
		{"all-equal", []float64{1.0, 1.0, 1.0}, 1.0, true},
		{"one-below", []float64{1.5, 0.5, 1.5}, 1.0, false},
		{"all-below", []float64{0.5, 0.6, 0.4}, 1.0, false},
		{"empty", nil, 1.0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allEnergyAboveCritical(tc.history, tc.threshold)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestEnergyDist_CatalogEntryRegistered confirms the stage-1 catalog wiring
// is intact and that the post-stage-2 detector satisfies the SeriesRemover
// contract — so it must NOT be in statelessDetectorAllowlist.
func TestEnergyDist_CatalogEntryRegistered(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "energydist" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "catalog must register an 'energydist' entry")
	assert.Equal(t, componentDetector, found.kind, "energydist must be registered as a detector")
	assert.True(t, found.defaultEnabled, "energydist must be defaultEnabled per the design plan")

	// The factory must produce a working detector instance.
	inst := found.factory(nil)
	det, ok := inst.(observer.Detector)
	require.True(t, ok, "energydist factory must produce an observer.Detector")
	assert.Equal(t, "energydist", det.Name())
	// And it must implement SeriesRemover so the engine can reclaim state
	// when storage evicts a series.
	_, isRemover := inst.(observer.SeriesRemover)
	assert.True(t, isRemover, "energydist detector must implement observer.SeriesRemover")

	// energydist holds per-series state, so it MUST NOT be on the stateless
	// allowlist (would otherwise leak memory as series count grows).
	_, allowed := statelessDetectorAllowlist["energydist"]
	assert.False(t, allowed, "stateful energydist must not be on statelessDetectorAllowlist")
}
