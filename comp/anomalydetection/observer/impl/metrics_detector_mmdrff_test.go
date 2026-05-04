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

// testMMDRFFTwoSampleDetector returns a detector restricted to AggregateAverage so each
// test only sees one state entry per series — keeps anomaly-count assertions
// unambiguous (a duplicate Count anomaly here would otherwise mask real false
// positives).
func testMMDRFFTwoSampleDetector() *MMDRFFTwoSampleDetector {
	d := NewMMDRFFTwoSampleDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedMMDRFFSeries appends values to a fresh storage with consecutive
// timestamps starting at t=1, then runs Detect once at the final timestamp.
// The positive offset matches the convention used by other detector tests; the
// algorithm z-scores its input so the offset is invisible to the score.
func feedMMDRFFSeries(t *testing.T, d *MMDRFFTwoSampleDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

// TestMMDRFFTwoSampleTwoSample_Name documents the catalog identifier the detector returns. The
// catalog entry's `name` field and the detector's Name() return value must
// agree because reporters key off DetectorName.
func TestMMDRFFTwoSampleTwoSample_Name(t *testing.T) {
	d := NewMMDRFFTwoSampleDetector()
	assert.Equal(t, "mmdrff", d.Name())
}

// TestMMDRFFTwoSampleTwoSample_FiresOnDistShift: 200 N(0,1) followed by 200 samples from a
// 50/50 mixture of N(-3,1) and N(3,1). Marginal mean is zero in both regimes
// (so the additivity gate stays open) but the modality changes dramatically —
// the kernel mean embedding under a Gaussian kernel separates the two
// distributions strongly. MMD² should clear 0.30 for at least three
// consecutive ticks, producing exactly one alert-onset anomaly.
func TestMMDRFFTwoSampleTwoSample_FiresOnDistShift(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := append(genGaussian(rng, 200, 0, 1), genBimodal(rng, 200)...)

	d := testMMDRFFTwoSampleDetector()
	result := feedMMDRFFSeries(t, d, "dist_shift", values)

	require.Len(t, result.Anomalies, 1, "unimodal→bimodal shift should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "mmdrff", a.DetectorName)
	assert.Contains(t, a.Title, "MMD-RFF")
	require.NotNil(t, a.DebugInfo)
	assert.GreaterOrEqual(t, a.DebugInfo.CurrentValue, 0.30,
		"mmd² at trigger must clear MMD2Threshold")
	assert.Greater(t, a.Timestamp, int64(200),
		"anomaly must be in the post-shift regime, not the warmup window")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestMMDRFFTwoSampleTwoSample_GatesOnMeanShift: 200 N(0,1) followed by 200 N(2,1). Variance
// stays put; only the mean shifts. ScanMW/BOCPD already cover pure mean
// shifts, so the additivity gate (|meanT-meanR|/sqrt(varR) < 0.5) MUST
// suppress every fire here. Without this gate the detector double-counts
// the same incident with the existing mean-shift detectors.
func TestMMDRFFTwoSampleTwoSample_GatesOnMeanShift(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := append(genGaussian(rng, 200, 0, 1), genGaussian(rng, 200, 2, 1)...)

	d := testMMDRFFTwoSampleDetector()
	result := feedMMDRFFSeries(t, d, "mean_shift", values)

	assert.Empty(t, result.Anomalies,
		"pure mean shift must not fire mmdrff — additivity gate against ScanMW/BOCPD")
}

// TestMMDRFFTwoSampleTwoSample_NoFireSameDist: 1000 i.i.d. N(0,1) → 0 anomalies. Validates the
// noise floor: under H0, mmd² is asymptotically distributed with null variance
// ≈ 1/(W·D) ≈ 2.6e-4 (Gretton 2012 Theorem 8), so the 0.30 threshold sits ~18
// standard deviations into the tail; persistence-of-3 makes spurious fires
// astronomically unlikely.
func TestMMDRFFTwoSampleTwoSample_NoFireSameDist(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := genGaussian(rng, 1000, 0, 1)

	d := testMMDRFFTwoSampleDetector()
	result := feedMMDRFFSeries(t, d, "stationary", values)

	assert.Empty(t, result.Anomalies, "stationary i.i.d. N(0,1) must not trigger mmdrff")
}

// TestMMDRFFTwoSampleTwoSample_StatelessAcrossSeries verifies state isolation between two
// interleaved series with different behaviour. Series A is stationary N(0,1);
// series B has a unimodal→bimodal shift. The stable A must remain quiet while
// B fires — proving per-series state keys (ref+agg) don't bleed.
func TestMMDRFFTwoSampleTwoSample_StatelessAcrossSeries(t *testing.T) {
	rngA := rand.New(rand.NewSource(10))
	rngB := rand.New(rand.NewSource(11))
	stableA := genGaussian(rngA, 400, 0, 1)
	shiftB := append(genGaussian(rngB, 200, 0, 1), genBimodal(rngB, 200)...)

	d := testMMDRFFTwoSampleDetector()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i := 0; i < 400; i++ {
		storage.Add("ns", "stableA", offset+stableA[i], int64(i+1), nil)
		storage.Add("ns", "shiftB", offset+shiftB[i], int64(i+1), nil)
	}
	result := d.Detect(storage, 400)

	countByName := map[string]int{}
	for _, a := range result.Anomalies {
		countByName[a.Source.Name]++
	}
	assert.Equal(t, 0, countByName["stableA"], "stable series must not fire")
	assert.Equal(t, 1, countByName["shiftB"], "shifting series must fire exactly once")
	assert.Len(t, d.series, 2, "per-series state must be allocated for each ref")
}

// TestMMDRFFTwoSampleTwoSample_RemoveSeries_FreesState verifies that RemoveSeries shrinks the
// per-series state map — the SeriesRemover contract that keeps detector
// memory in step with storage eviction. Each entry holds ~2 KB of fixed-size
// streaming state so this matters at scale.
func TestMMDRFFTwoSampleTwoSample_RemoveSeries_FreesState(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	values := genGaussian(rng, 200, 0, 1)

	d := testMMDRFFTwoSampleDetector()
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

// TestMMDRFFTwoSampleTwoSample_Reset documents that Reset clears every per-series state and the
// cached series list — needed by replay/reanalysis call sites. The (omega, b)
// embedding is intentionally preserved across Reset because it's fixed at
// construction, not learned.
func TestMMDRFFTwoSampleTwoSample_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	values := genGaussian(rng, 80, 0, 1)

	d := testMMDRFFTwoSampleDetector()
	feedMMDRFFSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	// Snapshot the embedding before Reset so we can verify it survives.
	preOmega := d.omega
	preB := d.b

	d.Reset()
	assert.Empty(t, d.series, "Reset should clear all state")
	assert.Nil(t, d.cachedSeries, "Reset should clear cached series")
	assert.Equal(t, preOmega, d.omega, "Reset must NOT alter the fixed RFF omega")
	assert.Equal(t, preB, d.b, "Reset must NOT alter the fixed RFF phase b")
}

// TestMMDRFFTwoSampleTwoSample_DeterministicEmbedding verifies the (omega, b) RFF parameters
// are reproducible across constructor calls — a non-negotiable property of
// the design (otherwise scores aren't comparable across runs or replicas).
func TestMMDRFFTwoSampleTwoSample_DeterministicEmbedding(t *testing.T) {
	d1 := NewMMDRFFTwoSampleDetector()
	d2 := NewMMDRFFTwoSampleDetector()
	assert.Equal(t, d1.omega, d2.omega, "omega must be deterministic across constructions")
	assert.Equal(t, d1.b, d2.b, "b must be deterministic across constructions")
}

// TestMMDRFFTwoSampleTwoSample_ColdStart_NoFire_BelowWarmup: a strong distribution shift in the
// middle of the [0, 2W) warmup region (W=60, 2W=120) must not fire because
// neither MMD² nor meanGap is computable until both rings are full.
func TestMMDRFFTwoSampleTwoSample_ColdStart_NoFire_BelowWarmup(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// Distribution change well before 2W=120 ticks — no anomaly should be
	// emitted because the score is undefined while either ring is filling.
	values := append(genGaussian(rng, 60, 0, 1), genBimodal(rng, 30)...)

	d := testMMDRFFTwoSampleDetector()
	result := feedMMDRFFSeries(t, d, "cold_start", values)

	assert.Empty(t, result.Anomalies,
		"detector must not emit before both rings are full (cold-start contract)")
}

// TestMMDRFFTwoSampleTwoSample_RecoveryPrevents_DoubleFire: a single sustained distribution
// shift must produce exactly ONE anomaly even though it persists for 400
// post-shift ticks. The post-fire structural reset (T zeroed, R copied from
// T, phi sums migrated) plus the recovery counter together prevent re-firing
// on the same incident.
func TestMMDRFFTwoSampleTwoSample_RecoveryPrevents_DoubleFire(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := append(genGaussian(rng, 200, 0, 1), genBimodal(rng, 400)...)

	d := testMMDRFFTwoSampleDetector()
	result := feedMMDRFFSeries(t, d, "double_fire_check", values)

	require.Len(t, result.Anomalies, 1,
		"a single sustained distribution shift must not produce repeat anomalies during recovery+refill")
}

// TestMMDRFFTwoSampleTwoSample_AllAboveThreshold exercises the persistence helper directly.
// Unlike VarShift's persistentLogRatio the MMD² history has no sign component
// — mmd² is non-negative by construction — but the all-above-threshold check
// is the same. An empty history must not pass; a single sub-threshold entry
// must veto the persistence.
func TestMMDRFFTwoSampleTwoSample_AllAboveThreshold(t *testing.T) {
	cases := []struct {
		name      string
		history   []float64
		threshold float64
		want      bool
	}{
		{"all-above", []float64{0.5, 0.4, 0.6}, 0.30, true},
		{"one-below", []float64{0.5, 0.2, 0.6}, 0.30, false},
		{"all-below", []float64{0.1, 0.05, 0.2}, 0.30, false},
		{"on-threshold", []float64{0.30, 0.30, 0.30}, 0.30, true},
		{"empty", nil, 0.30, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allAboveMMDThreshold(tc.history, tc.threshold)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestMMDRFFTwoSampleTwoSample_CatalogEntryRegistered confirms the stage-1 catalog wiring is
// intact: the catalog must contain a "mmdrff" entry with kind
// componentDetector. Lives next to the detector implementation rather than in
// component_catalog_test.go because it's a contract co-test for this detector.
func TestMMDRFFTwoSampleTwoSample_CatalogEntryRegistered(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "mmdrff" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "catalog must register an 'mmdrff' entry")
	assert.Equal(t, componentDetector, found.kind, "mmdrff must be registered as a detector")
	// The factory must produce a working detector instance.
	inst := found.factory(nil)
	det, ok := inst.(observer.Detector)
	require.True(t, ok, "mmdrff factory must produce an observer.Detector")
	assert.Equal(t, "mmdrff", det.Name())
	// And it must implement SeriesRemover so the engine can reclaim state
	// when storage evicts a series. (The catalog still allowlists "mmdrff"
	// from stage 1 as a stateless stub; that allowlist entry is now stale but
	// harmless — the engine still calls RemoveSeries on every detector that
	// implements SeriesRemover regardless of allowlist membership.)
	_, isRemover := inst.(manualSeriesRemover)
	assert.True(t, isRemover, "mmdrff detector must implement manualSeriesRemover")
}
