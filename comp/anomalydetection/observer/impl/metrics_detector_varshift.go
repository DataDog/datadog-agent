// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: VarShift detector — streaming F-style log-variance-
// ratio change detection between two abutting windows R (reference, [-2W,-W])
// and T (test, [-W,0]). The construction adapts the ICSS test
// (Inclán & Tiao 1994, "Use of cumulative sums of squares for retrospective
// detection of changes of variance", JASA) to a streaming windowed form so
// each tick is O(1).
//
// Why this fills a gap. ScanMW/ScanWelch/BOCPD all flag mean shifts;
// AcorrShift flags autocorrelation regime shifts; DenRatio targets full-
// distribution shifts via histogram divergence but is noisy at small ranges
// and shares a lot of ground with mean-shift detectors when ranges are
// large. None of these targets pure scale/variance shifts where the mean
// stays put but volatility changes — a class that arises in latency
// distributions, error-rate noise floors, and load-balanced traffic.
//
// CRITICAL design decision: a STRONG mean-stationarity gate
// (|meanT-meanR|/sigmaR < 0.5) prevents firing whenever ScanMW/BOCPD would
// already fire — so this detector is additive, not redundant. Recent
// experiments where similar detectors lacked such a gate caused broad recall
// collapse on scanmw-friendly scenarios. Keeping the gate is the explicit
// fix.
//
// Algorithm (per series, per aggregation):
//  1. Maintain two abutting windows of size W=60 with the same hand-off
//     pattern as DenRatio: x → T; if T was full, T's oldest rolls into R;
//     if R was also full, R's oldest drops.
//  2. Maintain running sum and sum-of-squares for each window, updated
//     O(1) per append/eviction. Variance(W) = sumSq/W - (sum/W)^2,
//     floored at 1e-12 to avoid 0-variance pathologies.
//  3. After both rings are full (warmup), on each new tick compute
//        meanR = sumR/W; meanT = sumT/W
//        varR  = max(sumSqR/W - meanR^2, 1e-12)
//        varT  = max(sumSqT/W - meanT^2, 1e-12)
//        logRatio = log(varT) - log(varR)
//        meanGap  = |meanT - meanR| / sqrt(varR)
//  4. Trigger gates (ALL must pass):
//     (a) |logRatio| ≥ LogRatioThreshold (default 1.6 ≈ 5× variance ratio,
//         well above sampling noise at W=60).
//     (b) meanGap < MeanStationaryGate (default 0.5σ) — additivity gate
//         against ScanMW/BOCPD; suppresses fires whenever mean has also
//         shifted enough for those detectors to catch it.
//     (c) Persistence: ALL last K=3 logRatios above threshold AND with
//         the same sign (so a single-tick spike or sign-flipping noise
//         can't trigger).
//  5. Alert lifecycle (mirrors DenRatio): on fire emit ONE anomaly, set
//     inAlert=true, copy T into R, zero T's running sums, require
//     RecoveryPoints=10 normal ticks before next fire.
//
// Per-tick cost: O(1) for running-sum updates, log/sqrt; <100 ns on modern
// hardware. No allocations on the hot path. Per-(series, agg) memory:
// 2×W floats (rings) + 2×K floats (logRatio history) + scalars ≈ 1.0 KB.
// Roughly 3× cheaper per tick than DenRatio (no histogram rebuild, no
// per-tick MAD).

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. Window size and persistence-K are state-array-shape
// inputs and so must be compile-time constants. Trigger thresholds are
// exposed on the detector struct so tests can adjust them without touching
// the per-series fixed-size buffers.
const (
	varshiftWindow             = 60
	varshiftLogRatioHistory    = 3
	varshiftLogRatioThreshold  = 1.6
	varshiftMeanStationaryGate = 0.5
	varshiftRecoveryPoints     = 10
	// varshiftVarianceFloor avoids log(0) and divide-by-zero in meanGap when
	// the reference window happens to be (nearly) constant. Picked well
	// below any realistic finite-sample variance at W=60.
	varshiftVarianceFloor = 1e-12
)

// varshiftStateKey identifies per-series state by ref and aggregation.
type varshiftStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// varshiftSeriesState holds per-series streaming state. Fixed-size; no hot-
// path allocations.
type varshiftSeriesState struct {
	// Two abutting windows of size W. ringR holds positions [-2W, -W];
	// ringT holds positions [-W, 0]. Same FIFO/head semantics as DenRatio:
	// when full, ringTHead is both "oldest in T" and "next slot to write".
	ringR     [varshiftWindow]float64
	ringT     [varshiftWindow]float64
	ringRHead int
	ringTHead int
	ringRN    int
	ringTN    int

	// Running sums and sum-of-squares — O(1) variance updates per push.
	sumR   float64
	sumSqR float64
	sumT   float64
	sumSqT float64

	// Last K=3 logRatios (ring); logRatioN counts valid entries until full.
	lastLogRatio [varshiftLogRatioHistory]float64
	logRatioHead int
	logRatioN    int

	// Alert lifecycle: inAlert suppresses re-emission while a regime shift
	// is ongoing; recoveryCnt counts consecutive non-triggering ticks toward
	// clearing the alert. After firing, T is zeroed and R is set to the
	// post-shift distribution (sums included), so logRatio is structurally
	// near zero until T refills (~W ticks).
	inAlert     bool
	recoveryCnt int

	// Cursor — same pattern as DenRatio/BOCPD/ScanMW.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// VarShiftDetector flags pure variance/scale-regime shifts via the streaming
// log-variance-ratio between two abutting windows. Implements
// observer.Detector + observer.SeriesRemover.
type VarShiftDetector struct {
	// LogRatioThreshold is the magnitude every entry of the K-tick log-
	// variance-ratio history must clear to count toward a fire. Default:
	// 1.6 (~5× variance ratio, well above N(0,1) sampling noise at W=60).
	LogRatioThreshold float64

	// MeanStationaryGate is the additivity gate: |meanT-meanR|/sqrt(varR)
	// at the trigger tick must be BELOW this. Default: 0.5. Without this
	// gate, any joint mean+variance shift would fire here AND in
	// ScanMW/BOCPD, multiplying false positives on the same incident.
	MeanStationaryGate float64

	// PersistenceK is the number of consecutive log-variance-ratio values
	// that must clear LogRatioThreshold (and share a sign) before a fire.
	// Default: 3. Fixed by the per-series ring size; setting >3 is silently
	// capped at 3 in ensureDefaults.
	PersistenceK int

	// RecoveryPoints is the number of consecutive non-triggering ticks that
	// resets an active alert. Default: 10.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state keyed by ref+agg.
	series map[varshiftStateKey]*varshiftSeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewVarShiftDetector constructs a VarShiftDetector with default settings.
func NewVarShiftDetector() *VarShiftDetector {
	return &VarShiftDetector{
		LogRatioThreshold:  varshiftLogRatioThreshold,
		MeanStationaryGate: varshiftMeanStationaryGate,
		PersistenceK:       varshiftLogRatioHistory,
		RecoveryPoints:     varshiftRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[varshiftStateKey]*varshiftSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*VarShiftDetector) Name() string { return "varshift" }

// Reset clears all per-series state for replay/reanalysis.
func (d *VarShiftDetector) Reset() {
	d.series = make(map[varshiftStateKey]*varshiftSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~1.0 KB of fixed-size streaming state, so without this teardown the map
// would grow with the cumulative count of series ever observed even after
// their storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *VarShiftDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, varshiftStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors DenRatio/BOCPD/
// AcorrShift: gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with
// a count+gen cursor → callback applies processPoint to each new visible
// point.
func (d *VarShiftDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := varshiftStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &varshiftSeriesState{}
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
func (d *VarShiftDetector) processPoint(state *varshiftSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	// Pipeline push: x → T; if T was full, oldest T rolls into R; if R was
	// also full, R's oldest drops. Running sums updated incrementally.
	d.pushPoint(state, x)

	// Both rings must be full before logRatio has meaning.
	if state.ringRN < varshiftWindow || state.ringTN < varshiftWindow {
		// Treat as a non-trigger tick for recovery accounting so any
		// outstanding alert clears even during the structural refill that
		// follows a fire (T is zeroed and refills over W ticks).
		if state.inAlert {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.recoveryCnt = 0
			}
		}
		return nil
	}

	w := float64(varshiftWindow)
	meanR := state.sumR / w
	meanT := state.sumT / w
	varR := state.sumSqR/w - meanR*meanR
	varT := state.sumSqT/w - meanT*meanT
	if varR < varshiftVarianceFloor {
		varR = varshiftVarianceFloor
	}
	if varT < varshiftVarianceFloor {
		varT = varshiftVarianceFloor
	}
	logRatio := math.Log(varT) - math.Log(varR)
	meanGap := math.Abs(meanT-meanR) / math.Sqrt(varR)

	// Push logRatio into the K-slot ring.
	state.lastLogRatio[state.logRatioHead] = logRatio
	state.logRatioHead = (state.logRatioHead + 1) % varshiftLogRatioHistory
	if state.logRatioN < varshiftLogRatioHistory {
		state.logRatioN++
	}

	// Trigger condition (all three must hold):
	//   (a) |logRatio| ≥ LogRatioThreshold for ALL last K entries
	//   (b) ALL last K entries share the same sign (same regime)
	//   (c) meanGap < MeanStationaryGate at the trigger tick
	// (a) and (b) are the persistence/regime check; (c) is the additivity
	// gate against ScanMW/BOCPD. The persistence test is cheap (3-element
	// scan) so we always run it; meanGap is a single fp op so always run
	// too — no short-circuit savings worth the branch complexity.
	triggered := false
	if state.logRatioN >= d.PersistenceK &&
		persistentLogRatio(state.lastLogRatio[:], d.LogRatioThreshold) &&
		meanGap < d.MeanStationaryGate {
		triggered = true
	}

	if triggered {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting on this incident; do not re-emit until recovery.
			return nil
		}
		state.inAlert = true
		// Reset post-fire structure: copy T into R wholesale (R becomes the
		// new normal), zero T (refills over W ticks), drop the logRatio ring
		// so we need K fresh values before any re-fire is even possible.
		// Running sums must be migrated in lock-step with the ring contents
		// to keep variance computations consistent.
		copy(state.ringR[:], state.ringT[:])
		state.ringRHead = state.ringTHead
		state.ringRN = state.ringTN
		state.sumR = state.sumT
		state.sumSqR = state.sumSqT
		for i := range state.ringT {
			state.ringT[i] = 0
		}
		state.ringTHead = 0
		state.ringTN = 0
		state.sumT = 0
		state.sumSqT = 0
		state.logRatioHead = 0
		state.logRatioN = 0
		return d.makeAnomaly(p, series, agg, logRatio, varR, varT)
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
// R's oldest drops. Running sums and sum-of-squares are updated incrementally
// in the same step so variance is always available in O(1).
func (d *VarShiftDetector) pushPoint(state *varshiftSeriesState, x float64) {
	if state.ringTN < varshiftWindow {
		// T not yet full: append, no spill.
		state.ringT[state.ringTHead] = x
		state.ringTHead = (state.ringTHead + 1) % varshiftWindow
		state.ringTN++
		state.sumT += x
		state.sumSqT += x * x
		return
	}
	// T is full: read its oldest (at ringTHead, the next-write slot),
	// migrate to R, then place x in the vacated slot.
	oldestT := state.ringT[state.ringTHead]
	state.sumT -= oldestT
	state.sumSqT -= oldestT * oldestT
	if state.ringRN < varshiftWindow {
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % varshiftWindow
		state.ringRN++
		state.sumR += oldestT
		state.sumSqR += oldestT * oldestT
	} else {
		// R also full: evict its oldest (FIFO replace).
		oldestR := state.ringR[state.ringRHead]
		state.sumR -= oldestR
		state.sumSqR -= oldestR * oldestR
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % varshiftWindow
		state.sumR += oldestT
		state.sumSqR += oldestT * oldestT
	}
	state.ringT[state.ringTHead] = x
	state.ringTHead = (state.ringTHead + 1) % varshiftWindow
	state.sumT += x
	state.sumSqT += x * x
}

// persistentLogRatio reports whether ALL log-ratio entries clear the
// magnitude threshold AND share the same sign. Same-sign matters because a
// regime shift produces a one-sided change — alternating signs are sampling
// noise, not a regime change.
func persistentLogRatio(history []float64, threshold float64) bool {
	if len(history) == 0 {
		return false
	}
	sign := 0
	for _, v := range history {
		if math.Abs(v) < threshold {
			return false
		}
		s := 1
		if v < 0 {
			s = -1
		}
		if sign == 0 {
			sign = s
		} else if sign != s {
			return false
		}
	}
	return true
}

// makeAnomaly constructs the alert-onset anomaly. Allocates only on the
// (rare) fire path.
func (d *VarShiftDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, logRatio, varR, varT float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()
	stddevR := math.Sqrt(varR)
	stddevT := math.Sqrt(varT)
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "VarShift variance regime change: " + displayName,
		Description: fmt.Sprintf("%s log-variance-ratio %.3f exceeded threshold %.2f for %d ticks (sigma R→T: %.3f → %.3f)",
			displayName, logRatio, d.LogRatioThreshold, d.PersistenceK, stddevR, stddevT),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineStddev: stddevR,
			Threshold:      d.LogRatioThreshold,
			CurrentValue:   stddevT,
			DeviationSigma: math.Abs(logRatio),
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults. Called from
// every public method that depends on configuration so the zero-valued
// struct works.
func (d *VarShiftDetector) ensureDefaults() {
	if d.LogRatioThreshold <= 0 {
		d.LogRatioThreshold = varshiftLogRatioThreshold
	}
	if d.MeanStationaryGate <= 0 {
		d.MeanStationaryGate = varshiftMeanStationaryGate
	}
	if d.PersistenceK <= 0 || d.PersistenceK > varshiftLogRatioHistory {
		// Capped at the per-series ring size — larger values would index
		// past the fixed-size history array.
		d.PersistenceK = varshiftLogRatioHistory
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = varshiftRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[varshiftStateKey]*varshiftSeriesState)
	}
}
