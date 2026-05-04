// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: HWRes detector — streaming Holt-Winters additive
// level+seasonal decomposition (Holt 1957, Winters 1960; streaming form
// following Hyndman & Athanasopoulos "Forecasting: Principles and Practice"
// §7.5) with a robust z-score on the residual.
//
// For each (series, aggregation) we maintain a level estimate and a length-L
// seasonal cycle updated by exponential smoothing with rates α and γ. The
// per-tick residual is
//
//	residual_t = x_t − level_t − seasonal[t mod L]
//
// and we fire when |residual_t|/MAD_t exceeds a threshold for K consecutive
// same-sign ticks AND a seasonal-strength gate (varSeasonal/varTotal > 0.20)
// confirms the series actually has seasonal structure to subtract. The gate
// is the explicit additivity argument: on non-seasonal series HWRes is
// silent and trendresid/pht/scanmw are free to fire instead; on strongly
// seasonal series HWRes can flag a step-shift in the level even though
// ScanMW/BOCPD would otherwise mistake the seasonal peaks/troughs for
// mean-shift events.
//
// MAD is approximated by tracking the q=0.84 quantile of |residual| with a
// streaming P² estimator (the same one used by AcorrShift) and dividing by
// 1.4826 (the standard MAD-to-σ rescaling, used here as a cheap σ proxy).
//
// Initialization deviation from the original plan: the plan called for
// `level = first_value, seasonal = zeros` at totalSeen==0. With γ=0.05 and
// the planned 3-cycle warmup (180 ticks), the seasonal cycle would have
// absorbed only ~14% of its equilibrium amplitude before the first scoring
// tick — the model would be diagnosing residuals it had not yet learned to
// predict. We instead use the standard Holt-Winters bootstrap: collect the
// first L values, then seed `level = mean` and `seasonal[i] = x_i − mean`.
// Subsequent recurrence updates are exactly as the plan specifies.
//
// Per-tick cost: O(1) for level/seasonal updates, P² update, Welford on raw
// values, and persistence-ring push. Strength is recomputed every
// hwresStrengthRecomputeEvery=30 ticks at O(L)=60, amortised to ~2 fp ops
// per tick. Per-(series, agg) memory: L floats for the seasonal cycle
// (≈480 B) + p2Quantile (~120 B) + K=4 lastZ ring (32 B) + scalars ≈ ~700 B.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. Period and persistence-K are state-array-shape inputs
// and so must be compile-time constants. Trigger thresholds are exposed on
// the detector struct so tests can adjust them without touching the per-
// series fixed-size buffers.
const (
	hwresPeriod                 = 60
	hwresAlpha                  = 0.05
	hwresGamma                  = 0.05
	hwresWarmupCycles           = 3
	hwresPersistenceK           = 4
	hwresRecoveryPoints         = 12
	hwresZThreshold             = 4.5
	hwresMADP2Q                 = 0.84
	hwresSeasStrengthGate       = 0.20
	hwresMADFloor               = 1e-9
	hwresStrengthRecomputeEvery = 30

	// hwresMADScale converts the streaming P² q0.84 of |residual| into an
	// approximate σ. The standard MAD-to-σ rescaling is 1.4826; using it
	// against q0.84 trades a small known bias for the cheaper P² statistic
	// (no median tracker required; both quantiles approach σ for normal
	// residuals to within tens of a percent).
	hwresMADScale = 1.4826
)

// hwresStateKey identifies per-series state by ref and aggregation.
type hwresStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// hwresSeriesState holds per-series streaming state. Fixed-size; no hot-path
// allocations.
type hwresSeriesState struct {
	// Holt-Winters additive level/seasonal state.
	level    float64
	seasonal [hwresPeriod]float64
	phase    int

	// Smart bootstrap: first L observations are stored to seed seasonal[i]
	// = x_i − mean(first cycle). With γ=0.05 a cold seasonal=zeros start
	// would need ~30 cycles to converge — far longer than the 3-cycle
	// warmup gate downstream — so all post-warmup residuals would reflect
	// the unlearned seasonal rather than genuine surprise. Bootstrap is
	// the standard HW initialization; recurrence updates afterward are
	// unchanged.
	bootstrap     [hwresPeriod]float64
	bootstrapDone bool

	// Welford on raw values: provides varTotal for the seasonal-strength
	// gate. Biased estimator (M2/N) per the plan.
	rawMean float64
	rawM2   float64
	rawN    int

	// P² q0.84 of |residual| — streaming MAD proxy used to z-score the
	// current residual without storing observations.
	absResidQ p2Quantile

	// Cached seasonal-strength: O(L) recompute is amortised across
	// hwresStrengthRecomputeEvery ticks. ticksSinceStrength is set to the
	// recompute-every bound at bootstrap completion so the first post-
	// warmup tick computes it deterministically.
	cachedSeasStrength float64
	ticksSinceStrength int

	// Persistence ring of the last K signed z-scores. The trigger gate
	// requires ALL K to clear the threshold AND share a sign, mirroring
	// varshift's persistentLogRatio shape. After a fire the ring is zeroed
	// so K fresh values are required before any re-fire is possible.
	lastZ     [hwresPersistenceK]float64
	lastZHead int
	lastZN    int

	// Trigger lifecycle (mirrors varshift line 296-303). totalSeen counts
	// every observation seen, including bootstrap; the warmup gate uses
	// hwresWarmupCycles*hwresPeriod against it.
	inAlert     bool
	recoveryCnt int
	totalSeen   int

	// Cursor — same pattern as varshift/BOCPD/ScanMW.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// HWResDetector flags level/step shifts on strongly seasonal series via a
// streaming Holt-Winters additive decomposition. It deliberately stays
// silent on series without seasonal structure (gated by the
// varSeasonal/varTotal strength ratio) so non-seasonal detectors —
// trendresid/pht/scanmw — can fire instead. Implements observer.Detector +
// observer.SeriesRemover.
type HWResDetector struct {
	// ZThreshold is the |residual|/MAD magnitude every entry of the K-tick
	// z ring must clear to count toward a fire. Default: 4.5.
	ZThreshold float64

	// SeasStrengthGate is the additivity gate: varSeasonal/varTotal at the
	// trigger tick must be at or above this. Default: 0.20. Without this
	// gate, the detector would fire on non-seasonal series where varshift
	// /trendresid/scanmw already cover the same incidents.
	SeasStrengthGate float64

	// PersistenceK is the number of consecutive z values that must clear
	// ZThreshold (and share a sign) before a fire. Default: 4. Fixed by
	// the per-series ring size; setting >4 is silently capped in
	// ensureDefaults.
	PersistenceK int

	// RecoveryPoints is the number of consecutive non-triggering ticks
	// that resets an active alert. Default: 12.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state keyed by ref+agg.
	series map[hwresStateKey]*hwresSeriesState

	// Cached series list across Detect calls. Refresh when storage reports
	// that the series set has changed.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewHWResDetector constructs an HWResDetector with default settings.
func NewHWResDetector() *HWResDetector {
	return &HWResDetector{
		ZThreshold:       hwresZThreshold,
		SeasStrengthGate: hwresSeasStrengthGate,
		PersistenceK:     hwresPersistenceK,
		RecoveryPoints:   hwresRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[hwresStateKey]*hwresSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*HWResDetector) Name() string { return "hwres" }

// Reset clears all per-series state for replay/reanalysis.
func (d *HWResDetector) Reset() {
	d.series = make(map[hwresStateKey]*hwresSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~700 B of fixed-size streaming state, so without this teardown the map
// would grow with the cumulative count of series ever observed even after
// their storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *HWResDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, hwresStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors varshift/BOCPD/
// AcorrShift: gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with
// a count+gen cursor → callback applies processPoint to each new visible
// point.
func (d *HWResDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := hwresStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &hwresSeriesState{absResidQ: newP2Quantile(hwresMADP2Q)}
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
func (d *HWResDetector) processPoint(state *hwresSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	// (a) Welford on raw value — feeds varTotal for the strength gate.
	state.rawN++
	rdelta := x - state.rawMean
	state.rawMean += rdelta / float64(state.rawN)
	rdelta2 := x - state.rawMean
	state.rawM2 += rdelta * rdelta2

	// (b) Bootstrap phase: collect the first hwresPeriod observations,
	// then seed level + seasonal from the detrended cycle. Recurrence
	// updates begin only after this completes.
	if !state.bootstrapDone {
		state.bootstrap[state.totalSeen] = x
		state.totalSeen++
		if state.totalSeen == hwresPeriod {
			var sum float64
			for _, v := range state.bootstrap {
				sum += v
			}
			mean := sum / float64(hwresPeriod)
			state.level = mean
			for i := 0; i < hwresPeriod; i++ {
				state.seasonal[i] = state.bootstrap[i] - mean
			}
			state.phase = 0
			state.bootstrapDone = true
			// Force the first post-warmup strength compute: bumping the
			// counter to the bound makes the next post-warmup tick run
			// the O(L) scan exactly once.
			state.ticksSinceStrength = hwresStrengthRecomputeEvery
		}
		return nil
	}

	// (c)-(f) HW recurrence: predict, residual, level update, seasonal
	// update, advance phase. The level update intentionally uses the
	// pre-update seasonal[phase] (the value used for prediction); the
	// seasonal update uses the just-updated newLevel. This is the
	// standard additive Holt-Winters form.
	predicted := state.level + state.seasonal[state.phase]
	residual := x - predicted

	newLevel := hwresAlpha*(x-state.seasonal[state.phase]) + (1-hwresAlpha)*state.level
	state.seasonal[state.phase] = hwresGamma*(x-newLevel) + (1-hwresGamma)*state.seasonal[state.phase]
	state.level = newLevel
	state.phase = (state.phase + 1) % hwresPeriod

	// (g) P² update on |residual|.
	state.absResidQ.add(math.Abs(residual))

	state.totalSeen++

	// (h) Pre-warmup gate: keep updating internals but do not score / fire.
	// Decay any latent alert (mirrors varshift line 296-303) so a stale
	// alert clears even during the structural refill window.
	if state.totalSeen < hwresWarmupCycles*hwresPeriod {
		if state.inAlert {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.recoveryCnt = 0
			}
		}
		return nil
	}

	// (i) MAD via P² q0.84. P² needs five observations to initialise; in
	// practice it's always ready by the time we reach warmup-end (we have
	// hwresPeriod*(hwresWarmupCycles-1) post-bootstrap residual updates),
	// but the not-ready branch is left explicit for safety.
	q, ok := state.absResidQ.value()
	if !ok {
		return nil
	}
	mad := q / hwresMADScale
	if mad < hwresMADFloor {
		mad = hwresMADFloor
	}
	z := residual / mad

	// Push z into the persistence ring.
	state.lastZ[state.lastZHead] = z
	state.lastZHead = (state.lastZHead + 1) % hwresPersistenceK
	if state.lastZN < hwresPersistenceK {
		state.lastZN++
	}

	// (j) Seasonal-strength: amortised O(L)/recompute-every. Recomputed
	// once per half-period to bound hot-path cost.
	state.ticksSinceStrength++
	if state.ticksSinceStrength >= hwresStrengthRecomputeEvery {
		state.cachedSeasStrength = computeSeasonalStrength(state.seasonal[:], state.rawM2, state.rawN)
		state.ticksSinceStrength = 0
	}

	// (k) Trigger gate. Cap PersistenceK at the ring size so a too-large
	// configured value can't read past the fixed buffer.
	persistK := d.PersistenceK
	if persistK > hwresPersistenceK {
		persistK = hwresPersistenceK
	}

	triggered := false
	if state.lastZN >= persistK &&
		persistentLogRatio(state.lastZ[:], d.ZThreshold) &&
		state.cachedSeasStrength >= d.SeasStrengthGate {
		triggered = true
	}

	// (l) Fire-onset path.
	if triggered {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting on this incident; do not re-emit until recovery.
			return nil
		}
		state.inAlert = true
		// Drop the persistence ring so K fresh same-sign over-threshold z's
		// are required before any re-fire is possible.
		for i := range state.lastZ {
			state.lastZ[i] = 0
		}
		state.lastZHead = 0
		state.lastZN = 0
		return d.makeAnomaly(p, series, agg, z, mad, state.level, state.cachedSeasStrength)
	}

	// (m) Recovery accounting (varshift line 371-377 pattern).
	if state.inAlert {
		state.recoveryCnt++
		if state.recoveryCnt >= d.RecoveryPoints {
			state.inAlert = false
			state.recoveryCnt = 0
		}
	}
	return nil
}

// computeSeasonalStrength returns varSeasonal/varTotal where varSeasonal is
// the variance of the L seasonal indices and varTotal is the running raw-
// value variance. Floored against hwresMADFloor in the denominator to avoid
// division by zero on perfectly flat series.
func computeSeasonalStrength(seasonal []float64, rawM2 float64, rawN int) float64 {
	var sum float64
	for _, s := range seasonal {
		sum += s
	}
	mean := sum / float64(len(seasonal))
	var varSum float64
	for _, s := range seasonal {
		dv := s - mean
		varSum += dv * dv
	}
	varSeas := varSum / float64(len(seasonal))
	denom := rawN
	if denom < 1 {
		denom = 1
	}
	varTotal := rawM2 / float64(denom)
	if varTotal < hwresMADFloor {
		varTotal = hwresMADFloor
	}
	return varSeas / varTotal
}

// makeAnomaly constructs the alert-onset anomaly. Allocates only on the
// (rare) fire path.
func (d *HWResDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, z, mad, level, strength float64) *observer.Anomaly {
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
		Title:        "Holt-Winters seasonal-residual: " + displayName,
		Description: fmt.Sprintf("%s residual z=%.2f exceeded threshold %.2f for %d ticks (MAD=%.4f, seasonal strength=%.2f)",
			displayName, z, d.ZThreshold, d.PersistenceK, mad, strength),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   level,
			BaselineStddev: mad,
			CurrentValue:   p.Value,
			Threshold:      d.ZThreshold,
			DeviationSigma: math.Abs(z),
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults. Called from
// every public method that depends on configuration so the zero-valued
// struct works.
func (d *HWResDetector) ensureDefaults() {
	if d.ZThreshold <= 0 {
		d.ZThreshold = hwresZThreshold
	}
	if d.SeasStrengthGate <= 0 {
		d.SeasStrengthGate = hwresSeasStrengthGate
	}
	if d.PersistenceK <= 0 || d.PersistenceK > hwresPersistenceK {
		// Capped at the per-series ring size — larger values would index
		// past the fixed-size lastZ array.
		d.PersistenceK = hwresPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = hwresRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[hwresStateKey]*hwresSeriesState)
	}
}
