// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// GLR mean-and-variance changepoint detector.
//
// Per (series, aggregation), maintain a sliding window of W=60 most recent
// raw values. Each new point recomputes the maximum joint-mean-and-variance
// log-likelihood ratio over all internal split points k in
// [Wmin, W-Wmin], comparing the two-segment Gaussian MLE likelihood to the
// single-segment MLE likelihood. An anomaly fires when 2*LR_max crosses a
// chi-squared(2 dof) critical threshold AND a same-sign consecutive
// confirmation passes AND a robust effect-size MAD gate passes — mirroring
// holt_residual's three-gate structure to keep house style consistent.
//
// The gate triplet plays specific roles:
//   - LR threshold: the hypothesis test (Chen-Gupta joint LR, 2 dof per split).
//   - Consecutive confirmation: filters single-window false positives, which
//     under correlated sliding windows are far more frequent than the per-
//     window FP rate alone would suggest.
//   - MinDeviationMAD effect size: blocks "statistically significant but
//     practically tiny" mean shifts that win the LR by virtue of var_full
//     collapsing rather than by a meaningful regime change.
//
// Memory per (series, aggregation): one 60-value FIFO window + a 16-entry
// timestamp ring + scalars ~ 600 B. Per-tick cost: O(W) prefix-sum pass +
// (W - 2*Wmin) ~ 40 branch-free split-point evaluations. The O(W log W)
// MAD computation runs only when LR clears the threshold, so quiet streams
// pay just the prefix-sum + scan cost.

// GLR tunable defaults — see the const block for rationale on the threshold
// (chi-squared 2 dof critical at p~1.2e-4) and the refractory length (matches
// holt_residual at 20 ticks).
const (
	glrWindow          = 60
	glrMinSegment      = 10
	glrLRThreshold     = 18.0
	glrConfirmM        = 2
	glrMinDeviationMAD = 3.0
	glrRefractory      = 20
	// glrTimestampRing matches the holt/spectral pattern: a 16-entry ring is
	// large enough for a robust median-of-deltas and small enough that the
	// shift-on-evict cost is negligible.
	glrTimestampRing = 16
	// glrVarianceFloor caps the log-likelihood explosion when an MLE segment
	// variance collapses to zero (constant or near-constant slice). Using the
	// same floor for var_full and the per-segment vars keeps the LR statistic
	// finite when one side is degenerate without biasing toward LR=0.
	glrVarianceFloor = 1e-12
)

// glrStateKey identifies per-series state by ref + aggregation.
type glrStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// glrSeriesState holds the streaming state for one (series, aggregation).
//
// Lifecycle: window fills FIFO until len(window) == Window (warmup); from
// then on each ingested point triggers a prefix-sum pass + split-point scan
// + 3-gate decision. consecutivePos / consecutiveNeg latch same-sign LR
// passes for the consecutive-confirmation gate; refractoryRemaining
// suppresses fires for Refractory ticks after the most recent fire.
type glrSeriesState struct {
	// cursor (mirrors holt_residual exactly)
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// sliding window of the last W raw values, FIFO.
	window []float64

	// detection counters (mirror holt_residual)
	consecutivePos      int
	consecutiveNeg      int
	refractoryRemaining int

	// captured series metadata (first non-empty observation suffices)
	seriesMetaCaptured bool
	seriesNamespace    string
	seriesName         string
	seriesTags         []string

	// recentTimestamps is a small ring of the most recently ingested point
	// timestamps; medianTimestampInterval reads it to estimate the sampling
	// cadence for downstream correlators.
	recentTimestamps  []int64
	lastSeenTimestamp int64
}

// GLRMeanVarianceDetector implements observer.Detector and observer.SeriesRemover.
//
// Tunable fields are exported so callers (testbench, tests) may override
// defaults after construction; NewGLRMeanVarianceDetector populates them.
// Mirrors the holt_residual / kl_divergence post-construction-override
// pattern, so the catalog entry needs no defaultConfig / parseJSON.
type GLRMeanVarianceDetector struct {
	// Window is the sliding-window size W in points.
	Window int
	// MinSegment is the minimum segment length Wmin on either side of a split.
	// Must satisfy 1 <= MinSegment <= Window/2.
	MinSegment int
	// LRThreshold is the gate value for 2*LR_max. The default 18.0 corresponds
	// to chi-squared(2 dof) critical at p ~ 1.2e-4.
	LRThreshold float64
	// ConfirmM is the minimum number of consecutive same-sign LR-passing
	// observations required to fire.
	ConfirmM int
	// MinDeviationMAD is the minimum |mean_right - mean_left| / MAD-sigma for
	// the effect-size gate to pass.
	MinDeviationMAD float64
	// Refractory is the number of ingested points to suppress fires for after
	// firing, matching the natural "regime is still in the window" length.
	Refractory int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[glrStateKey]*glrSeriesState

	// cached series list across Detect calls (mirrors the holt / kl pattern).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewGLRMeanVarianceDetector constructs a detector with default tunables.
func NewGLRMeanVarianceDetector() *GLRMeanVarianceDetector {
	return &GLRMeanVarianceDetector{
		Window:          glrWindow,
		MinSegment:      glrMinSegment,
		LRThreshold:     glrLRThreshold,
		ConfirmM:        glrConfirmM,
		MinDeviationMAD: glrMinDeviationMAD,
		Refractory:      glrRefractory,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[glrStateKey]*glrSeriesState),
	}
}

// Name implements observer.Detector.
func (d *GLRMeanVarianceDetector) Name() string { return "glr_mean_variance" }

// Reset clears all per-series state for replay/reanalysis.
func (d *GLRMeanVarianceDetector) Reset() {
	d.series = make(map[glrStateKey]*glrSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Mirrors HoltResidualDetector.RemoveSeries: each entry holds rolling
// buffers, so without the teardown the map grows unbounded with the
// cumulative series count even after storage shrinks.
func (d *GLRMeanVarianceDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, glrStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Streams new points into per-series
// GLR state and emits anomalies when all three gates pass.
//
// Structure mirrors HoltResidualDetector.Detect: cache the series list
// across calls, batch the per-series status under a single lock, replay-
// gate quiet streams, then ingest new points per (ref, agg).
func (d *GLRMeanVarianceDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := glrStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip when nothing has been written and the
			// cursor matches. Mirrors holt_residual: per-point detector,
			// no minimum batch size required.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			anomalies := d.ingestNewPoints(storage, meta.Ref, agg, state, dataTime)
			for j := range anomalies {
				anomalies[j].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}
			allAnomalies = append(allAnomalies, anomalies...)

			// Update cursor unconditionally — a quiet series should not
			// keep replaying through the gate next call.
			state.lastProcessedCount = status.pointCount
			state.lastProcessedTime = dataTime
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// newState allocates a per-series state with appropriately sized scratch
// buffers. Splitting allocation here keeps Detect's hot path branch-free.
func (d *GLRMeanVarianceDetector) newState() *glrSeriesState {
	return &glrSeriesState{
		window:           make([]float64, 0, d.Window),
		recentTimestamps: make([]int64, 0, glrTimestampRing),
	}
}

// ingestNewPoints streams points in (state.lastProcessedTime, dataTime] into
// the per-series GLR state. Returns any anomalies fired by the gate logic.
//
// Lifecycle:
//   - During warmup, points accumulate in the FIFO window. Once the window
//     is full, every subsequent ingested point triggers a scan.
//   - The refractory counter decrements on every post-warmup ingest,
//     regardless of whether the point would have fired — mirrors
//     HoltResidualDetector.ingestNewPoints.
func (d *GLRMeanVarianceDetector) ingestNewPoints(
	storage observer.StorageReader,
	ref observer.SeriesRef,
	agg observer.Aggregate,
	state *glrSeriesState,
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
		pushTimestampGLR(state, p.Timestamp)

		// Slide the new value into the FIFO window. pushFIFO is the same
		// helper used by holt_residual — O(W) shift, dominated by the
		// downstream prefix-sum pass for W=60.
		pushFIFO(&state.window, d.Window, p.Value)
		if len(state.window) < d.Window {
			// Still warming the window — no scan possible yet, no
			// refractory to drain.
			return
		}

		anomaly, hasFire := d.processPoint(state, p, agg)

		// Refractory countdown — decrement on every post-warmup ingest,
		// regardless of whether the point would have fired. processPoint
		// re-arms it on a fire BEFORE this decrement, so the value seen
		// by the next tick is Refractory-1 (one tick already consumed by
		// the firing tick itself).
		if state.refractoryRemaining > 0 {
			state.refractoryRemaining--
		}

		if hasFire {
			fired = append(fired, anomaly)
		}
	})

	return fired
}

// processPoint runs one GLR scan over the current 60-point window and
// returns a populated anomaly + true when all three gates pass and
// refractory has cleared.
//
// Algorithm:
//  1. Pre-compute prefix sums of x and x^2 in a single O(W) pass.
//  2. Compute the single-segment MLE log-likelihood ll_full (additive
//     constants dropped — they cancel in the LR).
//  3. Scan internal split points k in [MinSegment, W-MinSegment]; for each
//     k, derive (mean_left, var_left, mean_right, var_right) from the
//     prefix sums in O(1) and compute the two-segment ll_split. Track
//     LR_max = ll_split - ll_full and the corresponding k_star.
//  4. Apply the three-gate decision:
//     2*LR_max >= LRThreshold (likelihood gate),
//     consecutive-same-sign count >= ConfirmM (correlation gate),
//     |mean_right - mean_left| / MAD-sigma >= MinDeviationMAD (effect-size gate).
//  5. Emit if gateOK && refractoryRemaining == 0; reset confirmation
//     counters and arm refractory after a fire.
func (d *GLRMeanVarianceDetector) processPoint(
	state *glrSeriesState,
	p observer.Point,
	agg observer.Aggregate,
) (observer.Anomaly, bool) {
	win := state.window
	n := len(win)

	// 1. Prefix sums. sumX[i] = sum(win[0..i-1]); sumX2[i] = sum(win[0..i-1]^2).
	// One O(W) allocation per scan — trivial at W=60. If profiling ever
	// shows this on a hot stream's flame graph, the slices can be moved to
	// glrSeriesState as reusable scratch buffers.
	sumX := make([]float64, n+1)
	sumX2 := make([]float64, n+1)
	for i, v := range win {
		sumX[i+1] = sumX[i] + v
		sumX2[i+1] = sumX2[i] + v*v
	}

	// 2. Single-segment MLE log-likelihood (additive constants dropped).
	fW := float64(n)
	meanFull := sumX[n] / fW
	varFull := sumX2[n]/fW - meanFull*meanFull
	if varFull < glrVarianceFloor {
		varFull = glrVarianceFloor
	}
	llFull := -fW / 2 * math.Log(varFull)

	// 3. Scan internal split points and track the maximum LR.
	minSeg := d.MinSegment
	if minSeg < 1 {
		minSeg = 1
	}
	lrMax := 0.0
	kStar := -1
	var meanLeftStar, meanRightStar, varLeftStar, varRightStar float64
	for k := minSeg; k <= n-minSeg; k++ {
		n1 := float64(k)
		n2 := float64(n - k)
		s1L := sumX[k]
		s2L := sumX2[k]
		s1R := sumX[n] - s1L
		s2R := sumX2[n] - s2L
		meanL := s1L / n1
		meanR := s1R / n2
		varL := s2L/n1 - meanL*meanL
		varR := s2R/n2 - meanR*meanR
		if varL < glrVarianceFloor {
			varL = glrVarianceFloor
		}
		if varR < glrVarianceFloor {
			varR = glrVarianceFloor
		}
		ll := -n1/2*math.Log(varL) - n2/2*math.Log(varR)
		lr := ll - llFull
		if lr > lrMax {
			lrMax = lr
			kStar = k
			meanLeftStar = meanL
			meanRightStar = meanR
			varLeftStar = varL
			varRightStar = varR
		}
	}

	// LR is mathematically non-negative (the two-segment MLE always fits
	// the data at least as well as the single-segment one), but a
	// pathological all-equal window with kStar never updated leaves the
	// score at exactly 0 and the gate trivially fails — that is the
	// desired behavior. The 2x scale converts to chi-squared statistic.
	score := 2 * lrMax
	lrPasses := score >= d.LRThreshold

	// 4. Sign of the change for consecutive same-sign confirmation. We
	// use the best-split mean comparison; ties (no detectable sign) do
	// not advance either counter.
	sign := 0
	if lrPasses && kStar >= 0 {
		switch {
		case meanRightStar > meanLeftStar:
			sign = 1
		case meanRightStar < meanLeftStar:
			sign = -1
		}
	}

	// 5. Update consecutive counters BEFORE deciding to fire — the fire
	// requires the counter to reach ConfirmM, so the current point counts.
	// Mirrors HoltResidualDetector.processPoint's counter update order.
	if lrPasses && sign != 0 {
		if sign > 0 {
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

	// Short-circuit before the O(W log W) MAD computation when the early
	// gates already failed — this is the common path on quiet streams.
	if !lrPasses || !confirmed {
		return observer.Anomaly{}, false
	}

	// 6. Effect-size MAD gate over the raw value window. floorSigma is
	// reused from holt_residual: a bimodal transition window can collapse
	// MAD to zero, which without a floor would make every nonzero
	// |Δmean| pass and produce spurious post-refractory fires once the
	// scan re-arms.
	medianValue := detectorMedian(win)
	sigmaValue := floorSigma(detectorMAD(win, medianValue, true), win)
	devMAD := math.Abs(meanRightStar-meanLeftStar) / sigmaValue

	if devMAD < d.MinDeviationMAD {
		return observer.Anomaly{}, false
	}

	// 7. Refractory gate.
	if state.refractoryRemaining > 0 {
		return observer.Anomaly{}, false
	}

	// 8. Build the anomaly. Shape mirrors holt_residual / kl_divergence so
	// downstream correlators see a consistent payload across detectors.
	seriesName := state.seriesName + ":" + aggSuffix(agg)
	scoreCopy := score
	anomaly := observer.Anomaly{
		Type: observer.AnomalyTypeMetric,
		Source: observer.SeriesDescriptor{
			Namespace: state.seriesNamespace,
			Name:      state.seriesName,
			Tags:      state.seriesTags,
			Aggregate: agg,
		},
		DetectorName: d.Name(),
		Title:        "GLR mean/var shift: " + seriesName,
		Description: fmt.Sprintf(
			"%s changepoint at k=%d (mean_left=%.4f, mean_right=%.4f, var_left=%.4g, var_right=%.4g, 2*LR=%.2f, %.1f valueMADs)",
			seriesName, kStar, meanLeftStar, meanRightStar, varLeftStar, varRightStar, score, devMAD,
		),
		Timestamp:           state.lastSeenTimestamp,
		Score:               &scoreCopy,
		SamplingIntervalSec: medianTimestampInterval(state.recentTimestamps),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medianValue,
			BaselineMAD:    sigmaValue,
			CurrentValue:   p.Value,
			DeviationSigma: devMAD,
			Threshold:      d.LRThreshold,
		},
	}

	// 9. Reset confirmation counters and arm refractory.
	state.consecutivePos = 0
	state.consecutiveNeg = 0
	state.refractoryRemaining = d.Refractory

	return anomaly, true
}

// pushTimestampGLR mirrors HoltResidualDetector.pushTimestamp but typed for
// *glrSeriesState. Each detector keeps its own variant because Go does not
// let us reuse the helper across distinct state structs without an
// interface or generics, and the cost of a tiny per-detector copy is
// outweighed by avoiding a hot-path interface dispatch.
func pushTimestampGLR(state *glrSeriesState, ts int64) {
	if cap(state.recentTimestamps) == 0 {
		state.recentTimestamps = make([]int64, 0, glrTimestampRing)
	}
	if len(state.recentTimestamps) < cap(state.recentTimestamps) {
		state.recentTimestamps = append(state.recentTimestamps, ts)
		return
	}
	copy(state.recentTimestamps, state.recentTimestamps[1:])
	state.recentTimestamps[len(state.recentTimestamps)-1] = ts
}

// ensureDefaults populates zero-valued config fields with sensible defaults.
// Mirrors HoltResidualDetector.ensureDefaults so the detector behaves sanely
// even when constructed via reflective paths that bypass NewGLRMeanVarianceDetector.
func (d *GLRMeanVarianceDetector) ensureDefaults() {
	if d.Window <= 0 {
		d.Window = glrWindow
	}
	if d.MinSegment <= 0 {
		d.MinSegment = glrMinSegment
	}
	// Guard against a misconfigured MinSegment that would invert the scan
	// loop bounds: minSeg > Window/2 leaves no valid k. We clamp rather
	// than panic so a too-aggressive override degrades gracefully.
	if d.MinSegment > d.Window/2 {
		d.MinSegment = d.Window / 2
	}
	if d.LRThreshold <= 0 {
		d.LRThreshold = glrLRThreshold
	}
	if d.ConfirmM <= 0 {
		d.ConfirmM = glrConfirmM
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = glrMinDeviationMAD
	}
	if d.Refractory <= 0 {
		d.Refractory = glrRefractory
	}
	if d.series == nil {
		d.series = make(map[glrStateKey]*glrSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
