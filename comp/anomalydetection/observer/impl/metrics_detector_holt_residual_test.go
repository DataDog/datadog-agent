// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math/rand"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHoltResidualDetector returns a detector configured for the
// single-aggregation tests below. Default Aggregations includes Count;
// pinning to Average keeps anomaly counts deterministic.
func testHoltResidualDetector() *HoltResidualDetector {
	d := NewHoltResidualDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

func newDetectorTestStorage() *timeSeriesStorage {
	cfg := DefaultStorageConfig()
	cfg.PointRetentionSecs = 0
	return newTimeSeriesStorageWith(cfg)
}

// addConstant feeds n points of the same value at consecutive 1-second
// timestamps starting at startTs.
func addConstant(t *testing.T, storage *timeSeriesStorage, name string, count int, startTs int64, value float64) {
	t.Helper()
	for i := 0; i < count; i++ {
		storage.Add("ns", name, value, startTs+int64(i), nil)
	}
}

// addRamp feeds n points of a linear ramp starting at startTs. The first
// point has value baseValue + slope, the second baseValue + 2*slope, etc.
// (1-indexed within the call).
func addRamp(t *testing.T, storage *timeSeriesStorage, name string, count int, startTs int64, baseValue, slope float64) {
	t.Helper()
	for i := 0; i < count; i++ {
		storage.Add("ns", name, baseValue+slope*float64(i+1), startTs+int64(i), nil)
	}
}

// TestHoltResidual_Constant_NoFire feeds 500 identical values. The smoother
// converges instantly (level=value, trend=0) and every residual is zero —
// no anomaly should be emitted.
func TestHoltResidual_Constant_NoFire(t *testing.T) {
	d := testHoltResidualDetector()
	storage := newDetectorTestStorage()

	addConstant(t, storage, "metric", 500, 1, 7.0)

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "constant input must not trigger Holt residual")
}

// TestHoltResidual_PureRamp_NoFire feeds 500 points of a noise-free linear
// ramp. The Holt smoother locks onto the trend after warmup so steady-state
// residuals collapse to zero and the σ_value gate (which sees a wide raw
// MAD over the rolling 60-point window) blocks any residual standardisation
// noise from firing. This test pins the contract that drift alone is not
// an anomaly — only a forecast deviation is.
func TestHoltResidual_PureRamp_NoFire(t *testing.T) {
	d := testHoltResidualDetector()
	storage := newDetectorTestStorage()

	addRamp(t, storage, "metric", 500, 1, 0.0, 0.05)

	result := d.Detect(storage, 500)
	assert.Empty(t, result.Anomalies, "noise-free ramp must not fire — Holt should track the trend")
}

// TestHoltResidual_RampWithSpike_FiresOnce feeds a clean linear ramp
// punctuated by a 2-point spike. The first spike point arms the
// consecutive counter; the second confirms it and fires. The σ_value gate
// passes because the spike magnitude (20) dwarfs the ramp's MAD (~0.5
// over 60 points). The detector must emit exactly one anomaly.
func TestHoltResidual_RampWithSpike_FiresOnce(t *testing.T) {
	d := testHoltResidualDetector()
	storage := newDetectorTestStorage()

	const spikeStart int64 = 300
	const spikeLen = 2
	const spikeBoost = 20.0

	for ts := int64(1); ts <= 500; ts++ {
		v := 0.05 * float64(ts)
		if ts >= spikeStart && ts < spikeStart+spikeLen {
			v += spikeBoost
		}
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	require.Len(t, result.Anomalies, 1, "exactly one fire expected for a 2-point spike on a clean ramp")

	a := result.Anomalies[0]
	assert.Equal(t, "holt_residual", a.DetectorName)
	assert.Contains(t, a.Title, "Holt residual")
	require.NotNil(t, a.Score)
	assert.Greater(t, *a.Score, 4.5, "score should clear the |z| threshold")
	assert.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
	assert.Equal(t, 4.5, a.DebugInfo.Threshold)
	// Fire timestamp lands on the second spike point (M=2 confirmation).
	assert.Equal(t, spikeStart+spikeLen-1, a.Timestamp)
}

// TestHoltResidual_RampWithGaussianNoise_AtMostOne feeds a noisy linear
// ramp (σ=1) for 500 points. ConfirmM=2 plus the σ_value effect-size gate
// suppress the random tails: at the chosen seed we expect zero anomalies,
// but the contract is "at most one" — any more would point to a recall
// regression (the family of bugs exp-0008/0009 hit).
func TestHoltResidual_RampWithGaussianNoise_AtMostOne(t *testing.T) {
	d := testHoltResidualDetector()
	storage := newDetectorTestStorage()

	rng := rand.New(rand.NewSource(42))
	for ts := int64(1); ts <= 500; ts++ {
		v := 0.05*float64(ts) + rng.NormFloat64()
		storage.Add("ns", "metric", v, ts, nil)
	}

	result := d.Detect(storage, 500)
	assert.LessOrEqual(t, len(result.Anomalies), 1, "Gaussian noise must not produce more than one false fire")
}

// TestHoltResidual_StepChange_FiresAndAdapts feeds 200 baseline points
// followed by 100 post-step points. The fire must happen within ConfirmM
// points of the step (i.e. by the second post-step point) and the smoothed
// level must converge to the new regime — the smoother continues running
// through the fire and the refractory.
func TestHoltResidual_StepChange_FiresAndAdapts(t *testing.T) {
	d := testHoltResidualDetector()
	storage := newDetectorTestStorage()

	const stepStart int64 = 200
	const stepValue = 5.0

	addConstant(t, storage, "metric", 199, 1, 0.0)
	addConstant(t, storage, "metric", 100, stepStart, stepValue)

	result := d.Detect(storage, 299)
	require.NotEmpty(t, result.Anomalies, "step change must fire at least once")
	require.Len(t, result.Anomalies, 1, "refractory should suppress the second fire on a single step")

	a := result.Anomalies[0]
	// Fire must land within ConfirmM points of the step.
	assert.GreaterOrEqual(t, a.Timestamp, stepStart, "fire must come on or after the step")
	assert.LessOrEqual(t, a.Timestamp, stepStart+int64(d.ConfirmM-1), "fire must come within ConfirmM points of the step")

	// After running the full advance, the smoother must have tracked the
	// new regime — well past 1/α points the level should be close to the
	// new baseline. The strict "within 1/α" claim is asymptotic; we use a
	// generous post-refractory bound so the test isn't fragile.
	state := d.series[holtStateKey{ref: a.SourceRef.Ref, agg: observer.AggregateAverage}]
	require.NotNil(t, state)
	assert.Greater(t, state.level, 4.0, "smoothed level should converge toward the new regime (5.0)")
}

// TestHoltResidual_Refractory_BlocksNearbySpike confirms the refractory
// contract: a second spike starting fewer than Refractory points after the
// first fire is suppressed, while a spike past the refractory window fires
// again. Both halves run on a single fresh detector to avoid coupling.
func TestHoltResidual_Refractory_BlocksNearbySpike(t *testing.T) {
	t.Run("configured_window_is_not_consumed_by_firing_point", func(t *testing.T) {
		d := &HoltResidualDetector{
			Alpha:           0.01,
			Beta:            0.01,
			ResidualWindow:  4,
			ZThreshold:      0.5,
			ConfirmM:        1,
			MinDeviationMAD: 0,
			Refractory:      1,
			Aggregations:    []observer.Aggregate{observer.AggregateAverage},
			series:          make(map[holtStateKey]*holtSeriesState),
		}
		storage := newDetectorTestStorage()
		addConstant(t, storage, "metric", 28, 1, 0)
		addConstant(t, storage, "metric", 2, 29, 100)

		result := d.Detect(storage, 30)
		require.Len(t, result.Anomalies, 1, "Refractory=1 must suppress the point immediately after a fire")
		assert.Equal(t, int64(29), result.Anomalies[0].Timestamp)
	})

	t.Run("nearby_spike_suppressed", func(t *testing.T) {
		d := testHoltResidualDetector()
		storage := newDetectorTestStorage()

		// 99 baseline points (to give resWin/valWin time to fill).
		addConstant(t, storage, "metric", 99, 1, 0.0)
		// Spike A: 2 points at ts 100,101 — fires at 101.
		addConstant(t, storage, "metric", 2, 100, 20.0)
		// Gap of 4 baseline points.
		addConstant(t, storage, "metric", 4, 102, 0.0)
		// Spike B: 2 points at ts 106,107 — well within refractory.
		addConstant(t, storage, "metric", 2, 106, 20.0)
		// Trailing baseline so the cursor reaches the spike.
		addConstant(t, storage, "metric", 50, 108, 0.0)

		result := d.Detect(storage, 158)
		require.Len(t, result.Anomalies, 1, "refractory must suppress the second spike when 5 points apart")
		assert.Equal(t, int64(101), result.Anomalies[0].Timestamp)
	})

	t.Run("distant_spike_fires_again", func(t *testing.T) {
		d := testHoltResidualDetector()
		storage := newDetectorTestStorage()

		addConstant(t, storage, "metric", 99, 1, 0.0)
		addConstant(t, storage, "metric", 2, 100, 20.0)
		// Gap of 24 baseline points puts the second spike's fire at
		// t=127 — well past the 20-point refractory window armed at t=101.
		addConstant(t, storage, "metric", 24, 102, 0.0)
		addConstant(t, storage, "metric", 2, 126, 20.0)
		addConstant(t, storage, "metric", 50, 128, 0.0)

		result := d.Detect(storage, 178)
		require.Len(t, result.Anomalies, 2, "two spikes 25 points apart must both fire")
		assert.Equal(t, int64(101), result.Anomalies[0].Timestamp)
		assert.Equal(t, int64(127), result.Anomalies[1].Timestamp)
	})
}

// TestHoltResidual_RemoveSeries verifies the SeriesRemover contract:
// after Detect populates per-series state, RemoveSeries must drop it so
// the detector's memory tracks storage's series cardinality.
func TestHoltResidual_RemoveSeries(t *testing.T) {
	d := testHoltResidualDetector()
	storage := newDetectorTestStorage()

	addConstant(t, storage, "metric", 100, 1, 1.0)
	d.Detect(storage, 100)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})
	assert.Empty(t, d.series, "RemoveSeries must drop per-series state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries should invalidate the series cache")
}

// TestHoltResidual_Reset confirms that Reset wipes per-series state.
func TestHoltResidual_Reset(t *testing.T) {
	d := testHoltResidualDetector()
	storage := newDetectorTestStorage()

	addConstant(t, storage, "metric", 100, 1, 1.0)
	d.Detect(storage, 100)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}

// TestHoltResidual_DefaultsApplied confirms ensureDefaults populates a
// zero-valued struct so reflective construction (and any caller that
// bypasses NewHoltResidualDetector) still produces a usable detector.
func TestHoltResidual_DefaultsApplied(t *testing.T) {
	d := &HoltResidualDetector{}
	storage := newDetectorTestStorage()

	_ = d.Detect(storage, 1)

	assert.Equal(t, 0.2, d.Alpha)
	assert.Equal(t, 0.05, d.Beta)
	assert.Equal(t, 24, d.WarmupPoints)
	assert.Equal(t, 60, d.ResidualWindow)
	assert.Equal(t, 4.5, d.ZThreshold)
	assert.Equal(t, 2, d.ConfirmM)
	assert.Equal(t, 3.0, d.MinDeviationMAD)
	assert.Equal(t, 20, d.Refractory)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestHoltResidual_Name pins the catalog identifier so the catalog test
// stays in sync with the detector identifier.
func TestHoltResidual_Name(t *testing.T) {
	assert.Equal(t, "holt_residual", NewHoltResidualDetector().Name())
}

// TestHoltResidual_InterfaceContracts checks the structural promises that
// the catalog and engine both rely on: HoltResidualDetector must satisfy
// observer.Detector AND observer.SeriesRemover (it is stateful and is NOT
// listed in statelessDetectorAllowlist).
func TestHoltResidual_InterfaceContracts(_ *testing.T) {
	d := NewHoltResidualDetector()
	var _ observer.Detector = d
	var _ observer.SeriesRemover = d
}

// TestHoltResidual_IncrementalMatchesBatch verifies that streaming advances
// emit the same anomaly timestamps as a single batch replay over the same
// points. This guards the per-series cursor and rolling-window state.
func TestHoltResidual_IncrementalMatchesBatch(t *testing.T) {
	batch := testHoltResidualDetector()
	incremental := testHoltResidualDetector()
	batchStorage := newDetectorTestStorage()
	incrementalStorage := newDetectorTestStorage()

	const end int64 = 500
	values := make([]float64, end)
	for ts := int64(1); ts <= end; ts++ {
		v := 0.05 * float64(ts)
		if ts >= 300 && ts < 302 {
			v += 20.0
		}
		values[ts-1] = v
		batchStorage.Add("ns", "metric", v, ts, nil)
	}
	batchResult := batch.Detect(batchStorage, end)

	var incrementalAnomalies []observer.Anomaly
	for i, v := range values {
		ts := int64(i + 1)
		incrementalStorage.Add("ns", "metric", v, ts, nil)
		result := incremental.Detect(incrementalStorage, ts)
		incrementalAnomalies = append(incrementalAnomalies, result.Anomalies...)
	}

	assert.Equal(t, anomalyTimestamps(batchResult.Anomalies), anomalyTimestamps(incrementalAnomalies))
}

// TestHoltResidual_ReprocessesSameBucketMergeAndDoesNotSkipLatePoints pins two
// cursor invariants: same-timestamp storage merges must be replayed via
// WriteGeneration without retaining stale aggregate state, and a replay that
// sees no strictly-new bucket must not move lastProcessedTime past late points
// that still fall inside the dataTime range.
func TestHoltResidual_ReprocessesSameBucketMergeAndDoesNotSkipLatePoints(t *testing.T) {
	d := testHoltResidualDetector()
	d.WarmupPoints = 10
	d.ResidualWindow = 4
	storage := newDetectorTestStorage()

	storage.Add("ns", "metric", 10.0, 10, nil)
	d.Detect(storage, 10)

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref
	key := holtStateKey{ref: ref, agg: observer.AggregateAverage}
	state := d.series[key]
	require.NotNil(t, state)
	require.Equal(t, []float64{10.0}, state.warmupBuf)
	require.Equal(t, int64(10), state.lastProcessedTime)

	storage.Add("ns", "metric", 30.0, 10, nil)
	series := storage.GetSeriesRange(ref, 0, 10, observer.AggregateAverage)
	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	require.Equal(t, 20.0, series.Points[0].Value, "storage should expose the merged average")

	d.Detect(storage, 20)
	state = d.series[key]
	require.Equal(t, []float64{20.0}, state.warmupBuf)
	require.Equal(t, int64(10), state.lastProcessedTime, "merge replay must not advance to dataTime")

	storage.Add("ns", "metric", 50.0, 15, nil)
	d.Detect(storage, 20)
	state = d.series[key]
	require.Equal(t, []float64{20.0, 50.0}, state.warmupBuf)
	assert.Equal(t, int64(15), state.lastProcessedTime)
	assert.Equal(t, storage.WriteGeneration(ref), state.lastWriteGen)
}

func TestHoltResidual_RebuildsOnOutOfOrderBackfillBeforeCursor(t *testing.T) {
	d := testHoltResidualDetector()
	d.WarmupPoints = 10
	d.ResidualWindow = 4
	storage := newDetectorTestStorage()

	storage.Add("ns", "metric", 10.0, 10, nil)
	d.Detect(storage, 10)

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref
	key := holtStateKey{ref: ref, agg: observer.AggregateAverage}
	state := d.series[key]
	require.NotNil(t, state)
	require.Equal(t, []float64{10.0}, state.warmupBuf)
	require.Equal(t, int64(10), state.lastProcessedTime)

	storage.Add("ns", "metric", 5.0, 5, nil)
	d.Detect(storage, 10)

	state = d.series[key]
	require.NotNil(t, state)
	assert.Equal(t, []float64{5.0, 10.0}, state.warmupBuf)
	assert.Equal(t, 2, state.lastProcessedCount)
	assert.Equal(t, int64(10), state.lastProcessedTime)
}

func TestHoltResidual_RebuildsOnCursorMergeWithLaterAppend(t *testing.T) {
	d := testHoltResidualDetector()
	d.WarmupPoints = 10
	d.ResidualWindow = 4
	storage := newDetectorTestStorage()

	storage.Add("ns", "metric", 10.0, 10, nil)
	d.Detect(storage, 10)

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref
	key := holtStateKey{ref: ref, agg: observer.AggregateAverage}
	state := d.series[key]
	require.NotNil(t, state)
	require.Equal(t, []float64{10.0}, state.warmupBuf)
	require.Equal(t, int64(10), state.lastProcessedTime)

	storage.Add("ns", "metric", 30.0, 10, nil)
	storage.Add("ns", "metric", 40.0, 11, nil)
	d.Detect(storage, 11)

	state = d.series[key]
	require.NotNil(t, state)
	assert.Equal(t, []float64{20.0, 40.0}, state.warmupBuf)
	assert.Equal(t, 2, state.lastProcessedCount)
	assert.Equal(t, int64(11), state.lastProcessedTime)
}

// TestHoltResidual_ConfirmationStartsAfterWindowsReady verifies that
// under-filled MAD windows cannot pre-arm confirmation counters. The first
// point has a huge residual but is observed before both windows are full; the
// next ready-window breach should only set the first confirmation count, not
// fire immediately.
func TestHoltResidual_ConfirmationStartsAfterWindowsReady(t *testing.T) {
	d := &HoltResidualDetector{
		Alpha:           0.01,
		Beta:            0.01,
		ResidualWindow:  4,
		ZThreshold:      0.5,
		ConfirmM:        2,
		MinDeviationMAD: 0,
		Refractory:      0,
	}
	state := &holtSeriesState{
		warmedUp: true,
		resWin:   []float64{0, 0, 0},
		valWin:   []float64{0, 0, 0},
	}

	_, fired := d.processPoint(state, observer.Point{Timestamp: 1, Value: 100}, observer.AggregateAverage)
	require.False(t, fired)
	assert.Zero(t, state.consecutivePos, "under-filled windows must not pre-arm confirmation")
	assert.Zero(t, state.consecutiveNeg)

	_, fired = d.processPoint(state, observer.Point{Timestamp: 2, Value: 100}, observer.AggregateAverage)
	require.False(t, fired, "first ready-window breach should only arm confirmation")
	assert.Equal(t, 1, state.consecutivePos)
	assert.Zero(t, state.consecutiveNeg)

	_, fired = d.processPoint(state, observer.Point{Timestamp: 3, Value: 100}, observer.AggregateAverage)
	require.True(t, fired, "second ready-window breach should satisfy ConfirmM")
}

func anomalyTimestamps(anomalies []observer.Anomaly) []int64 {
	timestamps := make([]int64, len(anomalies))
	for i, anomaly := range anomalies {
		timestamps[i] = anomaly.Timestamp
	}
	return timestamps
}
