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

// testHWResDetector returns a detector restricted to AggregateAverage so each
// test only sees one state entry per series — keeps anomaly-count assertions
// unambiguous (a duplicate Count anomaly here would otherwise mask real false
// positives).
func testHWResDetector() *HWResDetector {
	d := NewHWResDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedHWResSeries appends values to a fresh storage with consecutive
// timestamps starting at t=1, then runs Detect once at the final timestamp.
// A positive offset keeps storage in its accepted range; the level/seasonal
// recurrence is invariant to translation so this doesn't perturb the test.
func feedHWResSeries(t *testing.T, d *HWResDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

// TestHWRes_Name documents the catalog identifier the detector returns.
// Reporters key off DetectorName, so the catalog entry name and Name() must
// agree.
func TestHWRes_Name(t *testing.T) {
	d := NewHWResDetector()
	assert.Equal(t, "hwres", d.Name())
}

// TestHWRes_PureSineNoFire: 800 ticks of sin(2π·t/L) with L=hwresPeriod,
// plus N(0, 0.1) noise. The seasonal cycle absorbs all the periodic
// variance during bootstrap, leaving residuals that look like the noise.
// Strength gate passes (varSeas/varTotal ≈ 1) but no z exceeds 4.5 — let
// alone for K consecutive same-sign ticks. Expect 0 anomalies.
func TestHWRes_PureSineNoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := make([]float64, 800)
	for i := range values {
		values[i] = math.Sin(2*math.Pi*float64(i)/float64(hwresPeriod)) + 0.1*rng.NormFloat64()
	}

	d := testHWResDetector()
	result := feedHWResSeries(t, d, "pure_sine", values)

	assert.Empty(t, result.Anomalies, "pure seasonal series at the modeled period must not fire HWRes")
}

// TestHWRes_NonSeasonalRandomWalk_NoFire: 400 ticks of unit random walk —
// drift, no period. The seasonal-strength gate (varSeas/varTotal) drops as
// the random walk's total variance grows, while the seasonal indices decay
// toward zero through the recurrence; the gate should keep us silent. This
// is the additivity test: trendresid/pht/scanmw are responsible for these
// series, and HWRes must not double-fire on top of them.
func TestHWRes_NonSeasonalRandomWalk_NoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	values := make([]float64, 400)
	var x float64
	for i := range values {
		x += rng.NormFloat64()
		values[i] = x
	}

	d := testHWResDetector()
	result := feedHWResSeries(t, d, "random_walk", values)

	assert.Empty(t, result.Anomalies, "non-seasonal random walk must not fire HWRes (strength-gate additivity)")
}

// TestHWRes_StepShift_Fires: 400 pre-shift ticks of a strongly seasonal
// signal, then a +5 level step shift superimposed on the same seasonal
// pattern for another 400 ticks. The residual jumps by ≈5 in magnitude
// while MAD is still set by the small pre-shift residuals, so z is many
// times the threshold. The strength gate passes (the underlying sine is
// still there) so HWRes fires — exactly once.
func TestHWRes_StepShift_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := make([]float64, 800)
	for i := range values {
		base := math.Sin(2*math.Pi*float64(i)/float64(hwresPeriod)) + 0.1*rng.NormFloat64()
		if i >= 400 {
			base += 5
		}
		values[i] = base
	}

	d := testHWResDetector()
	result := feedHWResSeries(t, d, "step_shift", values)

	require.Len(t, result.Anomalies, 1, "a single +5 step shift on a seasonal series must fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "hwres", a.DetectorName)
	assert.Contains(t, a.Title, "Holt-Winters seasonal-residual")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.DeviationSigma, 4.5,
		"|z| at trigger must clear ZThreshold")
	// Trigger must occur after the regime change at index 400, with slack
	// for the persistence ring to fill at the post-shift residuals.
	assert.Greater(t, a.Timestamp, int64(400), "anomaly must be in the post-shift regime")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestHWRes_PhaseShift_Fires: pre-shift the model trains on sin(2πt/L);
// at tick 400 we abruptly switch to cos(2πt/L) (a π/2 phase shift). The
// seasonal model can't catch up for many ticks, so residuals stay well
// above MAD with stretches of same-sign violations far longer than the
// K=4 persistence requirement. HWRes must fire.
func TestHWRes_PhaseShift_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := make([]float64, 800)
	for i := range values {
		t01 := 2 * math.Pi * float64(i) / float64(hwresPeriod)
		var s float64
		if i < 400 {
			s = math.Sin(t01)
		} else {
			s = math.Cos(t01) // π/2 phase shift
		}
		values[i] = s + 0.1*rng.NormFloat64()
	}

	d := testHWResDetector()
	result := feedHWResSeries(t, d, "phase_shift", values)

	require.NotEmpty(t, result.Anomalies, "an abrupt π/2 phase shift on a seasonal series must fire HWRes")
	first := result.Anomalies[0]
	assert.Equal(t, "hwres", first.DetectorName)
	assert.Greater(t, first.Timestamp, int64(400), "first anomaly must be in the post-shift regime")
}

// TestHWRes_BenignSeasonalSwing_NoFire: 800 ticks of sin with amplitude
// ramping linearly from 1.0 to 1.5. The seasonal model gradually adapts;
// the |residual| quantile tracked by P² also adapts, so z stays bounded.
// Expect 0 anomalies — slow benign drifts on seasonal series must not
// fire HWRes.
func TestHWRes_BenignSeasonalSwing_NoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := make([]float64, 800)
	for i := range values {
		amp := 1.0 + 0.5*float64(i)/float64(len(values)-1)
		values[i] = amp*math.Sin(2*math.Pi*float64(i)/float64(hwresPeriod)) + 0.1*rng.NormFloat64()
	}

	d := testHWResDetector()
	result := feedHWResSeries(t, d, "benign_swing", values)

	assert.Empty(t, result.Anomalies, "gradual amplitude ramp on a seasonal series must not fire HWRes")
}

// TestHWRes_RemoveSeries_TearsDownState verifies RemoveSeries shrinks the
// per-series state map. This is the SeriesRemover contract that keeps
// detector memory in step with storage eviction; without it, hwres would
// leak ~700 B per series-ever-observed.
func TestHWRes_RemoveSeries_TearsDownState(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	values := make([]float64, 200)
	for i := range values {
		values[i] = math.Sin(2*math.Pi*float64(i)/float64(hwresPeriod)) + 0.1*rng.NormFloat64()
	}

	d := testHWResDetector()
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

// TestHWRes_Reset documents that Reset clears every per-series state and
// the cached series list — needed by replay/reanalysis call sites.
func TestHWRes_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	values := make([]float64, 80)
	for i := range values {
		values[i] = math.Sin(2*math.Pi*float64(i)/float64(hwresPeriod)) + 0.1*rng.NormFloat64()
	}

	d := testHWResDetector()
	feedHWResSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset should clear all state")
	assert.Nil(t, d.cachedSeries, "Reset should clear cached series")
}

// TestHWRes_CatalogEntryRegistered confirms the stage-1 catalog wiring is
// intact and the post-stage-2 detector now satisfies the SeriesRemover
// contract — so it must NOT remain in statelessDetectorAllowlist (otherwise
// the contract test would silently miss future memory leaks here).
func TestHWRes_CatalogEntryRegistered(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "hwres" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "catalog must register an 'hwres' entry")
	assert.Equal(t, componentDetector, found.kind, "hwres must be registered as a detector")
	assert.False(t, found.defaultEnabled, "manual corpus candidates should be --only addressable, not default-enabled")

	inst := found.factory(nil)
	det, ok := inst.(observer.Detector)
	require.True(t, ok, "hwres factory must produce an observer.Detector")
	assert.Equal(t, "hwres", det.Name())
	_, isRemover := inst.(manualSeriesRemover)
	assert.True(t, isRemover, "hwres detector must implement manualSeriesRemover")

	// hwres holds per-series state, so it MUST NOT be on the stateless
	// allowlist (would otherwise leak memory as series count grows).
	_, allowed := statelessDetectorAllowlist["hwres"]
	assert.False(t, allowed, "stateful hwres must not be on statelessDetectorAllowlist")
}
