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

// Streaming seasonal-trend (STL-style) residual detector.
//
// Per (series, aggregation), maintain a rolling per-phase median table that
// stands in for the seasonal component and a rolling MAD window over the
// deseasonalised residuals. Each new point at logical phase
// p = pointIndex mod period subtracts seasonalIdx[p] from the raw value; an
// anomaly fires when the standardised residual |z_t| crosses the threshold
// AND the same sign repeats for ConfirmM consecutive points AND |r_t| /
// MAD(values) clears the effect-size gate. A short refractory period
// suppresses repeat fires while neighbouring residuals settle.
//
// This complements the level-shift, drift, and forecast-residual detectors:
// none of them subtract a learned seasonal cycle, so a perfectly periodic
// baseline (e.g. a daily traffic pattern) shows up as "noise" to them and
// drowns small anomalies. This detector cancels the periodic component
// first and then runs the same robust gate the rest of the family uses.
//
// Period auto-detection runs once at warmup boundary via biased ACF on
// lags MinPeriod..MaxPeriod. If no lag clears MinACF, the series is flagged
// nonseasonal and the detector degenerates to a robust-z gate on raw
// values (seasonalIdx == nil, s_p ≡ 0).
//
// Memory per (series, aggregation): warmup buffer (240 floats, freed at
// boundary), seasonalIdx (≤120 floats), phaseRings (≤120·6 = 720 floats),
// resWin/valWin (60 floats each), plus scalars — ≤8 KB.
// Per-tick cost: O(W log W) MAD recomputes (W=60), dominated by the two
// sort.Float64s calls inside detectorMAD. Period auto-detection is a
// one-shot O(N · (MaxPeriod-MinPeriod)) at warmup boundary, ~28K ops.

// STL tunable defaults.
const (
	stlWarmupPoints    = 240
	stlMinACF          = 0.3
	stlMinPeriod       = 4
	stlMaxPeriod       = 120
	stlPerPhaseHistory = 6
	stlResidualWindow  = 60
	stlZThreshold      = 4.5
	stlConfirmM        = 2
	stlMinDeviationMAD = 3.0
	stlRefractory      = 20
	stlTimestampRing   = 16
	stlPostFireSampleN = 5
)

// stlStateKey identifies per-series state by ref and aggregation.
type stlStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// stlSeriesState holds the streaming state for one (series, aggregation).
//
// Lifecycle: warmup → (seasonal | nonseasonal). During warmup we only
// accumulate raw values in warmupBuf; at the boundary the ACF auto-detect
// runs once, allocating seasonalIdx + phaseRings if a period is found.
// Post-warmup, the gate runs on every ingested point.
type stlSeriesState struct {
	// cursor (mirrors holt_residual / kl_divergence)
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// warmup buffer; cap = WarmupPoints. Discarded once warmedUp.
	warmupBuf []float64
	warmedUp  bool

	// period detected by ACF at warmup boundary; 0 ⇒ nonseasonal fallback.
	period int

	// pointIndex is a strictly monotonic count of ingested points (warmup
	// AND post-warmup). Used to derive the logical phase as pointIndex %
	// period. Tracking it through warmup keeps phase consistent with the
	// values used to seed seasonalIdx.
	pointIndex int

	// seasonalIdx[p] is the rolling median of recent values seen at phase p.
	// Length = period (nil when nonseasonal).
	seasonalIdx []float64

	// phaseRings[p] is a FIFO of the most-recent stlPerPhaseHistory values
	// observed at phase p. Used to recompute seasonalIdx[p] cheaply on each
	// non-fire ingest. Length = period (nil when nonseasonal).
	phaseRings [][]float64

	// FIFO rolling windows; cap = ResidualWindow. resWin holds the last W
	// deseasonalised residuals; valWin holds the last W raw values for the
	// effect-size gate.
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
	// cadence for downstream correlators.
	recentTimestamps  []int64
	lastSeenTimestamp int64
}

// STLSeasonalDetector fires when a deseasonalised residual exceeds a robust
// MAD-scaled threshold, with consecutive-confirmation and effect-size gates.
//
// Implements observer.Detector and observer.SeriesRemover. Tunable fields
// are exported so callers (testbench, tests) may override defaults after
// construction.
type STLSeasonalDetector struct {
	// WarmupPoints is the number of points collected before period auto-
	// detection runs. Must be >= 2 * MaxPeriod for a robust ACF estimate.
	WarmupPoints int
	// MinACF is the minimum biased autocorrelation required for a candidate
	// lag to be accepted as the period.
	MinACF float64
	// MinPeriod, MaxPeriod bound the lag search range for ACF.
	MinPeriod, MaxPeriod int
	// PerPhaseHistory is the per-phase ring buffer size whose median seeds
	// seasonalIdx[p]. Larger ⇒ more robust but slower to track regime
	// changes in the seasonal pattern.
	PerPhaseHistory int
	// ResidualWindow is the rolling FIFO size for residual MAD and value MAD.
	ResidualWindow int
	// ZThreshold is the standardised-residual gate.
	ZThreshold float64
	// ConfirmM is the minimum number of consecutive same-sign |z|>=threshold
	// observations required to fire.
	ConfirmM int
	// MinDeviationMAD is the minimum |r_t| / MAD(values) for the effect-size
	// gate to pass. Mirrors holt_residual / kl_divergence.
	MinDeviationMAD float64
	// Refractory is the number of ingested points to suppress fires for
	// after firing.
	Refractory int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[stlStateKey]*stlSeriesState

	// cache the discovered series list across Detect calls (mirrors the
	// scanwelch / scanmw / kl / holt pattern).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewSTLSeasonalDetector creates an STL-seasonal detector with default
// settings. The catalog factory calls this with no arguments.
func NewSTLSeasonalDetector() *STLSeasonalDetector {
	return &STLSeasonalDetector{
		WarmupPoints:    stlWarmupPoints,
		MinACF:          stlMinACF,
		MinPeriod:       stlMinPeriod,
		MaxPeriod:       stlMaxPeriod,
		PerPhaseHistory: stlPerPhaseHistory,
		ResidualWindow:  stlResidualWindow,
		ZThreshold:      stlZThreshold,
		ConfirmM:        stlConfirmM,
		MinDeviationMAD: stlMinDeviationMAD,
		Refractory:      stlRefractory,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[stlStateKey]*stlSeriesState),
	}
}

// Name implements observer.Detector.
func (d *STLSeasonalDetector) Name() string { return "stl_seasonal" }

// Reset clears all per-series state for replay/reanalysis.
func (d *STLSeasonalDetector) Reset() {
	d.series = make(map[stlStateKey]*stlSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Mirrors HoltResidualDetector.RemoveSeries: each entry holds rolling
// buffers, so without the teardown the map grows unbounded with the
// cumulative series count even after storage shrinks.
func (d *STLSeasonalDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, stlStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Streams new points into per-series
// STL state and emits anomalies when all gates pass.
func (d *STLSeasonalDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := stlStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip when nothing has been written and the
			// cursor matches. The detector is per-point, so any non-zero
			// delta is meaningful.
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
// buffers. Splitting allocation here keeps Detect's hot path branch-free.
func (d *STLSeasonalDetector) newState() *stlSeriesState {
	return &stlSeriesState{
		warmupBuf:        make([]float64, 0, d.WarmupPoints),
		resWin:           make([]float64, 0, d.ResidualWindow),
		valWin:           make([]float64, 0, d.ResidualWindow),
		recentTimestamps: make([]int64, 0, stlTimestampRing),
	}
}

// ingestNewPoints streams points in (state.lastProcessedTime, dataTime] into
// the per-series STL state. Returns any anomalies fired by the gate logic.
//
// Lifecycle:
//   - During warmup, points accumulate in warmupBuf. When the buffer fills
//     to WarmupPoints, period auto-detection runs and (if successful) the
//     per-phase median table is seeded from the buffer; warmedUp flips.
//   - Post-warmup, each point produces a residual; the gate decides whether
//     to fire. Phase-ring updates always advance on non-fire ingests so the
//     seasonal estimate tracks the current cycle.
func (d *STLSeasonalDetector) ingestNewPoints(
	storage observer.StorageReader,
	ref observer.SeriesRef,
	agg observer.Aggregate,
	state *stlSeriesState,
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
		pushTimestampSTL(state, p.Timestamp)

		if !state.warmedUp {
			state.warmupBuf = append(state.warmupBuf, p.Value)
			state.pointIndex++
			if len(state.warmupBuf) >= d.WarmupPoints {
				d.seedFromWarmup(state)
				// Free the bootstrap buffer — it is not used again.
				state.warmupBuf = nil
				state.warmedUp = true
			}
			return
		}

		anomaly, hasFire := d.processPoint(state, p, agg)

		// Refractory countdown — decrement on every post-warmup ingest,
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

// seedFromWarmup runs ACF auto-detection over warmupBuf and, on success,
// allocates seasonalIdx + phaseRings and seeds them from the buffer.
//
// On nonseasonal fallback, seasonalIdx and phaseRings remain nil; the
// post-warmup processPoint treats s_p ≡ 0, so the detector becomes a
// straight robust-z gate on raw values.
func (d *STLSeasonalDetector) seedFromWarmup(state *stlSeriesState) {
	period := autoDetectPeriod(state.warmupBuf, d.MinPeriod, d.MaxPeriod, d.MinACF)
	if period <= 0 {
		// Nonseasonal fallback — seasonalIdx and phaseRings stay nil.
		state.period = 0
		return
	}
	state.period = period
	state.seasonalIdx = make([]float64, period)
	state.phaseRings = make([][]float64, period)
	for i := 0; i < period; i++ {
		state.phaseRings[i] = make([]float64, 0, d.PerPhaseHistory)
	}
	// Seed phase rings from the warmup buffer in ingest order. pointIndex
	// has been incremented through warmup, so warmupBuf[i] was observed at
	// phase i % period (matching the post-warmup phase derivation).
	for i, v := range state.warmupBuf {
		phase := i % period
		pushFIFO(&state.phaseRings[phase], d.PerPhaseHistory, v)
	}
	for p := 0; p < period; p++ {
		state.seasonalIdx[p] = detectorMedian(state.phaseRings[p])
	}
}

// processPoint runs one post-warmup step: phase lookup → residual → gate →
// phase-ring + window updates. Returns the populated Anomaly when the gate
// fires (and refractory is clear); otherwise (Anomaly{}, false). The
// residual + value windows always advance so MAD baselines track the
// current regime.
func (d *STLSeasonalDetector) processPoint(
	state *stlSeriesState,
	p observer.Point,
	agg observer.Aggregate,
) (observer.Anomaly, bool) {
	// 1. Phase lookup and deseasonalised residual.
	var phase int
	var seasonal float64
	if state.period > 0 {
		phase = state.pointIndex % state.period
		seasonal = state.seasonalIdx[phase]
	}
	residual := p.Value - seasonal

	// 2. Standardise residual against the rolling-MAD baseline. Compute
	// gates BEFORE pushing the new residual into the window so the
	// standardisation uses the historical baseline, not a window that
	// already contains the candidate point.
	medianResidual := detectorMedian(state.resWin)
	sigmaResidual := detectorMAD(state.resWin, medianResidual, true)
	sigmaResidual = floorSigma(sigmaResidual, state.resWin)
	z := (residual - medianResidual) / sigmaResidual

	// σ_value gate denominator: rolling MAD over raw values. Same window
	// size, same scaleToSigma=true. Same range-based floor for the same
	// bimodal-distribution reason as in holt_residual.
	medianValue := detectorMedian(state.valWin)
	sigmaValue := detectorMAD(state.valWin, medianValue, true)
	sigmaValue = floorSigma(sigmaValue, state.valWin)
	devMAD := math.Abs(residual) / sigmaValue

	// 3. Update consecutive counters BEFORE deciding to fire — the fire
	// requires the counter to reach ConfirmM, so the current point counts.
	zMagPasses := math.Abs(z) >= d.ZThreshold
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
	// The MAD windows must be representative before firing: an under-filled
	// window collapses sigma to the floor and turns every modest residual
	// into a "huge" z. Mirrors kl_divergence / holt_residual.
	windowsReady := len(state.resWin) >= d.ResidualWindow && len(state.valWin) >= d.ResidualWindow
	gateOK := windowsReady && zMagPasses && confirmed && devMAD >= d.MinDeviationMAD

	// 4. Decide whether to actually emit a fire.
	fire := gateOK && state.refractoryRemaining == 0

	// 5. Push residual into the MAD window. On fire, replace it with the
	// median of the last N non-fire residuals (Hampel rejection) so the
	// flagged anomaly does not inflate σ_residual on subsequent points.
	residualForWindow := residual
	if fire {
		residualForWindow = medianOfTail(state.resWin, stlPostFireSampleN)
	}
	pushFIFO(&state.resWin, d.ResidualWindow, residualForWindow)
	pushFIFO(&state.valWin, d.ResidualWindow, p.Value)

	// 6. Update the per-phase ring + seasonalIdx[p]. On fire, skip the
	// update (Hampel-style rejection on the seasonal estimate) so the
	// anomaly does not contaminate seasonalIdx[p].
	if state.period > 0 && !fire {
		pushFIFO(&state.phaseRings[phase], d.PerPhaseHistory, p.Value)
		state.seasonalIdx[phase] = detectorMedian(state.phaseRings[phase])
	}

	// 7. Advance pointIndex on every ingest so phase derivation stays
	// monotone regardless of fire/refractory.
	state.pointIndex++

	if !fire {
		return observer.Anomaly{}, false
	}

	// 8. Build the anomaly.
	score := math.Abs(z)
	seriesName := state.seriesName + ":" + aggSuffix(agg)
	periodLabel := "nonseasonal"
	if state.period > 0 {
		periodLabel = fmt.Sprintf("period=%d phase=%d", state.period, phase)
	}
	anomaly := observer.Anomaly{
		Type: observer.AnomalyTypeMetric,
		Source: observer.SeriesDescriptor{
			Namespace: state.seriesNamespace,
			Name:      state.seriesName,
			Tags:      state.seriesTags,
			Aggregate: agg,
		},
		DetectorName: d.Name(),
		Title:        "STL seasonal: " + seriesName,
		Description: fmt.Sprintf(
			"%s deviated from seasonal baseline (observed=%.4f, seasonal=%.4f, residual=%.4f, |z|=%.2f, %.1f valueMADs, %s)",
			seriesName, p.Value, seasonal, residual, math.Abs(z), devMAD, periodLabel,
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

	// 9. Reset confirmation counters and arm refractory.
	state.consecutivePos = 0
	state.consecutiveNeg = 0
	state.refractoryRemaining = d.Refractory

	return anomaly, true
}

// autoDetectPeriod returns the lag k in [minLag, maxLag] with the largest
// biased autocorrelation r(k), provided r(k) >= minACF. Returns 0 when no
// lag clears the threshold (nonseasonal).
//
// Biased ACF:
//
//	r(k) = Σ_{i=k..n-1}(x_i - μ)(x_{i-k} - μ) / Σ_{i=0..n-1}(x_i - μ)²
//
// "Biased" means we divide by the same total-variance denominator at every
// lag, which is the conventional choice for streaming period auto-detection
// — it shrinks small-sample ACF estimates toward zero, making the threshold
// less sensitive to spurious peaks at high lags.
func autoDetectPeriod(buf []float64, minLag, maxLag int, minACF float64) int {
	n := len(buf)
	if n < 2*minLag || minLag < 1 || maxLag < minLag {
		return 0
	}
	if maxLag > n/2 {
		maxLag = n / 2
	}
	// Mean.
	var sum float64
	for _, v := range buf {
		sum += v
	}
	mu := sum / float64(n)
	// Total variance (denominator).
	var denom float64
	for _, v := range buf {
		d := v - mu
		denom += d * d
	}
	// Constant (or numerically-constant) series — no seasonality to find.
	// The 1e-12 floor guards against floating-point rounding in the mean
	// computation: a slice of identical values can produce a non-zero but
	// vanishingly small denom, which would otherwise let r(k) pick up
	// spurious "structure" from rounding noise.
	if denom <= 1e-12 {
		return 0
	}
	bestLag := 0
	bestR := -math.MaxFloat64
	for k := minLag; k <= maxLag; k++ {
		var num float64
		for i := k; i < n; i++ {
			num += (buf[i] - mu) * (buf[i-k] - mu)
		}
		r := num / denom
		if r > bestR {
			bestR = r
			bestLag = k
		}
	}
	if bestR < minACF {
		return 0
	}
	return bestLag
}

// pushTimestampSTL is the stl-state variant of pushTimestamp; same FIFO
// shape as the holt / matrix-profile / evt versions.
func pushTimestampSTL(state *stlSeriesState, ts int64) {
	if cap(state.recentTimestamps) == 0 {
		state.recentTimestamps = make([]int64, 0, stlTimestampRing)
	}
	if len(state.recentTimestamps) < cap(state.recentTimestamps) {
		state.recentTimestamps = append(state.recentTimestamps, ts)
		return
	}
	copy(state.recentTimestamps, state.recentTimestamps[1:])
	state.recentTimestamps[len(state.recentTimestamps)-1] = ts
}

// ensureDefaults fills zero-valued config fields with sensible defaults.
// Mirrors holt_residual / kl_divergence ensureDefaults so the detector
// behaves sanely even when constructed via reflective paths that bypass
// NewSTLSeasonalDetector.
func (d *STLSeasonalDetector) ensureDefaults() {
	if d.WarmupPoints <= 0 {
		d.WarmupPoints = stlWarmupPoints
	}
	if d.MinACF <= 0 {
		d.MinACF = stlMinACF
	}
	if d.MinPeriod <= 0 {
		d.MinPeriod = stlMinPeriod
	}
	if d.MaxPeriod <= 0 {
		d.MaxPeriod = stlMaxPeriod
	}
	if d.PerPhaseHistory <= 0 {
		d.PerPhaseHistory = stlPerPhaseHistory
	}
	if d.ResidualWindow <= 0 {
		d.ResidualWindow = stlResidualWindow
	}
	if d.ZThreshold <= 0 {
		d.ZThreshold = stlZThreshold
	}
	if d.ConfirmM <= 0 {
		d.ConfirmM = stlConfirmM
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = stlMinDeviationMAD
	}
	if d.Refractory <= 0 {
		d.Refractory = stlRefractory
	}
	if d.series == nil {
		d.series = make(map[stlStateKey]*stlSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
