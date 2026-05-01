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

// testHellingerCPDetector returns a detector configured for tests: only the
// Average aggregation so single-namespace storage doesn't double-count.
func testHellingerCPDetector() *HellingerCPDetector {
	cfg := DefaultHellingerCPConfig()
	cfg.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return NewHellingerCPDetectorWithConfig(cfg)
}

func TestHellingerCP_Name(t *testing.T) {
	d := NewHellingerCPDetector()
	assert.Equal(t, "hellingercp", d.Name())
}

// TestHellingerCP_StableData verifies the detector is silent on a long stretch
// of i.i.d. N(100, 1) data. With 16 bins, two i.i.d. samples of size 120 vs 20
// have an expected sampling Hellinger of ~0.3 (well below the 0.55 threshold)
// plus the median-deviation gate provides a second filter.
func TestHellingerCP_StableData(t *testing.T) {
	d := testHellingerCPDetector()
	storage := newTimeSeriesStorage()
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 200; i++ {
		v := 100.0 + r.NormFloat64()
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	result := d.Detect(storage, 200)
	assert.Empty(t, result.Anomalies, "stable N(100,1) data should not fire")
}

// TestHellingerCP_DetectsStepChange covers the canonical level shift: a clean
// 100 → 130 step. The reported timestamp should land near the changepoint
// because the algorithm reports the oldest timestamp in the short window at
// fire time, not the (lagging) detection tick.
func TestHellingerCP_DetectsStepChange(t *testing.T) {
	d := testHellingerCPDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 120; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	for i := 120; i < 150; i++ {
		storage.Add("ns", "metric", 130, int64(i+1), nil)
	}
	result := d.Detect(storage, 150)
	require.Len(t, result.Anomalies, 1, "should detect exactly one step change")
	assert.Contains(t, result.Anomalies[0].Title, "HellingerCP")
	assert.Equal(t, "hellingercp", result.Anomalies[0].DetectorName)
	// Shift starts at timestamp 121; the short-window backtrack puts the
	// reported timestamp inside the post-change region.
	assert.GreaterOrEqual(t, result.Anomalies[0].Timestamp, int64(116),
		"changepoint should not be reported before the actual shift")
	assert.LessOrEqual(t, result.Anomalies[0].Timestamp, int64(126),
		"changepoint should be reported within ±5 of the shift")
}

// TestHellingerCP_DetectsScaleChange covers a scenario the mean-based scan
// detectors miss: same level, larger spread. The clipped-tail bins fill on
// the short side while the long histogram stays concentrated, driving H high.
// The median-deviation gate still triggers on this seed because the sample
// median of 20 N(100, 10) points reliably wanders >2 long-MADs from 100
// (long MAD ≈ 0.67).
func TestHellingerCP_DetectsScaleChange(t *testing.T) {
	d := testHellingerCPDetector()
	storage := newTimeSeriesStorage()
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 120; i++ {
		v := 100.0 + r.NormFloat64()
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	for i := 120; i < 150; i++ {
		v := 100.0 + r.NormFloat64()*10
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	result := d.Detect(storage, 150)
	require.NotEmpty(t, result.Anomalies, "should detect variance shift")
	assert.LessOrEqual(t, len(result.Anomalies), 1,
		"should fire at most once per change (recovery + reset)")
}

// TestHellingerCP_RejectsSlowDrift checks that monotone slow drift does not
// trigger the detector. Both windows shift together, and periodic rebinning
// keeps the histograms aligned.
func TestHellingerCP_RejectsSlowDrift(t *testing.T) {
	d := testHellingerCPDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 200; i++ {
		v := 100.0 + 0.05*float64(i)
		storage.Add("ns", "metric", v, int64(i+1), nil)
	}
	result := d.Detect(storage, 200)
	assert.Empty(t, result.Anomalies, "slow linear drift should not fire")
}

// TestHellingerCP_RemoveSeries exercises the SeriesRemover contract: state
// for refs that storage has freed is dropped from the per-series map.
func TestHellingerCP_RemoveSeries(t *testing.T) {
	d := testHellingerCPDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 120; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	d.Detect(storage, 120)
	require.NotEmpty(t, d.series, "should have state after detection")

	var refs []observer.SeriesRef
	for k := range d.series {
		refs = append(refs, k.ref)
	}

	d.RemoveSeries(refs)
	assert.Empty(t, d.series, "RemoveSeries must free all per-series state for the given refs")
}

// TestHellingerCP_AlertRecovery confirms the segment cursor + recovery
// counter yield a single anomaly per change even when many post-change points
// follow. After fire, state resets and the warmup-of-LongWindow gate prevents
// another fire until a fresh post-CP baseline accumulates — well beyond the
// 50 post-change points provided here.
func TestHellingerCP_AlertRecovery(t *testing.T) {
	d := testHellingerCPDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 120; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	for i := 120; i < 170; i++ {
		storage.Add("ns", "metric", 130, int64(i+1), nil)
	}
	result := d.Detect(storage, 170)
	require.Len(t, result.Anomalies, 1,
		"should fire only once per change even with 50 post-change points")
}

// TestHellingerCP_NoNewDataNoFire confirms the writeGen short-circuit: a
// follow-up Detect with no new data emits nothing.
func TestHellingerCP_NoNewDataNoFire(t *testing.T) {
	d := testHellingerCPDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 60; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	d.Detect(storage, 60)
	r2 := d.Detect(storage, 60)
	assert.Empty(t, r2.Anomalies, "no new data should produce no anomalies")
}

// TestHellingerCP_BinIndex spot-checks the bin assignment helper across
// in-range, on-edge, and out-of-range values.
func TestHellingerCP_BinIndex(t *testing.T) {
	edges := []float64{0, 1, 2, 3, 4} // 4 bins

	assert.Equal(t, 0, binIndex(edges, -10), "below-range clips to bin 0")
	assert.Equal(t, 0, binIndex(edges, 0), "left edge of bin 0")
	assert.Equal(t, 0, binIndex(edges, 0.5), "interior of bin 0")
	assert.Equal(t, 1, binIndex(edges, 1), "left edge of bin 1")
	assert.Equal(t, 2, binIndex(edges, 2.5), "interior of bin 2")
	assert.Equal(t, 3, binIndex(edges, 3.5), "interior of bin 3")
	assert.Equal(t, 3, binIndex(edges, 4), "right edge clips to last bin")
	assert.Equal(t, 3, binIndex(edges, 100), "above-range clips to last bin")
}

// TestHellingerCP_HellingerDistance covers the closed-form metric on three
// canonical cases: identical, disjoint, and a known-half-overlap mix.
func TestHellingerCP_HellingerDistance(t *testing.T) {
	// Identical histograms: H == 0.
	a := []int{10, 10, 10, 10}
	b := []int{2, 2, 2, 2}
	assert.InDelta(t, 0.0, hellingerCPDistance(a, b, 40, 8), 1e-9,
		"identical normalized histograms must give H=0")

	// Disjoint support: H == 1.
	c := []int{4, 0, 0, 0}
	dh := []int{0, 0, 0, 4}
	assert.InDelta(t, 1.0, hellingerCPDistance(c, dh, 4, 4), 1e-9,
		"disjoint support must give H=1")

	// Self-distance.
	assert.InDelta(t, 0.0, hellingerCPDistance(a, a, 40, 40), 1e-9,
		"self-comparison must give H=0")
}
