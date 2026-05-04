// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// testCrossDecorrelateDetector returns a detector configured for the small,
// fast tests below. We keep the production gate values (HighCorrelation,
// LowCorrelation, RefractorySec, MinSamplingMatchSec) but shrink the windows
// and baseline minimums so a 300-point fixture exercises the gate. The
// production constants are validated indirectly by the
// TestDefaultCatalog_DetectorTeardownContract assertion.
func testCrossDecorrelateDetector() *CrossDecorrelateDetector {
	d := NewCrossDecorrelateDetector()
	d.LongWindow = 200
	d.ShortWindow = 30
	d.MinPointsBaseline = 200
	return d
}

// addPair seeds two series under the same host:web-1 / service:api scope so
// they are co-resident in the cross-decorrelate scope grouping. The detector's
// scopeKey logic builds a key from cdScopeTagPrefixes, so any pair of metrics
// with matching host:/service: tags share a scope.
func addPair(_ *testing.T, storage *timeSeriesStorage, name string, ts int64, value float64) {
	storage.Add("ns", name, value, ts, []string{"host:web-1", "service:api"})
}

// TestCrossDecorrelate_NoFireOnIndependentSeries feeds two sinusoids with
// independent phase. Their long-window |Pearson r| stays well below the high
// threshold (≈0 in expectation), so the detector must not fire across 400
// aligned points.
func TestCrossDecorrelate_NoFireOnIndependentSeries(t *testing.T) {
	d := testCrossDecorrelateDetector()
	storage := newTimeSeriesStorage()

	for ts := int64(1); ts <= 400; ts++ {
		// Independent phases: A leads with phase 0, B leads with phase π/2 plus
		// a different period. The orthogonal periods make r ≈ 0 over a full
		// fixture window — good enough to pin the no-fire contract.
		va := math.Sin(float64(ts) / 10.0)
		vb := math.Cos(float64(ts) / 7.0)
		addPair(t, storage, "metric_a", ts, va)
		addPair(t, storage, "metric_b", ts, vb)
	}

	result := d.Detect(storage, 400)
	assert.Empty(t, result.Anomalies, "independent sinusoids must not trigger cross-decorrelation")
}

// TestCrossDecorrelate_FiresOnDecorrelation builds a high-correlation regime
// (sin(t/10) for both A and B with small noise on B) for 220 points, then
// breaks the correlation by holding B constant for 50 more points. The
// detector must fire at least once during the constant tail. Attribution
// (which endpoint owns the anomaly) is mathematically ambiguous when r≈±1
// and σ_x≈σ_y because the cross-residual is symmetric in (x↔y); we therefore
// only pin "fire happens, score reflects rLong-rShort gap, timestamp lands in
// the broken tail, both endpoints are named in the title."
func TestCrossDecorrelate_FiresOnDecorrelation(t *testing.T) {
	d := testCrossDecorrelateDetector()
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(7))
	const baselineLen = 220
	const tailLen = 50
	const tailValue = 0.0

	// Phase 1: high correlation. A = sin(t/10), B = sin(t/10) + small noise.
	for ts := int64(1); ts <= baselineLen; ts++ {
		v := math.Sin(float64(ts) / 10.0)
		addPair(t, storage, "metric_a", ts, v)
		addPair(t, storage, "metric_b", ts, v+0.05*rng.NormFloat64())
	}
	// Phase 2: A keeps oscillating; B flatlines. Cross-correlation collapses.
	for ts := int64(baselineLen + 1); ts <= baselineLen+tailLen; ts++ {
		va := math.Sin(float64(ts) / 10.0)
		addPair(t, storage, "metric_a", ts, va)
		addPair(t, storage, "metric_b", ts, tailValue)
	}

	result := d.Detect(storage, baselineLen+tailLen)
	require.NotEmpty(t, result.Anomalies, "B going constant after a high-correlation regime must fire")

	// Attribution: the constant series (metric_b) should be the chosen series.
	// Its cross-residual is large because the long-window cross-prediction
	// expects it to move with A. The Score = |rLong| - |rShort| must be > 0
	// (long was above 0.7, short collapsed below 0.3, so score ≥ 0.4).
	a := result.Anomalies[0]
	assert.Equal(t, "cross_decorrelate", a.DetectorName)
	assert.Contains(t, a.Title, "Cross-series decorrelation")
	require.NotNil(t, a.Score)
	assert.Greater(t, *a.Score, 0.4, "score should reflect rLong-rShort gap")
	// Fire timestamp lands inside the constant tail — never before it.
	assert.Greater(t, a.Timestamp, int64(baselineLen),
		"fire timestamp must land in the post-baseline tail (got %d, baseline ends at %d)",
		a.Timestamp, baselineLen)
	assert.LessOrEqual(t, a.Timestamp, int64(baselineLen+tailLen))
	// Attribution: must be one of the two named endpoints. The title must
	// reference BOTH so the user sees the cross-series relationship even if
	// attribution falls on either side.
	assert.Contains(t, []string{"metric_a", "metric_b"}, a.Source.Name,
		"attribution should fall on one of the paired series")
	assert.Contains(t, a.Title, "metric_a")
	assert.Contains(t, a.Title, "metric_b")
}

// TestCrossDecorrelate_RemoveSeriesPrunesPairs builds a 3-series scope, lets
// the detector observe enough points to populate pair states, then calls
// RemoveSeries on one ref and verifies the pair map shrinks to only the
// remaining (a,b) combination. With 3 members there are 3 pairs; after
// removing one ref, only 1 pair must remain.
func TestCrossDecorrelate_RemoveSeriesPrunesPairs(t *testing.T) {
	d := testCrossDecorrelateDetector()
	storage := newTimeSeriesStorage()

	// Seed 3 series with enough points to populate pair state. The exact data
	// doesn't matter — we only care about pair-state allocation.
	for ts := int64(1); ts <= 50; ts++ {
		addPair(t, storage, "metric_a", ts, math.Sin(float64(ts)/10))
		addPair(t, storage, "metric_b", ts, math.Sin(float64(ts)/10))
		addPair(t, storage, "metric_c", ts, math.Cos(float64(ts)/10))
	}
	_ = d.Detect(storage, 50)

	// Locate the scope. With matching host:/service: tags all 3 metrics share
	// one scope.
	require.Len(t, d.scopes, 1, "all 3 series share one scope by tag")
	var scope *cdScopeState
	for _, s := range d.scopes {
		scope = s
		break
	}
	require.NotNil(t, scope)
	require.Len(t, scope.members, 3, "scope must contain all 3 admitted series")
	require.Len(t, scope.pairs, 3, "3 unordered pairs from 3 members (3 choose 2)")

	// Pick one ref to remove. Use members[0] — choice is arbitrary.
	removed := scope.members[0]
	d.RemoveSeries([]observer.SeriesRef{removed})

	// The post-remove scope should still exist with 2 members and 1 pair, OR
	// (if all members happened to share the same scope and were all evicted,
	// which can't happen here) be gone.
	require.Len(t, d.scopes, 1, "scope still resident with 2 surviving members")
	for _, s := range d.scopes {
		assert.Len(t, s.members, 2, "one member removed → 2 remain")
		assert.Len(t, s.pairs, 1, "pairs touching the removed ref must be pruned (3→1)")
		for pk := range s.pairs {
			assert.NotEqual(t, removed, pk.a, "no pair should reference removed ref")
			assert.NotEqual(t, removed, pk.b, "no pair should reference removed ref")
		}
	}
}

// TestCrossDecorrelate_RemoveSeriesEmptyScopeDropped pins the contract that
// removing every member of a scope drops the scope entry entirely. Without
// this, the detector's d.scopes map would grow unbounded with the cumulative
// count of host:/service: combinations ever seen.
func TestCrossDecorrelate_RemoveSeriesEmptyScopeDropped(t *testing.T) {
	d := testCrossDecorrelateDetector()
	storage := newTimeSeriesStorage()

	for ts := int64(1); ts <= 10; ts++ {
		addPair(t, storage, "metric_a", ts, 1.0)
		addPair(t, storage, "metric_b", ts, 2.0)
	}
	_ = d.Detect(storage, 10)
	require.Len(t, d.scopes, 1)

	// Snapshot all member refs.
	var allRefs []observer.SeriesRef
	for _, s := range d.scopes {
		allRefs = append(allRefs, s.members...)
	}
	d.RemoveSeries(allRefs)
	assert.Empty(t, d.scopes, "removing every member must drop the scope")
}
