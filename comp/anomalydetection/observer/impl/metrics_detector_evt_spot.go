// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// EVT-SPOT (Streaming Peaks-Over-Threshold) tail-anomaly detector.
//
// Implements the SPOT/DSPOT algorithm from Siffer et al., "Anomaly Detection
// in Streams with Extreme Value Theory" (KDD 2017). Per (series, aggregation)
// the detector:
//   1. Calibrates on the first CalibrationSize points to estimate an initial
//      tail threshold tInit at the (1-QInit) empirical quantile.
//   2. Fits a Generalized Pareto Distribution (GPD) to the empirical excesses
//      x - tInit using closed-form Method-of-Moments — no iterative MLE.
//   3. Streams subsequent points, comparing each to a calibrated alarm
//      threshold zQ derived from the GPD fit and a target false-alarm rate
//      QAlarm (default 1e-4).
//   4. The DSPOT variant subtracts a rolling-mean estimate before the threshold
//      check, so slow drift is handled without firing.
//
// This is structurally orthogonal to the rest of the catalog: scanmw /
// scanwelch / kl_divergence / ks_drift compare two windows; bocpd is
// Bayesian; holt_residual fits a one-step forecast; matrix_profile captures
// shape. EVT explicitly models the heavy-tail behaviour of "rare, legitimate
// spikes" and gives a *calibrated* false-alarm rate, so the FP ceiling stays
// bounded by construction (≤ q FPs per ingested point).
//
// Memory per (series, aggregation): CalibrationSize (200) + MaxExcesses (50)
// + DriftWindow (30) float64 buffers + scalars ≈ 2.2 KB.
//
// Per-tick cost: O(1) for the threshold check; O(N_excess)=O(50) every
// RefitEvery=10 new excesses for the GPD MoM refit. With ~2-5 % of points
// flagged as excesses, amortized cost ≈ O(1) per ingested point.

// EVT-SPOT tunable defaults — chosen to keep FP volume bounded by the
// QAlarm calibration rather than by a heuristic z-threshold (the failure
// mode that sank ADWIN / matrix-profile-discord / permutation-entropy
// in earlier experiments).
const (
	evtCalibrationSize = 200
	evtQInit           = 0.02 // top 2 % of calibration points form excesses
	evtQAlarm          = 1e-4 // target false-alarm rate
	evtMaxExcesses     = 50
	evtRefitEvery      = 10
	evtDriftWindow     = 30
	evtMinDeviationMAD = 3.0
	evtRefractory      = 24
	// evtTimestampRing matches the holt_residual ring so medianTimestampInterval
	// has enough samples for a robust median.
	evtTimestampRing = 16
)

// evtStateKey identifies per-series EVT state by ref and aggregation.
type evtStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// evtSeriesState holds the streaming SPOT/DSPOT state for one (series,
// aggregation) pair.
//
// Lifecycle: calibration → streaming. During calibration, raw values
// accumulate in calBuf until CalibrationSize is reached; at that point the
// initial threshold tInit is set to the (1-QInit) empirical quantile, the
// initial excess set is seeded, and the GPD parameters and zQ are computed.
// All subsequent points pass through the DSPOT detrend, the EVT alarm gate
// and the effect-size MAD gate.
type evtSeriesState struct {
	// cursor (mirrors holt_residual)
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64
	lastSeenTimestamp  int64

	// Calibration phase. calBuf is freed once calibrated=true.
	calBuf     []float64
	calibrated bool

	// tInit is the static initial threshold from the calibration quantile;
	// excesses are kept relative to tInit, so it is set once and never
	// updated (the GPD does the work of tracking the tail dynamics).
	tInit float64

	// GPD parameters fit by Method of Moments on the excess set.
	gpdGamma float64
	gpdSigma float64

	// excesses is a FIFO ring of the most recent post-tInit excesses
	// (x - tInit). Cap = MaxExcesses. Never includes points that fired
	// (those would skew the tail fit).
	excesses []float64

	// totalSeen counts all post-calibration ingested points (denominator
	// of the SPOT formula). totalExceed counts how many of those exceeded
	// tInit (numerator). excessSinceRefit is the trigger for periodic refit.
	totalSeen        int
	totalExceed      int
	excessSinceRefit int

	// driftBuf is the DSPOT rolling-mean buffer. Cap = DriftWindow. Even
	// during calibration we feed it so the detrend is warm by the time
	// streaming starts.
	driftBuf []float64

	// zQ is the calibrated alarm threshold; recomputed on every GPD refit.
	zQ float64

	// refractoryRemaining suppresses fires for the next N ingested points
	// after a fire — protects against runs of post-spike replicas.
	refractoryRemaining int

	// captured series metadata (first non-empty observation suffices)
	seriesMetaCaptured bool
	seriesNamespace    string
	seriesName         string
	seriesTags         []string

	// recentTimestamps is a small ring used by medianTimestampInterval
	// to estimate the sampling cadence for downstream correlators.
	recentTimestamps []int64
}

// EVTSpotDetector fires when a new point's detrended value exceeds an
// EVT-derived alarm threshold corresponding to a fixed false-alarm rate q.
//
// Implements observer.Detector and observer.SeriesRemover. Tunable fields
// are exported so callers (testbench, tests) may override defaults after
// construction; NewEVTSpotDetector populates them.
type EVTSpotDetector struct {
	// CalibrationSize is the number of points collected before SPOT begins.
	// Must be large enough to estimate the (1-QInit) quantile robustly.
	CalibrationSize int
	// QInit is the calibration quantile used to seed tInit (default 0.02 →
	// top 2 % of calibration values become excesses).
	QInit float64
	// QAlarm is the target false-alarm rate (default 1e-4 → ≤ 4 FPs per
	// 10⁴ points).
	QAlarm float64
	// MaxExcesses caps the FIFO excess buffer (default 50).
	MaxExcesses int
	// RefitEvery is the number of new excesses between consecutive GPD
	// refits (default 10).
	RefitEvery int
	// DriftWindow is the rolling-mean detrend window for DSPOT (default 30).
	DriftWindow int
	// MinDeviationMAD is the minimum |x - median| / MAD effect-size gate
	// applied on top of the EVT threshold. Mirrors the gate used by
	// kl_divergence / holt_residual to defend against firing on
	// big-by-EVT but small-by-effect-size values.
	MinDeviationMAD float64
	// Refractory suppresses fires for this many points after a fire.
	Refractory int
	// Aggregations to run detection on. Default [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[evtStateKey]*evtSeriesState

	// cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewEVTSpotDetector creates an EVTSpotDetector with default settings.
// The catalog factory calls this with no arguments; tunables can be
// overridden post-construction by setting the exported fields.
func NewEVTSpotDetector() *EVTSpotDetector {
	return &EVTSpotDetector{
		CalibrationSize: evtCalibrationSize,
		QInit:           evtQInit,
		QAlarm:          evtQAlarm,
		MaxExcesses:     evtMaxExcesses,
		RefitEvery:      evtRefitEvery,
		DriftWindow:     evtDriftWindow,
		MinDeviationMAD: evtMinDeviationMAD,
		Refractory:      evtRefractory,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[evtStateKey]*evtSeriesState),
	}
}

// NewEvtSpotDetector is the lower-camelcase alias kept for the catalog
// factory registered in stage 1. It just delegates to NewEVTSpotDetector
// so the catalog and the tests can reach the same detector instance.
func NewEvtSpotDetector() *EVTSpotDetector { return NewEVTSpotDetector() }

// Name implements observer.Detector.
func (d *EVTSpotDetector) Name() string { return "evt_spot" }

// Reset clears all per-series state for replay/reanalysis.
func (d *EVTSpotDetector) Reset() {
	d.series = make(map[evtStateKey]*evtSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Mirrors HoltResidualDetector.RemoveSeries: each entry holds rolling
// buffers, so without the teardown the map grows unbounded with the
// cumulative series count even after storage shrinks.
func (d *EVTSpotDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, evtStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Streams new points into per-series
// SPOT state and emits anomalies when both the EVT and effect-size gates
// pass.
func (d *EVTSpotDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := evtStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip when nothing has been written and the
			// cursor matches. Mirrors holt_residual.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			anomalies := d.ingestNewPoints(storage, meta.Ref, agg, state, dataTime)
			for j := range anomalies {
				anomalies[j].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}
			allAnomalies = append(allAnomalies, anomalies...)

			state.lastProcessedCount = status.pointCount
			state.lastProcessedTime = dataTime
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// newState allocates a per-series state with appropriately sized scratch
// buffers. Splitting the allocation here keeps Detect's hot path branch-free.
func (d *EVTSpotDetector) newState() *evtSeriesState {
	return &evtSeriesState{
		calBuf:           make([]float64, 0, d.CalibrationSize),
		excesses:         make([]float64, 0, d.MaxExcesses),
		driftBuf:         make([]float64, 0, d.DriftWindow),
		recentTimestamps: make([]int64, 0, evtTimestampRing),
	}
}

// ingestNewPoints streams points in (state.lastProcessedTime, dataTime] into
// the per-series SPOT state. Returns any anomalies fired by the gates.
func (d *EVTSpotDetector) ingestNewPoints(
	storage observer.StorageReader,
	ref observer.SeriesRef,
	agg observer.Aggregate,
	state *evtSeriesState,
	dataTime int64,
) []observer.Anomaly {
	if dataTime <= state.lastProcessedTime {
		return nil
	}

	var fired []observer.Anomaly

	storage.ForEachPoint(ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
		// Capture series metadata once.
		if !state.seriesMetaCaptured {
			state.seriesNamespace = s.Namespace
			state.seriesName = s.Name
			if len(s.Tags) > 0 {
				tagsCopy := make([]string, len(s.Tags))
				copy(tagsCopy, s.Tags)
				state.seriesTags = tagsCopy
			}
			state.seriesMetaCaptured = true
		}
		state.lastSeenTimestamp = p.Timestamp
		pushTimestampEVT(state, p.Timestamp)

		anomaly, hasFire := d.processPoint(state, p, agg)

		// Refractory countdown — decrement on every post-calibration ingest,
		// regardless of whether the point would have fired.
		if state.refractoryRemaining > 0 {
			state.refractoryRemaining--
		}

		if hasFire {
			fired = append(fired, anomaly)
		}
	})

	return fired
}

// processPoint runs one SPOT/DSPOT step for a post-warmup point: detrend →
// EVT gate → effect-size gate → optional fire / GPD update. Returns the
// populated Anomaly when both gates pass and refractory is clear.
func (d *EVTSpotDetector) processPoint(
	state *evtSeriesState,
	p observer.Point,
	agg observer.Aggregate,
) (observer.Anomaly, bool) {
	// 1. DSPOT detrend. We compute the rolling mean from the *current*
	// driftBuf (before pushing this point) so the detrended value
	// reflects the historical drift, not a window that already contains
	// the candidate point.
	driftMean := sliceMean(state.driftBuf)
	xPrime := p.Value
	if len(state.driftBuf) >= d.DriftWindow {
		xPrime = p.Value - driftMean
	}
	pushFIFO(&state.driftBuf, d.DriftWindow, p.Value)

	// 2. Calibration phase. During calibration we never fire — we accumulate
	// raw values and, when the buffer fills, compute tInit, seed the
	// excess set, fit the GPD and compute zQ.
	if !state.calibrated {
		state.calBuf = append(state.calBuf, p.Value)
		if len(state.calBuf) >= d.CalibrationSize {
			finalizeCalibration(state, d.QInit, d.MaxExcesses)
			d.refitGPD(state)
			d.computeZQ(state)
			state.calBuf = nil
			state.calibrated = true
		}
		return observer.Anomaly{}, false
	}

	// 3. Effect-size gate denominator: rolling MAD over the raw drift
	// buffer. A nearly-flat-low-noise series can have a value clear zQ
	// while being only marginally above the median; the MAD gate (shared
	// with kl_divergence / holt_residual) blocks those.
	medianValue := detectorMedian(state.driftBuf)
	madValue := detectorMAD(state.driftBuf, medianValue, true)
	madFloored := floorSigma(madValue, state.driftBuf)
	devMAD := math.Abs(p.Value-medianValue) / madFloored

	// 4. EVT gate. We never fire during refractory.
	state.totalSeen++
	overEVT := xPrime > state.zQ
	overInit := xPrime > state.tInit
	gateOK := overEVT && devMAD >= d.MinDeviationMAD
	fire := gateOK && state.refractoryRemaining == 0

	switch {
	case fire:
		// Do NOT update the GPD with a fired point — extreme outliers
		// would skew the tail fit and inflate zQ on subsequent ticks,
		// reducing recall on follow-up anomalies.
		state.refractoryRemaining = d.Refractory
	case overInit:
		// Update the GPD: this is a normal-tail excess.
		excess := xPrime - state.tInit
		pushFIFO(&state.excesses, d.MaxExcesses, excess)
		state.totalExceed++
		state.excessSinceRefit++
		// Two paths to a refit:
		//   (a) RefitEvery has elapsed — the steady-state amortized path.
		//   (b) We do not yet have a usable GPD fit (gpdSigma==0 means the
		//       last refit fell back to the safe-large default). Refit
		//       eagerly so zQ becomes meaningful as soon as enough
		//       excesses exist; the cost is bounded since this only runs
		//       until the first successful fit. Without this fast path
		//       the under-seeded calibration window (CalibrationSize *
		//       QInit ≈ 4 < 5) leaves zQ safe-large for ~RefitEvery
		//       additional excesses before the first fit, blinding the
		//       detector early in the stream.
		if state.excessSinceRefit >= d.RefitEvery || state.gpdSigma == 0 {
			d.refitGPD(state)
			d.computeZQ(state)
			state.excessSinceRefit = 0
		}
	}

	if !fire {
		return observer.Anomaly{}, false
	}

	// 5. Build the anomaly. Mirrors holt_residual / kl_divergence shape.
	score := xPrime - state.zQ
	if score < 0 {
		// Defensive: fire implies xPrime > zQ, so score should always
		// be positive. Clamp to zero just in case.
		score = 0
	}
	seriesName := state.seriesName + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type: observer.AnomalyTypeMetric,
		Source: observer.SeriesDescriptor{
			Namespace: state.seriesNamespace,
			Name:      state.seriesName,
			Tags:      state.seriesTags,
			Aggregate: agg,
		},
		DetectorName: d.Name(),
		Title:        "EVT-SPOT tail: " + seriesName,
		Description: fmt.Sprintf(
			"%s exceeded EVT alarm threshold (observed=%.4f, detrended=%.4f, zQ=%.4f, tInit=%.4f, %.1f valueMADs, n=%d, n_t=%d)",
			seriesName, p.Value, xPrime, state.zQ, state.tInit, devMAD, state.totalSeen, state.totalExceed,
		),
		Timestamp:           state.lastSeenTimestamp,
		Score:               &score,
		SamplingIntervalSec: medianTimestampInterval(state.recentTimestamps),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medianValue,
			BaselineMAD:    madValue,
			CurrentValue:   p.Value,
			DeviationSigma: devMAD,
			Threshold:      state.zQ,
		},
	}

	return anomaly, true
}

// finalizeCalibration computes tInit at the (1-qInit) empirical quantile of
// calBuf and seeds the excess set with all calBuf entries above tInit.
// Uses ascending-sorted-copy index round-toward-zero — equivalent to the
// classic "type 1" quantile estimator and good enough for the seeding step
// (the GPD does the heavy lifting from here on).
func finalizeCalibration(state *evtSeriesState, qInit float64, maxExcesses int) {
	n := len(state.calBuf)
	if n == 0 {
		return
	}
	sorted := make([]float64, n)
	copy(sorted, state.calBuf)
	sort.Float64s(sorted)

	idx := int(math.Floor((1.0 - qInit) * float64(n)))
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	state.tInit = sorted[idx]

	// Seed excesses from the calibration buffer. We honour the FIFO cap
	// (keep the most-recent maxExcesses excesses) by walking calBuf in
	// arrival order, not sorted order.
	for _, v := range state.calBuf {
		if v > state.tInit {
			pushFIFO(&state.excesses, maxExcesses, v-state.tInit)
			state.totalExceed++
		}
	}
	state.totalSeen = n
}

// refitGPD fits Generalized Pareto parameters to the excess buffer using
// the closed-form Method of Moments:
//
//	γ = ½ (1 - mean²/var),  σ = mean·(1 - γ)
//
// This is the formula from Hosking & Wallis (1987) and gives a good
// streaming estimate without the cost of an iterative MLE solver. With
// fewer than 5 excesses the parameters are not well-defined; we fall back
// to (γ=0, σ=0) which makes computeZQ pin zQ to a safe-large default.
func (d *EVTSpotDetector) refitGPD(state *evtSeriesState) {
	n := len(state.excesses)
	if n < 5 {
		state.gpdGamma = 0
		state.gpdSigma = 0
		return
	}
	var sum float64
	for _, e := range state.excesses {
		sum += e
	}
	mean := sum / float64(n)

	var m2 float64
	for _, e := range state.excesses {
		diff := e - mean
		m2 += diff * diff
	}
	variance := m2 / float64(n-1)
	if variance <= 0 {
		// Degenerate (constant excesses): fall back to exponential tail
		// (γ=0, σ=mean) so computeZQ stays defined.
		state.gpdGamma = 0
		state.gpdSigma = mean
		return
	}

	state.gpdGamma = 0.5 * (1.0 - mean*mean/variance)
	state.gpdSigma = mean * (1.0 - state.gpdGamma)
	if state.gpdSigma <= 0 {
		// Safety floor: σ≤0 would make zQ ill-defined. Fall back to the
		// excess-mean as scale; this is conservative (slightly lower
		// threshold) but keeps the detector live.
		state.gpdSigma = mean
	}
}

// computeZQ derives the alarm threshold zQ from the current GPD parameters
// using the SPOT formula:
//
//	z_q = t + (σ/γ) · ((q·n/n_t)^{-γ} - 1)
//
// For γ ≈ 0 we use the exponential-tail limit:
//
//	z_q = t - σ·ln(q·n/n_t)
//
// When n_t is zero (no excesses observed yet) we set zQ to a very large
// value so no fire can pass the gate — the detector is "armed but silent"
// until at least one calibration excess shows up.
func (d *EVTSpotDetector) computeZQ(state *evtSeriesState) {
	n := state.totalSeen
	nT := state.totalExceed
	if nT == 0 || n == 0 || state.gpdSigma <= 0 {
		state.zQ = state.tInit + 1e9
		return
	}
	ratio := d.QAlarm * float64(n) / float64(nT)
	if ratio <= 0 {
		state.zQ = state.tInit + 1e9
		return
	}
	if math.Abs(state.gpdGamma) < 1e-6 {
		state.zQ = state.tInit - state.gpdSigma*math.Log(ratio)
		return
	}
	state.zQ = state.tInit + (state.gpdSigma/state.gpdGamma)*(math.Pow(ratio, -state.gpdGamma)-1)
}

// pushTimestampEVT appends ts to the recent-timestamp ring, dropping the
// oldest if full. Mirrors pushTimestamp but is local to EVT to avoid
// coupling the Holt state shape.
func pushTimestampEVT(state *evtSeriesState, ts int64) {
	if cap(state.recentTimestamps) == 0 {
		state.recentTimestamps = make([]int64, 0, evtTimestampRing)
	}
	if len(state.recentTimestamps) < cap(state.recentTimestamps) {
		state.recentTimestamps = append(state.recentTimestamps, ts)
		return
	}
	copy(state.recentTimestamps, state.recentTimestamps[1:])
	state.recentTimestamps[len(state.recentTimestamps)-1] = ts
}

// sliceMean returns the arithmetic mean of buf, or 0 for an empty slice.
func sliceMean(buf []float64) float64 {
	if len(buf) == 0 {
		return 0
	}
	var sum float64
	for _, v := range buf {
		sum += v
	}
	return sum / float64(len(buf))
}

// ensureDefaults fills zero-valued config fields with sensible defaults.
// Mirrors holt_residual / kl_divergence ensureDefaults so the detector
// behaves sanely even when constructed via reflective paths that bypass
// NewEVTSpotDetector.
func (d *EVTSpotDetector) ensureDefaults() {
	if d.CalibrationSize <= 0 {
		d.CalibrationSize = evtCalibrationSize
	}
	if d.QInit <= 0 {
		d.QInit = evtQInit
	}
	if d.QAlarm <= 0 {
		d.QAlarm = evtQAlarm
	}
	if d.MaxExcesses <= 0 {
		d.MaxExcesses = evtMaxExcesses
	}
	if d.RefitEvery <= 0 {
		d.RefitEvery = evtRefitEvery
	}
	if d.DriftWindow <= 0 {
		d.DriftWindow = evtDriftWindow
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = evtMinDeviationMAD
	}
	if d.Refractory <= 0 {
		d.Refractory = evtRefractory
	}
	if d.series == nil {
		d.series = make(map[evtStateKey]*evtSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
