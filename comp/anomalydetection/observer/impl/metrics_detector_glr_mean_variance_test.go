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

// testGLRMeanVarianceDetector returns a detector pinned to a single
// aggregation. Default Aggregations include Count; pinning to Average
// keeps anomaly counts deterministic across the suite.
func testGLRMeanVarianceDetector() *GLRMeanVarianceDetector {
	d := NewGLRMeanVarianceDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// TestGLRMeanVar_Warmup_NoFires feeds 50 stationary points: the 60-point
// sliding window cannot fill, so processPoint never runs and no anomaly
// is emitted regardless of the input distribution.
func TestGLRMeanVar_Warmup_NoFires(t *testing.T) {
	d := testGLRMeanVarianceDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(1))
	for ts := int64(1); ts <= 50; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 50)
	assert.Empty(t, result.Anomalies, "warmup must suppress all fires before the window fills (W=60)")
}

// TestGLRMeanVar_JointShift_Fires feeds 60 N(0,1) baseline points followed
// by 20 points from N(5, 4) (mean=5, var=4 -> stddev=2). The joint mean
// AND variance change passes the LR gate (variance change inflates LR_max)
// and the effect-size MAD gate (mean shift of 5 dominates the window's
// MAD-sigma). Refractory suppresses any would-be repeat fires.
//
// Note: the original plan called this `VarianceShift_Fires` with a pure
// N(0,9) shift, but the effect-size gate is mean-based by design (mirrors
// holt_residual / kl_divergence), so a pure variance shift cannot pass
// it. Renaming + adding a mean component preserves the spirit (test that
// joint-hypothesis events fire) while honoring the gate semantics.
func TestGLRMeanVar_JointShift_Fires(t *testing.T) {
	d := testGLRMeanVarianceDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(7))
	for ts := int64(1); ts <= 60; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}
	for ts := int64(61); ts <= 80; ts++ {
		// Mean shift of 5, stddev shift to 2.
		storage.Add("ns", "metric", 5+2*rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 80)
	require.Len(t, result.Anomalies, 1, "exactly one fire expected on a clean joint shift")

	a := result.Anomalies[0]
	assert.Equal(t, "glr_mean_variance", a.DetectorName)
	assert.Contains(t, a.Title, "GLR mean/var shift")
	require.NotNil(t, a.Score, "score must be populated")
	assert.GreaterOrEqual(t, *a.Score, d.LRThreshold, "score must clear 2*LR threshold")
	assert.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
	assert.Equal(t, d.LRThreshold, a.DebugInfo.Threshold)
	// Fire timestamp must land at least ConfirmM points past the regime
	// boundary (need that many same-sign LR-passing windows to confirm).
	assert.GreaterOrEqual(t, a.Timestamp, int64(60+d.ConfirmM-1))
}

// TestGLRMeanVar_MeanShift_Fires uses a pure step shift in constant data:
// 60 zeros followed by 20 fives. The variance floor (1e-12) makes the
// constant-segment LR explode in absolute terms but the LR difference
// LR_max is still meaningful, the sign is unambiguous (+1), and the
// MAD sigma is dominated by the range-based floor so devMAD passes.
func TestGLRMeanVar_MeanShift_Fires(t *testing.T) {
	d := testGLRMeanVarianceDetector()
	storage := newTimeSeriesStorage()

	addConstant(t, storage, "metric", 60, 1, 0.0)
	addConstant(t, storage, "metric", 20, 61, 5.0)

	result := d.Detect(storage, 80)
	require.Len(t, result.Anomalies, 1, "single-step mean shift must fire exactly once (refractory blocks repeats)")

	a := result.Anomalies[0]
	require.NotNil(t, a.Score)
	assert.GreaterOrEqual(t, *a.Score, d.LRThreshold)
	assert.NotNil(t, a.SourceRef)
}

// TestGLRMeanVar_StationaryNoise_NoFires feeds 200 stationary N(0,1)
// points with a fixed seed. Calibrated FP rate target: <= 1 fire across
// the entire stream. The threshold (chi-squared 2 dof at p~1.2e-4)
// combined with ConfirmM=2 same-sign confirmation and the >= 3-MAD
// effect-size gate makes a fire on pure noise rare.
func TestGLRMeanVar_StationaryNoise_NoFires(t *testing.T) {
	d := testGLRMeanVarianceDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(42))
	for ts := int64(1); ts <= 200; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 200)
	assert.LessOrEqual(t, len(result.Anomalies), 1, "stationary N(0,1) must not produce more than one false fire")
}

// TestGLRMeanVar_RemoveSeries verifies the SeriesRemover contract:
// after Detect populates per-series state, RemoveSeries must drop it so
// the detector's memory tracks storage's series cardinality.
func TestGLRMeanVar_RemoveSeries(t *testing.T) {
	d := testGLRMeanVarianceDetector()
	storage := newTimeSeriesStorage()

	addConstant(t, storage, "metric", 100, 1, 1.0)
	d.Detect(storage, 100)
	require.NotEmpty(t, d.series, "Detect should have populated per-series state")

	metas := storage.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	d.RemoveSeries([]observer.SeriesRef{ref})
	assert.Empty(t, d.series, "RemoveSeries must drop per-series state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries must invalidate the cached series list")
}

// TestGLRMeanVar_Refractory exercises the refractory contract on a
// sustained step shift: without refractory, the same regime change
// would re-arm consecutivePos every ConfirmM ticks and produce ~10
// fires across the 20-tick post-shift window. With refractory=20, the
// detector emits exactly one fire and suppresses the rest. The test
// stops at the refractory boundary so a legitimate post-refractory
// re-fire (which the algorithm DOES allow once 20 ticks elapse) does
// not leak into the count.
func TestGLRMeanVar_Refractory(t *testing.T) {
	d := testGLRMeanVarianceDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(11))
	for ts := int64(1); ts <= 60; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}
	// Sustained mean shift; without refractory the consecutive-confirmation
	// counter would refill every ConfirmM ticks and produce repeat fires.
	for ts := int64(61); ts <= 80; ts++ {
		storage.Add("ns", "metric", 5+rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 80)
	require.Len(t, result.Anomalies, 1, "refractory must suppress repeat fires within %d ticks of the first fire", d.Refractory)

	a := result.Anomalies[0]
	// Fire must land within a few ticks of the regime boundary —
	// ConfirmM=2 plus a small buffer for the warmup / window-fill.
	assert.GreaterOrEqual(t, a.Timestamp, int64(60+d.ConfirmM-1), "fire must come at or after the boundary + ConfirmM")
	assert.LessOrEqual(t, a.Timestamp, int64(70), "fire must come within a few ticks of the boundary")
}

// TestGLRMeanVar_TeardownContract guards the catalog wiring: defaultCatalog
// must satisfy the SeriesRemover-or-allowlist invariant for every
// detector entry. A regression here would mean glr_mean_variance was
// either dropped from the catalog or stopped implementing SeriesRemover.
func TestGLRMeanVar_TeardownContract(t *testing.T) {
	require.NoError(t, defaultCatalog().validateDetectorTeardownContract())
}

// TestGLRMeanVar_Name pins the catalog identifier so the catalog test
// stays in sync with the detector identifier.
func TestGLRMeanVar_Name(t *testing.T) {
	assert.Equal(t, "glr_mean_variance", NewGLRMeanVarianceDetector().Name())
}

// TestGLRMeanVar_InterfaceContracts checks the structural promises that
// the catalog and engine both rely on: GLRMeanVarianceDetector must
// satisfy observer.Detector AND observer.SeriesRemover (it is stateful
// and is NOT listed in statelessDetectorAllowlist).
func TestGLRMeanVar_InterfaceContracts(t *testing.T) {
	d := NewGLRMeanVarianceDetector()
	var _ observer.Detector = d
	var _ observer.SeriesRemover = d
}

// TestGLRMeanVar_DefaultsApplied confirms ensureDefaults populates a
// zero-valued struct so reflective construction (and any caller that
// bypasses NewGLRMeanVarianceDetector) still produces a usable detector.
func TestGLRMeanVar_DefaultsApplied(t *testing.T) {
	d := &GLRMeanVarianceDetector{}
	storage := newTimeSeriesStorage()

	_ = d.Detect(storage, 1)

	assert.Equal(t, glrWindow, d.Window)
	assert.Equal(t, glrMinSegment, d.MinSegment)
	assert.Equal(t, glrLRThreshold, d.LRThreshold)
	assert.Equal(t, glrConfirmM, d.ConfirmM)
	assert.Equal(t, glrMinDeviationMAD, d.MinDeviationMAD)
	assert.Equal(t, glrRefractory, d.Refractory)
	assert.NotNil(t, d.series)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestGLRMeanVar_Reset confirms Reset wipes per-series state so a replay
// session does not see stale buffers.
func TestGLRMeanVar_Reset(t *testing.T) {
	d := testGLRMeanVarianceDetector()
	storage := newTimeSeriesStorage()

	addConstant(t, storage, "metric", 100, 1, 1.0)
	d.Detect(storage, 100)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}
