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

// tbGlitchZCap is the upper bound on |z| for a fire. Anything beyond is
// almost certainly a sensor glitch (NaN-converted 1e308, malformed counter
// reset) rather than a genuine regime change, and we'd rather miss it than
// emit a single anomaly with a runaway score.
const tbGlitchZCap = 50.0

// tbStateKey identifies per-series state by ref and aggregation.
type tbStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// tbSeriesState holds streaming state per (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	ring (80 * 16)               = 1280 B
//	scalars                      ~  100 B
//	                              -------
//	                             ~1.4 KB
type tbSeriesState struct {
	// Cursor over visible storage buckets plus in-place write generation.
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// Sliding window of recent points in chronological order when read via
	// windowSnapshot(). cap == WindowSize once full.
	ring        []observer.Point
	head, count int

	// ticksSinceScore counts new points ingested since the last scoring tick.
	// scoreBiweight is invoked only when this clears ScoreEvery, amortizing
	// the IRLS cost across multiple ticks.
	ticksSinceScore int

	// cooldownLeft is decremented on every ingested point so it expires
	// regardless of whether scoring runs on this tick.
	cooldownLeft int
	lastFireTime int64
}

// TukeyBiweightDetector implements a streaming Tukey biweight M-estimator
// changepoint detector (Tukey 1977; Beaton & Tukey 1974; Holland & Welsch
// 1977; Maronna, Martin & Yohai 2006 §2.6 / §6.4). On each scoring tick it
// fits a redescending biweight location/scale pair (mu, sigma) over the most
// recent WindowSize values via 4 IRLS sweeps, then standardizes the latest
// point against the IMMUNIZED baseline. Points beyond c·sigma get zero weight
// during baseline estimation, so a single historical spike does NOT contaminate
// the reference — unlike rank/AR detectors whose MAD or autocorrelation gets
// poisoned for the entire window. This is the structural property that makes
// biweight robust to "one historical glitch followed by a real shift later".
//
// Implements observer.Detector and observer.SeriesRemover. The iteration
// shape matches the other streaming metric detectors: cache series, bulk-fetch
// per-ref status, then advance each per-series cursor with ForEachPoint.
type TukeyBiweightDetector struct {
	// WindowSize is the number of recent points held in the IRLS window.
	// Default: 80, enough context for a robust local baseline without making
	// each scoring tick too expensive.
	WindowSize int
	// MinPoints is the minimum window fill before scoring runs.
	// Default: WindowSize.
	MinPoints int
	// BiweightC is the Tukey biweight tuning constant. Default: 4.685, the
	// canonical 95% Gaussian-efficiency value (Holland & Welsch 1977 fig. 2).
	// Tightening to 4.0 trades efficiency for outlier rejection.
	BiweightC float64
	// IRLSIterations bounds the number of biweight reweighting sweeps.
	// Default: 4. Maronna et al. §6.4 recommend 3-5; 3 is faster but
	// occasionally non-convergent on bimodal windows.
	IRLSIterations int
	// ZThreshold is the |z| score above which the latest point is flagged.
	// Default: 5.0 to keep the per-tick false-positive rate strict.
	ZThreshold float64
	// ScoreEvery amortizes IRLS cost: scoreBiweight runs every Nth point
	// once the window is full. Default: 4.
	ScoreEvery int
	// CooldownPoints is the per-series suppression window after a fire.
	// Default: 30.
	CooldownPoints int
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[tbStateKey]*tbSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// TukeyBiweightConfig holds catalog/testbench tunables for TukeyBiweightDetector.
type TukeyBiweightConfig struct {
	WindowSize     int      `json:"window_size"`
	MinPoints      int      `json:"min_points"`
	BiweightC      float64  `json:"biweight_c"`
	IRLSIterations int      `json:"irls_iterations"`
	ZThreshold     float64  `json:"z_threshold"`
	ScoreEvery     int      `json:"score_every"`
	CooldownPoints int      `json:"cooldown_points"`
	Aggregations   []string `json:"aggregations,omitempty"`
}

// DefaultTukeyBiweightConfig returns the production/testbench defaults.
func DefaultTukeyBiweightConfig() TukeyBiweightConfig {
	return TukeyBiweightConfig{
		WindowSize:     80,
		MinPoints:      80,
		BiweightC:      4.685,
		IRLSIterations: 4,
		ZThreshold:     5.0,
		ScoreEvery:     4,
		CooldownPoints: 30,
		Aggregations: []string{
			observer.AggregateString(observer.AggregateAverage),
			observer.AggregateString(observer.AggregateCount),
		},
	}
}

// NewTukeyBiweightDetector returns a TukeyBiweightDetector with default
// settings. Hyperparameter derivations are documented inline on each field.
func NewTukeyBiweightDetector() *TukeyBiweightDetector {
	return NewTukeyBiweightDetectorWithConfig(DefaultTukeyBiweightConfig())
}

// NewTukeyBiweightDetectorWithConfig returns a detector configured from cfg.
func NewTukeyBiweightDetectorWithConfig(cfg TukeyBiweightConfig) *TukeyBiweightDetector {
	return &TukeyBiweightDetector{
		WindowSize:     cfg.WindowSize,
		MinPoints:      cfg.MinPoints,
		BiweightC:      cfg.BiweightC,
		IRLSIterations: cfg.IRLSIterations,
		ZThreshold:     cfg.ZThreshold,
		ScoreEvery:     cfg.ScoreEvery,
		CooldownPoints: cfg.CooldownPoints,
		Aggregations:   parseAggregateConfig(cfg.Aggregations),
		series:         make(map[tbStateKey]*tbSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *TukeyBiweightDetector) Name() string { return "tukey_biweight" }

// Reset clears all per-series state for replay/reanalysis.
func (d *TukeyBiweightDetector) Reset() {
	d.series = make(map[tbStateKey]*tbSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Without this hook the per-series map would grow unbounded with the
// cumulative number of series ever observed even after storage shrinks
// because each entry owns a rolling window.
func (d *TukeyBiweightDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, tbStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. If storage mutates an already-processed
// bucket via a same-bucket merge or out-of-order backfill, the per-series state
// is rebuilt from visible storage to keep incremental processing equivalent to
// replaying the final stored points.
func (d *TukeyBiweightDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := tbStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &tbSeriesState{}
				d.series[sk] = state
			}

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
				prefixCount == state.lastProcessedCount && d.cursorPointChanged(storage, meta.Ref, agg, state)
			if mergeOccurred || prefixCount > state.lastProcessedCount || cursorBucketChangedWithAppend {
				state = &tbSeriesState{}
				d.series[sk] = state
				startTime = 0
			}

			var seriesMeta *observer.Series
			pointsSeen := false
			storage.ForEachPoint(meta.Ref, startTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				pointsSeen = true
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				d.appendRing(state, p)

				// Decrement cooldown per ingested point so it expires
				// regardless of whether scoring runs this tick.
				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}
				state.ticksSinceScore++

				if state.count >= d.MinPoints && state.cooldownLeft == 0 && state.ticksSinceScore >= d.ScoreEvery {
					state.ticksSinceScore = 0
					if anomaly, fired := d.scoreBiweight(state, seriesMeta, agg, p.Timestamp); fired {
						anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
						allAnomalies = append(allAnomalies, anomaly)
						state.cooldownLeft = d.CooldownPoints
						state.lastFireTime = p.Timestamp
					}
				}

				state.lastProcessedTime = p.Timestamp
			})

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

// appendRing appends a point to the per-series ring buffer in chronological
// order. While the buffer is filling, it grows; once full, the oldest entry
// at state.head is overwritten and head advances modulo WindowSize.
func (d *TukeyBiweightDetector) appendRing(state *tbSeriesState, p observer.Point) {
	if state.count < d.WindowSize {
		state.ring = append(state.ring, p)
		state.count++
		return
	}
	state.ring[state.head] = p
	state.head = (state.head + 1) % d.WindowSize
}

// windowSnapshot returns the current ring contents in chronological order as
// a fresh []float64 of length state.count. Allocated only on scoring ticks
// (gated by ScoreEvery) so allocation pressure is amortized.
func (d *TukeyBiweightDetector) windowSnapshot(state *tbSeriesState) []float64 {
	xs := make([]float64, state.count)
	if state.count < d.WindowSize {
		for i := 0; i < state.count; i++ {
			xs[i] = state.ring[i].Value
		}
		return xs
	}
	for i := 0; i < d.WindowSize; i++ {
		xs[i] = state.ring[(state.head+i)%d.WindowSize].Value
	}
	return xs
}

func (d *TukeyBiweightDetector) windowPointsSnapshot(state *tbSeriesState) []observer.Point {
	points := make([]observer.Point, state.count)
	if state.count < d.WindowSize {
		copy(points, state.ring)
		return points
	}
	for i := 0; i < d.WindowSize; i++ {
		points[i] = state.ring[(state.head+i)%d.WindowSize]
	}
	return points
}

func (d *TukeyBiweightDetector) cursorPointChanged(storage observer.StorageReader, ref observer.SeriesRef, agg observer.Aggregate, state *tbSeriesState) bool {
	if state.count == 0 {
		return false
	}
	idx := state.count - 1
	if state.count >= d.WindowSize {
		idx = (state.head + d.WindowSize - 1) % d.WindowSize
	}
	lastPoint := state.ring[idx]
	if lastPoint.Timestamp != state.lastProcessedTime {
		return false
	}
	changed := false
	storage.ForEachPoint(ref, state.lastProcessedTime-1, state.lastProcessedTime, agg, func(_ *observer.Series, p observer.Point) {
		if p.Timestamp == lastPoint.Timestamp && p.Value != lastPoint.Value {
			changed = true
		}
	})
	return changed
}

// scoreBiweight runs the IRLS biweight fit on the current window and decides
// whether to emit on the latest point. Returns (anomaly, fired). Pure with
// respect to state — no mutation of ring/cooldown happens here.
//
// The structural guarantee: points beyond c·sigma get zero biweight weight,
// so a single historical glitch CANNOT poison (mu, sigma). This is the property
// that distinguishes biweight from rank/AR detectors and motivates the
// "robust-to-historical-outlier" test case.
func (d *TukeyBiweightDetector) scoreBiweight(state *tbSeriesState, series *observer.Series, agg observer.Aggregate, dataTime int64) (observer.Anomaly, bool) {
	xs := d.windowSnapshot(state)
	n := len(xs)
	if n < 2 {
		return observer.Anomaly{}, false
	}

	// (b) Initial robust location/scale: median + 1.4826·MAD. The MAD scaling
	// gives a sigma-equivalent so the BiweightC·sigma cutoff matches the
	// canonical Holland & Welsch derivation. Floor on degenerate (constant)
	// windows to avoid divide-by-zero.
	mu := detectorMedian(xs)
	sigma := detectorMAD(xs, mu, true)
	if sigma < 1e-10 {
		sigma = math.Max(math.Abs(mu)*0.01, 1e-6)
	}

	c := d.BiweightC

	// (c) IRLS sweeps: at each step compute biweight weights w_j = (1-u_j^2)^2
	// for |u_j| < 1, zero otherwise (redescending psi), then update mu as the
	// weighted mean. Break early on convergence.
	for iter := 0; iter < d.IRLSIterations; iter++ {
		num, den := 0.0, 0.0
		for j := 0; j < n; j++ {
			u := (xs[j] - mu) / (c * sigma)
			if math.Abs(u) >= 1.0 {
				continue
			}
			w := 1 - u*u
			w *= w
			num += w * xs[j]
			den += w
		}
		if den < 1e-10 {
			break
		}
		newMu := num / den
		converged := math.Abs(newMu-mu) < 1e-6*sigma
		mu = newMu
		if converged {
			break
		}
	}

	// (d) Recompute sigma from the biweight-weighted residuals. Same trimming
	// rule as the mu update so points beyond c·sigma still contribute zero.
	s2num, s2den := 0.0, 0.0
	for j := 0; j < n; j++ {
		u := (xs[j] - mu) / (c * sigma)
		if math.Abs(u) >= 1.0 {
			continue
		}
		w := 1 - u*u
		w *= w
		r := xs[j] - mu
		s2num += w * r * r
		s2den += w
	}
	if s2den > 0 {
		sigma = math.Sqrt(s2num / s2den)
	}

	// (e) Score the latest point against the immunized baseline. The 1e-10
	// floor on sigma matches the initial-estimate floor and prevents a
	// pathological zero-variance window from emitting +Inf z.
	denom := sigma
	if denom < 1e-10 {
		denom = 1e-10
	}
	latest := xs[n-1]
	z := (latest - mu) / denom
	zAbs := math.Abs(z)
	if zAbs < d.ZThreshold {
		return observer.Anomaly{}, false
	}

	// (f) Suppress extreme glitches (sensor errors, NaN-converted 1e308, etc.)
	// without blocking real shifts. Points with |z| >= glitchZCap are treated
	// as instrumentation artifacts rather than genuine regime changes.
	if zAbs >= tbGlitchZCap {
		return observer.Anomaly{}, false
	}

	// (g) Score is |z|, capped at 50 to keep downstream UI / scoring sane.
	score := math.Min(zAbs, 50)

	direction := "above"
	if z < 0 {
		direction = "below"
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:                observer.AnomalyTypeMetric,
		Source:              observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName:        d.Name(),
		Title:               "Tukey biweight: " + seriesName,
		SamplingIntervalSec: medianPointInterval(d.windowPointsSnapshot(state)),
		Description: fmt.Sprintf("%s biweight baseline (z=%.2f, mu=%.4f, sigma=%.4f, n=%d)",
			direction, z, mu, sigma, n),
		Timestamp: dataTime,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: mu,
			BaselineMAD:    sigma,
			CurrentValue:   latest,
			DeviationSigma: zAbs,
		},
	}
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults so
// struct-literal construction (&TukeyBiweightDetector{}) still works.
func (d *TukeyBiweightDetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = 80
	}
	if d.MinPoints <= 0 {
		d.MinPoints = d.WindowSize
	}
	if d.MinPoints > d.WindowSize {
		// Scoring requires a full window, so cap MinPoints.
		d.MinPoints = d.WindowSize
	}
	if d.BiweightC <= 0 {
		d.BiweightC = 4.685
	}
	if d.IRLSIterations <= 0 {
		d.IRLSIterations = 4
	}
	if d.ZThreshold <= 0 {
		d.ZThreshold = 5.0
	}
	if d.ScoreEvery <= 0 {
		d.ScoreEvery = 4
	}
	if d.CooldownPoints < 0 {
		d.CooldownPoints = 0
	}
	if d.CooldownPoints == 0 {
		d.CooldownPoints = 30
	}
	if d.series == nil {
		d.series = make(map[tbStateKey]*tbSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
