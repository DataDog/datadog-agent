// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// ACorrShift detector — streaming lag-1 autocorrelation regime change.
//
// Concept-drift literature (Gama et al. 2014, "A survey on concept drift
// adaptation", §3.4) lists shifts in the temporal dependence structure as a
// distinct distribution-drift indicator alongside mean and variance shifts.
// The other detectors in this catalog (BOCPD, ScanMW, ScanWelch) target shifts
// in the marginal mean/variance of a series; none of them flags a series whose
// mean and variance stay constant but whose autocorrelation regime flips —
// e.g. a metric that goes from i.i.d. noise to bursty/persistent or vice
// versa. ACorrShift fills that gap.
//
// Algorithm (per series, per aggregation):
//   1. Maintain a rolling ring of the last W=60 raw values.
//   2. After W_warmup=30 points are buffered, compute the biased lag-1
//      autocorrelation ρ̂_t over the ring contents directly (textbook formula
//      — O(W) per tick is cheaper than maintaining streaming product sums).
//      Clamp ρ̂_t to [-1, 1].
//   3. Feed ρ̂_t into a P²-quantile estimator (Jain & Chlamtac, CACM 1985)
//      tracking the long-run median ρ_baseline. P² is a constant-state
//      streaming median: 5 floats, 5 marker positions, no sorting.
//   4. Maintain a ring of the last K=5 ρ̂ values. If ALL K satisfy
//      |ρ̂_i − ρ_baseline| > Δ (Δ=0.4), emit one anomaly and enter the alert
//      state. Re-trigger is suppressed until 10 consecutive non-trigger ticks
//      reset the alert.
//
// Per-tick cost: O(W) for ACF + O(1) for P² + O(K) for the persistence ring.
// Per-(series, agg) memory: 60 floats (ring) + 5 floats (last ρ̂s) + ~50
// bytes for P² markers ≈ ~600 B. No allocations on the hot path.

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Algorithm constants. These are state-array-shape inputs, so they have to be
// compile-time constants. The detector struct exposes the trigger thresholds
// (RhoDelta, PersistenceK, RecoveryPoints) so tests can adjust them without
// touching the per-series fixed-size buffers.
const (
	acorrshiftWindow         = 60
	acorrshiftWarmup         = 30
	acorrshiftPersistenceK   = 5
	acorrshiftRecoveryPoints = 10
	acorrshiftRhoDelta       = 0.4
)

// acorrshiftStateKey identifies per-series state by ref and aggregation.
type acorrshiftStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// acorrshiftSeriesState holds per-series streaming state.
type acorrshiftSeriesState struct {
	// Ring of the last W raw values used for the lag-1 ACF computation.
	ring     [acorrshiftWindow]float64
	ringHead int
	ringN    int

	// First-point gate: the lag-1 product is undefined for a single point, so
	// we drop the very first observation per the plan and only start filling
	// the ring once we have a "previous" anchor. After that, every new point
	// is appended to the ring.
	prev     float64
	havePrev bool

	// Ring of the last K computed ρ̂ values, used for the K-of-K persistence
	// check. rhoN counts valid entries until the ring fills.
	lastRho [acorrshiftPersistenceK]float64
	rhoHead int
	rhoN    int

	// P² streaming median estimator over the historical ρ̂ stream. Holds a
	// constant 5 floats of marker heights and 5 ints of positions; no sort,
	// no buffer growth. Provides ρ_baseline for the trigger comparison.
	baselineP2 p2Quantile

	// Alert lifecycle: inAlert suppresses re-emission while a regime shift is
	// ongoing; recoveryCnt counts consecutive non-triggered ticks toward
	// clearing the alert. triggerBaseline pins the P² baseline at the moment
	// the alert was raised so recovery measures "ρ̂ has returned to where it
	// was before the shift", not "ρ̂ is now close to the drifting median".
	// Without this anchor, a sustained shift would re-fire every time the
	// running median crawled back across the threshold.
	inAlert         bool
	recoveryCnt     int
	triggerBaseline float64

	// Cursor — same pattern as BOCPD/ScanMW (metrics_detector_bocpd.go:16-22).
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// AcorrShiftDetector flags shifts in the lag-1 autocorrelation regime of a
// series. Implements observer.Detector + observer.SeriesRemover.
type AcorrShiftDetector struct {
	// RhoDelta is the absolute |ρ̂ − ρ_baseline| threshold for a single tick
	// to count toward persistence. Default: 0.4.
	RhoDelta float64

	// PersistenceK is the number of consecutive over-threshold ρ̂ ticks
	// required to fire. Default: 5. Note: the per-series ring is fixed-size,
	// so values larger than acorrshiftPersistenceK are clamped.
	PersistenceK int

	// RecoveryPoints is the number of consecutive non-triggering ticks that
	// resets an active alert. Default: 10.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state keyed by ref+agg.
	series map[acorrshiftStateKey]*acorrshiftSeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewAcorrShiftDetector constructs an AcorrShiftDetector with default settings.
func NewAcorrShiftDetector() *AcorrShiftDetector {
	return &AcorrShiftDetector{
		RhoDelta:       acorrshiftRhoDelta,
		PersistenceK:   acorrshiftPersistenceK,
		RecoveryPoints: acorrshiftRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[acorrshiftStateKey]*acorrshiftSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*AcorrShiftDetector) Name() string { return "acorrshift" }

// Reset clears all per-series state for replay/reanalysis.
func (d *AcorrShiftDetector) Reset() {
	d.series = make(map[acorrshiftStateKey]*acorrshiftSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~600 B of fixed-size streaming state, so without this teardown the map would
// grow with the cumulative count of series ever observed even after their
// storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *AcorrShiftDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, acorrshiftStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors BOCPD/ScanMW:
// gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *AcorrShiftDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := acorrshiftStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &acorrshiftSeriesState{baselineP2: newP2Quantile(0.5)}
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
func (d *AcorrShiftDetector) processPoint(state *acorrshiftSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	// First-point gate: lag-1 ACF needs ≥ 2 points. Skip the very first.
	if !state.havePrev {
		state.prev = x
		state.havePrev = true
		return nil
	}
	state.prev = x

	// Append to the rolling W-window of values.
	state.ring[state.ringHead] = x
	state.ringHead = (state.ringHead + 1) % acorrshiftWindow
	if state.ringN < acorrshiftWindow {
		state.ringN++
	}

	// Need at least W_warmup ring entries to make the ACF estimate meaningful.
	if state.ringN < acorrshiftWarmup {
		return nil
	}

	rho := computeLag1ACF(state.ring[:], state.ringHead, state.ringN)

	// Update the P² baseline (long-run median of ρ̂_t).
	state.baselineP2.add(rho)

	// Append to the K-tick persistence ring.
	state.lastRho[state.rhoHead] = rho
	state.rhoHead = (state.rhoHead + 1) % acorrshiftPersistenceK
	if state.rhoN < acorrshiftPersistenceK {
		state.rhoN++
	}

	// Decision gate requires both the K-ring and the P² estimator to be ready.
	baseline, ok := state.baselineP2.value()
	persistK := d.PersistenceK
	if persistK > acorrshiftPersistenceK {
		persistK = acorrshiftPersistenceK
	}
	if !ok || state.rhoN < persistK {
		return nil
	}

	// Trigger gate: ALL K most recent ρ̂s must lie OUTSIDE the band relative
	// to the current (drifting) baseline. Even one ρ̂ inside the band breaks
	// the streak.
	allOver := true
	for i := 0; i < persistK; i++ {
		if math.Abs(state.lastRho[i]-baseline) <= d.RhoDelta {
			allOver = false
			break
		}
	}

	if allOver {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting on this regime; do not re-emit until recovery.
			return nil
		}
		state.inAlert = true
		state.triggerBaseline = baseline
		return d.makeAnomaly(p, series, agg, rho, baseline)
	}

	// Recovery counter is anchored to the baseline at the moment of the
	// trigger. A tick "recovers" iff the current ρ̂ has returned within the
	// band of that frozen anchor — i.e., the temporal structure has gone
	// back to what it looked like before the shift. Anchoring to the
	// drifting current median would let the running median drift up into
	// the post-shift ρ̂s and produce a spurious "calm" reading even though
	// nothing changed in the signal; that produced multiple anomalies for a
	// single regime change in dev runs against AR(1) test fixtures.
	if state.inAlert {
		if math.Abs(rho-state.triggerBaseline) <= d.RhoDelta {
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
func (d *AcorrShiftDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, rho, baseline float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	delta := math.Abs(rho - baseline)
	displayName := source.String()
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "AcorrShift autocorrelation regime change: " + displayName,
		Description: fmt.Sprintf("%s lag-1 autocorrelation shifted from baseline %.3f to %.3f (|Δ|=%.3f, sustained %d ticks above %.2f)",
			displayName, baseline, rho, delta, d.PersistenceK, d.RhoDelta),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: baseline,
			CurrentValue:   rho,
			DeviationSigma: delta,
			Threshold:      d.RhoDelta,
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults. Called from every
// public method that depends on configuration so the zero-valued struct works.
func (d *AcorrShiftDetector) ensureDefaults() {
	if d.RhoDelta <= 0 {
		d.RhoDelta = acorrshiftRhoDelta
	}
	if d.PersistenceK <= 0 {
		d.PersistenceK = acorrshiftPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = acorrshiftRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[acorrshiftStateKey]*acorrshiftSeriesState)
	}
}

// computeLag1ACF returns the biased lag-1 autocorrelation of the ring's
// chronologically ordered contents:
//
//	ρ̂ = Σ_{i=0..n-2} (x_i - μ)(x_{i+1} - μ) / Σ_{i=0..n-1} (x_i - μ)²
//
// Result is clamped to [-1, 1]. Returns 0 when the variance floor is hit
// (constant or near-constant series produce an undefined ACF).
//
// Indexing: when n < W the ring fills from index 0 forward (ringHead == n);
// when n == W the oldest entry sits at ringHead and reads wrap modulo W.
// The `(head - n + W) % W` formula handles both cases.
func computeLag1ACF(ring []float64, head, n int) float64 {
	w := len(ring)
	if n < 2 {
		return 0
	}
	oldest := (head - n + w) % w
	if oldest < 0 {
		oldest += w
	}

	var sum float64
	for i := 0; i < n; i++ {
		sum += ring[(oldest+i)%w]
	}
	mean := sum / float64(n)

	var varSum, covSum float64
	for i := 0; i < n; i++ {
		v := ring[(oldest+i)%w] - mean
		varSum += v * v
	}
	for i := 0; i < n-1; i++ {
		a := ring[(oldest+i)%w] - mean
		b := ring[(oldest+i+1)%w] - mean
		covSum += a * b
	}

	if varSum < 1e-12 {
		return 0
	}
	rho := covSum / varSum
	if rho > 1 {
		rho = 1
	} else if rho < -1 {
		rho = -1
	}
	return rho
}

// p2Quantile is the streaming P² quantile estimator (Jain & Chlamtac, CACM
// 1985, "The P² Algorithm for Dynamic Calculation of Quantiles and Histograms
// without Storing Observations"). Tracks an arbitrary quantile p ∈ (0,1) using
// 5 markers — 0, p/2, p, (1+p)/2, 1 — whose heights and positions are updated
// in place per observation. Constant memory (40 bytes) and O(1) per add — no
// sorting and no observation buffer.
type p2Quantile struct {
	p           float64    // target quantile
	q           [5]float64 // marker heights
	n           [5]int     // current marker positions (1-indexed conceptually)
	np          [5]float64 // desired marker positions
	dn          [5]float64 // per-observation increments to np
	count       int        // observations seen
	initialized bool
}

func newP2Quantile(p float64) p2Quantile {
	return p2Quantile{
		p:  p,
		dn: [5]float64{0, p / 2, p, (1 + p) / 2, 1},
	}
}

// add ingests one observation, updating marker heights and positions per the
// P² recurrence.
func (e *p2Quantile) add(x float64) {
	if !e.initialized {
		e.q[e.count] = x
		e.count++
		if e.count < 5 {
			return
		}
		// Sort the first 5 in place (5 elements → simple insertion sort).
		for i := 1; i < 5; i++ {
			for j := i; j > 0 && e.q[j-1] > e.q[j]; j-- {
				e.q[j-1], e.q[j] = e.q[j], e.q[j-1]
			}
		}
		for i := 0; i < 5; i++ {
			e.n[i] = i + 1
		}
		e.np = [5]float64{1, 1 + 2*e.p, 1 + 4*e.p, 3 + 2*e.p, 5}
		e.initialized = true
		return
	}

	// Cell location.
	var k int
	switch {
	case x < e.q[0]:
		e.q[0] = x
		k = 0
	case x < e.q[1]:
		k = 0
	case x < e.q[2]:
		k = 1
	case x < e.q[3]:
		k = 2
	case x <= e.q[4]:
		k = 3
	default:
		e.q[4] = x
		k = 3
	}

	for i := k + 1; i < 5; i++ {
		e.n[i]++
	}
	for i := 0; i < 5; i++ {
		e.np[i] += e.dn[i]
	}

	// Adjust the three middle markers if their position drifts by ≥1 from
	// desired. The boundary markers (0 and 4) are pinned to min/max.
	for i := 1; i <= 3; i++ {
		d := e.np[i] - float64(e.n[i])
		canUp := d >= 1 && e.n[i+1]-e.n[i] > 1
		canDown := d <= -1 && e.n[i-1]-e.n[i] < -1
		if !canUp && !canDown {
			continue
		}
		s := 1
		if d < 0 {
			s = -1
		}
		qNew := e.parabolic(i, s)
		if qNew > e.q[i-1] && qNew < e.q[i+1] {
			e.q[i] = qNew
		} else {
			e.q[i] = e.q[i] + float64(s)*(e.q[i+s]-e.q[i])/float64(e.n[i+s]-e.n[i])
		}
		e.n[i] += s
	}
	e.count++
}

// parabolic computes the P² parabolic interpolation for marker i moving by
// sgn ∈ {+1,-1}. See Jain & Chlamtac 1985, eq. (1).
func (e *p2Quantile) parabolic(i, sgn int) float64 {
	s := float64(sgn)
	denom := float64(e.n[i+1] - e.n[i-1])
	if denom == 0 {
		return e.q[i]
	}
	left := float64(e.n[i]-e.n[i-1]+sgn) * (e.q[i+1] - e.q[i]) / float64(e.n[i+1]-e.n[i])
	right := float64(e.n[i+1]-e.n[i]-sgn) * (e.q[i] - e.q[i-1]) / float64(e.n[i]-e.n[i-1])
	return e.q[i] + s/denom*(left+right)
}

// value returns the current quantile estimate. The second return is false
// before the estimator has seen 5 observations (initialization phase).
func (e *p2Quantile) value() (float64, bool) {
	if !e.initialized {
		return 0, false
	}
	return e.q[2], true
}
