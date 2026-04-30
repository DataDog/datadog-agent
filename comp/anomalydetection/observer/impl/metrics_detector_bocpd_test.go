// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
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
	// Use studentTPDF (df=4) to match the default LikelihoodKind="student_t".
	newLen := len(snapRunProbs) + 1
	standardProbs := make([]float64, newLen)
	var cpMass float64
	for r := range snapRunProbs {
		pred := studentTPDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r], 4.0)
		standardProbs[r+1] = snapRunProbs[r] * (1.0 - hazard) * pred
		cpMass += snapRunProbs[r] * pred
	}
	standardProbs[0] = hazard * cpMass
	normalizeProbs(standardProbs)
	expectedCpProb := standardProbs[0]

	// Independently compute the prior-only formula from the snapshot.
	// Uses the same likelihood (student_t) so the structural difference
	// (sum-over-run-lengths vs prior-only) is isolated.
	priorProbs := make([]float64, newLen)
	predPrior := studentTPDF(x, state.priorMean, state.obsVar+1.0/state.priorPrecision, 4.0)
	for r := range snapRunProbs {
		pred := studentTPDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r], 4.0)
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

func TestBOCPDDetector_StudentT_IgnoresIsolatedOutlier(t *testing.T) {
	// Isolated single-point outlier at +12σ should NOT trigger Student-t BOCPD
	// (df=4 heavy tail assigns non-negligible density, preserving run-length).
	// The same spike should trigger Gaussian BOCPD, demonstrating the contrast.
	//
	// Data: 35 stable jittered points (±2 around 100, σ≈1.63), one outlier at
	// value 120 (~12σ), then 20 more stable points.

	buildStorage := func() *timeSeriesStorage {
		st := newTimeSeriesStorage()
		for i := 0; i < 35; i++ {
			jitter := float64(i%3-1) * 2.0 // -2, 0, +2 cycling; σ≈1.63
			st.Add("ns", "test.metric", 100.0+jitter, int64(i+1), nil)
		}
		// Isolated outlier at point 36 (~12σ above baseline mean).
		st.Add("ns", "test.metric", 120.0, 36, nil)
		for i := 36; i < 56; i++ {
			jitter := float64(i%3-1) * 2.0
			st.Add("ns", "test.metric", 100.0+jitter, int64(i+1), nil)
		}
		return st
	}

	// Student-t detector (default): should ignore the isolated outlier.
	stConfig := DefaultBOCPDConfig()
	stConfig.WarmupPoints = 20
	stConfig.LikelihoodKind = "student_t"
	stDetector := NewBOCPDDetector(stConfig)
	stResult := stDetector.Detect(buildStorage(), 56)
	assert.Empty(t, stResult.Anomalies,
		"student_t BOCPD should not flag an isolated outlier; got %d anomalies", len(stResult.Anomalies))

	// Gaussian detector (legacy): should fire on the same spike.
	gConfig := DefaultBOCPDConfig()
	gConfig.WarmupPoints = 20
	gConfig.LikelihoodKind = "gaussian"
	gDetector := NewBOCPDDetector(gConfig)
	gResult := gDetector.Detect(buildStorage(), 56)
	assert.NotEmpty(t, gResult.Anomalies,
		"gaussian BOCPD should flag the isolated +12σ outlier (contrast check)")
}

func TestBOCPDDetector_StudentT_StillCatchesSustainedShift(t *testing.T) {
	// Student-t robustness should not prevent detection of a genuine sustained
	// step change. A +5σ shift maintained for 30 points accumulates enough
	// run-length posterior mass to trigger within the first 8 shifted points.
	//
	// Data: 30 stable jittered ±2 points (σ≈1.63), then 30 points at 108
	// (~+5σ above baseline).

	config := DefaultBOCPDConfig()
	config.WarmupPoints = 20
	config.LikelihoodKind = "student_t"
	d := NewBOCPDDetector(config)

	storage := newTimeSeriesStorage()
	for i := 0; i < 30; i++ {
		jitter := float64(i%3-1) * 2.0
		storage.Add("ns", "test.metric", 100.0+jitter, int64(i+1), nil)
	}
	shiftStart := int64(31)
	for i := 0; i < 30; i++ {
		storage.Add("ns", "test.metric", 108.0, int64(30+i+1), nil)
	}

	result := d.Detect(storage, 60)

	require.NotEmpty(t, result.Anomalies, "student_t BOCPD should detect a sustained +5σ step change")
	// Anomaly should fire within the first 8 points of the shifted segment.
	assert.LessOrEqual(t, result.Anomalies[0].Timestamp, shiftStart+7,
		"anomaly should fire within first 8 shifted points (got timestamp %d, shift started at %d)",
		result.Anomalies[0].Timestamp, shiftStart)
}

func TestBOCPDDetector_GaussianBackcompat(t *testing.T) {
	// Verify the legacy "gaussian" likelihood path remains functional.
	// Uses the same 100→140 step-change data as TestBOCPDDetector_DetectsStepChange,
	// but with an explicit LikelihoodKind="gaussian" override.

	config := DefaultBOCPDConfig()
	config.WarmupPoints = 20
	config.LikelihoodKind = "gaussian"
	d := NewBOCPDDetector(config)

	storage := newTimeSeriesStorage()
	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "test.metric", 140, int64(i+1), nil)
	}

	result := d.Detect(storage, 40)

	require.NotEmpty(t, result.Anomalies, "gaussian BOCPD should detect 100→140 step change")
	assert.Contains(t, result.Anomalies[0].Title, "BOCPD",
		"anomaly title should identify the BOCPD detector")
	assert.GreaterOrEqual(t, result.Anomalies[0].Timestamp, int64(21),
		"anomaly should fire after the warmup period ends")
}

func TestBOCPDDetector_StudentTPDF_Basic(t *testing.T) {
	// Sanity check studentTPDF: at the mean, value should equal the normalizing
	// constant, and the pdf should decrease as x moves away from mean.
	df := 4.0
	mean := 0.0
	scale2 := 1.0

	atMean := studentTPDF(mean, mean, scale2, df)
	atOneSigma := studentTPDF(mean+1.0, mean, scale2, df)
	atTenSigma := studentTPDF(mean+10.0, mean, scale2, df)

	assert.Greater(t, atMean, 0.0, "pdf at mean should be positive")
	assert.Greater(t, atMean, atOneSigma, "pdf should decrease away from mean")
	assert.Greater(t, atOneSigma, atTenSigma, "pdf should decrease further at 10σ")

	// Heavy tail: Student-t at 10σ should be orders of magnitude larger than Gaussian at 10σ.
	gaussianAt10Sigma := gaussianPDF(mean+10.0, mean, scale2)
	assert.Greater(t, atTenSigma, gaussianAt10Sigma*100,
		"student_t df=4 tail should be much heavier than Gaussian at 10σ")
}

func TestBOCPDDetector_StudentTPDF_MinScale2Clamp(t *testing.T) {
	// Degenerate scale2 should not produce NaN or Inf.
	result := studentTPDF(0.0, 0.0, 0.0, 4.0)
	assert.False(t, math.IsNaN(result), "studentTPDF with scale2=0 should not return NaN")
	assert.False(t, math.IsInf(result, 0), "studentTPDF with scale2=0 should not return Inf")
	assert.Greater(t, result, 0.0, "studentTPDF with scale2=0 should return positive value")
}

func TestBOCPDDetector_LikelihoodKindDefault(t *testing.T) {
	// Empty LikelihoodKind should default to "student_t" via NewBOCPDDetector.
	config := DefaultBOCPDConfig()
	config.LikelihoodKind = ""
	d := NewBOCPDDetector(config)
	assert.Equal(t, "student_t", d.config.LikelihoodKind,
		"empty LikelihoodKind should default to student_t")
}

func TestBOCPDDetector_UnknownLikelihoodKindFallsBackToStudentT(t *testing.T) {
	// An unrecognized LikelihoodKind should silently fall through the else
	// branch in updatePosterior, which is the student_t path.
	config := DefaultBOCPDConfig()
	config.WarmupPoints = 20
	config.LikelihoodKind = "laplace" // unrecognized
	d := NewBOCPDDetector(config)

	storage := newTimeSeriesStorage()
	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "test.metric", 140, int64(i+1), nil)
	}

	// Should still detect the step change (student_t path), not panic or NaN.
	result := d.Detect(storage, 40)
	require.NotEmpty(t, result.Anomalies, "unknown LikelihoodKind should fall through to student_t and still detect step change")
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
