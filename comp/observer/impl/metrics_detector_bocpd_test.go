// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"reflect"
	"strings"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBOCPDDetector returns a BOCPD detector with a short warmup suitable for
// unit tests with small data sets.
func testBOCPDDetector() *BOCPDDetector {
	config := DefaultBOCPDConfig()
	config.WarmupPoints = 20
	return NewBOCPDDetector(config)
}

func TestBOCPDDetector_Name(t *testing.T) {
	d := NewBOCPDDetector(DefaultBOCPDConfig())
	assert.Equal(t, "bocpd_detector", d.Name())
}

func TestBOCPDDetector_NotEnoughPoints(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()
	storage.Add("ns", "test.metric", 100, 1, nil)

	result := d.Detect(storage, 1)
	assert.Empty(t, result.Anomalies)
}

func TestBOCPDDetector_StableData(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 40; i++ {
		storage.Add("ns", "test.metric", 100+float64(i%3-1), int64(i+1), nil)
	}

	result := d.Detect(storage, 40)
	assert.Empty(t, result.Anomalies, "stable data should not trigger BOCPD")
}

func TestBOCPDDetector_DetectsStepChange(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "test.metric", 140, int64(i+1), nil)
	}

	result := d.Detect(storage, 40)

	require.NotEmpty(t, result.Anomalies, "should detect step change")
	assert.Contains(t, result.Anomalies[0].Title, "BOCPD")
	assert.GreaterOrEqual(t, result.Anomalies[0].Timestamp, int64(21))
}

func TestBOCPDDetector_DetectsDownwardStepChange(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 25; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 25; i < 50; i++ {
		storage.Add("ns", "test.metric", 70, int64(i+1), nil)
	}

	result := d.Detect(storage, 50)

	require.NotEmpty(t, result.Anomalies, "should detect downward step change")
	assert.Contains(t, result.Anomalies[0].Title, "BOCPD")
	assert.Contains(t, result.Anomalies[0].Description, "exceeded threshold")
}

func TestBOCPDDetector_DetectsSustainedShiftViaShortRunMass(t *testing.T) {
	config := DefaultBOCPDConfig()
	config.WarmupPoints = 20
	config.CPThreshold = 0.99     // discourage pure r_t=0 triggers
	config.CPMassThreshold = 0.55 // allow short-run posterior mass trigger
	config.ShortRunLength = 6
	d := NewBOCPDDetector(config)

	storage := newTimeSeriesStorage()

	// Use jittered warmup so the detector learns real variance (~10 stddev)
	// rather than relying on the MinVariance floor. This keeps the 100→115
	// shift small enough to trigger via short-run mass rather than cpProb.
	for i := 0; i < 30; i++ {
		storage.Add("ns", "test.metric", 100+float64(i%3-1)*5, int64(i+1), nil)
	}
	for i := 30; i < 60; i++ {
		storage.Add("ns", "test.metric", 115, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)

	require.NotEmpty(t, result.Anomalies, "should detect sustained shift")
	assert.Contains(t, result.Anomalies[0].Description, "short-run posterior mass")
}

func TestBOCPDDetector_SustainedIncidentEmitsOnce(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	// 20 stable points, then 40 shifted points.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 20; i < 60; i++ {
		storage.Add("ns", "test.metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)

	// Should emit at most one anomaly per series/agg despite sustained shift.
	anomalyCount := 0
	for _, a := range result.Anomalies {
		if a.Source.String() == "test.metric:avg" {
			anomalyCount++
		}
	}
	assert.Equal(t, 1, anomalyCount, "sustained incident should emit exactly one anomaly per series/agg")
}

func TestBOCPDDetector_IncrementalAdvance(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	// First advance: 20 stable points.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	result1 := d.Detect(storage, 20)
	assert.Empty(t, result1.Anomalies, "no anomaly in stable data")

	// Second advance: add a big jump.
	for i := 20; i < 30; i++ {
		storage.Add("ns", "test.metric", 200, int64(i+1), nil)
	}
	result2 := d.Detect(storage, 30)
	assert.NotEmpty(t, result2.Anomalies, "should detect step change on second advance")

	// Third advance: no new data — should emit nothing.
	result3 := d.Detect(storage, 30)
	assert.Empty(t, result3.Anomalies, "no new data should produce no anomalies")
}

func TestBOCPDDetector_RecoveryAndReAlert(t *testing.T) {
	config := DefaultBOCPDConfig()
	config.WarmupPoints = 20
	config.RecoveryPoints = 5
	d := NewBOCPDDetector(config)
	storage := newTimeSeriesStorage()
	ts := int64(0)

	addN := func(n int, val float64) {
		for i := 0; i < n; i++ {
			ts++
			storage.Add("ns", "m", val, ts, nil)
		}
	}

	// Warmup + stable baseline.
	addN(25, 100)
	r1 := d.Detect(storage, ts)
	assert.Empty(t, r1.Anomalies)

	// First incident.
	addN(10, 300)
	r2 := d.Detect(storage, ts)
	assert.NotEmpty(t, r2.Anomalies, "should detect first incident")

	// Recovery: stable for enough points.
	addN(20, 100)
	r3 := d.Detect(storage, ts)

	// Second incident.
	addN(10, 300)
	r4 := d.Detect(storage, ts)

	// Count total anomalies for m:avg across r3 and r4.
	avgAnomalies := 0
	for _, a := range append(r3.Anomalies, r4.Anomalies...) {
		if a.Source.String() == "m:avg" {
			avgAnomalies++
		}
	}
	assert.GreaterOrEqual(t, avgAnomalies, 1, "should detect second incident after recovery")
}

func TestBOCPDDetector_Reset(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 30; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	d.Detect(storage, 30)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
}

func TestBOCPDDetector_DeterministicReplay(t *testing.T) {
	makeDetector := func() *BOCPDDetector { return testBOCPDDetector() }

	storage := newTimeSeriesStorage()
	for i := 0; i < 20; i++ {
		storage.Add("ns", "m", 100, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "m", 200, int64(i+1), nil)
	}

	d1 := makeDetector()
	r1 := d1.Detect(storage, 40)

	d2 := makeDetector()
	r2 := d2.Detect(storage, 40)

	require.Equal(t, len(r1.Anomalies), len(r2.Anomalies), "replay should produce same anomaly count")
	for i := range r1.Anomalies {
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp, "anomaly timestamps should match")
		assert.Equal(t, r1.Anomalies[i].Source, r2.Anomalies[i].Source, "anomaly sources should match")
	}
}

func TestBOCPDDetector_DefaultAggregations(t *testing.T) {
	cfg := DefaultBOCPDConfig()
	assert.Equal(t, []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}, cfg.Aggregations)
}

func TestBOCPDDetector_DefaultWarmup120(t *testing.T) {
	cfg := DefaultBOCPDConfig()
	assert.Equal(t, 120, cfg.WarmupPoints, "default warmup should be 120 points")
}

func TestBOCPDConfig_DefaultMinVarianceIsPositive(t *testing.T) {
	cfg := DefaultBOCPDConfig()
	assert.Greater(t, cfg.MinVariance, 0.0,
		"default MinVariance should be positive")
}

func TestFindingH3_CPProbUsesOnlyPriorPredictiveNotSumOverRunLengths(t *testing.T) {
	// Confirmed as bug by author (lukesteensen) -- cascading shift detection
	// is intended. Prior-only formula was a shortcut, not a design choice.

	// Test strategy: snapshot posterior state, call updatePosterior, then
	// independently compute both standard and prior-only formulas through
	// normalization and compare against the implementation's actual output.

	warmup := 120
	config := DefaultBOCPDConfig()
	config.WarmupPoints = warmup
	config.PriorVarianceScale = 100.0
	config.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewBOCPDDetector(config)

	storage := newTimeSeriesStorage()
	for i := 0; i < warmup; i++ {
		storage.Add("ns", "metric", 10.0, int64(i+1), nil)
	}
	for i := warmup; i < warmup+150; i++ {
		storage.Add("ns", "metric", 12.0, int64(i+1), nil)
	}
	d.Detect(storage, int64(warmup+150))

	var state *bocpdSeriesState
	for _, s := range d.series {
		state = s
		break
	}
	require.NotNil(t, state)
	require.True(t, state.initialized)

	x := 14.0
	hazard := d.config.Hazard

	// Snapshot state before updatePosterior mutates it.
	snapRunProbs := make([]float64, len(state.runProbs))
	copy(snapRunProbs, state.runProbs)
	snapMeans := make([]float64, len(state.means))
	copy(snapMeans, state.means)
	snapPrecisions := make([]float64, len(state.precisions))
	copy(snapPrecisions, state.precisions)

	// Call the implementation.
	_, implCpProb, _ := d.updatePosterior(state, x)

	// Independently compute the standard BOCPD formula from the snapshot.
	// Route through the detector's predictivePDF so this test verifies the
	// recurrence structure regardless of whether Student-t or Gaussian is
	// active — what we care about is "summed predictive over run lengths,"
	// not the specific likelihood.
	newLen := len(snapRunProbs) + 1
	standardProbs := make([]float64, newLen)
	var cpMass float64
	for r := range snapRunProbs {
		pred := d.predictivePDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r])
		standardProbs[r+1] = snapRunProbs[r] * (1.0 - hazard) * pred
		cpMass += snapRunProbs[r] * pred
	}
	standardProbs[0] = hazard * cpMass
	normalizeProbs(standardProbs)
	expectedCpProb := standardProbs[0]

	// Independently compute the prior-only formula from the snapshot.
	priorProbs := make([]float64, newLen)
	predPrior := d.predictivePDF(x, state.priorMean, state.obsVar+1.0/state.priorPrecision)
	for r := range snapRunProbs {
		pred := d.predictivePDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r])
		priorProbs[r+1] = snapRunProbs[r] * (1.0 - hazard) * pred
	}
	priorProbs[0] = hazard * predPrior
	normalizeProbs(priorProbs)
	priorOnlyCpProb := priorProbs[0]

	t.Logf("standard cpProb:   %e", expectedCpProb)
	t.Logf("prior-only cpProb: %e", priorOnlyCpProb)
	t.Logf("impl cpProb:       %e", implCpProb)

	// The two formulas should differ for this scenario.
	require.Greater(t, math.Abs(expectedCpProb-priorOnlyCpProb), 1e-6,
		"test setup: standard and prior-only formulas should differ")

	// Assert the implementation matches the standard formula.
	assert.InDelta(t, expectedCpProb, implCpProb, 1e-10,
		"implementation cpProb should match standard BOCPD recurrence, not prior-only")
}

func TestFindingM6_BOCPDSkipsSameBucketValueMerges(t *testing.T) {
	// When two values arrive at the same timestamp, storage merges them into
	// one bucket. But PointCountUpTo doesn't change (still 1 bucket), so
	// BOCPD's cache check skips the series on the second Detect call.
	//
	// Steps:
	// 1. Add first value at timestamp T, call Detect
	// 2. Add second value at same timestamp T (storage merges), call Detect again
	// 3. Assert the detector processed the updated merged value

	config := DefaultBOCPDConfig()
	config.WarmupPoints = 5
	config.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewBOCPDDetector(config)

	storage := newTimeSeriesStorage()

	// Build warmup data (timestamps 1-4)
	for i := 1; i <= 4; i++ {
		storage.Add("ns", "metric", 100.0, int64(i), nil)
	}

	// Add first value at timestamp 5 (this completes warmup)
	storage.Add("ns", "metric", 100.0, 5, nil)
	d.Detect(storage, 5)

	// Add a second value at timestamp 5 -- storage merges into the same bucket.
	// Average of {100, 200} = 150.
	storage.Add("ns", "metric", 200.0, 5, nil)

	// Verify storage actually merged: average should be 150, not 100.
	series := storage.GetSeriesRange(observer.SeriesRef(0), 4, 5, observer.AggregateAverage)
	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	assert.Equal(t, 150.0, series.Points[0].Value,
		"storage should have merged the two values at timestamp 5")

	// Now Detect again. The detector should process the updated merged value.
	// But the bug is: PointCountUpTo still returns 5 (same as before), so the
	// detector's lastProcessedCount check causes it to skip this series.

	// To detect whether the detector re-processed, we check its internal state.
	// After processing x=100 at t=5, the posterior was updated with x=100.
	// After re-processing x=150 (merged average), it should update with x=150.
	// But if skipped, the posterior still reflects x=100.

	// Snapshot the writeGeneration the detector saw after first Detect.
	var stateBefore *bocpdSeriesState
	for _, s := range d.series {
		stateBefore = s
		break
	}
	require.NotNil(t, stateBefore)
	genBefore := stateBefore.lastWriteGen

	d.Detect(storage, 5)

	var stateAfter *bocpdSeriesState
	for _, s := range d.series {
		stateAfter = s
		break
	}
	genAfter := stateAfter.lastWriteGen

	pointCount := storage.PointCountUpTo(observer.SeriesRef(0), 5)
	t.Logf("genBefore=%d, genAfter=%d, PointCountUpTo=%d, writeGen=%d",
		genBefore, genAfter, pointCount, storage.WriteGeneration(observer.SeriesRef(0)))

	// The detector should notice the merge via writeGeneration even though
	// PointCountUpTo didn't change. If it re-processed, genAfter > genBefore.
	assert.Greater(t, genAfter, genBefore,
		"detector should re-process when a same-bucket merge changes the value; "+
			"lastWriteGen should advance but didn't (%d == %d)", genBefore, genAfter)
}

func TestFindingM7_WarmupPointsOneCausesNaN(t *testing.T) {
	config := DefaultBOCPDConfig()
	config.WarmupPoints = 1
	config.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewBOCPDDetector(config)

	storage := newTimeSeriesStorage()

	// Feed a few points. With WarmupPoints=1, the first point triggers
	// initializeFromWarmup with warmupCount=1, causing 0/0 = NaN in
	// variance = warmupM2 / (warmupCount - 1).
	for i := 0; i < 10; i++ {
		storage.Add("ns", "metric", 100.0+float64(i), int64(i+1), nil)
	}

	result := d.Detect(storage, 10)

	// Check that no anomaly has NaN in its debug info.
	for _, a := range result.Anomalies {
		if a.DebugInfo != nil {
			assert.False(t, math.IsNaN(a.DebugInfo.BaselineMean),
				"NaN in BaselineMean due to WarmupPoints=1")
			assert.False(t, math.IsNaN(a.DebugInfo.BaselineStddev),
				"NaN in BaselineStddev due to WarmupPoints=1")
			assert.False(t, math.IsNaN(a.DebugInfo.CurrentValue),
				"NaN in CurrentValue due to WarmupPoints=1")
			assert.False(t, math.IsNaN(a.DebugInfo.DeviationSigma),
				"NaN in DeviationSigma due to WarmupPoints=1")
		}
	}

	// Also verify the detector's internal state is not corrupted with NaN.
	for key, state := range d.series {
		assert.False(t, math.IsNaN(state.baselineMean),
			"NaN baselineMean in series state %s", key)
		assert.False(t, math.IsNaN(state.baselineStddev),
			"NaN baselineStddev in series state %s", key)
		assert.False(t, math.IsNaN(state.obsVar),
			"NaN obsVar in series state %s", key)
		assert.False(t, math.IsNaN(state.priorMean),
			"NaN priorMean in series state %s", key)
		assert.False(t, math.IsNaN(state.priorPrecision),
			"NaN priorPrecision in series state %s", key)
		if state.initialized {
			for i, p := range state.runProbs {
				assert.False(t, math.IsNaN(p),
					"NaN in runProbs[%d] for series %s", i, key)
			}
		}
	}
}

func TestFindingM8_ShortRunMassExcludesCPProb(t *testing.T) {
	// shortRunLengthMass should sum runProbs[1] through runProbs[ShortRunLength],
	// excluding runProbs[0] (cpProb). The two trigger conditions (peak cpProb
	// vs short-run mass) should be independent.

	runProbs := make([]float64, 20)
	runProbs[0] = 0.55 // cpProb
	runProbs[1] = 0.05
	runProbs[2] = 0.04
	runProbs[3] = 0.04
	runProbs[4] = 0.04
	runProbs[5] = 0.04
	remaining := 1.0 - (0.55 + 0.05 + 0.04*4)
	for i := 6; i < len(runProbs); i++ {
		runProbs[i] = remaining / float64(len(runProbs)-6)
	}

	shortRunLength := 5

	mass := shortRunLengthMass(runProbs, shortRunLength)

	// Expected: sum of runProbs[1..5] = 0.05 + 0.04*4 = 0.21
	expectedMass := 0.05 + 0.04*4
	t.Logf("shortRunMass = %.4f, expected = %.4f (excluding cpProb=%.2f)", mass, expectedMass, runProbs[0])

	assert.InDelta(t, expectedMass, mass, 0.001,
		"shortRunLengthMass should exclude runProbs[0] (cpProb); "+
			"got %.4f but expected %.4f", mass, expectedMass)

	// Confirm this mass is below CPMassThreshold -- proving the short-run
	// trigger would NOT fire on its own without cpProb inflating it.
	assert.Less(t, mass, 0.7,
		"short-run mass (%.4f) without cpProb should be below CPMassThreshold (0.7)", mass)
}

// ---------------------------------------------------------------------------
// H2B-bocpd-clean-stack-validation: combined Student-t + persistence stack.
// ---------------------------------------------------------------------------

// jitteredBaseline writes nPoints with a small deterministic jitter so the
// detector sees non-trivial baseline variance (otherwise the warmup variance
// is floored at MinVariance=1.0 and any deviation reads as hundreds of σ —
// a regime where Student-t's heavier tails are still relatively-tiny across
// run-length hypotheses, so the test doesn't actually exercise the
// likelihood layer).
func jitteredBaseline(s *timeSeriesStorage, mean float64, nPoints int, startTs int64) int64 {
	ts := startTs
	pattern := []float64{0, 5, -3, 2, -4, 6, -2, 4, -5, 1}
	for i := 0; i < nPoints; i++ {
		s.Add("ns", "test.metric", mean+pattern[i%len(pattern)], ts, nil)
		ts++
	}
	return ts
}

// TestBOCPDDetector_StudentTSuppressesIsolatedOutlier verifies that the
// combined Student-t + persistence stack does not fire on an isolated
// outlier embedded in a jittered baseline, while the same data under
// Gaussian + persistence=1 (legacy behavior) does fire. This locks in the
// candidate's headline robustness claim.
func TestBOCPDDetector_StudentTSuppressesIsolatedOutlier(t *testing.T) {
	makeStorage := func() *timeSeriesStorage {
		s := newTimeSeriesStorage()
		ts := jitteredBaseline(s, 100, 60, 1) // baseline σ ≈ 4
		// One isolated outlier ≈ 6σ above baseline.
		s.Add("ns", "test.metric", 125, ts, nil)
		ts++
		// Return to baseline for many points so any ringing has time to settle.
		jitteredBaseline(s, 100, 60, ts)
		return s
	}

	// With Student-t (DoF=5) and persistence=3 (defaults), the outlier
	// produces low predictive density without inflating cpProb or short-run
	// mass for ≥3 consecutive points → no alert.
	cfgRobust := DefaultBOCPDConfig()
	cfgRobust.WarmupPoints = 30
	cfgRobust.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewBOCPDDetector(cfgRobust)
	result := d.Detect(makeStorage(), 200)
	assert.Empty(t, result.Anomalies,
		"isolated 6σ outlier should not alert with default Student-t + persistence")

	// Sanity check: same scenario with Gaussian likelihood AND persistence=1
	// (the legacy stack) DOES alert on the outlier. If this assertion fails
	// then the scenario isn't actually exercising the spike path and the
	// "no alert" above is a false-negative pass.
	cfgFragile := cfgRobust
	cfgFragile.StudentTDoF = 0
	cfgFragile.PersistenceCount = 1
	dFragile := NewBOCPDDetector(cfgFragile)
	resultFragile := dFragile.Detect(makeStorage(), 200)
	require.NotEmpty(t, resultFragile.Anomalies,
		"sanity: Gaussian + persistence=1 must fire on the same outlier; "+
			"otherwise the robust-stack pass is meaningless")
}

// TestBOCPDDetector_StudentTAlonePersistenceDisabled isolates the Student-t
// contribution: with persistence disabled (=1), Student-t alone should fire
// substantially fewer alerts on a 6σ outlier than Gaussian alone. This
// proves Student-t is doing real work at the likelihood layer rather than
// being shadowed entirely by persistence.
func TestBOCPDDetector_StudentTAlonePersistenceDisabled(t *testing.T) {
	makeStorage := func() *timeSeriesStorage {
		s := newTimeSeriesStorage()
		ts := jitteredBaseline(s, 100, 60, 1)
		s.Add("ns", "test.metric", 125, ts, nil)
		ts++
		jitteredBaseline(s, 100, 60, ts)
		return s
	}

	cfgT := DefaultBOCPDConfig()
	cfgT.WarmupPoints = 30
	cfgT.PersistenceCount = 1
	cfgT.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	dT := NewBOCPDDetector(cfgT)
	resultT := dT.Detect(makeStorage(), 200)

	cfgG := cfgT
	cfgG.StudentTDoF = 0
	dG := NewBOCPDDetector(cfgG)
	resultG := dG.Detect(makeStorage(), 200)

	t.Logf("student-t alerts: %d   gaussian alerts: %d",
		len(resultT.Anomalies), len(resultG.Anomalies))
	// Strict claim: Student-t fires strictly fewer false alerts than Gaussian
	// on the same outlier-only data. This is the per-likelihood gain that
	// motivates including Student-t in the stack at all.
	assert.Less(t, len(resultT.Anomalies), len(resultG.Anomalies),
		"Student-t alone should fire fewer false alerts than Gaussian alone "+
			"on a single-outlier scenario (persistence disabled in both)")
}

// TestBOCPDDetector_PersistenceSuppressesShortBurst verifies that a short
// burst (fewer points than PersistenceCount) does not raise an alert. We
// intentionally build a configuration where the per-point trigger DOES fire
// at every burst point so the test isolates the persistence-gate effect.
func TestBOCPDDetector_PersistenceSuppressesShortBurst(t *testing.T) {
	cfg := DefaultBOCPDConfig()
	cfg.WarmupPoints = 30
	cfg.PersistenceCount = 4 // require 4 consecutive triggered points
	// Disable Student-t so the per-point trigger fires reliably on the burst;
	// otherwise we'd be testing a confounded Student-t × persistence effect.
	cfg.StudentTDoF = 0
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewBOCPDDetector(cfg)
	storage := newTimeSeriesStorage()

	ts := jitteredBaseline(storage, 100, 30, 1)
	// 2-point burst (less than PersistenceCount=4) then return to baseline.
	// Stays well under PersistenceCount even after accounting for one or two
	// points of post-burst short-run mass redistribution before things
	// settle.
	for i := 0; i < 2; i++ {
		storage.Add("ns", "test.metric", 200, ts, nil)
		ts++
	}
	jitteredBaseline(storage, 100, 30, ts)

	// Sanity baseline: with persistence=1, the same burst DOES fire.
	cfgLegacy := cfg
	cfgLegacy.PersistenceCount = 1
	dLegacy := NewBOCPDDetector(cfgLegacy)
	resultLegacy := dLegacy.Detect(storage, ts)
	require.NotEmpty(t, resultLegacy.Anomalies,
		"sanity: persistence=1 must fire on the 2-point burst; otherwise the "+
			"persistence-suppression assertion below is vacuously true")

	result := d.Detect(storage, ts)
	// Allow the burst + at most one or two points of post-burst ringing —
	// the burst should never sustain triggers for ≥PersistenceCount=4
	// consecutive points → no alert at all.
	assert.Empty(t, result.Anomalies,
		"2-point burst should be suppressed when PersistenceCount=4")
}

// TestBOCPDDetector_PersistenceAllowsSustainedChangepoint verifies recall:
// a sustained changepoint that lasts longer than PersistenceCount must still
// fire. This guards against persistence becoming a silent kill switch and
// is the "sustained changepoint recall" test from the candidate spec.
func TestBOCPDDetector_PersistenceAllowsSustainedChangepoint(t *testing.T) {
	cfg := DefaultBOCPDConfig()
	cfg.WarmupPoints = 30
	// Use the production defaults for both pieces — this test has to pass
	// with the same config that the eval pipeline uses.
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewBOCPDDetector(cfg)
	storage := newTimeSeriesStorage()

	ts := jitteredBaseline(storage, 100, 40, 1)
	cpStart := ts
	// Sustained 6σ shift for 40 points, well past PersistenceCount=3.
	for i := 0; i < 40; i++ {
		storage.Add("ns", "test.metric", 125+float64(i%3-1), ts, nil)
		ts++
	}

	result := d.Detect(storage, ts)
	require.NotEmpty(t, result.Anomalies,
		"sustained 40-point changepoint must fire under default Student-t + persistence")
	// First emission must be at or after the PersistenceCount-th spike point
	// (cpStart + PersistenceCount - 1), confirming the gate is real and the
	// detector isn't firing pre-changepoint by accident.
	minExpected := cpStart + int64(cfg.PersistenceCount) - 1
	assert.GreaterOrEqual(t, result.Anomalies[0].Timestamp, minExpected,
		"alert timestamp should be at or after the persistence-confirmation point (cpStart=%d, persistence=%d)",
		cpStart, cfg.PersistenceCount)
}

// TestBOCPDDetector_StudentTPDFIntegralIsClose1 sanity-checks the Student-t
// PDF normalization via Riemann integration over a wide window. A bug in the
// log-gamma constant would silently shift cpProb across all triggers; this
// test guards against that without depending on full BOCPD dynamics.
func TestBOCPDDetector_StudentTPDFIntegralIsClose1(t *testing.T) {
	// dof=5, location=0, variance=1. Integrate over [-50, 50] with step 0.01.
	const dof, loc, variance = 5.0, 0.0, 1.0
	const lo, hi, step = -50.0, 50.0, 0.01
	var integral float64
	for x := lo; x < hi; x += step {
		integral += studentTPDF(x, loc, variance, dof) * step
	}
	assert.InDelta(t, 1.0, integral, 0.005,
		"Student-t PDF should integrate to ~1; got %.4f", integral)
}

// TestBOCPDDetector_StudentTHasHeavierTailsThanGaussian asserts the headline
// property that justifies using Student-t at all: at deep tails (5σ) the
// Student-t density should be substantially larger than the Gaussian, so a
// single outlier no longer drives cpProb to 1 by single-handedly making all
// non-cp hypotheses' likelihoods underflow.
func TestBOCPDDetector_StudentTHasHeavierTailsThanGaussian(t *testing.T) {
	const variance = 1.0
	x := 5.0 // 5 sigma
	gaussian := gaussianPDF(x, 0, variance)
	tDensity := studentTPDF(x, 0, variance, 5.0)
	t.Logf("gaussian(5σ) = %.6e   student-t(5σ, dof=5) = %.6e", gaussian, tDensity)
	assert.Greater(t, tDensity, gaussian*100,
		"Student-t (dof=5) at 5σ should be >100x the Gaussian density "+
			"(heavy-tailed predictive is the whole point of this stack)")
}

// TestBOCPDDetector_NoEntropyGate documents and locks in the deliberate
// decision to NOT add a posterior-entropy gate to this candidate. The
// existing CPThreshold/CPMassThreshold already threshold posterior shape; an
// entropy gate would either re-test the same thing or, used as suppression,
// mute alerts during the high-entropy transient that immediately precedes a
// new run-length winning out — which is exactly when a real changepoint
// should fire.
//
// We assert this contract by:
//  1. Confirming the public BOCPDConfig has no entropy-related field.
//  2. Confirming the runtime behavior: cpProb and short-run mass alone are
//     sufficient to trigger and suppress as expected (covered by the
//     suppression and recall tests above), without any entropy threshold.
func TestBOCPDDetector_NoEntropyGate(t *testing.T) {
	cfg := DefaultBOCPDConfig()
	// If a future PR adds an EntropyThreshold (or similar), this test should
	// trip and the author must justify it on its own merits — and re-run the
	// suppression/recall tests above. The "negative" assertion is intentional
	// design pressure: stack one knob at a time, each with evidence.
	v := reflect.ValueOf(cfg)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		assert.NotContains(t, strings.ToLower(name), "entropy",
			"BOCPDConfig must not grow an entropy gate without an explicit "+
				"motivating test (see comment on PersistenceCount in metrics_detector_bocpd.go)")
	}
}
