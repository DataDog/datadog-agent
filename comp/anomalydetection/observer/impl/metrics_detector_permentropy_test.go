// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math/rand"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPermEntropyDetector returns a detector restricted to AggregateAverage so
// each test only sees one state entry per series — keeps anomaly-count
// assertions unambiguous.
func testPermEntropyDetector() *PermEntropyDetector {
	d := NewPermEntropyDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedPermEntropy adds values to fresh storage with consecutive timestamps
// starting at t=1 and runs Detect once at the final timestamp. PermEntropy is
// invariant to translation, so values can lie in any positive range; we add
// an offset to keep them within the storage's accepted finite range.
func feedPermEntropy(t *testing.T, d *PermEntropyDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

func TestPermEntropy_Name(t *testing.T) {
	d := NewPermEntropyDetector()
	assert.Equal(t, "permentropy", d.Name())
}

// TestPermEntropy_FlagsRegularToChaoticTransition: 200 sawtooth points
// (y=t%5, only 5 distinct ordinal patterns → low entropy) followed by 200
// uniform-random noise (all 24 patterns roughly equiprobable → high entropy).
// The marginal distribution has the same range for both segments — only the
// ordinal-complexity regime changes — exactly the signal class the existing
// detectors cannot see. Expect at least one anomaly inside the chaotic
// segment.
func TestPermEntropy_FlagsRegularToChaoticTransition(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := make([]float64, 0, 400)
	for i := 0; i < 200; i++ {
		values = append(values, float64(i%5))
	}
	for i := 0; i < 200; i++ {
		values = append(values, rng.Float64()*5)
	}

	d := testPermEntropyDetector()
	result := feedPermEntropy(t, d, "regular_to_chaotic", values)

	require.NotEmpty(t, result.Anomalies, "regular→chaotic transition should fire")
	a := result.Anomalies[0]
	assert.Equal(t, "permentropy", a.DetectorName)
	assert.Contains(t, a.Title, "PermEntropy")
	require.NotNil(t, a.DebugInfo)
	assert.Greater(t, a.DebugInfo.DeviationSigma, a.DebugInfo.Threshold,
		"firing tick must clear the dimensionless trigger threshold")
	assert.Greater(t, a.Timestamp, int64(200), "anomaly timestamp must be in the chaotic regime")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestPermEntropy_FlagsChaoticToRegularTransition: reverse of the above —
// entropy DROPS at the transition and the |ΔH| trigger fires symmetrically.
// Expect at least one anomaly inside the regular segment.
func TestPermEntropy_FlagsChaoticToRegularTransition(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := make([]float64, 0, 400)
	for i := 0; i < 200; i++ {
		values = append(values, rng.Float64()*5)
	}
	for i := 0; i < 200; i++ {
		values = append(values, float64(i%5))
	}

	d := testPermEntropyDetector()
	result := feedPermEntropy(t, d, "chaotic_to_regular", values)

	require.NotEmpty(t, result.Anomalies, "chaotic→regular transition should fire")
	a := result.Anomalies[0]
	assert.Equal(t, "permentropy", a.DetectorName)
	require.NotNil(t, a.DebugInfo)
	assert.Greater(t, a.Timestamp, int64(200), "anomaly timestamp must be in the regular regime")
}

// TestPermEntropy_QuietOnRandomWalk: 600 cumulative-sum points of N(0,1)
// increments. The pattern distribution under iid increments is
// translation-invariant and stationary, so the entropy estimate fluctuates
// only with W=128 sample variance — no regime change is present and 0
// anomalies are expected.
func TestPermEntropy_QuietOnRandomWalk(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := make([]float64, 600)
	var x float64
	for i := range values {
		x += rng.NormFloat64()
		values[i] = x
	}

	d := testPermEntropyDetector()
	result := feedPermEntropy(t, d, "random_walk", values)

	assert.Empty(t, result.Anomalies, "stationary random walk should not trigger any permentropy anomaly")
}

// TestPermEntropy_IncrementalMatchesNaive runs the detector tick-by-tick on a
// noise series and, on each tick where the pattern ring is full, asserts that
// the incrementally maintained `state.entropy` matches a brute-force
// recomputation from `state.patternCounts` to within 1e-9. Validates the
// streaming Shannon-entropy update used on the hot path.
func TestPermEntropy_IncrementalMatchesNaive(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	d := testPermEntropyDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 500; i++ {
		v := rng.NormFloat64()
		storage.Add("ns", "metric", 100+v, int64(i+1), nil)
		d.Detect(storage, int64(i+1))
		for _, state := range d.series {
			if state.patternRingN < permentropyWindow {
				continue
			}
			naive := computeEntropyFromCounts(&state.patternCounts, permentropyWindow)
			assert.InDelta(t, naive, state.entropy, 1e-9,
				"tick %d: incremental entropy must match brute-force recomputation", i+1)
		}
	}
}

// TestPermEntropy_RemoveSeries verifies that RemoveSeries shrinks the
// per-series state map symmetrically by (3 × |Aggregations|) entries — the
// SeriesRemover contract that keeps detector-side memory in step with storage
// eviction.
func TestPermEntropy_RemoveSeries(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	d := NewPermEntropyDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	storage := newTimeSeriesStorage()
	const seriesCount = 5
	const pointsPerSeries = 200
	for s := 0; s < seriesCount; s++ {
		name := fmt.Sprintf("metric_%d", s)
		for i := 0; i < pointsPerSeries; i++ {
			storage.Add("ns", name, 100+rng.NormFloat64(), int64(i+1), nil)
		}
	}
	d.Detect(storage, int64(pointsPerSeries))

	require.Len(t, d.series, seriesCount*len(d.Aggregations),
		"5 series × 2 aggregations should populate all per-(series,agg) state entries")

	// Pull 3 distinct refs out of the keyed state and free them.
	seen := make(map[observer.SeriesRef]struct{})
	var refs []observer.SeriesRef
	for k := range d.series {
		if _, ok := seen[k.ref]; ok {
			continue
		}
		seen[k.ref] = struct{}{}
		refs = append(refs, k.ref)
		if len(refs) == 3 {
			break
		}
	}
	require.Len(t, refs, 3, "must select 3 distinct series refs")

	d.RemoveSeries(refs)
	assert.Len(t, d.series, (seriesCount-3)*len(d.Aggregations),
		"RemoveSeries must drop 3 × |Aggregations| state entries")
	assert.Nil(t, d.cachedSeries, "RemoveSeries must invalidate cachedSeries")
}

// TestComputeOrdinalPattern_Determinism verifies the factorial-base ranking
// agrees with hand-computed pattern indices on small fixed inputs, including
// the chronological ordering convention. Also exercises the tie-break (equal
// values resolved by later position being greater).
func TestComputeOrdinalPattern_Determinism(t *testing.T) {
	const ringSize = permentropyEmbedDim + permentropyWindow
	var ring [ringSize]float64

	// Strictly ascending tuple (1, 2, 3, 4) — c_0=c_1=c_2=0, idx=0.
	ring[0], ring[1], ring[2], ring[3] = 1, 2, 3, 4
	assert.Equal(t, 0, computeOrdinalPattern(&ring, 4, ringSize),
		"strictly ascending tuple must encode to 0")

	// Strictly descending tuple (4, 3, 2, 1) — c_0=3, c_1=2, c_2=1, idx=23.
	ring[0], ring[1], ring[2], ring[3] = 4, 3, 2, 1
	assert.Equal(t, 23, computeOrdinalPattern(&ring, 4, ringSize),
		"strictly descending tuple must encode to 23 (= m!-1)")

	// Tie-break: (1, 1, 1, 1). Strict comparisons → c_0=c_1=c_2=0 → idx=0.
	// Documents that ties never count toward the "later strictly smaller"
	// counter, so equal-valued runs encode as the ascending pattern.
	ring[0], ring[1], ring[2], ring[3] = 1, 1, 1, 1
	assert.Equal(t, 0, computeOrdinalPattern(&ring, 4, ringSize),
		"all-ties must encode as the ascending pattern (0) under the tie-break")
}
