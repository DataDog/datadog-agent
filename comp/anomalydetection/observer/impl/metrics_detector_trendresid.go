// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: TrendResid detector — streaming residual-against-trend
// detector for breaks in established linear trends.
//
// Why this fills a gap. The other catalog detectors target the marginal mean
// (BOCPD, ScanMW, ScanWelch, CUSUM), the marginal distribution (DenRatio),
// the lag-1 autocorrelation regime (AcorrShift), the ordinal-pattern
// complexity (PermEntropy), or slow drift onset relative to an EWMA baseline
// (PHT). None of them flags a series that has been steadily ramping up/down
// (counter saturation, capacity drift, leak ramp) and then suddenly *stops
// trending* — every point on a sustained ramp looks anomalous to a stationary
// detector, while PHT catches the start of drift but not the end. TrendResid
// fits a streaming linear regression over a rolling window and flags when
// the residual against that fit becomes large.
//
// Algorithm (per series, per aggregation):
//
//  1. Maintain a ring of the last W=60 (deltaT, value) pairs where deltaT =
//     p.Timestamp − firstTimestamp. Maintain running sums sumX, sumY, sumXX,
//     sumXY in O(1) per tick (additions on append, subtractions on eviction
//     once the ring is full).
//
//  2. Once the ring is full, compute slope and intercept from the running
//     sums via the closed-form OLS formula. Variance is floored at 1e-12 so a
//     constant time-base (degenerate input) cannot divide by zero.
//
//  3. Predict the current point and compute the residual r = y − ŷ. Feed
//     |r| into a P²-quantile estimator at p=0.75. The asymptotic mapping
//     from MAD/IQR to σ for normal residuals is q75/0.6745.
//
//  4. Compute the dimensionless trend strength t = |slope| · √varX / σ_resid.
//     This is essentially the Student t-statistic on the slope: when the
//     trend explains more variance than the residual scale, t > 1; when the
//     series is stationary noise, t collapses to ~0. We require t ≥ 0.5 at
//     the trigger tick — the additivity gate that keeps stationary series
//     out of TrendResid's territory and in ScanMW/BOCPD's.
//
//  5. Trigger when ALL of:
//     (a) |residual| ≥ ResidualK · σ_resid (default K=3.5).
//     (b) trendStrength ≥ TrendGate (default 0.5) at the trigger tick only.
//     (c) Persistence: J=4 consecutive ticks all pass (a). The trendStrength
//     gate need only pass at the trigger tick; slope is slow-varying
//     within W.
//
//  6. Alert lifecycle: emit ONE anomaly on fire and freeze σ_resid as
//     triggerSigma. Recovery clears the alert after RecoveryPoints=12
//     consecutive ticks where |residual| < 1.5 · triggerSigma. Anchoring to
//     the frozen σ_resid (mirroring AcorrShift's triggerBaseline pattern at
//     metrics_detector_acorrshift.go:90-95) prevents re-fires while the
//     slope refits to the post-break regime and pulls σ_resid up.
//
// Per-tick cost: O(1) — running-sum update, slope/intercept divisions, one P²
// add ≈ ~50 ns. No allocations on the hot path. Per-(series, agg) memory:
// 2×60 floats (rings) + p2Quantile (~64 B) + scalars ≈ 1.1 KB.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. Buffer sizes are compile-time constants; trigger
// thresholds (ResidualK, TrendGate, PersistenceK, RecoveryPoints) are exposed
// on the detector struct so tests can adjust them without touching the
// per-series fixed-size buffers.
const (
	trendresidWindow         = 60
	trendresidPersistenceK   = 4
	trendresidRecoveryPoints = 12
	trendresidResidualK      = 3.5
	trendresidTrendGate      = 0.5
	trendresidRecoveryFactor = 1.5
	// q75/0.6745 ≈ σ for normal residuals; constant pulled out to avoid the
	// magic-number-in-formula smell.
	trendresidQuartileToSigma = 0.6745
	trendresidVarFloor        = 1e-12
	trendresidSigmaFloor      = 1e-9
)

// trendresidStateKey identifies per-series state by ref and aggregation.
type trendresidStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// trendresidSeriesState holds per-series streaming state.
type trendresidSeriesState struct {
	// Rolling W=60 buffers of relative time and value. Pairs are evicted
	// together when the ring is full so the running sums stay symmetric.
	ringX [trendresidWindow]float64
	ringY [trendresidWindow]float64
	head  int
	n     int

	// Running OLS sums. Updated O(1) on append; decremented O(1) on eviction
	// once the ring fills.
	sumX, sumY, sumXX, sumXY float64

	// Reference timestamp anchoring relative time. Holding the absolute
	// origin as int64 and the per-point delta as float64 sidesteps the
	// precision loss that would happen if we converted Unix-epoch seconds
	// directly to float64 throughout the ring.
	firstTimestamp int64
	haveFirst      bool

	// P²-quantile tracker for the 0.75-quantile of |residual|. σ_resid is
	// derived from this on each trigger evaluation.
	residP2 p2Quantile

	// Persistence streak counter for condition (a) — |residual| ≥ K·σ.
	streak int

	// Alert lifecycle. triggerSigma freezes the σ at fire time so recovery
	// is anchored to the pre-break scale, not the inflated scale produced
	// while the regression refits to the post-break regime. Same pattern as
	// AcorrShift's triggerBaseline (acorrshift.go lines 90-95).
	inAlert      bool
	recoveryCnt  int
	triggerSigma float64

	// Cursor — same pattern as AcorrShift/PermEntropy.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// TrendResidDetector flags breaks in established linear trends. Implements
// observer.Detector + observer.SeriesRemover.
type TrendResidDetector struct {
	// ResidualK is the number of σ_resid units a residual must exceed for a
	// tick to count toward persistence. Default: 3.5.
	ResidualK float64

	// TrendGate is the minimum |slope|·√varX/σ_resid required at the trigger
	// tick. Below this, the series is considered stationary noise and falls
	// through to ScanMW/BOCPD. Default: 0.5.
	TrendGate float64

	// PersistenceK is the number of consecutive |residual| ≥ K·σ ticks
	// required to fire. Default: 4.
	PersistenceK int

	// RecoveryPoints is the number of consecutive |residual| < 1.5·σ_resid
	// ticks required to clear an active alert. Default: 12.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state.
	series map[trendresidStateKey]*trendresidSeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewTrendResidDetector constructs a TrendResidDetector with default settings.
func NewTrendResidDetector() *TrendResidDetector {
	return &TrendResidDetector{
		ResidualK:      trendresidResidualK,
		TrendGate:      trendresidTrendGate,
		PersistenceK:   trendresidPersistenceK,
		RecoveryPoints: trendresidRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[trendresidStateKey]*trendresidSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*TrendResidDetector) Name() string { return "trendresid" }

// Reset clears all per-series state for replay/reanalysis.
func (d *TrendResidDetector) Reset() {
	d.series = make(map[trendresidStateKey]*trendresidSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~1.1 KB of fixed-size streaming state, so without this teardown the map
// would grow with the cumulative count of series ever observed even after
// their storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *TrendResidDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, trendresidStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors AcorrShift/BOCPD:
// gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *TrendResidDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := trendresidStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &trendresidSeriesState{residP2: newP2Quantile(0.75)}
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

// processPoint applies the streaming algorithm to a single new point. Returns
// a non-nil anomaly only on alert onset (not while still in alert).
func (d *TrendResidDetector) processPoint(state *trendresidSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	// Anchor the time origin on the first point. Keeping deltaT as float64
	// against an int64 reference avoids float precision loss when timestamps
	// are at Unix-epoch magnitudes (~1.7e9 seconds).
	if !state.haveFirst {
		state.firstTimestamp = p.Timestamp
		state.haveFirst = true
	}
	x := float64(p.Timestamp - state.firstTimestamp)
	y := p.Value

	// Eviction first when the ring is full, then append. This keeps the
	// running sums in lockstep with the buffer contents at every step.
	if state.n == trendresidWindow {
		oldX := state.ringX[state.head]
		oldY := state.ringY[state.head]
		state.sumX -= oldX
		state.sumY -= oldY
		state.sumXX -= oldX * oldX
		state.sumXY -= oldX * oldY
	} else {
		state.n++
	}
	state.ringX[state.head] = x
	state.ringY[state.head] = y
	state.sumX += x
	state.sumY += y
	state.sumXX += x * x
	state.sumXY += x * y
	state.head = (state.head + 1) % trendresidWindow

	// Defer scoring until the window is full. With a partially filled window
	// the slope estimate is dominated by the warmup transient and the
	// residual quantile estimator hasn't converged.
	if state.n < trendresidWindow {
		return nil
	}

	W := float64(trendresidWindow)
	meanX := state.sumX / W
	meanY := state.sumY / W
	varX := state.sumXX/W - meanX*meanX
	if varX < trendresidVarFloor {
		varX = trendresidVarFloor
	}
	cov := state.sumXY/W - meanX*meanY
	slope := cov / varX
	icept := meanY - slope*meanX

	predicted := icept + slope*x
	residual := y - predicted
	absResid := math.Abs(residual)

	// Update the |residual| quantile tracker on every full-window tick. P²
	// keeps σ_resid current with the residual scale of the present regime.
	state.residP2.add(absResid)

	q75, ok := state.residP2.value()
	if !ok {
		// P² needs 5 observations to initialize. With W=60 ring fill before
		// the first add, this branch is theoretically unreachable in steady
		// state — it's defensive against any future timing change.
		return nil
	}
	sigmaResid := q75 / trendresidQuartileToSigma
	if sigmaResid < trendresidSigmaFloor {
		sigmaResid = trendresidSigmaFloor
	}

	// Persistence: streak only over condition (a). The trend gate is
	// evaluated at the trigger tick.
	if absResid >= d.ResidualK*sigmaResid {
		state.streak++
	} else {
		state.streak = 0
	}

	// Trigger evaluation: streak meets persistence AND trend gate fires NOW.
	if state.streak >= d.PersistenceK {
		trendStrength := math.Abs(slope) * math.Sqrt(varX) / math.Max(sigmaResid, trendresidSigmaFloor)
		if trendStrength >= d.TrendGate {
			state.recoveryCnt = 0
			if state.inAlert {
				// Already alerting on this regime; do not re-emit until
				// recovery clears the latch.
				return nil
			}
			state.inAlert = true
			state.triggerSigma = sigmaResid
			return d.makeAnomaly(p, series, agg, residual, sigmaResid)
		}
	}

	// Recovery is anchored to the σ frozen at trigger. Anchoring to the
	// drifting current σ would let the post-break regression's inflated
	// residual scale make later residuals look "normal" and fire a duplicate
	// anomaly on the next slope refit.
	if state.inAlert {
		if absResid < trendresidRecoveryFactor*state.triggerSigma {
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
func (d *TrendResidDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, residual, sigmaResid float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()
	zScore := math.Abs(residual) / math.Max(sigmaResid, trendresidSigmaFloor)
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "TrendResid trend-break: " + displayName,
		Description: fmt.Sprintf("%s residual against rolling linear trend reached %.3f (%.2fσ_resid, threshold %.2f, sustained %d ticks)",
			displayName, residual, zScore, d.ResidualK, d.PersistenceK),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineStddev: sigmaResid,
			CurrentValue:   residual,
			DeviationSigma: zScore,
			Threshold:      d.ResidualK,
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults so the
// zero-valued struct works.
func (d *TrendResidDetector) ensureDefaults() {
	if d.ResidualK <= 0 {
		d.ResidualK = trendresidResidualK
	}
	if d.TrendGate <= 0 {
		d.TrendGate = trendresidTrendGate
	}
	if d.PersistenceK <= 0 {
		d.PersistenceK = trendresidPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = trendresidRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[trendresidStateKey]*trendresidSeriesState)
	}
}
