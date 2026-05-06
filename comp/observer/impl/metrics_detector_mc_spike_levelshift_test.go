// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math/rand"
	"sort"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testMCDetector returns a detector with a small baseline/anomaly window
// suitable for unit tests.
func testMCDetector() *MCSpikeLevelShiftDetector {
	cfg := DefaultMCSpikeLevelShiftConfig()
	cfg.BaselinePoints = 40
	cfg.AnomalyPoints = 10
	cfg.RecoveryPoints = 5
	return NewMCSpikeLevelShiftDetector(cfg)
}

func TestMCDetector_Name(t *testing.T) {
	d := NewMCSpikeLevelShiftDetector(DefaultMCSpikeLevelShiftConfig())
	assert.Equal(t, "mc_spike_levelshift_detector", d.Name())
}

func TestMCDetector_NotEnoughPoints(t *testing.T) {
	d := testMCDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 30; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	result := d.Detect(storage, 30)
	assert.Empty(t, result.Anomalies, "no detection until baseline+anomaly windows fill")
}

func TestMCDetector_StableDataNoAlerts(t *testing.T) {
	d := testMCDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(1))

	// 80 points with mean 100, small noise.
	for i := 0; i < 80; i++ {
		v := 100 + rng.NormFloat64()*0.5
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	result := d.Detect(storage, 80)
	assert.Empty(t, result.Anomalies, "stable data should not trip MC defaults")
}

func TestMCDetector_DetectsLevelShift(t *testing.T) {
	d := testMCDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(2))

	// 40 baseline points around 100.
	for i := 0; i < 40; i++ {
		v := 100 + rng.NormFloat64()*0.5
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	// 10 anomaly-window points at 200 (well outside 1.5 * p99 ≈ 1.5 * 101).
	for i := 40; i < 50; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	result := d.Detect(storage, 50)
	require.NotEmpty(t, result.Anomalies, "should detect level shift")
	assert.Contains(t, result.Anomalies[0].Title, "MC")
}

func TestMCDetector_DetectsSpike(t *testing.T) {
	d := testMCDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(3))

	// Baseline near 100 with std ~1; 5σ threshold = ~5 above mean.
	for i := 0; i < 49; i++ {
		v := 100 + rng.NormFloat64()*1.0
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	// Single dramatic spike: 100 → 500.
	storage.Add("ns", "metric", 500, 50, nil)
	result := d.Detect(storage, 50)
	require.NotEmpty(t, result.Anomalies, "should detect dramatic spike")
}

func TestMCDetector_NoisyButStableNoFalsePositives(t *testing.T) {
	d := testMCDetector()
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(4))

	// 200 points: noisy stationary series. Mean 50, std ~5.
	for i := 0; i < 200; i++ {
		v := 50 + rng.NormFloat64()*5
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	result := d.Detect(storage, 200)
	// At 5σ + percentile guards, occasional small swings should not fire.
	assert.Empty(t, result.Anomalies, "noisy stationary data should not trip a 5σ MC detector")
}

func TestMCDetector_TinyShiftSuppressedByFloor(t *testing.T) {
	cfg := DefaultMCSpikeLevelShiftConfig()
	cfg.BaselinePoints = 40
	cfg.AnomalyPoints = 10
	cfg.RecoveryPoints = 5
	cfg.MinAbsDeviation = 0.5 // require ≥50% relative magnitude to fire
	d := NewMCSpikeLevelShiftDetector(cfg)
	storage := newTimeSeriesStorage()

	// Baseline ≈ 100.000, no noise. Then anomaly window ≈ 100.001.
	for i := 0; i < 40; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	for i := 40; i < 50; i++ {
		storage.Add("ns", "metric", 100.001, int64(i+1), nil)
	}
	result := d.Detect(storage, 50)
	assert.Empty(t, result.Anomalies, "tiny absolute shifts should be suppressed by the magnitude floor")
}

func TestMCDetector_RemoveSeriesClearsState(t *testing.T) {
	d := testMCDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 80; i++ {
		storage.Add("ns", "metric", 100+float64(i%3), int64(i+1), nil)
	}
	_ = d.Detect(storage, 80)
	require.NotEmpty(t, d.series)

	// Take any ref seen and free its state.
	for k := range d.series {
		d.RemoveSeries([]observer.SeriesRef{k.ref})
	}
	assert.Empty(t, d.series)
}

func TestMCDetector_SuppressesDuplicateFirings(t *testing.T) {
	d := testMCDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 40; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	// Sustained shift: the detector should fire once and then go quiet
	// while in alert state, despite continuous out-of-band points.
	for i := 40; i < 100; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	result := d.Detect(storage, 100)
	require.NotEmpty(t, result.Anomalies)
	assert.Equal(t, 1, len(result.Anomalies), "sustained shift should fire exactly once during alert state")
}

// TestMCDetector_KurtosisSpikeBranch exercises the kurtosis-only path:
// a single anomaly-window outlier that sits below the 5σ spike threshold
// but still inflates anomaly-window kurtosis past KurtosisMultiplier ×
// baseline kurtosis.
func TestMCDetector_KurtosisSpikeBranch(t *testing.T) {
	cfg := DefaultMCSpikeLevelShiftConfig()
	cfg.BaselinePoints = 60
	cfg.AnomalyPoints = 60
	cfg.RecoveryPoints = 5
	cfg.MinAbsDeviation = 0
	cfg.MinAlertGapSec = 0
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewMCSpikeLevelShiftDetector(cfg)
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(42))

	// Baseline: 60 noisy points around 100 (std ~10).
	for i := 0; i < 60; i++ {
		v := 100 + rng.NormFloat64()*10
		storage.Add("ns", "m", v, int64(i+1), nil)
	}
	// Anomaly window: 59 calm points then a single outlier at 130
	// (≈3σ — below the 5σ spike threshold) so only kurtosis can fire.
	for i := 60; i < 119; i++ {
		storage.Add("ns", "m", 100, int64(i+1), nil)
	}
	storage.Add("ns", "m", 130, 120, nil)

	result := d.Detect(storage, 120)
	require.NotEmpty(t, result.Anomalies, "kurtosis test should fire on heavy-tailed anomaly window")
	assert.Contains(t, result.Anomalies[0].Title, "kurtosis_spike",
		"the std-only spike path should not fire (130 is below 5σ); kurtosis path should win")
}

// TestMCDetector_LevelShiftModeMAD covers the opt-in MAD-based level-shift
// bounds, which use [median ± K×MAD] instead of [LowerCoef×p1, UpperCoef×p99].
func TestMCDetector_LevelShiftModeMAD(t *testing.T) {
	cfg := DefaultMCSpikeLevelShiftConfig()
	cfg.BaselinePoints = 60
	cfg.AnomalyPoints = 10
	cfg.RecoveryPoints = 5
	cfg.MinAbsDeviation = 0
	cfg.MinAlertGapSec = 0
	cfg.LevelShiftMode = "mad"
	cfg.MADMultiplier = 3.0
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewMCSpikeLevelShiftDetector(cfg)
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(11))

	for i := 0; i < 60; i++ {
		v := 100 + rng.NormFloat64()
		storage.Add("ns", "m", v, int64(i+1), nil)
	}
	// Shift by 30 — far outside [median ± 3×MAD] (MAD ~0.7 on noise std 1).
	for i := 60; i < 70; i++ {
		storage.Add("ns", "m", 130, int64(i+1), nil)
	}

	result := d.Detect(storage, 70)
	require.NotEmpty(t, result.Anomalies, "MAD bounds should detect level shift")
}

// TestMCDetector_HysteresisGapBlocksRefire ensures MinAlertGapSec
// suppresses a second alert that would otherwise fire after the in-alert
// flag clears.
func TestMCDetector_HysteresisGapBlocksRefire(t *testing.T) {
	cfg := DefaultMCSpikeLevelShiftConfig()
	cfg.BaselinePoints = 40
	cfg.AnomalyPoints = 10
	cfg.RecoveryPoints = 3
	cfg.MinAlertGapSec = 100
	cfg.MinAbsDeviation = 0
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewMCSpikeLevelShiftDetector(cfg)
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(7))

	// Baseline: 40 noisy points around 100.
	for i := 0; i < 40; i++ {
		storage.Add("ns", "m", 100+rng.NormFloat64(), int64(i+1), nil)
	}
	// 10 anomaly-window points at 200 (first alert fires here).
	for i := 40; i < 50; i++ {
		storage.Add("ns", "m", 200, int64(i+1), nil)
	}
	// 5 calm points: in-alert flag clears after RecoveryPoints=3.
	for i := 50; i < 55; i++ {
		storage.Add("ns", "m", 100+rng.NormFloat64(), int64(i+1), nil)
	}
	// 10 more anomaly-window points at 200 — now post-recovery, but only
	// ~25s after first alert. MinAlertGapSec=100 should suppress the refire.
	for i := 55; i < 65; i++ {
		storage.Add("ns", "m", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 65)
	assert.Equal(t, 1, len(result.Anomalies),
		"hysteresis gap must suppress the second alert within MinAlertGapSec")
}

// TestMCDetector_HysteresisGapAllowsRefire is the converse: once the gap
// has elapsed, a fresh trigger condition can fire again.
func TestMCDetector_HysteresisGapAllowsRefire(t *testing.T) {
	cfg := DefaultMCSpikeLevelShiftConfig()
	cfg.BaselinePoints = 40
	cfg.AnomalyPoints = 10
	cfg.RecoveryPoints = 3
	cfg.MinAlertGapSec = 5 // very short cooldown
	cfg.MinAbsDeviation = 0
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewMCSpikeLevelShiftDetector(cfg)
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(8))

	for i := 0; i < 40; i++ {
		storage.Add("ns", "m", 100+rng.NormFloat64(), int64(i+1), nil)
	}
	for i := 40; i < 50; i++ {
		storage.Add("ns", "m", 200, int64(i+1), nil)
	}
	// Long calm period so MinAlertGapSec elapses.
	for i := 50; i < 100; i++ {
		storage.Add("ns", "m", 100+rng.NormFloat64(), int64(i+1), nil)
	}
	for i := 100; i < 110; i++ {
		storage.Add("ns", "m", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 110)
	assert.GreaterOrEqual(t, len(result.Anomalies), 2,
		"second alert should fire once MinAlertGapSec has elapsed")
}

func TestMCMomentsAndQuantileHelpers(t *testing.T) {
	// 100 evenly-spaced values 1..100.
	vals := make([]float64, 100)
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	mean, std, kurt := momentMoments(vals)
	assert.InDelta(t, 50.5, mean, 1e-9)
	assert.InDelta(t, 29.011, std, 0.01) // sample stddev of 1..100
	// Uniform-ish distribution has Pearson kurt ≈ 1.8 (Gaussian = 3.0).
	assert.InDelta(t, 1.8, kurt, 0.05)

	sort.Float64s(vals)
	// nearest-rank: ceil(q*n)-1. q=0.01,n=100 → idx 0 → 1. q=0.99 → idx 98 → 99.
	assert.InDelta(t, 1.0, nearestRank(vals, 0.01), 1e-9)
	assert.InDelta(t, 99.0, nearestRank(vals, 0.99), 1e-9)
	assert.InDelta(t, 50.5, midpoint(vals), 1e-9)
}
