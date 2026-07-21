// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Holt's-method forecast-residual detector.
//
// Per (series, aggregation), maintain Holt's double exponential smoothing
// state (level + trend) plus a rolling window of one-step forecast residuals.
// Each new point is forecast as L_{t-1} + T_{t-1}; an anomaly fires when the
// MAD-standardised residual |z_t| crosses the threshold AND the same sign
// repeats for ConfirmM consecutive points AND the raw deviation
// |x_t - L_{t-1}| / MAD(values) clears the effect-size gate. A short
// refractory period suppresses repeat fires while the smoother adapts to
// the new regime.
//
// This complements level-shift scans and the Bayesian changepoint detector:
// the HW residual gate operates on a forecast that adapts to drifting /
// trending baselines, so a slow ramp punctuated by a jump produces a small
// forecast residual on the ramp itself and a large one on the jump.
//
// Memory per (series, aggregation): a 24-point warmup buffer, a
// 60-residual MAD window, a 60-value MAD window, plus scalars — ~1.5 KB.
// Per-tick cost: O(1) smoother update + O(W log W) MAD recompute (W=60),
// dominated by the two sort.Float64s calls inside detectorMAD.

// Holt's tunable defaults. The gates are conservative: a residual must repeat
// with the same sign and clear a raw effect-size check before firing.
const (
	holtAlpha           = 0.2
	holtBeta            = 0.05
	holtWarmupPoints    = 24
	holtResidualWindow  = 60
	holtZThreshold      = 4.5
	holtConfirmM        = 2
	holtMinDeviationMAD = 3.0
	holtRefractory      = 20
	// holtTimestampRing keeps the last N point timestamps for sampling-interval
	// estimation — sized to match the warmup buffer for cheap reuse.
	holtTimestampRing = 16
	// holtPostFireSampleN is the number of recent non-fire residuals whose
	// median is injected into the residual window in place of the fire's
	// own residual (Hampel rejection, applied only to the threshold).
	holtPostFireSampleN = 5
)

// holtStateKey identifies per-series state by ref and aggregation.
type holtStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// holtSeriesState holds the streaming state for one (series, aggregation).
//
// The lifecycle is warmup → smoothing. During warmup, points accumulate in
// warmupBuf until WarmupPoints points have been seen; at that boundary the
// level and trend are seeded and warmedUp flips to true. From then on the
// recurrences run on every ingested point; a separate residual window and
// a raw-value window feed the two MAD-based gates.
type holtSeriesState struct {
	// Cursor over visible storage buckets plus in-place write generation.
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64
	lastProcessedValue float64

	// warmup buffer; cap = WarmupPoints. Discarded once warmedUp.
	warmupBuf []float64
	warmedUp  bool

	// Holt smoothed state. After warmup, level ≈ x_t and trend ≈ Δx_t.
	level float64
	trend float64

	// FIFO rolling windows; cap = ResidualWindow. resWin holds the last
	// W one-step forecast residuals; valWin holds the last W raw values
	// — used for the σ_value gate (effect size).
	resWin []float64
	valWin []float64

	// detection counters
	consecutivePos      int
	consecutiveNeg      int
	refractoryRemaining int

	// captured series metadata (first non-empty observation suffices)
	seriesMetaCaptured bool
	seriesNamespace    string
	seriesName         string
	seriesTags         []string

	// recentTimestamps is a small ring of the most recently ingested point
	// timestamps; used by medianTimestampInterval to estimate the sampling
	// cadence for downstream correlators. We don't need a full window —
	// 16 entries gives a robust median.
	recentTimestamps  []int64
	lastSeenTimestamp int64
}

// HoltResidualDetector fires when a Holt forecast residual exceeds a robust
// MAD-scaled threshold, with consecutive-confirmation and effect-size gates.
//
// Implements observer.Detector and observer.SeriesRemover. Tunable fields
// are exported so callers (testbench, tests) may override defaults after
// construction; NewHoltResidualDetector populates them.
type HoltResidualDetector struct {
	// Alpha is the level smoothing factor (0..1). Higher = more reactive.
	Alpha float64
	// Beta is the trend smoothing factor (0..1). Lower = more stable.
	Beta float64
	// WarmupPoints is the number of points collected before smoothing
	// begins. Must be >= 2 (split into halves to seed level + trend).
	WarmupPoints int
	// ResidualWindow is the rolling FIFO size for residual MAD and value MAD.
	ResidualWindow int
	// ZThreshold is the standardised-residual gate.
	ZThreshold float64
	// ConfirmM is the minimum number of consecutive same-sign |z|≥threshold
	// observations required to fire.
	ConfirmM int
	// MinDeviationMAD is the minimum |x_t - L_{t-1}| / MAD(values) for the
	// effect-size gate to pass.
	MinDeviationMAD float64
	// Refractory is the number of ingested points to suppress fires for
	// after firing, while the smoother adapts.
	Refractory int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[holtStateKey]*holtSeriesState

	// cache the discovered series list across Detect calls (mirrors the
	// scanwelch / scanmw / bocpd pattern).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// HoltResidualConfig holds catalog/testbench tunables for HoltResidualDetector.
type HoltResidualConfig struct {
	Alpha           float64  `json:"alpha"`
	Beta            float64  `json:"beta"`
	WarmupPoints    int      `json:"warmup_points"`
	ResidualWindow  int      `json:"residual_window"`
	ZThreshold      float64  `json:"z_threshold"`
	ConfirmM        int      `json:"confirm_m"`
	MinDeviationMAD float64  `json:"min_deviation_mad"`
	Refractory      int      `json:"refractory"`
	Aggregations    []string `json:"aggregations,omitempty"`
}

// DefaultHoltResidualConfig returns the production/testbench defaults.
func DefaultHoltResidualConfig() HoltResidualConfig {
	return HoltResidualConfig{
		Alpha:           holtAlpha,
		Beta:            holtBeta,
		WarmupPoints:    holtWarmupPoints,
		ResidualWindow:  holtResidualWindow,
		ZThreshold:      holtZThreshold,
		ConfirmM:        holtConfirmM,
		MinDeviationMAD: holtMinDeviationMAD,
		Refractory:      holtRefractory,
		Aggregations: []string{
			observer.AggregateString(observer.AggregateAverage),
			observer.AggregateString(observer.AggregateCount),
		},
	}
}

// NewHoltResidualDetector creates a HoltResidual detector with default
// settings. The catalog factory calls this with no arguments; tunables can
// be overridden post-construction.
func NewHoltResidualDetector() *HoltResidualDetector {
	return NewHoltResidualDetectorWithConfig(DefaultHoltResidualConfig())
}

// NewHoltResidualDetectorWithConfig creates a detector configured from cfg.
func NewHoltResidualDetectorWithConfig(cfg HoltResidualConfig) *HoltResidualDetector {
	return &HoltResidualDetector{
		Alpha:           cfg.Alpha,
		Beta:            cfg.Beta,
		WarmupPoints:    cfg.WarmupPoints,
		ResidualWindow:  cfg.ResidualWindow,
		ZThreshold:      cfg.ZThreshold,
		ConfirmM:        cfg.ConfirmM,
		MinDeviationMAD: cfg.MinDeviationMAD,
		Refractory:      cfg.Refractory,
		Aggregations:    parseAggregateConfig(cfg.Aggregations),
		series:          make(map[holtStateKey]*holtSeriesState),
	}
}

// Name implements observer.Detector.
func (d *HoltResidualDetector) Name() string { return "holt_residual" }

// Reset clears all per-series state for replay/reanalysis.
func (d *HoltResidualDetector) Reset() {
	d.series = make(map[holtStateKey]*holtSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed. Each
// entry holds rolling buffers, so without teardown the map grows unbounded with
// the cumulative series count even after storage shrinks.
func (d *HoltResidualDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, holtStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Streams new points into per-series
// Holt state and emits anomalies when all three gates pass.
func (d *HoltResidualDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := holtStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip when no new bucket or in-place merge is visible.
			mergeOccurred := status.pointCount == state.lastProcessedCount && status.writeGeneration != state.lastWriteGen
			if status.pointCount <= state.lastProcessedCount && !mergeOccurred {
				continue
			}
			startTime := state.lastProcessedTime
			countIncreased := status.pointCount > state.lastProcessedCount
			prefixCount := state.lastProcessedCount
			if countIncreased {
				prefixCount = storage.PointCountUpTo(meta.Ref, state.lastProcessedTime)
			}
			cursorBucketChangedWithAppend := countIncreased && status.writeGeneration != state.lastWriteGen &&
				prefixCount == state.lastProcessedCount && holtCursorPointChanged(storage, meta.Ref, agg, state)
			if mergeOccurred || prefixCount > state.lastProcessedCount || cursorBucketChangedWithAppend {
				state = d.newState()
				d.series[sk] = state
				startTime = 0
			}

			anomalies, pointsSeen := d.ingestNewPoints(storage, meta.Ref, agg, state, startTime, dataTime)
			for j := range anomalies {
				anomalies[j].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}
			allAnomalies = append(allAnomalies, anomalies...)

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

// newState allocates a per-series state with appropriately sized scratch
// buffers. Splitting allocation here keeps Detect's hot path branch-free.
func (d *HoltResidualDetector) newState() *holtSeriesState {
	return &holtSeriesState{
		warmupBuf:        make([]float64, 0, d.WarmupPoints),
		resWin:           make([]float64, 0, d.ResidualWindow),
		valWin:           make([]float64, 0, d.ResidualWindow),
		recentTimestamps: make([]int64, 0, holtTimestampRing),
	}
}

// ingestNewPoints streams points in (startTime, dataTime] into the per-series
// Holt state. Returns any anomalies fired by the gate logic and whether any
// points were ingested.
//
// Lifecycle:
//   - During warmup, points accumulate in warmupBuf. When the buffer fills
//     to WarmupPoints, we seed level and trend from its halves and flip
//     warmedUp.
//   - Post-warmup, each point produces a forecast and residual; the gate
//     decides whether to fire. The smoother always updates regardless of
//     whether the gate fires (so the model tracks new regimes).
func (d *HoltResidualDetector) ingestNewPoints(
	storage observer.StorageReader,
	ref observer.SeriesRef,
	agg observer.Aggregate,
	state *holtSeriesState,
	startTime int64,
	dataTime int64,
) ([]observer.Anomaly, bool) {
	if dataTime <= startTime {
		return nil, false
	}

	var fired []observer.Anomaly
	pointsSeen := false

	storage.ForEachPoint(ref, startTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
		pointsSeen = true
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
		state.lastProcessedTime = p.Timestamp
		state.lastProcessedValue = p.Value
		pushTimestamp(state, p.Timestamp)

		if !state.warmedUp {
			state.warmupBuf = append(state.warmupBuf, p.Value)
			if len(state.warmupBuf) >= d.WarmupPoints {
				seedLevelTrend(state, d.WarmupPoints)
				// Free the bootstrap buffer — it is not used again.
				state.warmupBuf = nil
				state.warmedUp = true
			}
			return
		}

		anomaly, hasFire := d.processPoint(state, p, agg)

		if hasFire {
			fired = append(fired, anomaly)
		} else if state.refractoryRemaining > 0 {
			// Refractory countdown — decrement on every non-firing post-warmup
			// ingest. The point that arms refractory must not consume one of
			// the configured suppressed points.
			state.refractoryRemaining--
		}
	})

	return fired, pointsSeen
}

func holtCursorPointChanged(storage observer.StorageReader, ref observer.SeriesRef, agg observer.Aggregate, state *holtSeriesState) bool {
	if state.lastProcessedCount == 0 {
		return false
	}
	changed := false
	storage.ForEachPoint(ref, state.lastProcessedTime-1, state.lastProcessedTime, agg, func(_ *observer.Series, p observer.Point) {
		if p.Timestamp == state.lastProcessedTime && p.Value != state.lastProcessedValue {
			changed = true
		}
	})
	return changed
}

// processPoint runs one Holt step: forecast → residual → gate → smoother
// update. Returns the populated Anomaly when the gate fires (and refractory
// is clear); otherwise returns false. The smoother recurrences always
// advance so the level/trend track new regimes through and after fires.
func (d *HoltResidualDetector) processPoint(
	state *holtSeriesState,
	p observer.Point,
	agg observer.Aggregate,
) (observer.Anomaly, bool) {
	// 1. One-step forecast and residual.
	forecast := state.level + state.trend
	residual := p.Value - forecast

	// 2. Standardise residual against the rolling-MAD baseline. We compute
	// the gate values BEFORE pushing the new residual into the window so
	// the standardisation uses the historical baseline, not a window that
	// already contains the candidate point.
	//
	// The MAD floor is range-based: when the window is bimodal (e.g. old
	// regime + new regime during a transition) the median collapses to
	// the larger mode and MAD evaluates to zero — without this floor, any
	// residual would standardise as |z| → ∞, producing spurious
	// post-refractory fires once the smoother is mid-adaptation. Using
	// 5% of the observed value range as a noise floor keeps the threshold
	// proportional to the data's natural scale.
	medianResidual := detectorMedian(state.resWin)
	sigmaResidual := detectorMAD(state.resWin, medianResidual, true)
	sigmaResidual = floorSigma(sigmaResidual, state.resWin)
	z := (residual - medianResidual) / sigmaResidual

	// σ_value gate denominator: rolling MAD over raw values. Same window
	// size, same scaleToSigma=true → MAD ≈ σ. Uses the same range-based
	// floor for the same bimodal-distribution reason.
	medianValue := detectorMedian(state.valWin)
	sigmaValue := detectorMAD(state.valWin, medianValue, true)
	sigmaValue = floorSigma(sigmaValue, state.valWin)
	devMAD := math.Abs(p.Value-state.level) / sigmaValue

	// 3. Update consecutive counters only after both baseline windows are
	// representative. Under-filled windows collapse sigma to the floor and can
	// otherwise pre-arm confirmation before the gate is meaningful.
	windowsReady := len(state.resWin) >= d.ResidualWindow && len(state.valWin) >= d.ResidualWindow
	zMagPasses := windowsReady && math.Abs(z) >= d.ZThreshold
	if zMagPasses {
		if z > 0 {
			state.consecutivePos++
			state.consecutiveNeg = 0
		} else {
			state.consecutiveNeg++
			state.consecutivePos = 0
		}
	} else {
		state.consecutivePos = 0
		state.consecutiveNeg = 0
	}

	confirmed := state.consecutivePos >= d.ConfirmM || state.consecutiveNeg >= d.ConfirmM
	gateOK := windowsReady && zMagPasses && confirmed && devMAD >= d.MinDeviationMAD

	// 4. Smoother update — runs on every post-warmup ingest, regardless of
	// fire / refractory. This is what lets the model adapt through and
	// after a regime shift; the gate decides what to emit, never what to
	// learn.
	newLevel := d.Alpha*p.Value + (1-d.Alpha)*(state.level+state.trend)
	state.trend = d.Beta*(newLevel-state.level) + (1-d.Beta)*state.trend
	state.level = newLevel

	// 5. Decide whether to actually emit a fire.
	fire := gateOK && state.refractoryRemaining == 0

	// 6. Push residual into the MAD window. On fire, replace it with the
	// median of the last N non-fire residuals (Hampel rejection — applied
	// only to the threshold, not the smoothing recurrence). This prevents
	// the very anomaly we just flagged from inflating σ_residual and
	// blinding us to subsequent shifts.
	residualForWindow := residual
	if fire {
		residualForWindow = medianOfTail(state.resWin, holtPostFireSampleN)
	}
	pushFIFO(&state.resWin, d.ResidualWindow, residualForWindow)
	pushFIFO(&state.valWin, d.ResidualWindow, p.Value)

	if !fire {
		return observer.Anomaly{}, false
	}

	// 7. Build the anomaly using the common metric-detector anomaly shape.
	score := math.Abs(z)
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
		Title:        "Holt residual: " + seriesName,
		Description: fmt.Sprintf(
			"%s deviated from forecast (observed=%.4f, forecast=%.4f, residual=%.4f, |z|=%.2f, level=%.4f, trend=%.4f, %.1f valueMADs)",
			seriesName, p.Value, forecast, residual, math.Abs(z), state.level, state.trend, devMAD,
		),
		Timestamp:           state.lastSeenTimestamp,
		Score:               &score,
		SamplingIntervalSec: medianTimestampInterval(state.recentTimestamps),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medianResidual,
			BaselineMAD:    sigmaResidual,
			CurrentValue:   p.Value,
			DeviationSigma: math.Abs(z),
			Threshold:      d.ZThreshold,
		},
	}

	// 8. Reset confirmation counters and arm refractory.
	state.consecutivePos = 0
	state.consecutiveNeg = 0
	state.refractoryRemaining = d.Refractory

	return anomaly, true
}

// seedLevelTrend bootstraps the smoother from the warmup buffer using the
// classic two-half average:
//
//	L_0 = mean(first half), T_0 = (mean(last half) - mean(first half)) / half.
//
// Caller must pass n equal to len(state.warmupBuf) at the boundary; this
// keeps the math obvious even though n is duplicated with d.WarmupPoints.
func seedLevelTrend(state *holtSeriesState, n int) {
	half := n / 2
	if half < 1 {
		// Degenerate config: fall back to single-point seed.
		if n == 1 {
			state.level = state.warmupBuf[0]
			state.trend = 0
		}
		return
	}
	var sumFirst, sumLast float64
	for i := 0; i < half; i++ {
		sumFirst += state.warmupBuf[i]
	}
	for i := n - half; i < n; i++ {
		sumLast += state.warmupBuf[i]
	}
	meanFirst := sumFirst / float64(half)
	meanLast := sumLast / float64(half)
	state.level = meanFirst
	state.trend = (meanLast - meanFirst) / float64(half)
}

// floorSigma returns sigma if it is already meaningful, or a range-based
// noise floor when sigma has collapsed (which happens when the rolling
// window is bimodal: a sample from a transitioning regime can have median
// equal to one mode and MAD equal to zero, since over half the values sit
// exactly on the median). The floor is 5% of the window's observed range,
// bounded by 1e-6 to avoid absolute zero. This keeps the standardised z
// proportional to the data's natural scale during regime transitions.
func floorSigma(sigma float64, win []float64) float64 {
	const minAbsoluteFloor = 1e-6
	const rangeFraction = 0.05
	if sigma >= minAbsoluteFloor && sigma > 0 {
		// Compute the range only when sigma has collapsed below the
		// fraction-of-range threshold; otherwise the MAD itself is the
		// stronger floor.
		if sigma >= rangeFraction*windowRange(win) {
			return sigma
		}
	}
	rangeFloor := rangeFraction * windowRange(win)
	if rangeFloor < minAbsoluteFloor {
		rangeFloor = minAbsoluteFloor
	}
	if sigma > rangeFloor {
		return sigma
	}
	return rangeFloor
}

// windowRange returns max-min over the window. Returns 0 for an empty
// window. O(N) but called inside a hot path that already does O(N log N)
// MAD computations on the same slice, so the cost is dominated.
func windowRange(win []float64) float64 {
	if len(win) == 0 {
		return 0
	}
	minV, maxV := win[0], win[0]
	for _, v := range win[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return maxV - minV
}

// pushFIFO appends v to buf, dropping the oldest entry once buf reaches
// maxLen.
func pushFIFO(buf *[]float64, maxLen int, v float64) {
	if len(*buf) < maxLen {
		*buf = append(*buf, v)
		return
	}
	// FIFO evict: shift left, place new value at tail.
	copy(*buf, (*buf)[1:])
	(*buf)[maxLen-1] = v
}

// pushTimestamp appends ts to the recent-timestamp ring, dropping the
// oldest if full. The ring is small (cap holtTimestampRing) so the
// shift-left cost is negligible.
func pushTimestamp(state *holtSeriesState, ts int64) {
	if cap(state.recentTimestamps) == 0 {
		state.recentTimestamps = make([]int64, 0, holtTimestampRing)
	}
	if len(state.recentTimestamps) < cap(state.recentTimestamps) {
		state.recentTimestamps = append(state.recentTimestamps, ts)
		return
	}
	copy(state.recentTimestamps, state.recentTimestamps[1:])
	state.recentTimestamps[len(state.recentTimestamps)-1] = ts
}

// medianOfTail returns the median of the last n entries of buf. If buf has
// fewer than n entries, uses everything available. Returns 0 on an empty
// buf — the caller (post-fire path) will then push a zero residual, which
// is a sane neutral value: it does not bias the threshold up or down.
func medianOfTail(buf []float64, n int) float64 {
	if len(buf) == 0 {
		return 0
	}
	if n > len(buf) {
		n = len(buf)
	}
	tail := buf[len(buf)-n:]
	return detectorMedian(tail)
}

// ensureDefaults fills zero-valued config fields with sensible defaults so the
// detector behaves sanely when constructed via reflective paths that bypass
// NewHoltResidualDetector.
func (d *HoltResidualDetector) ensureDefaults() {
	if d.Alpha <= 0 {
		d.Alpha = holtAlpha
	}
	if d.Beta <= 0 {
		d.Beta = holtBeta
	}
	if d.WarmupPoints <= 0 {
		d.WarmupPoints = holtWarmupPoints
	}
	if d.ResidualWindow <= 0 {
		d.ResidualWindow = holtResidualWindow
	}
	if d.ZThreshold <= 0 {
		d.ZThreshold = holtZThreshold
	}
	if d.ConfirmM <= 0 {
		d.ConfirmM = holtConfirmM
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = holtMinDeviationMAD
	}
	if d.Refractory <= 0 {
		d.Refractory = holtRefractory
	}
	if d.series == nil {
		d.series = make(map[holtStateKey]*holtSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
