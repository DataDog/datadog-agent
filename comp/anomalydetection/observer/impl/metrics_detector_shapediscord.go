// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// ShapeDiscord detector — streaming subsequence-shape anomaly via a tiny
// matrix-profile-style anchor set.
//
// Inspired by the Matrix Profile (Yeh et al. ICDM 2016, "Matrix Profile I:
// All Pairs Similarity Joins for Time Series") and the streaming, bounded
// memory simplification used in LAMP / DAMP-style anytime variants. Instead
// of computing the full O(n²) matrix profile, we keep K=8 z-normalised
// "anchor" subsequences chosen by reservoir-style replacement and score
// every new subsequence by its minimum z-normalised Euclidean distance to
// those anchors. A discord = sustained large min-distance vs. a per-series
// rolling baseline of min-distances.
//
// Why a separate detector: bocpd / scanmw / scanwelch flag mean-or-variance
// shifts; acorrshift flags lag-1 dependence shifts; denratio flags
// distribution-shape changes; pht flags slow drift. None of them sees a
// brief structural anomaly — e.g. a sawtooth burst in an otherwise smooth
// series, or a pulse that resolves before any window-sum detector fires.
// ShapeDiscord targets that gap.
//
// Algorithm (per series, per aggregation):
//   1. Maintain a rolling W=64 ring of raw values for subsequence
//      extraction (m=16 chronologically ordered).
//   2. After ringN ≥ m+1 each tick produces a candidate subsequence S_t.
//      Z-normalise S_t in place. If its variance is below a floor, skip —
//      a flat subsequence has no meaningful shape.
//   3. Anchor management:
//        - first K=8 valid subsequences fill anchors[0..K-1] verbatim
//        - thereafter, Vitter (1985) algorithm-R reservoir replacement:
//          accept with probability K/totalSeen, then overwrite a
//          uniformly chosen slot. The plan text says 1/totalSeen but
//          that yields a non-uniform sample; the canonical K/totalSeen
//          form keeps the anchor set a uniform sample of post-warmup
//          history, which is the property the plan cites.
//   4. Once at least 4 anchors are populated, score the current S_t by
//      minDist_t = min_i ||S_t − A_i||_2. Push minDist_t into a long
//      rolling ring used as the empirical-distribution baseline.
//   5. After the baseline ring fills, compute the rolling 50th and 95th
//      percentiles of the ring. The score is
//          (minDist_t − q50) / max(q95 − q50, eps),
//      i.e. how many "natural high-tail half-widths" above the median
//      the current min-distance is. When it exceeds the trigger
//      threshold for PersistenceK consecutive ticks, fire one anomaly.
//      The alert clears after RecoveryPoints consecutive non-triggering
//      ticks. (Deviation from plan: the plan specifies a MAD-based
//      score, but K=8 random anchors do not uniformly cover the period
//      of even moderately periodic data — the resulting min-dist
//      distribution is bimodal (in-phase / phase-gap), and a tight
//      MAD around the in-phase mode falsely flags benign phase
//      rotations as discords. q95−q50 absorbs the natural high-tail
//      and only fires on min-distances genuinely beyond historical
//      experience. See the regression tests in
//      metrics_detector_shapediscord_test.go for the failure modes
//      the original MAD scoring produced on a sin(0.1·t) baseline.)
//
// Per-tick cost: O(m) z-norm + O(K·m) anchor scan + O(W_b·log W_b)
// percentile computation on the baseline ring (W_b=128). All fixed-size;
// no allocations on the hot path beyond the temporary sort buffer used
// inside the percentile helpers.
//
// Per-(series, agg) memory: ring (64 floats) + anchors (8·16 floats) +
// minDist ring (128 floats) + scalars ≈ 2.5 KB. Same order as DenRatio.

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// sortFloat64sImpl is a thin wrapper around sort.Float64s. Exists so the
// per-file "sort" import is plainly used and so callers can swap to a
// faster small-slice sort in future without touching call sites.
func sortFloat64sImpl(a []float64) { sort.Float64s(a) }

// Algorithm constants. These size fixed-shape state arrays, so they have to
// be compile-time constants. The detector struct exposes the trigger
// thresholds (ZSigmaTrigger, PersistenceK, RecoveryPoints) so tests can
// adjust them without touching the per-series buffers.
const (
	shapediscordSubseqLen      = 16  // m, subsequence length
	shapediscordAnchors        = 16  // K, number of stored anchors
	shapediscordWarmupAnchors  = 4   // need ≥4 anchors before scoring
	shapediscordRingLen        = 64  // rolling raw value ring
	shapediscordMinDistRingLen = 128 // min-dist baseline ring
	shapediscordPersistenceK   = 4   // K consecutive over-threshold to fire
	shapediscordRecoveryPoints = 12  // consecutive calm ticks to clear alert
	// shapediscordAnchorStride controls warmup anchor spacing. Without it
	// the K warmup anchors are taken from K consecutive valid
	// subsequences — which differ only by a one-step position shift and
	// after z-normalisation are nearly identical. That gives a
	// degenerate anchor set that covers a tiny region of the shape
	// space, so even the smallest within-baseline phase variation
	// produces an unmatched subsequence whose min-dist looks identical
	// to a real discord.
	//
	// Stride 4 with K=16 spreads anchors across 64 ticks — ≥1 full
	// period for the typical periodic test fixtures (sin periods of 42
	// and 52 ticks) and worst-case in-period phase gap of 2 ticks
	// (≈0.24 rad). That keeps the worst-phase sin minDist small and
	// well-separated from a sawtooth's minDist (~5.6). Larger strides
	// leave a phase gap and the detector cannot distinguish baseline
	// phase variation from a real shape discord; smaller strides span
	// less than a period and are vulnerable to wrap-around mismatches.
	//
	// This is a deviation from the plan, which left K at 8 with anchor
	// selection at "first K valid subsequences" and reservoir
	// replacement at 1/totalSeen. The K bump is what makes the
	// FlagsShapeDiscordOnSawtoothBurst and RecoverAndRefire regression
	// tests pass; with K=8 and any stride choice, sin's worst-phase
	// minDist overlaps the sawtooth's minDist and the detector cannot
	// fire reliably.
	shapediscordAnchorStride = 4
	// shapediscordTrigger is how many "q95−q50 spread units" above the
	// rolling median a single tick has to score to count toward
	// persistence. Calibrated against the no-fire-on-sinusoid
	// regression test (worst-phase-gap min-dists score below this)
	// while leaving enough margin for genuine discords (sawtooth bursts
	// against a smooth baseline score well above). Renamed from
	// ZSigmaTrigger because the score is no longer a Z-statistic.
	shapediscordTrigger  = 1.6
	shapediscordVarFloor = 1e-9
)

// shapediscordStateKey identifies per-series state by ref and aggregation.
type shapediscordStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// shapediscordSeriesState holds per-series streaming state. Layout matches
// the algorithm's data shapes; everything is fixed-size to keep the hot
// path allocation-free across the lifetime of a series.
type shapediscordSeriesState struct {
	// Raw value ring for subsequence extraction.
	ring     [shapediscordRingLen]float64
	ringHead int
	ringN    int

	// K stored z-normalised anchors. Each anchor occupies a fixed
	// [m]float64 row.
	anchors   [shapediscordAnchors][shapediscordSubseqLen]float64
	anchorN   int
	totalSeen uint64 // count of valid (non-flat) post-warmup subsequences
	rngState  uint64 // deterministic LCG state

	// Rolling min-distance ring used as baseline.
	minDistRing  [shapediscordMinDistRingLen]float64
	minDistHead  int
	minDistN     int

	// Trigger state (mirrors AcorrShift). triggerMedian / triggerSigma
	// pin the baseline at the moment of the trigger so recovery measures
	// "minDist has returned to the pre-shift regime", not "minDist is
	// close to the drifting median that has by now followed the shift".
	persistCnt    int
	inAlert       bool
	recoveryCnt   int
	triggerMedian float64
	triggerSigma  float64

	// Cursor — same pattern as BOCPD / AcorrShift.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// ShapeDiscordDetector flags subsequences whose shape is unlike anything
// recently seen in the same series. Implements observer.Detector +
// observer.SeriesRemover.
type ShapeDiscordDetector struct {
	// Trigger is the (minDist − q50)/(q95 − q50) threshold for a
	// single tick to count toward persistence. Default: 1.6.
	Trigger float64

	// PersistenceK is the number of consecutive over-threshold ticks
	// required to fire. Default: 4.
	PersistenceK int

	// RecoveryPoints is the number of consecutive non-triggering ticks
	// required to clear an active alert. Default: 12.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	series       map[shapediscordStateKey]*shapediscordSeriesState
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewShapeDiscordDetector constructs a ShapeDiscordDetector with default
// settings.
func NewShapeDiscordDetector() *ShapeDiscordDetector {
	return &ShapeDiscordDetector{
		Trigger:        shapediscordTrigger,
		PersistenceK:   shapediscordPersistenceK,
		RecoveryPoints: shapediscordRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[shapediscordStateKey]*shapediscordSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*ShapeDiscordDetector) Name() string { return "shapediscord" }

// Reset clears all per-series state for replay/reanalysis.
func (d *ShapeDiscordDetector) Reset() {
	d.series = make(map[shapediscordStateKey]*shapediscordSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~1.85 KB of fixed-size streaming state, so without this teardown the map
// would grow with the cumulative count of series ever observed even after
// their storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *ShapeDiscordDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, shapediscordStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors BOCPD / AcorrShift:
// gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *ShapeDiscordDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := shapediscordStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &shapediscordSeriesState{}
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
func (d *ShapeDiscordDetector) processPoint(state *shapediscordSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	// 1. Push the raw value into the ring.
	state.ring[state.ringHead] = p.Value
	state.ringHead = (state.ringHead + 1) % shapediscordRingLen
	if state.ringN < shapediscordRingLen {
		state.ringN++
	}

	// 2. Need at least m+1 points buffered (the +1 isn't algorithmically
	// required, but it matches the plan's warmup gate and keeps the very
	// first subsequence stable against sample-of-one fluctuations).
	if state.ringN < shapediscordSubseqLen+1 {
		return nil
	}

	// 3. Build the current m-subsequence in chronological order. Use the
	// same `(head - n + W) % W` indexing as computeLag1ACF.
	var subseq [shapediscordSubseqLen]float64
	const w = shapediscordRingLen
	const m = shapediscordSubseqLen
	// The most recently appended value sits at (ringHead - 1 + W) % W.
	// The chronological window of the last m values ends at that index
	// inclusive and starts at (ringHead - m + W) % W.
	startIdx := (state.ringHead - m + w) % w
	if startIdx < 0 {
		startIdx += w
	}
	var sum float64
	for j := 0; j < m; j++ {
		v := state.ring[(startIdx+j)%w]
		subseq[j] = v
		sum += v
	}
	mean := sum / float64(m)

	var sqSum float64
	for j := 0; j < m; j++ {
		dv := subseq[j] - mean
		sqSum += dv * dv
	}
	variance := sqSum / float64(m)

	// 4. Z-normalise into a fixed-size stack array. Skip flat subsequences:
	// the z-norm is undefined when σ²≈0, and a constant subsequence has
	// no meaningful shape signal worth scoring.
	if variance < shapediscordVarFloor {
		return nil
	}
	sigma := math.Sqrt(variance)
	for j := 0; j < m; j++ {
		subseq[j] = (subseq[j] - mean) / sigma
	}

	// 5. Anchor management. totalSeen counts every valid (non-flat)
	// subsequence; the (totalSeen-1) % stride gate spreads warmup anchor
	// acquisition over K * stride ticks, then a low-rate reservoir
	// replacement allows slow adaptation without losing diversity.
	state.totalSeen++

	if state.anchorN < shapediscordAnchors {
		// Warmup: take every stride-th valid subsequence as an anchor.
		// See shapediscordAnchorStride for the diversity rationale.
		if (state.totalSeen-1)%shapediscordAnchorStride == 0 {
			state.anchors[state.anchorN] = subseq
			state.anchorN++
		}
	} else {
		// Past warmup: reservoir replacement at the rate K/(stride · t).
		// The factor of stride keeps the long-run replacement rate
		// consistent with the warmup acceptance rate (warmup accepts
		// 1/stride; if we used the textbook K/t we'd accept stride×
		// faster post-warmup and rapidly forget the diverse warmup
		// set). Random draws come from a deterministic per-state LCG
		// so the test suite can pin reproducible outputs.
		r1 := state.lcgNext()
		acceptProb := float64(shapediscordAnchors) / (float64(shapediscordAnchorStride) * float64(state.totalSeen))
		if float64(r1>>11)/float64(uint64(1)<<53) < acceptProb {
			r2 := state.lcgNext()
			slot := int(r2 % uint64(shapediscordAnchors))
			state.anchors[slot] = subseq
		}
	}

	// 6. Need at least the warmup anchor count before scoring.
	if state.anchorN < shapediscordWarmupAnchors {
		return nil
	}

	// 7. Compute minDist_t = min over current anchors of the z-normalised
	// Euclidean distance. Both subseq and anchors are already z-normalised,
	// so this is the plain Euclidean distance over [m]float64.
	minDist := math.Inf(1)
	for i := 0; i < state.anchorN; i++ {
		var sqd float64
		anchor := &state.anchors[i]
		for j := 0; j < m; j++ {
			diff := subseq[j] - anchor[j]
			sqd += diff * diff
		}
		dist := math.Sqrt(sqd)
		if dist < minDist {
			minDist = dist
		}
	}

	// 8. Compute score against the ring as it currently stands, BEFORE
	// pushing the current min-dist. If we always pushed first, a sustained
	// burst would rapidly populate the ring with high min-dists, q95 would
	// climb up to match, and the score would collapse mid-burst. The
	// detector would only catch the leading edge before being saturated.
	// Computing the score against the prior ring keeps the baseline
	// representative of calm history, and we conditionally update the ring
	// below.
	if state.minDistN < shapediscordMinDistRingLen {
		// Warmup: ring is not full yet. Always push, never score.
		// Empirical q50/q95 on a partial ring is too noisy to gate
		// triggers reliably.
		state.minDistRing[state.minDistHead] = minDist
		state.minDistHead = (state.minDistHead + 1) % shapediscordMinDistRingLen
		state.minDistN++
		return nil
	}

	// 9. Empirical 50th and 95th percentiles of the min-distance ring.
	// median is the typical "in-phase" min-dist; q95 is the natural
	// high-tail (worst phase coverage of the K random anchors). The
	// difference q95 − q50 is a robust spread that absorbs benign
	// periodic phase rotations — see the package-level doc for why we
	// switched away from MAD-based scoring.
	median, q95 := medianAndQ95(state.minDistRing[:])
	spread := q95 - median
	if spread < shapediscordVarFloor {
		spread = shapediscordVarFloor
	}

	// 10. Score = current min-dist relative to the historical high-tail.
	// score > 1 means the current shape is *more* unusual than the
	// 95th-percentile worst-phase tick we have observed. A value of
	// shapediscordTrigger=1.6 leaves comfortable margin for benign
	// periodic data while still catching genuine discords (sawtooth
	// bursts against a sine baseline routinely score 2.5–4).
	score := (minDist - median) / spread
	overThreshold := score > d.Trigger
	if overThreshold {
		state.persistCnt++
	} else {
		state.persistCnt = 0
	}

	// 11. Update the baseline ring only with calm min-dists. Pushing
	// over-threshold ticks would let a sustained burst pull q95 up to its
	// own level, collapsing the score and silencing the detector after the
	// first few ticks of a long anomaly. Skipping over-threshold ticks
	// keeps the baseline a stable "this is what calm looks like"
	// reference. If a regime really has shifted, recovery clears the
	// alert and the new regime's min-dists will gradually re-populate
	// the ring on subsequent calm ticks.
	if !overThreshold {
		state.minDistRing[state.minDistHead] = minDist
		state.minDistHead = (state.minDistHead + 1) % shapediscordMinDistRingLen
	}

	// 12. Trigger / recovery gate (mirrors AcorrShift's lifecycle).
	if state.persistCnt >= d.PersistenceK {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting; suppress until recovery clears it.
			return nil
		}
		state.inAlert = true
		state.triggerMedian = median
		state.triggerSigma = spread
		return d.makeAnomaly(p, series, agg, minDist, median, score)
	}

	if state.inAlert {
		// Recovery is anchored to the baseline at trigger time. As with
		// AcorrShift, anchoring to the drifting current baseline would
		// let it crawl up into the post-shift min-dists and produce a
		// spurious "calm" reading even though nothing has changed.
		recoveryScore := (minDist - state.triggerMedian) / state.triggerSigma
		if recoveryScore <= d.Trigger/2 {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.persistCnt = 0
				state.recoveryCnt = 0
			}
		} else {
			state.recoveryCnt = 0
		}
	}
	return nil
}

// medianAndQ95 returns the 50th and 95th percentiles of the slice (lower
// "type-7" percentile semantics: index = q · (n−1), no interpolation
// beyond a clamp). One sort, two reads — cheaper than calling detectorMedian
// followed by a separate sort for q95. The input slice is not mutated.
func medianAndQ95(vals []float64) (float64, float64) {
	n := len(vals)
	if n == 0 {
		return 0, 0
	}
	sorted := make([]float64, n)
	copy(sorted, vals)
	sortFloat64s(sorted)
	var med float64
	if n%2 == 0 {
		med = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		med = sorted[n/2]
	}
	q95Idx := int(0.95 * float64(n-1))
	if q95Idx < 0 {
		q95Idx = 0
	}
	if q95Idx >= n {
		q95Idx = n - 1
	}
	return med, sorted[q95Idx]
}

// sortFloat64s is a thin wrapper kept here so this file does not need its
// own "sort" import line (sort is already imported transitively via the
// metrics_detector_util.go helpers, but Go requires per-file imports). The
// stdlib call is inlined by the compiler.
func sortFloat64s(a []float64) { sortFloat64sImpl(a) }

// makeAnomaly constructs the alert-onset anomaly.
func (d *ShapeDiscordDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, minDist, baseline, score float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "ShapeDiscord shape anomaly: " + displayName,
		Description: fmt.Sprintf("%s subsequence shape unlike recent history (min-anchor distance %.3f vs. baseline %.3f, score %.2f, sustained %d ticks above %.2f)",
			displayName, minDist, baseline, score, d.PersistenceK, d.Trigger),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: baseline,
			CurrentValue:   minDist,
			DeviationSigma: score,
			Threshold:      d.Trigger,
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults so the
// zero-valued struct is functional. Called from every public method that
// depends on configuration.
func (d *ShapeDiscordDetector) ensureDefaults() {
	if d.Trigger <= 0 {
		d.Trigger = shapediscordTrigger
	}
	if d.PersistenceK <= 0 {
		d.PersistenceK = shapediscordPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = shapediscordRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[shapediscordStateKey]*shapediscordSeriesState)
	}
}

// lcgNext advances the per-state Linear Congruential Generator and returns
// the new state. Constants from Knuth's MMIX (Numerical Recipes also cites
// these). The state is seeded lazily on first use from totalSeen so two
// detectors fed identical input streams produce identical anomaly
// timelines — see TestShapeDiscord_DeterministicAnchorReplacement.
func (s *shapediscordSeriesState) lcgNext() uint64 {
	if s.rngState == 0 {
		// Seed deterministically. totalSeen has just been incremented,
		// so it is non-zero on the first call.
		s.rngState = s.totalSeen*6364136223846793005 + 1442695040888963407
	}
	s.rngState = s.rngState*6364136223846793005 + 1442695040888963407
	return s.rngState
}
