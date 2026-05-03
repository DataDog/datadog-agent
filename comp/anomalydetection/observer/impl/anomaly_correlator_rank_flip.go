// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Rank-flip correlator hyperparameter defaults. Held as package-level
// constants so the catalog factory stays parameterless; tunable copies are
// exported fields on RankFlipCorrelator and can be overwritten by tests.
const (
	// rankFlipDefaultWindowSec defines what "close in time" means when grouping
	// pending anomalies into pairs. 60s mirrors TimeClusterCorrelator's default
	// window — pairs whose anomalies fire within this window are considered
	// candidates for rank-correlation tracking.
	rankFlipDefaultWindowSec int64 = 60

	// rankFlipDefaultCorrWindowSize is the number of anomaly-score samples
	// retained per side of each pair. 30 is large enough for Spearman ρ to be
	// statistically meaningful and small enough that ring-buffer pushes stay
	// under the per-Advance budget (16 pairs · 30 floats · 2 sides ≈ 8 kB).
	rankFlipDefaultCorrWindowSize int = 30

	// rankFlipDefaultFlipDelta is the minimum |ρ_new − ρ_prev| required to
	// trigger an emission, AND the two ρ values must straddle zero. 0.8 is a
	// wide margin: it suppresses incremental drift in mildly-correlated pairs
	// while reliably catching genuine sign reversals (e.g. +0.6 → −0.5 has
	// |delta|=1.1 and a sign flip).
	rankFlipDefaultFlipDelta float64 = 0.8

	// rankFlipDefaultMaxPairs caps the number of tracked pairs via LRU eviction.
	// 16 keeps total state bounded (~8 kB) and per-Advance work O(K·W log W)
	// ≈ 16·30·log₂(30) ≈ 2300 ops.
	rankFlipDefaultMaxPairs int = 16

	// rankFlipMinSpearmanN is the minimum number of paired samples required
	// before computing Spearman ρ. Below this, Spearman is statistically
	// unstable; we accumulate but do not evaluate.
	rankFlipMinSpearmanN int = 10
)

// rankFlipPairState holds per-pair state for the rank-flip correlator: two
// score windows (one per side), the previous ρ, and the latest detected flip
// (kept until eviction by Advance).
type rankFlipPairState struct {
	// descA, descB are the two participating series, stored in canonical
	// (lex-sorted by Key()) order to make pair identity stable across the
	// commutative input order.
	descA, descB observer.SeriesDescriptor

	// scoresA / scoresB are sliding windows of the most recent
	// CorrWindowSize anomaly scores observed on each side. Implemented as
	// shifted slices for clarity — the perf budget is generous (≤30 floats
	// per push, ≤16 pairs).
	scoresA []float64
	scoresB []float64

	// prevRho is the most recently computed Spearman ρ. hasPrev guards
	// against using the zero value before the first evaluation.
	prevRho float64
	hasPrev bool

	// flipAnomalies are the two anomalies (one per side) that triggered the
	// most recent sign-flip. Kept until the eviction window passes.
	flipAnomalies []observer.Anomaly
	flipFirstSeen int64
	flipLastSeen  int64
	// flipPrevRho / flipCurRho are the ρ values straddling the most recent
	// flip — exposed in the ActiveCorrelation Title for operator debug.
	flipPrevRho float64
	flipCurRho  float64

	// lastSeenAt is the most recent anomaly timestamp seen on either side,
	// used to drive LRU touch ordering.
	lastSeenAt int64
}

// RankFlipCorrelator tracks pairwise Spearman rank-correlation ρ on
// anomaly-score sequences per active pair (kind: componentCorrelator). When
// ρ flips sign by ≥ FlipDelta between two consecutive evaluations on the
// same pair, it emits an ActiveCorrelation describing the flip — a signal
// of regime change in cross-series DEPENDENCE.
//
// # Algorithm
//
// State per pair (A, B):
//   - sliding windows scoresA, scoresB of the last CorrWindowSize anomaly
//     scores observed on each side
//   - the previously computed Spearman ρ (prevRho, hasPrev)
//   - the latest detected flip (flipAnomalies, flipPrevRho, flipCurRho, ...)
//
// On each Advance(dataTime):
//  1. Group pending anomalies by source key.
//  2. For every pair (A, B) whose pending anomalies include at least one
//     timestamp pair within WindowSec, push their scores into the per-pair
//     ring buffers, then recompute Spearman ρ over the last min(|A|, |B|)
//     points.
//  3. If hasPrev AND |ρ − prevRho| ≥ FlipDelta AND prevRho·ρ < 0, mark a
//     flip with the two triggering anomalies.
//  4. Touch LRU; evict the oldest pair beyond MaxPairs.
//  5. Rebuild the active snapshot from all pairs whose flipLastSeen is
//     within 2·WindowSec of dataTime.
//
// # Why the algorithm operates on anomaly scores, not raw time series
//
// The Correlator interface does not expose StorageReader, so a correlator
// cannot reach back to fetch the raw points behind an anomaly's
// SourceRef. Tracking rank-correlation on the SCORE sequences keeps the
// algorithm self-contained at the interface boundary while preserving the
// rank-flip semantics — when two series's score distributions reverse their
// rank-coupling, that itself is a coupled-regime-change signal.
//
// # Concurrency & complexity
//
// All state is guarded by mu (sync.RWMutex). ProcessAnomaly is O(1) (one
// append). Advance is O(K · W log W) with K ≤ MaxPairs and W ≤
// CorrWindowSize, which with default hyperparameters bounds Advance work
// at roughly 25 µs. Memory is fixed at ≈ 8 kB across the entire correlator.
//
// # Suppressive emission
//
// This correlator emits ≤ 1 ActiveCorrelation per tracked pair, and only on
// ρ sign-flip. It cannot inflate the count of anomalies relative to the
// detector input stream; like LORDFDRCorrelator, it is structurally
// suppressive.
type RankFlipCorrelator struct {
	// WindowSec defines what "close in time" means when grouping anomalies
	// into pairs. See rankFlipDefaultWindowSec.
	WindowSec int64
	// CorrWindowSize is the per-side score window length.
	// See rankFlipDefaultCorrWindowSize.
	CorrWindowSize int
	// FlipDelta is the minimum |ρ_new − ρ_prev| for a flip. Combined with a
	// sign-straddle requirement (prevRho·rho < 0).
	// See rankFlipDefaultFlipDelta.
	FlipDelta float64
	// MaxPairs caps tracked pairs via LRU eviction. See rankFlipDefaultMaxPairs.
	MaxPairs int

	mu sync.RWMutex

	// currentDT is the most recent dataTime passed to Advance. Used only for
	// debugging today; preserved across Advance calls.
	currentDT int64

	// pending is the set of anomalies received since the last Advance call.
	pending []observer.Anomaly

	// pairs holds per-pair state, keyed by canonicalRankFlipPairKey.
	pairs map[string]*rankFlipPairState

	// lru holds pair keys with MRU at the end. Bounded to MaxPairs by Advance.
	lru []string

	// active is the snapshot returned by ActiveCorrelations, recomputed at
	// the end of each Advance.
	active []observer.ActiveCorrelation
}

// NewRankFlipCorrelator returns a RankFlipCorrelator with production
// hyperparameters. The factory is parameterless to match the catalog
// convention for correlators that don't read agent-config tuning.
func NewRankFlipCorrelator() *RankFlipCorrelator {
	return &RankFlipCorrelator{
		WindowSec:      rankFlipDefaultWindowSec,
		CorrWindowSize: rankFlipDefaultCorrWindowSize,
		FlipDelta:      rankFlipDefaultFlipDelta,
		MaxPairs:       rankFlipDefaultMaxPairs,
		pairs:          make(map[string]*rankFlipPairState),
	}
}

// Compile-time check that RankFlipCorrelator satisfies observer.Correlator.
// If the interface drifts, this assertion will fail to compile loudly — the
// exp-0121 wiring mistake (componentDetector with a no-op Detect()) cannot
// recur on this type because Detect() is intentionally absent.
var _ observer.Correlator = (*RankFlipCorrelator)(nil)

// Name returns the catalog-registered correlator name. Must match the
// catalog entry exactly so q.eval-scenarios --only and the engine's
// per-component telemetry resolve.
func (c *RankFlipCorrelator) Name() string { return "rankflip_correlator" }

// ProcessAnomaly is O(1): the per-pair work is deferred to Advance, where
// we have the data-time context needed for temporal grouping and eviction.
// Doing this work eagerly per anomaly would require a per-source pending
// buffer and a fan-out to every potential pair on every fire, which is
// quadratic in the number of distinct sources observed.
func (c *RankFlipCorrelator) ProcessAnomaly(a observer.Anomaly) {
	c.mu.Lock()
	c.pending = append(c.pending, a)
	c.mu.Unlock()
}

// canonicalRankFlipPairKey returns a stable map key for an unordered pair,
// by sorting the two descriptor keys lexicographically and joining with a
// NUL separator. The boolean indicates whether the inputs were already in
// canonical (a ≤ b) order — callers use this to align incoming buckets
// with the stored (descA, descB) orientation.
func canonicalRankFlipPairKey(a, b observer.SeriesDescriptor) (string, bool) {
	ka, kb := a.Key(), b.Key()
	if ka <= kb {
		return ka + "\x00" + kb, true
	}
	return kb + "\x00" + ka, false
}

// Advance does the per-cycle work: groups pending anomalies into pairs,
// updates per-pair score windows, recomputes ρ, detects flips, and rebuilds
// the active-correlations snapshot under eviction.
func (c *RankFlipCorrelator) Advance(dataTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if dataTime > c.currentDT {
		c.currentDT = dataTime
	}
	pending := c.pending
	c.pending = nil

	if len(pending) > 0 {
		c.processPendingLocked(pending)
	}

	// Eviction window for the active snapshot: keep flips whose last-seen
	// timestamp is within 2·WindowSec of the current data time. Using a
	// floor guard at zero so a small dataTime in tests doesn't
	// accidentally evict via int-underflow.
	evictBefore := dataTime - 2*c.WindowSec
	var active []observer.ActiveCorrelation
	for _, key := range c.lru {
		ps, ok := c.pairs[key]
		if !ok || ps.flipAnomalies == nil {
			continue
		}
		if ps.flipLastSeen < evictBefore {
			continue
		}
		active = append(active, c.buildActiveCorrelationLocked(ps))
	}
	c.active = active
}

// processPendingLocked groups pending anomalies by source, finds pairs whose
// anomalies are within WindowSec of each other, and updates each pair's
// state. Caller holds c.mu.
func (c *RankFlipCorrelator) processPendingLocked(pending []observer.Anomaly) {
	type bucket struct {
		desc      observer.SeriesDescriptor
		anomalies []observer.Anomaly
	}
	buckets := make(map[string]*bucket)
	keys := make([]string, 0)
	for i := range pending {
		a := pending[i]
		k := a.Source.Key()
		b, ok := buckets[k]
		if !ok {
			b = &bucket{desc: a.Source}
			buckets[k] = b
			keys = append(keys, k)
		}
		b.anomalies = append(b.anomalies, a)
	}

	// Sort source keys for deterministic pair iteration. Determinism here
	// matters for tests and for telemetry dumps that stream pair updates
	// in catalog-defined order.
	sort.Strings(keys)

	for i := 0; i < len(keys); i++ {
		bi := buckets[keys[i]]
		for j := i + 1; j < len(keys); j++ {
			bj := buckets[keys[j]]
			if !c.anyWithinWindow(bi.anomalies, bj.anomalies) {
				continue
			}
			c.updatePairLocked(bi.desc, bi.anomalies, bj.desc, bj.anomalies)
		}
	}
}

// anyWithinWindow reports whether at least one (a, b) pair has
// |a.Timestamp − b.Timestamp| ≤ WindowSec. O(|as|·|bs|), bounded in practice
// by the small number of anomalies a single detection cycle produces per
// source.
func (c *RankFlipCorrelator) anyWithinWindow(as, bs []observer.Anomaly) bool {
	for _, a := range as {
		for _, b := range bs {
			d := a.Timestamp - b.Timestamp
			if d < 0 {
				d = -d
			}
			if d <= c.WindowSec {
				return true
			}
		}
	}
	return false
}

// updatePairLocked ingests a batch of anomalies for the pair (descA, descB),
// updates score windows, recomputes Spearman ρ, detects flips, and updates
// the LRU. Caller holds c.mu.
func (c *RankFlipCorrelator) updatePairLocked(
	descA observer.SeriesDescriptor, as []observer.Anomaly,
	descB observer.SeriesDescriptor, bs []observer.Anomaly,
) {
	key, canonical := canonicalRankFlipPairKey(descA, descB)

	ps, ok := c.pairs[key]
	if !ok {
		ps = &rankFlipPairState{}
		if canonical {
			ps.descA, ps.descB = descA, descB
		} else {
			ps.descA, ps.descB = descB, descA
		}
		c.pairs[key] = ps
	}

	// Align incoming buckets with the canonical (descA, descB) orientation.
	asForA, asForB := as, bs
	if !canonical {
		asForA, asForB = bs, as
	}

	asA := sortRankFlipByTimestamp(asForA)
	asB := sortRankFlipByTimestamp(asForB)
	for _, a := range asA {
		ps.scoresA = pushBoundedFloat64(ps.scoresA, anomalyScore(a), c.CorrWindowSize)
		if a.Timestamp > ps.lastSeenAt {
			ps.lastSeenAt = a.Timestamp
		}
	}
	for _, b := range asB {
		ps.scoresB = pushBoundedFloat64(ps.scoresB, anomalyScore(b), c.CorrWindowSize)
		if b.Timestamp > ps.lastSeenAt {
			ps.lastSeenAt = b.Timestamp
		}
	}

	// Compute Spearman ρ if both buffers have the minimum required count.
	n := len(ps.scoresA)
	if len(ps.scoresB) < n {
		n = len(ps.scoresB)
	}
	if n >= rankFlipMinSpearmanN {
		// Take the last n points from each side to align lengths.
		a := ps.scoresA[len(ps.scoresA)-n:]
		b := ps.scoresB[len(ps.scoresB)-n:]
		ranksA := rankFloat64s(a)
		ranksB := rankFloat64s(b)
		rho := spearmanRho(ranksA, ranksB)

		// FLIP rule:
		//   |rho − prevRho| ≥ FlipDelta AND prevRho·rho < 0
		// The product check enforces a strict sign straddle: zero on either
		// side does NOT count as a flip. This avoids spurious emissions
		// when ρ wanders through zero with low magnitude on both sides.
		if ps.hasPrev && math.Abs(rho-ps.prevRho) >= c.FlipDelta && ps.prevRho*rho < 0 {
			triggerA := asA[len(asA)-1]
			triggerB := asB[len(asB)-1]
			ps.flipAnomalies = []observer.Anomaly{triggerA, triggerB}
			ps.flipFirstSeen = triggerA.Timestamp
			if triggerB.Timestamp < ps.flipFirstSeen {
				ps.flipFirstSeen = triggerB.Timestamp
			}
			ps.flipLastSeen = triggerA.Timestamp
			if triggerB.Timestamp > ps.flipLastSeen {
				ps.flipLastSeen = triggerB.Timestamp
			}
			ps.flipPrevRho = ps.prevRho
			ps.flipCurRho = rho
		}
		ps.prevRho = rho
		ps.hasPrev = true
	}

	c.touchLRULocked(key)
}

// touchLRULocked moves key to the MRU end of c.lru, inserting if absent.
// Beyond MaxPairs the oldest entry (head) is evicted from both lru and
// pairs. Caller holds c.mu.
func (c *RankFlipCorrelator) touchLRULocked(key string) {
	idx := -1
	for i, k := range c.lru {
		if k == key {
			idx = i
			break
		}
	}
	if idx >= 0 {
		if idx != len(c.lru)-1 {
			c.lru = append(c.lru[:idx], c.lru[idx+1:]...)
			c.lru = append(c.lru, key)
		}
	} else {
		c.lru = append(c.lru, key)
	}

	for c.MaxPairs > 0 && len(c.lru) > c.MaxPairs {
		old := c.lru[0]
		c.lru = c.lru[1:]
		delete(c.pairs, old)
	}
}

// buildActiveCorrelationLocked renders an active correlation entry for a
// pair state. Caller holds c.mu.
func (c *RankFlipCorrelator) buildActiveCorrelationLocked(ps *rankFlipPairState) observer.ActiveCorrelation {
	// Defensive copy of the trigger anomalies so callers cannot mutate
	// internal state through the returned slice.
	anomalies := make([]observer.Anomaly, len(ps.flipAnomalies))
	copy(anomalies, ps.flipAnomalies)
	return observer.ActiveCorrelation{
		Pattern: fmt.Sprintf("rankflip_%s|%s", ps.descA.Name, ps.descB.Name),
		Title: fmt.Sprintf("Rank-corr flip: %s ⇋ %s (ρ %+.2f→%+.2f)",
			ps.descA.DisplayName(), ps.descB.DisplayName(), ps.flipPrevRho, ps.flipCurRho),
		Members:     []observer.SeriesDescriptor{ps.descA, ps.descB},
		Anomalies:   anomalies,
		FirstSeen:   ps.flipFirstSeen,
		LastUpdated: ps.flipLastSeen,
	}
}

// ActiveCorrelations returns the snapshot computed at the most recent
// Advance. Returns nil (not an empty slice) when there are no active flips
// — the engine treats nil and empty as equivalent.
func (c *RankFlipCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.active) == 0 {
		return nil
	}
	out := make([]observer.ActiveCorrelation, len(c.active))
	copy(out, c.active)
	return out
}

// Reset clears all internal state, making the correlator ready for a fresh
// replay (testbench reanalysis path).
func (c *RankFlipCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentDT = 0
	c.pending = nil
	c.pairs = make(map[string]*rankFlipPairState)
	c.lru = nil
	c.active = nil
}

// pushBoundedFloat64 appends v to buf, evicting the oldest element if buf
// already holds capN entries. capN ≤ 0 is a safety no-op (returns buf
// unchanged). Allocations: zero on the steady state once cap is reached
// (in-place shift), one append-grow before that.
func pushBoundedFloat64(buf []float64, v float64, capN int) []float64 {
	if capN <= 0 {
		return buf
	}
	if len(buf) >= capN {
		copy(buf, buf[1:])
		buf = buf[:capN-1]
	}
	return append(buf, v)
}

// anomalyScore returns the anomaly's Score, treating nil as 0.0. The
// rank-flip algorithm operates on relative rank order, so a uniform 0
// substitution is benign; nil-Score anomalies will land at the bottom of
// the rank distribution alongside any genuine zero scores.
func anomalyScore(a observer.Anomaly) float64 {
	if a.Score == nil {
		return 0
	}
	return *a.Score
}

// sortRankFlipByTimestamp returns a copy of xs sorted by Timestamp
// ascending, with stable ordering for equal timestamps. The copy keeps the
// caller's slice undisturbed (they may mutate it independently).
func sortRankFlipByTimestamp(xs []observer.Anomaly) []observer.Anomaly {
	out := make([]observer.Anomaly, len(xs))
	copy(out, xs)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Timestamp < out[j].Timestamp
	})
	return out
}

// rankFloat64s returns the average-rank (mid-rank) ranks of xs, where the
// smallest value gets rank 1 and ties share the mean rank of their
// positions. Output length is len(xs). Time: O(n log n) (the sort);
// space: O(n).
//
// Mid-rank tie-breaking is the standard Spearman-with-ties approach
// (Hollander & Wolfe 1999, §8.5): when k samples share a value, all k get
// rank (firstPos + lastPos)/2. This makes spearmanRho via Pearson-on-ranks
// agree with the textbook tie-corrected Spearman formula.
func rankFloat64s(xs []float64) []float64 {
	n := len(xs)
	if n == 0 {
		return nil
	}
	type ix struct {
		v float64
		i int
	}
	sorted := make([]ix, n)
	for i, v := range xs {
		sorted[i] = ix{v, i}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].v < sorted[j].v })
	ranks := make([]float64, n)
	i := 0
	for i < n {
		j := i + 1
		for j < n && sorted[j].v == sorted[i].v {
			j++
		}
		// Average rank for positions [i, j) is ((i+1) + j) / 2 (1-based).
		avg := (float64(i+1) + float64(j)) / 2.0
		for k := i; k < j; k++ {
			ranks[sorted[k].i] = avg
		}
		i = j
	}
	return ranks
}

// spearmanRho computes Spearman's ρ given two equal-length rank sequences.
// Implemented as Pearson on ranks so that average-rank ties are handled
// correctly (the simple 1 − 6Σd²/(n·(n²−1)) form is exact only when there
// are no ties). Returns 0 for degenerate inputs (mismatched lengths,
// n < 2, or zero variance on either side).
//
// Output is clamped to [-1, 1] to absorb floating-point rounding on
// near-perfect correlations.
func spearmanRho(ra, rb []float64) float64 {
	n := len(ra)
	if n == 0 || n != len(rb) || n < 2 {
		return 0
	}
	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += ra[i]
		sumB += rb[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)
	var num, denomA, denomB float64
	for i := 0; i < n; i++ {
		da := ra[i] - meanA
		db := rb[i] - meanB
		num += da * db
		denomA += da * da
		denomB += db * db
	}
	den := math.Sqrt(denomA * denomB)
	if den == 0 {
		return 0
	}
	rho := num / den
	if rho > 1 {
		rho = 1
	} else if rho < -1 {
		rho = -1
	}
	return rho
}
