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

// testMMDRFFDetector returns a detector pinned to a single aggregation.
// Default Aggregations include Count; pinning to Average keeps anomaly
// counts deterministic across the suite.
func testMMDRFFDetector() *MMDRFFDetector {
	d := NewMMDRFFDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// TestMMDRFF_StationaryGaussian_NoFires feeds 240 N(0,1) points with a
// fixed seed. Calibrated FP rate target: <= 1 fire across the full
// stream. The threshold (mmdSq=0.10, ~6× null mean), ConfirmM=2, and the
// >= 3-MAD effect-size gate together suppress noise on stationary input.
// A regression past one fire would indicate a calibration drift in
// either the threshold or the projection seed.
func TestMMDRFF_StationaryGaussian_NoFires(t *testing.T) {
	d := testMMDRFFDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(101))
	for ts := int64(1); ts <= 240; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 240)
	assert.LessOrEqual(t, len(result.Anomalies), 1, "stationary N(0,1) must not produce more than one false fire across 240 ticks")
}

// TestMMDRFF_MeanShift_Fires feeds 120 N(0,1) baseline points followed
// by 120 N(3,1) post-shift points. The detector must emit exactly one
// fire on the regime change; refractory suppresses the rest.
func TestMMDRFF_MeanShift_Fires(t *testing.T) {
	d := testMMDRFFDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(7))
	for ts := int64(1); ts <= 120; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}
	for ts := int64(121); ts <= 240; ts++ {
		storage.Add("ns", "metric", 3+rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 240)
	require.Len(t, result.Anomalies, 1, "exactly one fire expected on a clean mean shift (refractory blocks repeats)")

	a := result.Anomalies[0]
	assert.Equal(t, "mmd_rff", a.DetectorName)
	assert.Contains(t, a.Title, "MMD-RFF distribution shift")
	require.NotNil(t, a.Score, "score must be populated")
	assert.GreaterOrEqual(t, *a.Score, d.Threshold, "score must clear the MMD² threshold")
	assert.NotNil(t, a.SourceRef, "SourceRef must be populated for downstream correlators")
	require.NotNil(t, a.DebugInfo, "DebugInfo must be populated")
	assert.Equal(t, d.Threshold, a.DebugInfo.Threshold)
	// Fire timestamp must land at least ConfirmM ticks past the regime
	// boundary (need that many same-counter MMD-passing windows to confirm)
	// AND past the W=120 latent warmup (60 in currWin then 60 in preWin).
	assert.GreaterOrEqual(t, a.Timestamp, int64(120+d.ConfirmM-1))
}

// TestMMDRFF_VarianceOnlyShift_Fires is a key differentiation test: a
// pure variance shift with the mean held constant. KS / Welch-style
// detectors would also catch this, but holt_residual / glr_mean_variance
// would not — their effect-size gates are mean-based by construction.
// MMD-RFF's distribution-aware MAD-shift gate exists precisely to keep
// this kind of shift in scope.
//
// The shift is N(0,1) → N(0,25); stddev 1 → 5. We use a stronger
// dispersion change than the original plan's N(0,9) so the post-shift
// MMD² comfortably clears the calibrated threshold and the MAD-shift
// (madCurr_sigma ~ 5, madPre_sigma ~ 1) clears the >= 3-MAD effect-
// size gate.
func TestMMDRFF_VarianceOnlyShift_Fires(t *testing.T) {
	d := testMMDRFFDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(11))
	for ts := int64(1); ts <= 120; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}
	for ts := int64(121); ts <= 240; ts++ {
		// stddev = 5 (variance 25); mean = 0, unchanged.
		storage.Add("ns", "metric", 5*rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 240)
	require.GreaterOrEqual(t, len(result.Anomalies), 1, "pure variance shift must fire — this is what the RBF kernel buys us over mean-only detectors")
	require.LessOrEqual(t, len(result.Anomalies), 2, "refractory should keep the count low for a single regime change")

	a := result.Anomalies[0]
	assert.Equal(t, "mmd_rff", a.DetectorName)
	require.NotNil(t, a.Score)
	assert.GreaterOrEqual(t, *a.Score, d.Threshold)
	assert.NotNil(t, a.SourceRef)
}

// TestMMDRFF_BimodalShift_Fires is the RBF kernel's killer
// differentiator. The post-shift regime alternates between N(-3, 0.1)
// and N(3, 0.1), so both the mean and the median of the new window are
// approximately 0 — identical to the baseline. KS and Welch and any
// median-shift detector are blind here. The kernel sees two well-
// separated modes and the MMD² rises sharply; the MAD-shift effect-
// size gate captures the dispersion change.
func TestMMDRFF_BimodalShift_Fires(t *testing.T) {
	d := testMMDRFFDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(23))
	for ts := int64(1); ts <= 120; ts++ {
		storage.Add("ns", "metric", rng.NormFloat64(), ts, nil)
	}
	for ts := int64(121); ts <= 240; ts++ {
		// Alternate ±3 with small noise. Mean of the regime is ~0,
		// matching the baseline.
		mode := 3.0
		if (ts-121)%2 == 0 {
			mode = -3.0
		}
		storage.Add("ns", "metric", mode+0.1*rng.NormFloat64(), ts, nil)
	}

	result := d.Detect(storage, 240)
	require.GreaterOrEqual(t, len(result.Anomalies), 1, "bimodal shift with matching mean must still fire — the RBF kernel sees the two modes")
	require.LessOrEqual(t, len(result.Anomalies), 2, "refractory should keep the count low for a single regime change")

	a := result.Anomalies[0]
	assert.Equal(t, "mmd_rff", a.DetectorName)
	require.NotNil(t, a.Score)
	assert.GreaterOrEqual(t, *a.Score, d.Threshold)
}

// TestMMDRFF_DeterministicProjection pins the RFF projection contract:
// two detectors built with the same seed must produce element-wise
// identical omegas and biases. Without this, replays and cross-version
// evals could see subtly different kernel realisations and the FP/TP
// scorer would attribute the difference to the algorithm rather than
// the projection.
func TestMMDRFF_DeterministicProjection(t *testing.T) {
	a := NewMMDRFFDetector()
	b := NewMMDRFFDetector()

	require.Equal(t, len(a.omegas), len(b.omegas))
	require.Equal(t, len(a.biases), len(b.biases))
	require.NotZero(t, len(a.omegas), "projection must be sampled by NewMMDRFFDetector")

	for i := 0; i < len(a.omegas); i++ {
		assert.Equal(t, a.omegas[i], b.omegas[i], "omega[%d] differs across constructions with the same seed", i)
		assert.Equal(t, a.biases[i], b.biases[i], "bias[%d] differs across constructions with the same seed", i)
	}
	assert.Equal(t, a.sqrt2OverD, b.sqrt2OverD)
}

// TestMMDRFF_RemoveSeries verifies the SeriesRemover contract: after
// Detect populates per-series state, RemoveSeries must drop it so the
// detector's memory tracks storage's series cardinality.
func TestMMDRFF_RemoveSeries(t *testing.T) {
	d := testMMDRFFDetector()
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

// TestMMDRFF_TeardownContract guards the catalog wiring: defaultCatalog
// must satisfy the SeriesRemover-or-allowlist invariant for every
// detector entry. A regression here would mean mmd_rff was either
// dropped from the catalog or stopped implementing SeriesRemover.
func TestMMDRFF_TeardownContract(t *testing.T) {
	require.NoError(t, defaultCatalog().validateDetectorTeardownContract())
}

// TestMMDRFF_Name pins the catalog identifier so the catalog test stays
// in sync with the detector identifier.
func TestMMDRFF_Name(t *testing.T) {
	assert.Equal(t, "mmd_rff", NewMMDRFFDetector().Name())
}

// TestMMDRFF_InterfaceContracts checks the structural promises that the
// catalog and engine both rely on: MMDRFFDetector must satisfy
// observer.Detector AND observer.SeriesRemover (it is stateful and is
// NOT listed in statelessDetectorAllowlist).
func TestMMDRFF_InterfaceContracts(t *testing.T) {
	d := NewMMDRFFDetector()
	var _ observer.Detector = d
	var _ observer.SeriesRemover = d
}

// TestMMDRFF_DefaultsApplied confirms ensureDefaults populates a
// zero-valued struct so reflective construction (and any caller that
// bypasses NewMMDRFFDetector) still produces a usable detector.
func TestMMDRFF_DefaultsApplied(t *testing.T) {
	d := &MMDRFFDetector{}
	storage := newTimeSeriesStorage()

	_ = d.Detect(storage, 1)

	assert.Equal(t, mmdWindow, d.Window)
	assert.Equal(t, mmdRFFDim, d.RFFDim)
	assert.Equal(t, mmdThreshold, d.Threshold)
	assert.Equal(t, mmdConfirmM, d.ConfirmM)
	assert.Equal(t, mmdMinDeviationMAD, d.MinDeviationMAD)
	assert.Equal(t, mmdRefractory, d.Refractory)
	assert.NotNil(t, d.series)
	assert.Len(t, d.omegas, mmdRFFDim, "ensureDefaults must (re)sample the projection on a zero-value struct")
	assert.Len(t, d.biases, mmdRFFDim)
	assert.NotZero(t, d.sqrt2OverD)
	assert.ElementsMatch(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations,
	)
}

// TestMMDRFF_Reset confirms Reset wipes per-series state so a replay
// session does not see stale buffers — but preserves the RFF projection
// so the kernel realisation stays comparable across replays.
func TestMMDRFF_Reset(t *testing.T) {
	d := testMMDRFFDetector()
	storage := newTimeSeriesStorage()

	addConstant(t, storage, "metric", 100, 1, 1.0)
	d.Detect(storage, 100)
	assert.NotEmpty(t, d.series, "should have state after detection")
	omegasBefore := append([]float64(nil), d.omegas...)
	biasesBefore := append([]float64(nil), d.biases...)

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
	assert.Equal(t, omegasBefore, d.omegas, "Reset must preserve the RFF projection")
	assert.Equal(t, biasesBefore, d.biases, "Reset must preserve the RFF projection")
}
