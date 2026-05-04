// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: PHT detector — streaming Page-Hinkley change-point
// test (Page, Biometrika 1954; Hinkley, Biometrika 1971), in the streaming
// concept-drift adaptation popularised by Gama et al. 2014 ("A survey on
// concept drift adaptation", §3.4 Sequential analysis).
//
// Why this fills a gap. The other catalog detectors target the marginal mean
// (CUSUM, BOCPD, ScanMW/ScanWelch), the temporal-dependence structure
// (AcorrShift), or the full distribution shape (DenRatio). Each is tuned to
// strong, localised events. None reliably picks up *slow, sustained, low
// amplitude* mean drift relative to an adaptive baseline. CUSUM uses a fixed
// reference mean estimated once during warmup; PHT instead tracks the slow
// component itself with an EWMA-style adaptive mean μ̂_t and accumulates
// deviations *relative to that drifting baseline*, firing only when those
// deviations grow faster than the baseline can follow. BOCPD's posterior
// hazard model dampens slow drift below CPThreshold=0.6 with hazard 0.05.
// PHT is structurally different from both and additive in coverage.
//
// Algorithm (per series, per aggregation), Gama et al. 2014 §3.4 notation:
//
//  1. Adaptive baseline. Maintain an EWMA mean μ̂_t = (1−α) μ̂_{t−1} + α x_t
//     with α=0.005 (≈ 200-tick effective window). Initialised on the first
//     observation. Updated *before* the deviation is computed so m_t reads
//     against the freshly-updated baseline.
//
//  2. Streaming scale σ̂_t. A P²-quantile estimator (the same `p2Quantile`
//     used by the AcorrShift detector) tracks the 0.75 quantile of
//     |x_t − μ̂_t|. σ̂_t = q_0.75 / 0.6745 (the asymptotic MAD-to-σ scale
//     under normality). Floored at 1e-9 so constant series can't blow up the
//     trigger ratio.
//
//  3. Cumulative deviation. m_t = max(0, m_{t−1} + (x_t − μ̂_t) − δ_t) with
//     δ_t = 0.005 · σ̂_t — the "minimum amplitude allowed" slack from the
//     paper, rescaled per-series. m_0 = 0. Symmetric mirror m'_t accumulates
//     the negative side: m'_t = max(0, m'_{t−1} − (x_t − μ̂_t) − δ_t).
//
//  4. Recent minima. M_t = min over the last W_min=300 values of m_t (and
//     M'_t for m'). Held in a length-300 ring with a cached argmin, so
//     update is O(1) on most ticks and O(W_min) only when the leaving
//     entry was the current argmin.
//
//  5. Trigger. Fire when (m_t − M_t) > λ · σ̂_t (positive drift) OR
//     (m'_t − M'_t) > λ · σ̂_t (negative drift), where λ=50. To avoid firing
//     on a single spike (which is denratio/scanmw's territory), require the
//     active-side condition to hold for PersistenceK=8 consecutive ticks.
//
//  6. Alert lifecycle. On fire emit one anomaly, set inAlert=true, and freeze
//     the side so each subsequent tick where the active-side gap drops below
//     0.5·λ·σ̂_t increments recoveryCnt. When recoveryCnt reaches
//     RecoveryPoints=20 clear the alert and reset m, m', M, M', and the
//     W_min ring. Without the freeze a sustained shift would re-fire
//     repeatedly as the EWMA crawled forward.
//
//  7. Warmup. Skip emission until the first WarmupPoints=60 points have been
//     consumed so μ̂_t and σ̂_t are stable. Drift mathematics on a barely
//     initialised quantile estimator produced false positives in dev runs.
//
// Per-tick cost: O(1) for μ̂, m, m'; O(1) amortised for the W_min ring (only
// O(W_min) on the rare argmin recompute); O(1) for P². Per-(series, agg)
// memory: 2×W_min floats (rings) + ~120 B P² + scalars ≈ ~5.0 KB. No
// allocations on the hot path.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. The W_min ring is an array literal in the per-series
// state struct, so its size must be a compile-time constant. The trigger
// thresholds (Lambda, PersistenceK, RecoveryPoints, WarmupPoints) are exposed
// on the detector struct so tests can adjust them without touching fixed-size
// buffers.
const (
	phtMinWindow       = 300
	phtPersistenceK    = 8
	phtRecoveryPoints  = 20
	phtWarmupPoints    = 60
	phtLambda          = 50.0
	phtAlpha           = 0.005 // EWMA coefficient for μ̂_t
	phtDeltaCoeff      = 0.005 // δ_t = phtDeltaCoeff · σ̂_t
	phtMADToSigma      = 0.6745
	phtScaleFloor      = 1e-9
	phtScaleQuantile   = 0.75
	phtRecoveryDivisor = 2.0 // recovery threshold = λ·σ̂ / phtRecoveryDivisor
)

// phtSide enumerates the two PHT sides; the symmetric mirror is identical to
// the positive side with the deviation sign flipped.
type phtSide int

const (
	phtSideNone phtSide = iota
	phtSidePositive
	phtSideNegative
)

// phtStateKey identifies per-series state by ref and aggregation.
type phtStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// phtRingMin is a length-W_min ring that tracks min(m_t over the window).
// Cached argmin avoids a full O(W) scan on every tick: it is recomputed only
// when the leaving entry equals the current argmin or when a new value beats
// the cached argmin.
type phtRingMin struct {
	buf       [phtMinWindow]float64
	head      int
	n         int
	argmin    int     // index in buf of the current minimum (valid iff n>0)
	minVal    float64 // cached min value (valid iff n>0)
	hasArgmin bool
}

// push appends v to the ring, evicting the oldest entry when full. Maintains
// the cached argmin in O(1) amortised time.
func (r *phtRingMin) push(v float64) {
	writeIdx := r.head
	leaving := r.buf[writeIdx]
	leavingValid := r.n == phtMinWindow

	r.buf[writeIdx] = v
	r.head = (writeIdx + 1) % phtMinWindow
	if r.n < phtMinWindow {
		r.n++
	}

	if !r.hasArgmin {
		r.argmin = writeIdx
		r.minVal = v
		r.hasArgmin = true
		return
	}

	// New value beats the cached min — easy update.
	if v < r.minVal {
		r.argmin = writeIdx
		r.minVal = v
		// If the leaving slot was also the previous argmin, the new write
		// already overwrote it, so no extra work is needed.
		return
	}

	// Did the eviction kick out the current argmin? If yes, rescan.
	if leavingValid && writeIdx == r.argmin && leaving == r.minVal {
		r.recomputeArgmin()
		return
	}
	// Otherwise the argmin is unaffected.
}

// recomputeArgmin walks the buffer to find the new minimum. Called only when
// the previous argmin was just evicted — an O(W) tail event amortised against
// long stretches of O(1) updates.
func (r *phtRingMin) recomputeArgmin() {
	if r.n == 0 {
		r.hasArgmin = false
		return
	}
	bestIdx := 0
	best := math.Inf(1)
	// Iterate over the n valid slots starting at the oldest.
	oldest := (r.head - r.n + phtMinWindow) % phtMinWindow
	for i := 0; i < r.n; i++ {
		idx := (oldest + i) % phtMinWindow
		if r.buf[idx] < best {
			best = r.buf[idx]
			bestIdx = idx
		}
	}
	r.argmin = bestIdx
	r.minVal = best
	r.hasArgmin = true
}

// minValue returns the current ring minimum. Returns 0 when empty (PHT init
// uses M_0 = 0 by convention, so an empty ring must read as 0 — not +Inf).
func (r *phtRingMin) minValue() float64 {
	if r.n == 0 || !r.hasArgmin {
		return 0
	}
	return r.minVal
}

// reset clears the ring contents and the cached argmin. Used on alert
// recovery to start the W_min window over from the post-shift baseline.
func (r *phtRingMin) reset() {
	r.head = 0
	r.n = 0
	r.argmin = 0
	r.minVal = 0
	r.hasArgmin = false
}

// phtSeriesState holds per-series streaming state.
type phtSeriesState struct {
	// Adaptive mean μ̂_t. Cumulative-mean during warmup, EWMA afterward.
	// Updated at the start of each tick before the deviation is computed.
	mu float64

	// Streaming P² estimator over |x − μ̂|, used to derive σ̂_t = q_0.75/0.6745.
	scaleP2 p2Quantile

	// Two-sided cumulative deviation and its rolling min over W_min ticks.
	mPos, mNeg float64
	ringPos    phtRingMin
	ringNeg    phtRingMin

	// Persistence counters per side.
	persistPos, persistNeg int

	// Warmup counter: number of points consumed.
	pointsSeen int

	// Alert lifecycle. firedSide records which side raised the alert so
	// recovery measures the correct gap. recoveryCnt counts consecutive
	// ticks where the firing-side gap is back below the recovery threshold.
	inAlert     bool
	firedSide   phtSide
	recoveryCnt int

	// Cursor — same pattern as BOCPD/AcorrShift.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// PHTDetector flags slow, sustained mean drift via the streaming Page-Hinkley
// test. Implements observer.Detector + observer.SeriesRemover.
type PHTDetector struct {
	// Lambda is the trigger ratio: fire when (m_t − M_t)/σ̂_t > Lambda on
	// either side. Default: 50.0. Larger values mean fewer/later fires.
	Lambda float64

	// PersistenceK is the number of consecutive ticks the trigger must hold
	// on a single side before firing. Default: 8. Suppresses single-spike
	// false positives that belong to the spike detectors.
	PersistenceK int

	// RecoveryPoints is the number of consecutive ticks the firing-side gap
	// must stay below 0.5·λ·σ̂ before the alert clears and m/M reset.
	// Default: 20.
	RecoveryPoints int

	// WarmupPoints gates emission until the EWMA + P² estimator have seen
	// this many ticks. Default: 60.
	WarmupPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state keyed by ref+agg.
	series map[phtStateKey]*phtSeriesState

	// Cache the discovered series list across Detect calls. Refreshed when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewPHTDetector constructs a PHTDetector with default settings.
func NewPHTDetector() *PHTDetector {
	return &PHTDetector{
		Lambda:         phtLambda,
		PersistenceK:   phtPersistenceK,
		RecoveryPoints: phtRecoveryPoints,
		WarmupPoints:   phtWarmupPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[phtStateKey]*phtSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*PHTDetector) Name() string { return "pht" }

// Reset clears all per-series state for replay/reanalysis.
func (d *PHTDetector) Reset() {
	d.series = make(map[phtStateKey]*phtSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~5 KB of fixed-size streaming state, so without this teardown the map would
// grow with the cumulative count of series ever observed even after their
// storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *PHTDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, phtStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors AcorrShift/DenRatio:
// gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *PHTDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := phtStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &phtSeriesState{scaleP2: newP2Quantile(phtScaleQuantile)}
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

// processPoint applies the streaming PHT update to a single new point.
// Returns a non-nil anomaly only on alert onset (not while still in alert and
// not during warmup).
func (d *PHTDetector) processPoint(state *phtSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value
	state.pointsSeen++

	// μ̂ update. Use a simple cumulative mean during warmup and switch to the
	// α=0.005 EWMA afterward.
	//
	// Why this matters: a one-sample EWMA seed (μ̂_0 = x_0, then α-step toward
	// each new value) leaves μ̂ heavily biased toward x_0 for ~1/α ≈ 200
	// ticks. With PHT's recurrence m_t = max(0, m_{t-1} + (x_t − μ̂_t) − δ),
	// any sustained bias in dev — even bias from a noisy initial sample —
	// gets integrated and crosses λ·σ̂ during the warmup tail. In dev runs
	// against pure N(0,1) input this produced a spurious downward-drift fire
	// inside the first 200 ticks. Cumulative-mean warmup anchors μ̂ to the
	// true mean of the first WarmupPoints observations, so drift detection
	// starts from an unbiased baseline.
	if state.pointsSeen <= d.WarmupPoints {
		state.mu += (x - state.mu) / float64(state.pointsSeen)
	} else {
		state.mu = (1-phtAlpha)*state.mu + phtAlpha*x
	}
	dev := x - state.mu

	// Feed |dev| into the P² σ̂ estimator. Skip the very first sample because
	// its deviation is identically 0 by construction (μ̂_1 = x_1) and pinning
	// the P² markers at 0 distorts subsequent quantile estimates.
	if state.pointsSeen > 1 {
		state.scaleP2.add(math.Abs(dev))
	}
	q75, scaleReady := state.scaleP2.value()
	sigma := q75 / phtMADToSigma
	if sigma < phtScaleFloor {
		sigma = phtScaleFloor
	}

	// During warmup we only stabilise μ̂ and σ̂. Cumulative deviations,
	// rolling minima, and persistence counters stay at their zero values so
	// drift detection effectively starts at point WarmupPoints+1 with a
	// clean slate.
	if !scaleReady || state.pointsSeen < d.WarmupPoints {
		return nil
	}

	delta := phtDeltaCoeff * sigma

	// Cumulative deviations. Both sides are clamped at 0 — that is what makes
	// PHT a one-sided test per side rather than a random walk.
	state.mPos = math.Max(0, state.mPos+dev-delta)
	state.mNeg = math.Max(0, state.mNeg-dev-delta)

	// W_min rolling minima. M_t = min over the last W_min m values.
	state.ringPos.push(state.mPos)
	state.ringNeg.push(state.mNeg)
	mMinPos := state.ringPos.minValue()
	mMinNeg := state.ringNeg.minValue()

	gapPos := state.mPos - mMinPos
	gapNeg := state.mNeg - mMinNeg

	threshold := d.Lambda * sigma
	recoveryThreshold := threshold / phtRecoveryDivisor

	// Persistence counters — count consecutive ticks above threshold per side.
	if gapPos > threshold {
		state.persistPos++
	} else {
		state.persistPos = 0
	}
	if gapNeg > threshold {
		state.persistNeg++
	} else {
		state.persistNeg = 0
	}

	// Recovery before fire decisions: if already in alert, see whether the
	// firing-side gap has dropped under the recovery threshold for long
	// enough to clear.
	if state.inAlert {
		var activeGap float64
		switch state.firedSide {
		case phtSidePositive:
			activeGap = gapPos
		case phtSideNegative:
			activeGap = gapNeg
		}
		if activeGap < recoveryThreshold {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				// Alert resolved: clear all PHT state so the next drift gets
				// a clean slate. m/M reset to 0 and W_min ring drained per
				// the plan.
				state.inAlert = false
				state.firedSide = phtSideNone
				state.recoveryCnt = 0
				state.mPos = 0
				state.mNeg = 0
				state.persistPos = 0
				state.persistNeg = 0
				state.ringPos.reset()
				state.ringNeg.reset()
			}
		} else {
			state.recoveryCnt = 0
		}
		// Suppress new fires while an alert is active. The recovery state
		// machine is the only path out of inAlert.
		return nil
	}

	// Trigger evaluation. If both sides happen to fire on the same tick (rare —
	// would require the signal to be simultaneously deviating positively from
	// the EWMA when accumulated and negatively when accumulated, which can
	// only happen after a side flip), prefer whichever side has the larger
	// gap so the emitted anomaly reflects the dominant drift.
	persistK := d.PersistenceK
	if persistK < 1 {
		persistK = 1
	}
	posTriggered := state.persistPos >= persistK
	negTriggered := state.persistNeg >= persistK
	if !posTriggered && !negTriggered {
		return nil
	}
	side := phtSidePositive
	if negTriggered && (!posTriggered || gapNeg > gapPos) {
		side = phtSideNegative
	}

	state.inAlert = true
	state.firedSide = side
	state.recoveryCnt = 0
	return d.makeAnomaly(state, p, series, agg, side, gapPos, gapNeg, sigma)
}

// makeAnomaly builds the alert-onset anomaly for the firing side.
func (d *PHTDetector) makeAnomaly(state *phtSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate, side phtSide, gapPos, gapNeg, sigma float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()

	gap := gapPos
	dirLabel := "upward"
	if side == phtSideNegative {
		gap = gapNeg
		dirLabel = "downward"
	}

	// Express the gap in σ-equivalent units for display + downstream
	// comparison. sigma is floored at phtScaleFloor so the division is safe.
	deviationSigma := gap / sigma

	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "PHT mean drift: " + displayName,
		Description: fmt.Sprintf("%s page-hinkley %s drift detected: gap %.3f, σ̂ %.3g, μ̂ %.3g, persistence %d ≥ %d, λ=%.0f",
			displayName, dirLabel, gap, sigma, state.mu, persistenceForSide(state, side), d.PersistenceK, d.Lambda),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   state.mu,
			CurrentValue:   p.Value,
			DeviationSigma: deviationSigma,
			Threshold:      d.Lambda,
		},
	}
}

// persistenceForSide returns the persistence counter for the firing side.
// Used only by makeAnomaly so the description reflects the actual streak.
func persistenceForSide(state *phtSeriesState, side phtSide) int {
	if side == phtSideNegative {
		return state.persistNeg
	}
	return state.persistPos
}

// ensureDefaults populates zero-valued fields with defaults so a zero-valued
// struct works.
func (d *PHTDetector) ensureDefaults() {
	if d.Lambda <= 0 {
		d.Lambda = phtLambda
	}
	if d.PersistenceK <= 0 {
		d.PersistenceK = phtPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = phtRecoveryPoints
	}
	if d.WarmupPoints <= 0 {
		d.WarmupPoints = phtWarmupPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[phtStateKey]*phtSeriesState)
	}
}
