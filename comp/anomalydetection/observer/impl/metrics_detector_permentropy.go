// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: PermEntropy detector — streaming permutation-entropy
// regime change detection (Bandt & Pompe, Phys. Rev. Lett. 88, 174102, 2002,
// "Permutation Entropy: A Natural Complexity Measure for Time Series").
//
// Why this fills a gap. The other catalog detectors target shifts in the
// marginal mean (BOCPD, ScanMW, ScanWelch, CUSUM), the temporal-dependence
// structure at lag-1 (AcorrShift), the full marginal distribution (DenRatio),
// or slow drift relative to an EWMA baseline (PHT). None flags a series
// whose first and second moments stay constant but whose deterministic vs.
// stochastic *complexity* regime flips — e.g. a metric that goes from
// regular periodic ramp-up cycles to chaotic noise of identical mean and
// variance. PermEntropy reads only the relative ordering of consecutive
// m-tuples, so its signal is invariant to translation, positive scaling,
// and any monotonic transform of the series.
//
// Algorithm (per series, per aggregation):
//
//  1. Maintain a small ring of raw values long enough to read the latest
//     m-tuple (m=4, τ=1) at every tick.
//
//  2. Compute the ordinal pattern of the latest m-tuple via factorial-base
//     ranking. There are m!=24 patterns, fitting in a uint8. Tie-break:
//     "later position is greater", so equal values do not perturb the
//     pattern distribution under noise.
//
//  3. Maintain a rolling pattern history of length W=128 and a per-pattern
//     count vector (24 ints). Once the pattern ring fills, every new
//     pattern evicts the oldest entry; counts sum to W exactly.
//
//  4. Compute Shannon entropy H_t = -Σ p_i log p_i, p_i = count_i / W. Use
//     the standard incremental update (Lall et al. 2006, "Data Streaming
//     Algorithms for Estimating Entropy of Network Traffic"): when a single
//     bin's count changes by ±1, H changes by exactly four xlogx
//     evaluations (old/new contribution, before/after) — O(1) per tick
//     instead of the O(24) brute-force recomputation.
//
//  5. Track the long-run median Ĥ_baseline via a P²-quantile estimator
//     (the same `p2Quantile` used by AcorrShift) and the per-series MAD
//     scale via a 32-slot rolling ring of recent H values: σ̂ =
//     max(MAD * 1.4826, 0.02).
//
//  6. Trigger when |H_t − Ĥ_baseline| / σ̂ exceeds HDelta / 0.20 (the
//     dimensionless threshold that encodes a 1.5σ shift around a typical
//     0.20 H-MAD) for PersistenceK=5 consecutive ticks. Emit one anomaly
//     on alert onset; freeze the trigger baseline so a sustained shift
//     does not re-fire as P² drifts. Recovery clears the alert after
//     RecoveryPoints=12 consecutive ticks where |H − triggerBaseline| ≤
//     HDelta.
//
// Per-tick cost: O(m²)=16 ops for the ordinal pattern, O(1) ring + count
// updates, four log evaluations for the streaming entropy update, O(1) P²
// add, O(W_MAD log W_MAD)=O(32 log 32) median+MAD recomputation post-warmup
// — ~700 ns/tick aggregate. Per-(series, agg) memory: ring (132*8) +
// patternRing (128*1) + patternCounts (24*8) + madRing (32*8) + p2 (~64) +
// scalars ≈ 1.65 KB. No allocations on the hot path.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. The state-array shapes (ring, patternRing,
// patternCounts, madRing) must be compile-time constants. Trigger thresholds
// (HDelta, PersistenceK, RecoveryPoints) are exposed on the detector struct
// so tests can adjust them without touching the per-series fixed-size buffers.
const (
	permentropyEmbedDim       = 4
	permentropyEmbedDelay     = 1 // τ — only τ=1 is implemented; constant retained for spec parity
	permentropyNumPatterns    = 24
	permentropyWindow         = 128
	permentropyWarmup         = 64 // entropy ticks past pattern-ring fill before emission
	permentropyMADWindow      = 32
	permentropyPersistenceK   = 5
	permentropyRecoveryPoints = 12
	permentropyHDelta         = 0.30
	permentropyMADToSigma     = 1.4826
	permentropyTypicalSigma   = 0.20 // expected H-σ used to make HDelta dimensionless
	permentropySigmaFloor     = 0.02 // σ floor for pathologically narrow per-series H
)

// permentropyFactorials are the (m-1-j)! weights used by the factorial-base
// ranking that encodes an m=4 ordinal pattern in [0, 24).
var permentropyFactorials = [permentropyEmbedDim - 1]int{6, 2, 1}

// permentropyStateKey identifies per-series state by ref and aggregation.
type permentropyStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// permentropySeriesState holds per-series streaming state.
type permentropySeriesState struct {
	// Raw value ring. Sized per the plan; only the most recent m values are
	// read on the hot path, the surplus capacity is reserved for future
	// τ>1 embeddings without changing the struct shape.
	ring     [permentropyEmbedDim + permentropyWindow]float64
	ringHead int
	ringN    int

	// Pattern history ring of factorial-base ordinal indices. uint8 fits
	// the [0, 24) range with room to spare.
	patternRing  [permentropyWindow]uint8
	patternHead  int
	patternRingN int

	// Per-pattern frequency counts. Sum of counts == patternRingN until the
	// ring fills, == permentropyWindow after.
	patternCounts [permentropyNumPatterns]int

	// Running entropy maintained via the four-log incremental update once
	// the pattern ring is full. Brute-force seeded on the first full tick
	// so the increments stay O(1).
	entropy      float64
	entropyTicks int

	// Long-run baseline median + MAD ring for σ-scaling.
	baselineP2 p2Quantile
	madRing    [permentropyMADWindow]float64
	madHead    int
	madN       int

	// Trigger lifecycle (mirrors acorrshift). triggerBaseline pins the P²
	// median at alert onset so recovery measures "H has returned to where
	// it was before the shift", not "H is now close to the drifting
	// median" — a sustained shift would otherwise re-fire as P² crawled
	// across the threshold.
	persistCnt      int
	inAlert         bool
	recoveryCnt     int
	triggerBaseline float64

	// Cursor — same pattern as BOCPD/ScanMW/AcorrShift.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// PermEntropyDetector flags shifts in the ordinal-pattern complexity of a
// series. Implements observer.Detector + observer.SeriesRemover.
type PermEntropyDetector struct {
	// HDelta is the |H − H_baseline| threshold in raw H units, used both as
	// the recovery gate and (after dividing by the typical H-σ of 0.20) as
	// the dimensionless trigger gate. Default: 0.30.
	HDelta float64

	// PersistenceK is the number of consecutive over-threshold ticks
	// required to fire. Default: 5.
	PersistenceK int

	// RecoveryPoints is the number of consecutive non-triggering ticks that
	// resets an active alert. Default: 12.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state.
	series map[permentropyStateKey]*permentropySeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewPermEntropyDetector constructs a PermEntropyDetector with default settings.
func NewPermEntropyDetector() *PermEntropyDetector {
	return &PermEntropyDetector{
		HDelta:         permentropyHDelta,
		PersistenceK:   permentropyPersistenceK,
		RecoveryPoints: permentropyRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[permentropyStateKey]*permentropySeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*PermEntropyDetector) Name() string { return "permentropy" }

// Reset clears all per-series state for replay/reanalysis.
func (d *PermEntropyDetector) Reset() {
	d.series = make(map[permentropyStateKey]*permentropySeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~1.65 KB of fixed-size streaming state, so without this teardown the map
// would grow with the cumulative count of series ever observed even after
// their storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *PermEntropyDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, permentropyStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors AcorrShift/BOCPD:
// gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *PermEntropyDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	refs := make([]observer.SeriesRef, len(d.cachedSeries))
	for i, meta := range d.cachedSeries {
		refs[i] = meta.Ref
	}
	bulkStatus := bulkSeriesStatus(storage, refs, dataTime)

	var allAnomalies []observer.Anomaly
	for i, meta := range d.cachedSeries {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			sk := permentropyStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &permentropySeriesState{baselineP2: newP2Quantile(0.5)}
				d.series[sk] = state
			}

			mergeOccurred := status.pointCount == state.lastProcessedCount && status.writeGeneration != state.lastWriteGen
			if status.pointCount <= state.lastProcessedCount && !mergeOccurred {
				continue
			}

			startTime := state.lastProcessedTime
			if mergeOccurred {
				startTime = state.lastProcessedTime - 1
				if startTime < 0 {
					startTime = 0
				}
			}

			pointsSeen := false
			prevLen := len(allAnomalies)
			storage.ForEachPoint(meta.Ref, startTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				pointsSeen = true
				if anomaly := d.processPoint(state, p, s, agg); anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})
			for k := prevLen; k < len(allAnomalies); k++ {
				allAnomalies[k].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}

			if !pointsSeen && mergeOccurred {
				state.lastWriteGen = status.writeGeneration
				continue
			}
			if pointsSeen {
				state.lastProcessedCount = status.pointCount
				state.lastWriteGen = status.writeGeneration
			}
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// processPoint applies the streaming algorithm to a single new point.
// Returns a non-nil anomaly only on alert onset (not while still in alert).
func (d *PermEntropyDetector) processPoint(state *permentropySeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	// Push raw value into the ring. ringN saturates at the buffer size; the
	// only useful threshold downstream is `>= permentropyEmbedDim` (we have
	// enough values to read an m-tuple).
	const ringSize = permentropyEmbedDim + permentropyWindow
	state.ring[state.ringHead] = x
	state.ringHead = (state.ringHead + 1) % ringSize
	if state.ringN < ringSize {
		state.ringN++
	}
	if state.ringN < permentropyEmbedDim {
		return nil
	}

	patternIdx := computeOrdinalPattern(&state.ring, state.ringHead, ringSize)

	// Pattern ring update with bookkeeping for the incremental entropy
	// formula below. We need both the pre-decrement and post-decrement
	// counts for the evicted bin and both the pre-increment and
	// post-increment counts for the new bin.
	ringWasFull := state.patternRingN == permentropyWindow
	var (
		evictedIdx     uint8
		evictedBefore  int
		evictedAfter   int
	)
	if ringWasFull {
		evictedIdx = state.patternRing[state.patternHead]
		evictedBefore = state.patternCounts[evictedIdx]
		state.patternCounts[evictedIdx]--
		evictedAfter = state.patternCounts[evictedIdx]
	}

	newBefore := state.patternCounts[patternIdx]
	state.patternCounts[patternIdx]++
	newAfter := state.patternCounts[patternIdx]

	state.patternRing[state.patternHead] = uint8(patternIdx)
	state.patternHead = (state.patternHead + 1) % permentropyWindow
	if !ringWasFull {
		state.patternRingN++
	}

	if !ringWasFull {
		// While the pattern ring is filling, defer scoring entirely. When
		// it fills exactly, brute-force seed `entropy` so subsequent ticks
		// can update it incrementally without ever paying the O(24) cost
		// on the hot path.
		if state.patternRingN == permentropyWindow {
			state.entropy = computeEntropyFromCounts(&state.patternCounts, permentropyWindow)
		}
		return nil
	}

	// Incremental Shannon entropy update. Only two bins changed (or zero
	// bins changed if the evicted pattern equals the new pattern, in which
	// case the counts are identical). Each affected bin contributes one
	// "before" subtraction and one "after" addition, four logs total.
	if int(evictedIdx) != patternIdx {
		W := float64(permentropyWindow)
		state.entropy -= entropyTerm(evictedBefore, W)
		state.entropy -= entropyTerm(newBefore, W)
		state.entropy += entropyTerm(evictedAfter, W)
		state.entropy += entropyTerm(newAfter, W)
		// Floor numerical drift from accumulated rounding.
		if state.entropy < 0 {
			state.entropy = 0
		}
	}
	H := state.entropy

	// Long-run baseline median and per-series scale.
	state.baselineP2.add(H)
	state.madRing[state.madHead] = H
	state.madHead = (state.madHead + 1) % permentropyMADWindow
	if state.madN < permentropyMADWindow {
		state.madN++
	}

	state.entropyTicks++
	if state.entropyTicks < permentropyWarmup || state.madN < permentropyMADWindow {
		return nil
	}
	baseline, ok := state.baselineP2.value()
	if !ok {
		return nil
	}

	median := detectorMedian(state.madRing[:state.madN])
	mad := detectorMAD(state.madRing[:state.madN], median, false)
	sigma := mad * permentropyMADToSigma
	if sigma < permentropySigmaFloor {
		sigma = permentropySigmaFloor
	}

	delta := math.Abs(H - baseline)
	zScore := delta / sigma
	threshold := d.HDelta / permentropyTypicalSigma

	// Two-stage gate: a tick counts toward persistence iff it clears both
	// the σ-significance threshold AND the raw |ΔH| threshold. The σ gate
	// alone fires on stationary random-walk fluctuations where the per-
	// series MAD-σ collapses to ~0.02 and modest 0.05–0.10 H excursions
	// look "significant" by ratio. HDelta is documented as a raw |H −
	// baseline| threshold and is reused by the recovery gate below, so
	// pairing it with the σ gate keeps trigger and recovery symmetric.
	if zScore > threshold && delta >= d.HDelta {
		state.persistCnt++
	} else {
		state.persistCnt = 0
	}

	persistK := d.PersistenceK
	if persistK > permentropyWindow {
		// Defensive: a persistence streak longer than the rolling window is
		// physically meaningless and would dead-lock the trigger.
		persistK = permentropyWindow
	}

	if state.persistCnt >= persistK {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting on this regime; do not re-emit until recovery.
			return nil
		}
		state.inAlert = true
		state.triggerBaseline = baseline
		return d.makeAnomaly(p, series, agg, H, baseline, delta, sigma)
	}

	if state.inAlert {
		// Recovery is anchored to the baseline at the moment of trigger.
		// |H − triggerBaseline| ≤ HDelta means H has returned to its
		// pre-shift level. Anchoring to the drifting current median would
		// let the running median crawl into post-shift H values and
		// produce a spurious "calm" reading.
		if math.Abs(H-state.triggerBaseline) <= d.HDelta {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.recoveryCnt = 0
			}
		} else {
			state.recoveryCnt = 0
		}
	}
	return nil
}

// makeAnomaly constructs the alert-onset anomaly.
func (d *PermEntropyDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, H, baseline, delta, sigma float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()
	zScore := delta / sigma
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "PermEntropy ordinal-complexity regime change: " + displayName,
		Description: fmt.Sprintf("%s permutation entropy shifted from baseline %.3f to %.3f (|Δ|=%.3f, %.2fσ, sustained %d ticks)",
			displayName, baseline, H, delta, zScore, d.PersistenceK),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: baseline,
			CurrentValue:   H,
			DeviationSigma: zScore,
			Threshold:      d.HDelta / permentropyTypicalSigma,
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults so the
// zero-valued struct works.
func (d *PermEntropyDetector) ensureDefaults() {
	if d.HDelta <= 0 {
		d.HDelta = permentropyHDelta
	}
	if d.PersistenceK <= 0 {
		d.PersistenceK = permentropyPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = permentropyRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[permentropyStateKey]*permentropySeriesState)
	}
}

// computeOrdinalPattern returns the factorial-base ordinal-pattern index in
// [0, 24) of the most-recent m=4 tuple in the ring. The tuple is read in
// chronological order from positions [head-m, head-1] modulo ringSize. Tie
// break: equal values are resolved by position (later position is greater)
// so the encoded pattern is stable under noise.
func computeOrdinalPattern(ring *[permentropyEmbedDim + permentropyWindow]float64, head, ringSize int) int {
	var x [permentropyEmbedDim]float64
	for i := 0; i < permentropyEmbedDim; i++ {
		idx := (head - permentropyEmbedDim + i) % ringSize
		if idx < 0 {
			idx += ringSize
		}
		x[i] = ring[idx]
	}
	pattern := 0
	for j := 0; j < permentropyEmbedDim-1; j++ {
		c := 0
		for k := j + 1; k < permentropyEmbedDim; k++ {
			// Strict comparison only: ties go to the later position, so
			// equal values do not contribute to the count of "later
			// positions strictly smaller than j".
			if x[k] < x[j] {
				c++
			}
		}
		pattern += c * permentropyFactorials[j]
	}
	return pattern
}

// entropyTerm returns -p log p for p = n/W, with the standard 0 log 0 = 0
// convention. Used by both the brute-force seed and the incremental update.
func entropyTerm(n int, W float64) float64 {
	if n <= 0 {
		return 0
	}
	p := float64(n) / W
	return -p * math.Log(p)
}

// computeEntropyFromCounts brute-force computes the Shannon entropy of the
// pattern distribution. Called once per series when the pattern ring fills,
// to seed `state.entropy` for subsequent incremental updates. Also used by
// tests as a parallel reference.
func computeEntropyFromCounts(counts *[permentropyNumPatterns]int, total int) float64 {
	if total <= 0 {
		return 0
	}
	W := float64(total)
	var H float64
	for i := 0; i < permentropyNumPatterns; i++ {
		H += entropyTerm(counts[i], W)
	}
	return H
}
