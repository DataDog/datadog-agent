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

// DFA / Hurst-exponent detector tunables.
const (
	// dfaWindowSize is the rolling window length over which the cumulative
	// profile and per-scale fluctuations are computed. 256 is the smallest
	// power of two that gives at least 4 cleanly-spaced log-scale points
	// (16, 32, 64, 128) under the constraint that each scale evenly divides
	// the window — the closed-form OLS regression assumes exact partitioning.
	dfaWindowSize = 256

	// dfaBaselineWarmup is the number of scoring ticks that must accumulate
	// into the baseline EWMA before the detector can fire. Mirrors the
	// "no-fire-during-warmup" gate used by tukey_biweight / grubbs_loo, with
	// a count chosen so the baseline EWMA reaches steady state before any
	// |H_now - H_baseline| comparison is trusted.
	dfaBaselineWarmup = 64

	// dfaCooldownPoints is the per-series suppression window after a fire,
	// matching mannkendall / burgar / esn / tukey_biweight / grubbs_loo so
	// per-detector eval telemetry stays comparable.
	dfaCooldownPoints = 30

	// dfaScoreEvery amortizes the DFA cost (~1300 flops per tick) across
	// multiple ingests. Half the rate of tukey_biweight (4) keeps the per-
	// point CPU cost within the same order of magnitude.
	dfaScoreEvery = 8

	// dfaHurstShiftThreshold is the |H_now - H_baseline| value above which
	// the detector fires. 0.20 corresponds to a transition between standard
	// regimes (e.g. white-noise H≈0.5 → strongly persistent H≈0.7+) per
	// Peng et al. 1994. Sub-threshold drift is treated as quiet baseline
	// motion and feeds the EWMA.
	dfaHurstShiftThreshold = 0.20

	// dfaBaselineGate bounds the |H_now - H_baseline| at which the EWMA
	// continues to track. Excursions beyond this gate are presumed to be
	// regime changes and are NOT folded into the baseline — preventing the
	// EWMA from "chasing" a sustained shift and silencing future fires.
	dfaBaselineGate = 0.05

	// dfaBaselineEWMAAlpha is the smoothing factor for the baseline H
	// estimate. α=0.05 gives an effective memory of ~20 scoring ticks =
	// ~160 ingested points at ScoreEvery=8 — long enough that one
	// noise-driven outlier H estimate cannot perturb the baseline.
	dfaBaselineEWMAAlpha = 0.05

	// dfaScoreScale converts |H_now - H_baseline| into the anomaly Score
	// field. 1 / dfaBaselineGate keeps a 1-baseline-gate excursion at
	// score 1.0 and a clean Hurst-shift threshold at score 4.0; cap at 50
	// matches the family-wide tbGlitchZCap / grubbsGlitchCap convention.
	dfaScoreScale = 1.0 / dfaBaselineGate
	dfaScoreCap   = 50.0
)

// dfaScales lists the segment lengths used to estimate F(s). Each must
// evenly divide dfaWindowSize so the closed-form OLS detrending sees an
// integer number of segments. {16, 32, 64, 128} gives a clean octave
// spacing across two orders of magnitude — the Peng et al. 1994
// recommendation for short windows.
var dfaScales = [4]int{16, 32, 64, 128}

// dfaLogScales caches log(s) for each entry in dfaScales. Computed once at
// package init so each scoring tick avoids 4 redundant math.Log calls.
var dfaLogScales = [4]float64{
	math.Log(16),
	math.Log(32),
	math.Log(64),
	math.Log(128),
}

// dfaLogScaleMean and dfaLogScaleDenom precompute the constant numerator
// pieces of the 4-point log-log linear regression. Defined once at package
// init so each Hurst estimate is a pure dot-product (no allocation).
//
//	H = (Σ log(s_i)·log(F_i) − N·logSMean·logFMean) / (Σ log(s_i)² − N·logSMean²)
//
// where N=4. The denominator depends only on dfaLogScales and is constant.
var (
	dfaLogScaleMean  float64
	dfaLogScaleDenom float64
)

// Per-scale OLS-detrending constants. Indexing into dfaScales gives the
// segment length s; Sx[k] and Sxx[k] are Σ_{i=0..s-1} i and Σ i² for
// that scale. The closed-form per-segment slope is
//
//	slope = (s·Σ(i·y_i) − Sx·Σy_i) / (s·Sxx − Sx²)
//
// — only Σ(i·y_i) and Σy_i depend on segment data.
var (
	dfaSx       [4]float64
	dfaSxx      [4]float64
	dfaSegDenom [4]float64 // s·Sxx − Sx²
)

func init() {
	for k, s := range dfaScales {
		var sx, sxx float64
		for i := 0; i < s; i++ {
			fi := float64(i)
			sx += fi
			sxx += fi * fi
		}
		dfaSx[k] = sx
		dfaSxx[k] = sxx
		dfaSegDenom[k] = float64(s)*sxx - sx*sx
	}

	var sum float64
	for _, ls := range dfaLogScales {
		sum += ls
	}
	dfaLogScaleMean = sum / float64(len(dfaLogScales))

	for _, ls := range dfaLogScales {
		d := ls - dfaLogScaleMean
		dfaLogScaleDenom += d * d
	}
}

// dfaStateKey identifies per-series state by ref and aggregation.
type dfaStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// dfaSeriesState holds streaming state per (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	ring (256 * 8)             = 2048 B
//	scalars                    ~   80 B
//	                            -------
//	                           ~ 2.1 KB
//
// Comparable to the wasserstein detector. The ring stores values only —
// timestamps are carried by the iteration cursor, mirroring grubbs_loo.
type dfaSeriesState struct {
	// Cursor (mirrors mannkendall / esn / tukey_biweight / grubbs_loo).
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// Sliding window of recent values in ring-buffer order. cap == WindowSize
	// once full.
	ring        [dfaWindowSize]float64
	head, count int

	// ticksSinceScore counts new points ingested since the last scoring tick.
	// scoreDFA runs only when this clears ScoreEvery, amortizing the per-tick
	// O(W) DFA work.
	ticksSinceScore int

	// Baseline H tracker. hBaseline is an EWMA of past Hurst estimates, but
	// only updates while |H_now - hBaseline| < dfaBaselineGate so a regime
	// change cannot poison the reference. baselineFilled counts admitted
	// updates up to dfaBaselineWarmup; the detector stays silent until the
	// baseline is filled.
	hBaseline      float64
	baselineFilled int

	// cooldownLeft is decremented on every ingested point so it expires
	// regardless of whether scoring runs on this tick.
	cooldownLeft int
	lastFireTime int64
}

// DFAHurstDetector implements a streaming Detrended Fluctuation Analysis
// changepoint detector (Peng et al. 1994, "Mosaic organization of DNA
// nucleotides", Phys. Rev. E 49). On each scoring tick it computes the
// rolling cumulative profile y[i] = Σ (x[j] − mean(x)), partitions y into
// non-overlapping segments of size s ∈ {16, 32, 64, 128}, fits a degree-1
// least-squares trend to each segment by closed form, and reports
// F(s) = sqrt(mean of squared residuals). The Hurst estimate H is the
// slope of log(F(s)) vs log(s) by 4-point linear regression. A baseline
// H is maintained as an EWMA over "quiet" estimates; fires happen when
// |H_now − H_baseline| ≥ dfaHurstShiftThreshold and the baseline has
// warmed up.
//
// The detector targets a regime-change axis ORTHOGONAL to the
// level/scale/moment family: a transition from white noise to AR(0.9)
// leaves the mean, variance, skewness, and kurtosis nominal but shifts
// the long-range dependence H from ~0.5 to ~0.8. Tukey biweight,
// Grubbs-LOO, hi_moments, and hl_shift cannot see that shift.
//
// Implements observer.Detector and observer.SeriesRemover. The Detect
// iteration shape mirrors TukeyBiweightDetector.Detect verbatim
// (metrics_detector_tukey_biweight.go:160-228).
type DFAHurstDetector struct {
	// WindowSize is the rolling window length. Default: 256.
	WindowSize int
	// BaselineWarmup is the number of admitted scoring ticks that must
	// accumulate before the detector will fire. Default: 64.
	BaselineWarmup int
	// CooldownPoints is the per-series suppression window after a fire.
	// Default: 30.
	CooldownPoints int
	// ScoreEvery amortizes scoring across multiple ingests. Default: 8.
	ScoreEvery int

	// HurstShiftThreshold is the |H − H_baseline| value above which a fire
	// is emitted. Default: 0.20.
	HurstShiftThreshold float64
	// BaselineEWMAAlpha is the smoothing factor for the baseline EWMA.
	// Default: 0.05.
	BaselineEWMAAlpha float64
	// BaselineGate is the |H − H_baseline| ceiling at which the EWMA
	// continues to track. Default: 0.05.
	BaselineGate float64

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[dfaStateKey]*dfaSeriesState

	// Cache the discovered series list across Detect calls; refreshed only
	// when SeriesGeneration changes.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewDFAHurstDetector returns a DFAHurstDetector with default settings.
// Parameterless to mirror NewGrubbsLOODetector / NewTukeyBiweightDetector.
func NewDFAHurstDetector() *DFAHurstDetector {
	return &DFAHurstDetector{
		WindowSize:          dfaWindowSize,
		BaselineWarmup:      dfaBaselineWarmup,
		CooldownPoints:      dfaCooldownPoints,
		ScoreEvery:          dfaScoreEvery,
		HurstShiftThreshold: dfaHurstShiftThreshold,
		BaselineEWMAAlpha:   dfaBaselineEWMAAlpha,
		BaselineGate:        dfaBaselineGate,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[dfaStateKey]*dfaSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *DFAHurstDetector) Name() string { return "dfa_hurst" }

// Reset clears all per-series state for replay/reanalysis.
func (d *DFAHurstDetector) Reset() {
	d.series = make(map[dfaStateKey]*dfaSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Without this hook the per-series map would grow unbounded with the
// cumulative number of series ever observed (mirrors the tukey_biweight /
// grubbs_loo contract).
func (d *DFAHurstDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, dfaStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration shape mirrors
// TukeyBiweightDetector.Detect (metrics_detector_tukey_biweight.go:160-228):
// cache series on SeriesGeneration, bulk-fetch status, replay-skip when
// nothing has changed, then process only the strictly-new points via
// ForEachPoint.
func (d *DFAHurstDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := dfaStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &dfaSeriesState{}
				d.series[sk] = state
			}

			// Replay-skip: no new points and no in-place writes.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				d.appendRing(state, p.Value)

				// Decrement cooldown per ingested point so it expires
				// regardless of whether scoring runs this tick.
				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}
				state.ticksSinceScore++

				if state.count >= d.WindowSize && state.cooldownLeft == 0 && state.ticksSinceScore >= d.ScoreEvery {
					state.ticksSinceScore = 0
					if anomaly, fired := d.scoreDFA(state, seriesMeta, agg, p.Value, p.Timestamp); fired {
						anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
						allAnomalies = append(allAnomalies, anomaly)
						state.cooldownLeft = d.CooldownPoints
						state.lastFireTime = p.Timestamp
					}
				}

				state.lastProcessedTime = p.Timestamp
			})

			state.lastProcessedCount = status.pointCount
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// appendRing writes the latest value into the per-series ring buffer in
// chronological order. While the ring is filling it grows; once full,
// the oldest entry at state.head is overwritten and head advances modulo
// WindowSize.
func (d *DFAHurstDetector) appendRing(state *dfaSeriesState, newest float64) {
	if state.count < d.WindowSize {
		state.ring[state.count] = newest
		state.count++
		return
	}
	state.ring[state.head] = newest
	state.head = (state.head + 1) % d.WindowSize
}

// scoreDFA runs the Hurst-exponent estimate on the current window and
// decides whether to emit on the latest point. Pure with respect to state
// ring and cooldown — only the baseline-tracking fields hBaseline and
// baselineFilled are mutated here.
func (d *DFAHurstDetector) scoreDFA(state *dfaSeriesState, series *observer.Series, agg observer.Aggregate, newest float64, dataTime int64) (observer.Anomaly, bool) {
	if state.count < d.WindowSize {
		return observer.Anomaly{}, false
	}

	hNow, ok := dfaHurst(state.ring[:], state.head, state.count, d.WindowSize)
	if !ok {
		// Degenerate window (constant values, or one of the F(s) collapsed
		// to the numerical floor in a way that produced NaN/Inf). Skip
		// without poisoning the baseline.
		return observer.Anomaly{}, false
	}

	// Baseline maintenance. While the baseline is still warming up, fold
	// every estimate in unconditionally so the EWMA can converge from the
	// h=0 seed. Once warm, only quiet excursions (|H_now − baseline| <
	// BaselineGate) feed the EWMA — a sustained regime change does NOT
	// chase the baseline, otherwise future fires would silently silence.
	deviation := hNow - state.hBaseline
	absDev := math.Abs(deviation)

	if state.baselineFilled < d.BaselineWarmup {
		// Warmup: track unconditionally so the EWMA converges from the seed.
		if state.baselineFilled == 0 {
			state.hBaseline = hNow
		} else {
			state.hBaseline += d.BaselineEWMAAlpha * (hNow - state.hBaseline)
		}
		state.baselineFilled++
		return observer.Anomaly{}, false
	}

	// Post-warmup: gate the EWMA so a regime change can't poison the baseline.
	if absDev < d.BaselineGate {
		state.hBaseline += d.BaselineEWMAAlpha * (hNow - state.hBaseline)
	}

	if absDev < d.HurstShiftThreshold {
		return observer.Anomaly{}, false
	}

	score := absDev * dfaScoreScale
	if score > dfaScoreCap {
		score = dfaScoreCap
	}

	direction := "above"
	if deviation < 0 {
		direction = "below"
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "DFA Hurst: " + seriesName,
		Description: fmt.Sprintf("%s baseline (H=%.3f, H_baseline=%.3f, dev=%.3f, n=%d)",
			direction, hNow, state.hBaseline, deviation, state.count),
		Timestamp: dataTime,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   state.hBaseline,
			Threshold:      d.HurstShiftThreshold,
			CurrentValue:   hNow,
			DeviationSigma: absDev,
		},
	}
	return anomaly, true
}

// dfaHurst returns the DFA-derived Hurst exponent for the values in ring.
// `head`/`count`/`win` describe a chronologically-ordered ring buffer
// (the same convention used by tukey_biweight.windowSnapshot). Returns
// (H, true) on success or (0, false) when the estimate is degenerate
// (NaN/Inf, division by zero in the log-log regression, etc.).
//
// O(W) per call: one linear pass to compute the cumulative profile, then
// for each scale s a partition-then-OLS pass that visits each profile
// point exactly once.
func dfaHurst(ring []float64, head, count, win int) (float64, bool) {
	if count < win {
		return 0, false
	}

	// Materialize the chronological window into a stack-friendly array.
	// 256 * 8 bytes = 2 KiB; ringValuesAndCum returns a fresh array each
	// call, but at ScoreEvery=8 the allocation pressure is acceptable.
	xs := make([]float64, win)
	if count < win {
		copy(xs, ring[:count])
	} else {
		// head points at the OLDEST entry once the ring is full; iterate
		// (head+i) mod win for i = 0..win-1 to reconstruct chronological order.
		for i := 0; i < win; i++ {
			xs[i] = ring[(head+i)%win]
		}
	}

	// Step 1: subtract mean and accumulate the integrated "profile" y.
	var mean float64
	for i := 0; i < win; i++ {
		mean += xs[i]
	}
	mean /= float64(win)

	y := make([]float64, win)
	var run float64
	for i := 0; i < win; i++ {
		run += xs[i] - mean
		y[i] = run
	}

	// Step 2: per-scale fluctuation F(s).
	var fs [4]float64
	for k, s := range dfaScales {
		segDenom := dfaSegDenom[k]
		if segDenom <= 0 {
			return 0, false
		}
		segs := win / s
		var sumSq float64
		for seg := 0; seg < segs; seg++ {
			off := seg * s

			// Accumulate Σy_i and Σ(i·y_i) over the segment. The OLS slope
			// is then (s·Σi*y - Sx·Σy) / (s·Sxx − Sx²); intercept follows
			// from intercept = mean(y) − slope·mean(i).
			var sumY, sumIY float64
			for i := 0; i < s; i++ {
				yi := y[off+i]
				sumY += yi
				sumIY += float64(i) * yi
			}

			sx := dfaSx[k]
			fs64 := float64(s)
			slope := (fs64*sumIY - sx*sumY) / segDenom
			intercept := (sumY - slope*sx) / fs64

			// Σ residual² over the segment. Detrend each point and accumulate
			// squared error; this is the per-segment F²(s) contribution.
			var segSq float64
			for i := 0; i < s; i++ {
				resid := y[off+i] - (intercept + slope*float64(i))
				segSq += resid * resid
			}
			sumSq += segSq
		}

		mse := sumSq / float64(segs*s)
		// 1e-12 floor avoids log(0). A genuinely-constant series falls
		// here for every scale; the subsequent log-log regression sees a
		// zero-variance row of log(F) values and the denominator guard
		// below catches it.
		if mse < 1e-24 {
			mse = 1e-24
		}
		fs[k] = 0.5 * math.Log(mse) // log(F(s)) = 0.5·log(F²(s))
	}

	// Step 3: linear regression of log(F) on log(s) over 4 points.
	if dfaLogScaleDenom <= 0 {
		return 0, false
	}
	var logFMean float64
	for _, lf := range fs {
		logFMean += lf
	}
	logFMean /= float64(len(fs))

	var num float64
	for k, lf := range fs {
		num += (dfaLogScales[k] - dfaLogScaleMean) * (lf - logFMean)
	}
	h := num / dfaLogScaleDenom
	if math.IsNaN(h) || math.IsInf(h, 0) {
		return 0, false
	}
	return h, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults
// so a struct-literal construction (&DFAHurstDetector{}) still produces a
// working detector. Mirrors TukeyBiweightDetector.ensureDefaults.
func (d *DFAHurstDetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = dfaWindowSize
	}
	// dfaScales partition dfaWindowSize exactly. Tuning WindowSize away
	// from the default would invalidate the closed-form OLS denominator
	// table, so cap it back to the supported value rather than silently
	// produce wrong numbers.
	if d.WindowSize != dfaWindowSize {
		d.WindowSize = dfaWindowSize
	}
	if d.BaselineWarmup <= 0 {
		d.BaselineWarmup = dfaBaselineWarmup
	}
	if d.CooldownPoints < 0 {
		d.CooldownPoints = 0
	}
	if d.CooldownPoints == 0 {
		d.CooldownPoints = dfaCooldownPoints
	}
	if d.ScoreEvery <= 0 {
		d.ScoreEvery = dfaScoreEvery
	}
	if d.HurstShiftThreshold <= 0 {
		d.HurstShiftThreshold = dfaHurstShiftThreshold
	}
	if d.BaselineEWMAAlpha <= 0 {
		d.BaselineEWMAAlpha = dfaBaselineEWMAAlpha
	}
	if d.BaselineGate <= 0 {
		d.BaselineGate = dfaBaselineGate
	}
	if d.series == nil {
		d.series = make(map[dfaStateKey]*dfaSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
