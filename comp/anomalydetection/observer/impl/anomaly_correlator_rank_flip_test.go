// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rankFlipScorePtr returns a *float64 for use in observer.Anomaly.Score.
func rankFlipScorePtr(v float64) *float64 { return &v }

// rankFlipAnomaly builds a minimal Anomaly with the fields the rank-flip
// correlator reads. The detector name is irrelevant to the algorithm — it
// keys exclusively on Source — but we set it so anomaly traces are readable
// in test output.
func rankFlipAnomaly(sourceName string, ts int64, score float64) observer.Anomaly {
	return observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		DetectorName: "test",
		Source:       observer.SeriesDescriptor{Name: sourceName},
		Timestamp:    ts,
		Score:        rankFlipScorePtr(score),
	}
}

// TestRankFlip_Name asserts the correlator's exposed name matches the
// catalog registration. The engine's per-component telemetry and CLI --only
// resolution depend on this string being stable.
func TestRankFlip_Name(t *testing.T) {
	c := NewRankFlipCorrelator()
	assert.Equal(t, "rankflip_correlator", c.Name())
}

// TestRankFlip_DefaultEnabled walks the catalog and asserts the
// rankflip_correlator entry registers as a CORRELATOR (not a detector) with
// defaultEnabled=true. This is the structural guard against repeating the
// exp-0121 wiring mistake (componentDetector with a no-op Detect()) — if a
// future change re-classifies this as componentDetector, this test fails.
func TestRankFlip_DefaultEnabled(t *testing.T) {
	cat := defaultCatalog()
	for _, e := range cat.entries {
		if e.name == "rankflip_correlator" {
			assert.True(t, e.defaultEnabled,
				"rankflip_correlator must be defaultEnabled=true to be visible in the coordinator eval pipeline")
			assert.Equal(t, componentCorrelator, e.kind,
				"rankflip_correlator MUST be a correlator kind — wiring it as componentDetector reproduces the exp-0121 failure")
			instance := e.factory(e.defaultConfig)
			c, ok := instance.(*RankFlipCorrelator)
			require.True(t, ok, "factory must produce *RankFlipCorrelator")
			assert.Equal(t, "rankflip_correlator", c.Name())
			// Structural guard: factory product must satisfy observer.Correlator.
			var _ observer.Correlator = c
			return
		}
	}
	t.Fatal("rankflip_correlator not found in catalog")
}

// TestRankFlip_DetectsFlip drives 30 paired anomalies with positively
// correlated scores, then 30 with anti-correlated scores. After the second
// batch's Advance, ActiveCorrelations must contain the pair, demonstrating
// that the algorithm catches a sign reversal in the rank correlation.
//
// Score generators are deterministic (no RNG noise) so the test is stable
// — the algorithm's robustness to noise is covered by the wider
// |ρ_new − ρ_prev| ≥ FlipDelta threshold, not by the test setup.
func TestRankFlip_DetectsFlip(t *testing.T) {
	c := NewRankFlipCorrelator()

	const batchSize = 30
	descA := observer.SeriesDescriptor{Name: "metric.a"}
	descB := observer.SeriesDescriptor{Name: "metric.b"}

	// Batch 1: positively correlated scores. scoreB = scoreA * 0.9 with a
	// small monotone offset so that A's and B's rank orderings agree.
	for i := 0; i < batchSize; i++ {
		ts := int64(1000 + i)
		scoreA := float64(i) // strictly increasing → ranks 1..30
		scoreB := scoreA*0.9 + 0.001*float64(i)
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descA, Timestamp: ts, Score: rankFlipScorePtr(scoreA),
		})
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descB, Timestamp: ts, Score: rankFlipScorePtr(scoreB),
		})
	}
	c.Advance(int64(1000 + batchSize))

	// Verify state after batch 1: ρ should be ≈ +1, no flip yet.
	c.mu.RLock()
	require.Len(t, c.pairs, 1, "exactly one pair tracked after batch 1")
	var ps *rankFlipPairState
	for _, v := range c.pairs {
		ps = v
	}
	require.NotNil(t, ps)
	assert.True(t, ps.hasPrev, "first batch must have established prevRho")
	assert.GreaterOrEqual(t, ps.prevRho, 0.95, "batch 1 ρ must be near +1; got %.3f", ps.prevRho)
	assert.Nil(t, ps.flipAnomalies, "no flip can be detected on first evaluation")
	c.mu.RUnlock()

	// Batch 2: anti-correlated. scoreB = -scoreA*0.9 — A and B ranks
	// reverse, ρ → near -1.
	tsBase := int64(1000 + batchSize)
	for i := 0; i < batchSize; i++ {
		ts := tsBase + int64(i)
		scoreA := float64(i)
		scoreB := -scoreA * 0.9
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descA, Timestamp: ts, Score: rankFlipScorePtr(scoreA),
		})
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descB, Timestamp: ts, Score: rankFlipScorePtr(scoreB),
		})
	}
	c.Advance(tsBase + int64(batchSize))

	corrs := c.ActiveCorrelations()
	require.NotEmpty(t, corrs, "rank-flip must emit an ActiveCorrelation after the sign reversal")
	require.Len(t, corrs, 1, "only one pair was tracked; expected one correlation")

	ac := corrs[0]
	memberNames := []string{ac.Members[0].Name, ac.Members[1].Name}
	assert.ElementsMatch(t, []string{"metric.a", "metric.b"}, memberNames,
		"emitted correlation must contain both pair members")
	require.Len(t, ac.Anomalies, 2, "flip must surface two trigger anomalies (one per side)")
	assert.Contains(t, ac.Pattern, "rankflip_",
		"Pattern must use the rankflip_ prefix for correlation-id parsing")
}

// TestRankFlip_NoFireOnStableCorrelation feeds 60 paired positively
// correlated anomalies across two Advance batches. ρ stays near +1 the
// whole time — there is no sign flip — so ActiveCorrelations must remain
// empty. This guards against a relaxed flip rule emitting on stable pairs.
func TestRankFlip_NoFireOnStableCorrelation(t *testing.T) {
	c := NewRankFlipCorrelator()
	descA := observer.SeriesDescriptor{Name: "metric.stable.a"}
	descB := observer.SeriesDescriptor{Name: "metric.stable.b"}

	for batch := 0; batch < 2; batch++ {
		base := int64(1000 + batch*30)
		for i := 0; i < 30; i++ {
			ts := base + int64(i)
			scoreA := float64(i)
			scoreB := scoreA*0.9 + 0.001*float64(i)
			c.ProcessAnomaly(observer.Anomaly{
				Type: observer.AnomalyTypeMetric, DetectorName: "test",
				Source: descA, Timestamp: ts, Score: rankFlipScorePtr(scoreA),
			})
			c.ProcessAnomaly(observer.Anomaly{
				Type: observer.AnomalyTypeMetric, DetectorName: "test",
				Source: descB, Timestamp: ts, Score: rankFlipScorePtr(scoreB),
			})
		}
		c.Advance(base + 30)
	}

	corrs := c.ActiveCorrelations()
	assert.Empty(t, corrs,
		"a stable positive correlation must not produce a rank-flip emission; got %d", len(corrs))
}

// TestRankFlip_LRUEvicts pushes MaxPairs+1 distinct pair-fires through the
// correlator and asserts the pairs map is bounded by MaxPairs. Each pair
// uses a unique source on each side so that every fire creates a new pair.
func TestRankFlip_LRUEvicts(t *testing.T) {
	c := NewRankFlipCorrelator()
	require.Equal(t, rankFlipDefaultMaxPairs, c.MaxPairs)

	// Fire MaxPairs+1 distinct pairs, one per Advance. Each pair has its own
	// pair of source names; each side fires once per Advance with the same
	// timestamp so that anyWithinWindow is satisfied.
	for i := 0; i <= c.MaxPairs; i++ {
		ts := int64(1000 + i)
		descA := observer.SeriesDescriptor{Name: fmt.Sprintf("metric.lru.a.%d", i)}
		descB := observer.SeriesDescriptor{Name: fmt.Sprintf("metric.lru.b.%d", i)}
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descA, Timestamp: ts, Score: rankFlipScorePtr(1.0),
		})
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descB, Timestamp: ts, Score: rankFlipScorePtr(2.0),
		})
		c.Advance(ts)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	assert.LessOrEqual(t, len(c.pairs), c.MaxPairs,
		"LRU must bound tracked pairs at MaxPairs; got %d > %d", len(c.pairs), c.MaxPairs)
	assert.LessOrEqual(t, len(c.lru), c.MaxPairs,
		"lru slice must be bounded at MaxPairs; got %d > %d", len(c.lru), c.MaxPairs)
}

// TestRankFlip_Reset drives some pairs through Advance, calls Reset, and
// asserts all internal state is cleared and ActiveCorrelations returns nil.
func TestRankFlip_Reset(t *testing.T) {
	c := NewRankFlipCorrelator()
	descA := observer.SeriesDescriptor{Name: "metric.reset.a"}
	descB := observer.SeriesDescriptor{Name: "metric.reset.b"}

	for i := 0; i < 12; i++ {
		ts := int64(1000 + i)
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descA, Timestamp: ts, Score: rankFlipScorePtr(float64(i)),
		})
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descB, Timestamp: ts, Score: rankFlipScorePtr(float64(i) * 0.9),
		})
	}
	c.Advance(1012)

	c.mu.RLock()
	require.NotEmpty(t, c.pairs, "pairs must be populated before Reset to make the test meaningful")
	c.mu.RUnlock()

	c.Reset()

	assert.Nil(t, c.ActiveCorrelations(), "ActiveCorrelations must be nil after Reset")
	c.mu.RLock()
	defer c.mu.RUnlock()
	assert.Empty(t, c.pairs, "pairs must be empty after Reset")
	assert.Empty(t, c.lru, "lru must be empty after Reset")
	assert.Empty(t, c.pending, "pending must be empty after Reset")
	assert.Empty(t, c.active, "active must be empty after Reset")
	assert.Equal(t, int64(0), c.currentDT, "currentDT must be zero after Reset")
}

// TestRankFlip_RankFloat64s_TieHandling verifies the average-rank
// (mid-rank) tie-breaking on a slice with a 3-way tie, a 2-way tie, and a
// unique value. For [10, 20, 20, 20, 30, 40, 40] the expected ranks are
// [1, 3, 3, 3, 5, 6.5, 6.5]: the three 20s share rank (2+3+4)/3=3, the two
// 40s share rank (6+7)/2=6.5.
func TestRankFlip_RankFloat64s_TieHandling(t *testing.T) {
	got := rankFloat64s([]float64{10, 20, 20, 20, 30, 40, 40})
	want := []float64{1, 3, 3, 3, 5, 6.5, 6.5}
	require.Len(t, got, len(want))
	for i := range want {
		assert.InDelta(t, want[i], got[i], 1e-9, "rank[%d]", i)
	}
}

// TestRankFlip_SpearmanRho_Extremes exercises the helper on three known-
// answer inputs:
//   - identical sequences: ρ = +1
//   - reversed sequences:  ρ = −1
//   - constant b sequence: ρ = 0 (zero variance)
func TestRankFlip_SpearmanRho_Extremes(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	rev := []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	const_ := []float64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}

	rhoIdentity := spearmanRho(a, a)
	rhoReversed := spearmanRho(a, rev)
	rhoConstant := spearmanRho(a, const_)

	assert.InDelta(t, 1.0, rhoIdentity, 1e-9, "identity ρ must be +1")
	assert.InDelta(t, -1.0, rhoReversed, 1e-9, "reversed ρ must be −1")
	assert.InDelta(t, 0.0, rhoConstant, 1e-9, "zero-variance ρ must be 0")
	assert.False(t, math.IsNaN(rhoConstant), "ρ must never be NaN even on degenerate input")
}

// TestRankFlip_NoEmissionBelowMinSamples verifies that we do NOT emit (or
// even compute ρ) before each side's window holds at least
// rankFlipMinSpearmanN samples. This protects against unstable Spearman
// estimates on tiny windows.
func TestRankFlip_NoEmissionBelowMinSamples(t *testing.T) {
	c := NewRankFlipCorrelator()
	descA := observer.SeriesDescriptor{Name: "metric.tiny.a"}
	descB := observer.SeriesDescriptor{Name: "metric.tiny.b"}

	// Push fewer than rankFlipMinSpearmanN paired anomalies.
	for i := 0; i < rankFlipMinSpearmanN-1; i++ {
		ts := int64(1000 + i)
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descA, Timestamp: ts, Score: rankFlipScorePtr(float64(i)),
		})
		c.ProcessAnomaly(observer.Anomaly{
			Type: observer.AnomalyTypeMetric, DetectorName: "test",
			Source: descB, Timestamp: ts, Score: rankFlipScorePtr(-float64(i)),
		})
	}
	c.Advance(int64(1000 + rankFlipMinSpearmanN))

	c.mu.RLock()
	require.Len(t, c.pairs, 1, "pair must be created even before Spearman is evaluable")
	var ps *rankFlipPairState
	for _, v := range c.pairs {
		ps = v
	}
	c.mu.RUnlock()

	assert.False(t, ps.hasPrev, "hasPrev must remain false until min samples reached")
	assert.Nil(t, ps.flipAnomalies, "no flip can be reported before first ρ is computed")
	assert.Empty(t, c.ActiveCorrelations(), "no active correlations before min samples")
}

// TestRankFlip_PairKeyCanonicalization asserts that processing anomalies
// for (A, B) and then (B, A) updates the SAME pair entry — pair identity is
// commutative.
func TestRankFlip_PairKeyCanonicalization(t *testing.T) {
	c := NewRankFlipCorrelator()
	descA := observer.SeriesDescriptor{Name: "alpha"}
	descB := observer.SeriesDescriptor{Name: "beta"}

	// First batch: order A then B.
	c.ProcessAnomaly(rankFlipAnomaly("alpha", 1000, 1.0))
	c.ProcessAnomaly(rankFlipAnomaly("beta", 1001, 2.0))
	c.Advance(1002)

	// Second batch: order B then A.
	c.ProcessAnomaly(rankFlipAnomaly("beta", 2000, 3.0))
	c.ProcessAnomaly(rankFlipAnomaly("alpha", 2001, 4.0))
	c.Advance(2002)

	c.mu.RLock()
	defer c.mu.RUnlock()
	assert.Len(t, c.pairs, 1,
		"pair (alpha, beta) and (beta, alpha) must share a single canonical entry")

	// Verify canonicalization: the stored pair must hold descA and descB
	// in lexicographic Key() order.
	keyA, _ := canonicalRankFlipPairKey(descA, descB)
	keyB, _ := canonicalRankFlipPairKey(descB, descA)
	assert.Equal(t, keyA, keyB, "canonical pair key is symmetric in its inputs")
}
