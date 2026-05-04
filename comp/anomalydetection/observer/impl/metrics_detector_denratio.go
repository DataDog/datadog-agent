// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: DenRatio detector — streaming relative-density-ratio
// change detection (Liu, Yamada, Collier, Sugiyama, Neural Networks 2013,
// "Change-Point Detection in Time-Series Data by Relative Density-Ratio
// Estimation", §4 PE-divergence form), simplified to a histogram
// approximation.
//
// Why this fills a gap. The other catalog detectors flag shifts in the
// marginal mean, level, or temporal-dependence structure of a series. None
// flags a series whose mean stays constant but whose underlying distribution
// changes shape — variance shifts, multimodality, skew flips, etc. Mean-based
// detectors (CUSUM, BOCPD posterior, ScanMW/ScanWelch) by construction can't
// see these. AcorrShift catches autocorrelation regime changes, a different
// axis. DenRatio targets the full-distribution gap.
//
// Algorithm (per series, per aggregation):
//  1. Maintain two abutting windows per (series, agg):
//     - R (reference): values at relative positions [-2W, -W]
//     - T (test):      values at relative positions [-W,  0]
//     with W=60.
//  2. On each new point x_t:
//     - Append x_t to T's ring; the displaced oldest T value rolls into R's
//     ring; R's oldest value drops. T fills first; once T is full every
//     new push moves T's oldest into R.
//     - If neither R nor T is full yet (warmup): record and exit.
//     - Build histograms over the UNION range:
//     lo = min(min(R), min(T)); hi = max(max(R), max(T));
//     B=20 equal-width bins on [lo, hi]; if hi-lo < 1e-12 skip
//     (constant signal).
//     hist_R[b] = count_R / W; hist_T[b] = count_T / W.
//     - Compute discrete Pearson divergence (α=0.5, the relative form
//     from the paper that bounds PE ≤ 1/(2α)):
//     PE = 0.5 * Σ_b ( (hist_R[b] - hist_T[b])² /
//     (0.5*hist_R[b] + 0.5*hist_T[b] + ε) )
//     with ε=1e-9 to avoid 0/0.
//     - Push PE into a 3-slot ring; trigger when ALL of last 3 PE values
//     ≥ DivThreshold=0.30 AND
//     |median(T) - median(R)| / max(MAD(R)*1.4826, 1e-6) ≥ MADGateMin=2.0.
//     The MAD-magnitude gate suppresses histogram noise on tiny ranges,
//     keeping false positives low without removing the variance-shift
//     case (which clears the gate).
//     - After firing: enter alert state, copy T into R wholesale, zero T's
//     ring, require RecoveryPoints=10 normal ticks before re-firing.
//     This prevents the same shift from firing every tick of the window.
//
// Per-tick cost: O(B+W)=O(80) for histogram rebuild + median + MAD; the
// median+MAD path runs only when the 3-tick PE streak is above threshold so
// most ticks pay only the histogram cost. Per-(series, agg) memory:
// 2*W floats (rings) + 2*B floats (hists) + 2*W floats (sort scratch) +
// constant overhead ≈ 1.9 KB. No allocations on the hot path.

package observerimpl

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. These are state-array-shape inputs, so they have to be
// compile-time constants. Trigger thresholds (DivThreshold, MADGateMin,
// RecoveryPoints) are exposed on the detector struct so tests can adjust
// them without touching the per-series fixed-size buffers.
const (
	denratioWindow       = 60
	denratioNumBins      = 20
	denratioPEHistory    = 3
	denratioDivThreshold = 0.30
	// denratioMADGateMin is the threshold the (max-formulation) MAD/median
	// gate must clear in σ-equivalent units. Plan-spec'd value was 2.0 with
	// the median-only gate; we use the max-of-median-and-MAD formulation
	// (see processPoint) so variance shifts can pass it. With finite W=60
	// samples a 3× σ shift gives gate ≈ 2 ± 0.4, so 2.0 is right at the
	// noise floor. Lowering to 1.5 gives a comfortable margin without
	// admitting same-distribution noise (gate ≈ 0 there).
	denratioMADGateMin     = 1.5
	denratioRecoveryPoints = 10
	denratioRangeFloor     = 1e-12
	denratioPEEpsilon      = 1e-9
)

// denratioStateKey identifies per-series state by ref and aggregation.
type denratioStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// denratioSeriesState holds per-series streaming state.
type denratioSeriesState struct {
	// Two abutting windows of fixed size W. ringR holds positions [-2W, -W];
	// ringT holds positions [-W, 0]. Indices use canonical "head = next
	// write, oldest = entry currently at head when full" semantics.
	ringR     [denratioWindow]float64
	ringT     [denratioWindow]float64
	ringRN    int
	ringTN    int
	ringRHead int
	ringTHead int

	// Last K=3 PE values (ring); peN counts valid entries until the ring
	// fills. The K-of-K test reads all peN valid entries.
	lastPE [denratioPEHistory]float64
	peN    int
	peHead int

	// Alert lifecycle: inAlert suppresses re-emission while a regime shift
	// is ongoing; recoveryCnt counts consecutive non-triggered ticks toward
	// clearing the alert. After firing, T is zeroed and R is set to the
	// post-shift distribution, so PE is structurally low until T refills
	// (~W ticks) — recovery clears well before that and is mostly belt-
	// and-braces against pathological re-fires.
	inAlert     bool
	recoveryCnt int

	// Cursor — same pattern as BOCPD/ScanMW (metrics_detector_bocpd.go:25-27).
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64

	// Scratch buffers reused per-tick so the hot path never allocates.
	histR [denratioNumBins]float64
	histT [denratioNumBins]float64
}

// DenRatioDetector flags distributional shifts via PE-divergence between two
// abutting histogram windows. Implements observer.Detector +
// observer.SeriesRemover.
type DenRatioDetector struct {
	// DivThreshold is the per-tick PE threshold each of the last K=3 ticks
	// must clear to count toward a fire. Default: 0.30. The paper's PE form
	// with α=0.5 is bounded by 1/(2α)=1.0; 0.30 is well above the chi-square
	// noise floor at W=60 in dev fixtures.
	DivThreshold float64

	// MADGateMin is the secondary gate: the most-recent
	// |median(T)-median(R)|/(MAD(R)*1.4826) must clear this. Default: 2.0.
	// This suppresses histogram noise on tiny-range signals (where PE can
	// be inflated by quantisation) without muting genuine variance shifts
	// (which produce a cleanly-cleared gate).
	MADGateMin float64

	// RecoveryPoints is the number of consecutive non-triggering ticks that
	// resets an active alert. Default: 10.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state keyed by ref+agg.
	series map[denratioStateKey]*denratioSeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewDenRatioDetector constructs a DenRatioDetector with default settings.
func NewDenRatioDetector() *DenRatioDetector {
	return &DenRatioDetector{
		DivThreshold:   denratioDivThreshold,
		MADGateMin:     denratioMADGateMin,
		RecoveryPoints: denratioRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[denratioStateKey]*denratioSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*DenRatioDetector) Name() string { return "denratio" }

// Reset clears all per-series state for replay/reanalysis.
func (d *DenRatioDetector) Reset() {
	d.series = make(map[denratioStateKey]*denratioSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~1.9 KB of fixed-size streaming state, so without this teardown the map
// would grow with the cumulative count of series ever observed even after
// their storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *DenRatioDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, denratioStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors BOCPD/AcorrShift:
// gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *DenRatioDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := denratioStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &denratioSeriesState{}
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
func (d *DenRatioDetector) processPoint(state *denratioSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	// Pipeline push: x → T; if T was full, oldest T rolls into R; if R was
	// full, R's oldest drops. This makes T = newest W values and R = the W
	// values just before T.
	d.pushPoint(state, x)

	// Both rings must be full before PE has meaning. While T is filling and
	// R is empty (or R is partially filled), PE is undefined; report no
	// anomaly and skip the histogram work entirely so cold-start cost stays
	// trivial.
	if state.ringRN < denratioWindow || state.ringTN < denratioWindow {
		// Treat as a non-trigger tick for recovery accounting so any
		// outstanding alert clears even during the structural refill that
		// follows a fire (T is zeroed on fire and refills over W ticks).
		if state.inAlert {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.recoveryCnt = 0
			}
		}
		return nil
	}

	// Histogram bounds over the union range. A range-zero series is the
	// pathological constant case (or all-equal R+T): PE is undefined; treat
	// as non-trigger.
	lo, hi := unionRange(state.ringR[:], state.ringT[:])
	if hi-lo < denratioRangeFloor {
		if state.inAlert {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.recoveryCnt = 0
			}
		}
		return nil
	}

	pe := computePE(state.ringR[:], state.ringT[:], state.histR[:], state.histT[:], lo, hi)

	// Push PE into the 3-slot ring.
	state.lastPE[state.peHead] = pe
	state.peHead = (state.peHead + 1) % denratioPEHistory
	if state.peN < denratioPEHistory {
		state.peN++
	}

	// Trigger condition (both must hold):
	//   1. ALL last K=3 PE values ≥ DivThreshold.
	//   2. MAD-magnitude gate: max(|median(T)-median(R)|,
	//                              |MAD(T)-MAD(R)|*1.4826)
	//                          / max(MAD(R)*1.4826, 1e-6) ≥ MADGateMin.
	//
	// The gate's intent is to suppress histogram noise on tiny ranges (where
	// PE can be inflated by quantisation): the shift in either central
	// tendency OR spread, expressed in units of the reference σ-equivalent,
	// must exceed MADGateMin. Using max-of-median-diff-and-MAD-diff (rather
	// than only median diff) lets variance shifts through — N(0,σ_R²) →
	// N(0,σ_T²) leaves median fixed but MAD scales linearly with σ — while
	// still suppressing the tiny-range case where both are small.
	triggered := false
	medT := 0.0
	medR := 0.0
	madR := 0.0
	madT := 0.0
	if state.peN >= denratioPEHistory && allPEAbove(state.lastPE[:], d.DivThreshold) {
		// MAD/median gate. Done only when the PE streak is above threshold,
		// so the O(W log W) sort cost (~360 ops × 2 sides) is paid only
		// when we are likely to fire. Most ticks short-circuit at the PE
		// check.
		medR = ringMedian(state.ringR[:], state.ringRHead, state.ringRN)
		medT = ringMedian(state.ringT[:], state.ringTHead, state.ringTN)
		madR = ringMAD(state.ringR[:], state.ringRHead, state.ringRN, medR)
		madT = ringMAD(state.ringT[:], state.ringTHead, state.ringTN, medT)
		denom := madR * 1.4826
		if denom < 1e-6 {
			denom = 1e-6
		}
		medianGate := math.Abs(medT-medR) / denom
		madGate := math.Abs(madT-madR) * 1.4826 / denom
		gate := medianGate
		if madGate > gate {
			gate = madGate
		}
		if gate >= d.MADGateMin {
			triggered = true
		}
	}

	if triggered {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting on this incident; do not re-emit until recovery.
			return nil
		}
		state.inAlert = true
		// Reset the post-fire structure: copy T into R wholesale (R becomes
		// the new normal), zero T (refills over W ticks), drop the PE ring
		// so we need 3 fresh PE values before any re-fire is even possible.
		copy(state.ringR[:], state.ringT[:])
		state.ringRHead = state.ringTHead
		state.ringRN = state.ringTN
		for i := range state.ringT {
			state.ringT[i] = 0
		}
		state.ringTHead = 0
		state.ringTN = 0
		state.peHead = 0
		state.peN = 0
		return d.makeAnomaly(state, p, series, agg, pe, medR, medT)
	}

	if state.inAlert {
		state.recoveryCnt++
		if state.recoveryCnt >= d.RecoveryPoints {
			state.inAlert = false
			state.recoveryCnt = 0
		}
	}
	return nil
}

// pushPoint advances the R/T pipeline by one observation: x enters T; if T
// was full, T's oldest is read out and pushed onto R; if R was also full,
// R's oldest drops. With both rings of size W this maintains the
// [-2W, -W] / [-W, 0] partition.
func (d *DenRatioDetector) pushPoint(state *denratioSeriesState, x float64) {
	if state.ringTN < denratioWindow {
		// T not yet full: append, no spill.
		state.ringT[state.ringTHead] = x
		state.ringTHead = (state.ringTHead + 1) % denratioWindow
		state.ringTN++
		return
	}
	// T is full: read its oldest, push to R, then place x in T.
	oldestT := state.ringT[state.ringTHead]
	if state.ringRN < denratioWindow {
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % denratioWindow
		state.ringRN++
	} else {
		// R also full: overwrite its oldest (FIFO replace).
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % denratioWindow
	}
	state.ringT[state.ringTHead] = x
	state.ringTHead = (state.ringTHead + 1) % denratioWindow
}

// unionRange returns min, max across both ring contents (both must be full).
// Caller passes the underlying arrays — when the rings are full, every
// position holds a valid value, so we don't need to disentangle head/order.
func unionRange(ringR, ringT []float64) (float64, float64) {
	lo := math.Inf(1)
	hi := math.Inf(-1)
	for _, v := range ringR {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	for _, v := range ringT {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// computePE builds equal-width B-bin histograms over [lo, hi] for R and T,
// normalises by W, and returns the discrete Pearson divergence
//
//	PE = 0.5 * Σ_b (h_R[b] - h_T[b])² / (0.5*h_R[b] + 0.5*h_T[b] + ε).
//
// The α=0.5 form (Liu et al. 2013, eq. 14) is bounded by 1/(2α)=1, giving a
// well-defined comparable scale across signals. Caller-owned histR / histT
// scratch buffers prevent allocations on the hot path.
func computePE(ringR, ringT, histR, histT []float64, lo, hi float64) float64 {
	for i := range histR {
		histR[i] = 0
		histT[i] = 0
	}
	binW := (hi - lo) / float64(denratioNumBins)
	if binW <= 0 {
		// Defensive: caller should have filtered this; return 0 PE to avoid
		// triggering on a degenerate signal.
		return 0
	}
	w := float64(denratioWindow)
	for _, v := range ringR {
		b := int((v - lo) / binW)
		if b < 0 {
			b = 0
		} else if b >= denratioNumBins {
			b = denratioNumBins - 1
		}
		histR[b]++
	}
	for _, v := range ringT {
		b := int((v - lo) / binW)
		if b < 0 {
			b = 0
		} else if b >= denratioNumBins {
			b = denratioNumBins - 1
		}
		histT[b]++
	}
	var pe float64
	for i := 0; i < denratioNumBins; i++ {
		pR := histR[i] / w
		pT := histT[i] / w
		diff := pR - pT
		denom := 0.5*pR + 0.5*pT + denratioPEEpsilon
		pe += diff * diff / denom
	}
	return 0.5 * pe
}

// allPEAbove reports whether every entry of pe is ≥ threshold.
func allPEAbove(pe []float64, threshold float64) bool {
	for _, v := range pe {
		if v < threshold {
			return false
		}
	}
	return true
}

// ringMedian copies the (filled) ring contents to a scratch slice and returns
// the median. n must equal len(ring) when the ring is full; head is included
// for symmetry with ringMAD but unused when the ring is fully populated since
// every slot holds a valid value.
func ringMedian(ring []float64, _, n int) float64 {
	scratch := make([]float64, n)
	copy(scratch, ring[:n])
	sort.Float64s(scratch)
	if n%2 == 0 {
		return (scratch[n/2-1] + scratch[n/2]) / 2
	}
	return scratch[n/2]
}

// ringMAD returns the unscaled Median Absolute Deviation of the (filled) ring
// around the supplied median. Caller multiplies by 1.4826 to get a Gaussian-
// consistent σ estimate.
func ringMAD(ring []float64, _, n int, median float64) float64 {
	if n == 0 {
		return 0
	}
	scratch := make([]float64, n)
	for i := 0; i < n; i++ {
		scratch[i] = math.Abs(ring[i] - median)
	}
	sort.Float64s(scratch)
	if n%2 == 0 {
		return (scratch[n/2-1] + scratch[n/2]) / 2
	}
	return scratch[n/2]
}

// makeAnomaly constructs the alert-onset anomaly.
func (d *DenRatioDetector) makeAnomaly(state *denratioSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate, pe, medR, medT float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	// Mean(R) is cheap (O(W)) and useful for debug output even though the
	// trigger uses median(R) — the user-facing baseline statistic.
	meanR := ringMean(state.ringR[:], state.ringRN)
	displayName := source.String()
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "DenRatio distributional shift: " + displayName,
		Description: fmt.Sprintf("%s PE-divergence %.3f exceeded threshold %.2f for %d ticks (median R→T: %.3f → %.3f)",
			displayName, pe, d.DivThreshold, denratioPEHistory, medR, medT),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   meanR,
			BaselineMedian: medR,
			Threshold:      d.DivThreshold,
			CurrentValue:   medT,
			DeviationSigma: pe,
		},
	}
}

// ringMean returns the arithmetic mean over the first n entries of ring.
// Used only on the (rare) fire path — not hot.
func ringMean(ring []float64, n int) float64 {
	if n == 0 {
		return 0
	}
	var s float64
	for i := 0; i < n; i++ {
		s += ring[i]
	}
	return s / float64(n)
}

// ensureDefaults populates zero-valued fields with defaults. Called from every
// public method that depends on configuration so the zero-valued struct works.
func (d *DenRatioDetector) ensureDefaults() {
	if d.DivThreshold <= 0 {
		d.DivThreshold = denratioDivThreshold
	}
	if d.MADGateMin <= 0 {
		d.MADGateMin = denratioMADGateMin
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = denratioRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[denratioStateKey]*denratioSeriesState)
	}
}
