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

// testBOCPDDetector returns a BOCPD detector with a short warmup suitable for
// unit tests with small data sets.
func testBOCPDDetector() *BOCPDDetector {
	d := NewBOCPDDetector()
	d.WarmupPoints = 20
	return d
}

func TestBOCPDDetector_Name(t *testing.T) {
	d := NewBOCPDDetector()
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
	d := testBOCPDDetector()
	d.CPThreshold = 0.99     // discourage pure r_t=0 triggers
	d.CPMassThreshold = 0.55 // allow short-run posterior mass trigger
	d.ShortRunLength = 6

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
		if a.Source == "test.metric:avg" {
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
	d := testBOCPDDetector()
	d.RecoveryPoints = 5
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
		if a.Source == "m:avg" {
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
	d := NewBOCPDDetector()
	assert.Equal(t, []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}, d.Aggregations)
}

func TestBOCPDDetector_DefaultWarmup120(t *testing.T) {
	d := NewBOCPDDetector()
	assert.Equal(t, 120, d.WarmupPoints, "default warmup should be 120 points")
}

func TestFindingH2_MinVarianceZeroNotGuarded(t *testing.T) {
	// ensureDefaults has no guard against MinVariance <= 0.
	// After ensureDefaults runs, MinVariance should be >= some positive floor.
	// This test verifies the config guard is missing.
	d := &BOCPDDetector{
		MinVariance: 0,
	}
	d.ensureDefaults()
	assert.Greater(t, d.MinVariance, 0.0,
		"ensureDefaults should reject MinVariance=0 and set a positive floor, but it does not")

	dNeg := &BOCPDDetector{
		MinVariance: -1.0,
	}
	dNeg.ensureDefaults()
	assert.Greater(t, dNeg.MinVariance, 0.0,
		"ensureDefaults should reject MinVariance<0 and set a positive floor, but it does not")
}

func TestFindingH3_CPProbUsesOnlyPriorPredictiveNotSumOverRunLengths(t *testing.T) {
	// Confirmed as bug by author (lukesteensen) -- cascading shift detection
	// is intended. Prior-only formula was a shortcut, not a design choice.

	// Test strategy: snapshot posterior state, call updatePosterior, then
	// independently compute both standard and prior-only formulas through
	// normalization and compare against the implementation's actual output.

	warmup := 120
	d := &BOCPDDetector{
		WarmupPoints:       warmup,
		Hazard:             0.05,
		CPThreshold:        0.6,
		ShortRunLength:     5,
		CPMassThreshold:    0.7,
		MaxRunLength:       200,
		PriorVarianceScale: 100.0,
		MinVariance:        1.0,
		RecoveryPoints:     10,
		Aggregations:       []observer.Aggregate{observer.AggregateAverage},
		series:             make(map[bocpdStateKey]*bocpdSeriesState),
	}

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
	hazard := d.Hazard

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
	newLen := len(snapRunProbs) + 1
	standardProbs := make([]float64, newLen)
	var cpMass float64
	for r := range snapRunProbs {
		pred := gaussianPDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r])
		standardProbs[r+1] = snapRunProbs[r] * (1.0 - hazard) * pred
		cpMass += snapRunProbs[r] * pred
	}
	standardProbs[0] = hazard * cpMass
	normalizeProbs(standardProbs)
	expectedCpProb := standardProbs[0]

	// Independently compute the prior-only formula from the snapshot.
	priorProbs := make([]float64, newLen)
	predPrior := gaussianPDF(x, state.priorMean, state.obsVar+1.0/state.priorPrecision)
	for r := range snapRunProbs {
		pred := gaussianPDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r])
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

	d := NewBOCPDDetector()
	d.WarmupPoints = 5
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}

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
	series := storage.GetSeriesRange(observer.SeriesHandle(0), 4, 5, observer.AggregateAverage)
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

	pointCount := storage.PointCountUpTo(observer.SeriesHandle(0), 5)
	t.Logf("genBefore=%d, genAfter=%d, PointCountUpTo=%d, writeGen=%d",
		genBefore, genAfter, pointCount, storage.WriteGeneration(observer.SeriesHandle(0)))

	// The detector should notice the merge via writeGeneration even though
	// PointCountUpTo didn't change. If it re-processed, genAfter > genBefore.
	assert.Greater(t, genAfter, genBefore,
		"detector should re-process when a same-bucket merge changes the value; "+
			"lastWriteGen should advance but didn't (%d == %d)", genBefore, genAfter)
}

func TestFindingM7_WarmupPointsOneCausesNaN(t *testing.T) {
	d := NewBOCPDDetector()
	d.WarmupPoints = 1
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}

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
