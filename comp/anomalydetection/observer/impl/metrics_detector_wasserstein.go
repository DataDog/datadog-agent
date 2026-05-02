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

// wsStateKey identifies per-series state by ref and aggregation.
type wsStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// wsSeriesState holds per-series streaming state for the Wasserstein detector.
//
// Memory footprint per (series, agg) pair:
//
//	refBuf       (60 * 16)                    =  960 B
//	curBuf       (60 * 16)                    =  960 B
//	scalars                                   ~  100 B
//	                                          -------
//	                                          ~2.0 KB
//
// ~5x lighter than bocpd's per-key footprint.
type wsSeriesState struct {
	// Cursor (mirrors mannkendall: metrics_detector_mannkendall.go:27-34).
	lastProcessedCount int
	lastWriteGen       int64
	// lastProcessedTime is the highest point timestamp consumed so far. Used
	// as the exclusive lower bound for ForEachPoint so each point is appended
	// to the windows exactly once across replays/incremental advances.
	lastProcessedTime int64

	// refBuf is the older "reference" sliding window in ring-buffer form.
	// While filling, refHead stays at 0 and entries grow with append; once
	// full, refHead points to the oldest entry and cycles modulo WindowSize.
	refBuf   []observer.Point
	refHead  int
	refCount int

	// curBuf is the newer "current" sliding window. Same ring semantics as
	// refBuf. Points enter curBuf first; once it's full, every new point
	// causes the oldest curBuf entry (curBuf[curHead]) to slide into refBuf.
	curBuf   []observer.Point
	curHead  int
	curCount int

	// cooldownLeft counts down on each new point after a fire so we don't
	// repeatedly emit on the same drift segment.
	cooldownLeft int
	lastFireTime int64
}

// WassersteinDetector implements a streaming two-sample test on a pair of
// sliding windows using the 1-D Wasserstein-1 (earth-mover) distance:
//
//	W1(ref, cur) = (1/N)·Σ_{i=0..N-1} |sorted(ref)[i] - sorted(cur)[i]|
//
// (Vallender 1974, "Calculation of the Wasserstein distance between
// probability distributions on the line"; Ramdas, García Trillos & Cuturi
// 2017, "On Wasserstein two-sample testing and related families of
// nonparametric tests").
//
// W1 measures the mass transport cost across the empirical CDF, which
// captures dispersion and shape changes that pure rank-based tests
// (scanmw / Mann-Whitney) and mean-based tests (scanwelch) can miss.
// Per-tick cost is dominated by sorting two N=60 windows — ~1500 comparisons,
// amortized to ~50 by the cooldown — comparable to mannkendall's Theil-Sen
// pair scan amortized cost.
//
// Implements observer.Detector with explicit cursoring (modeled after
// MannKendall) and observer.SeriesRemover for compatibility with the catalog
// teardown contract validated by TestDefaultCatalog_DetectorTeardownContract.
type WassersteinDetector struct {
	// WindowSize is the number of points per sliding window (ref and cur).
	// Default: 60 — matches mannkendall (1 minute at 1 Hz) so per-tick state
	// size is comparable.
	WindowSize int
	// MinPoints is the minimum total fill before scoring runs. Default:
	// WindowSize. Both windows must be full (each at MinPoints) before any
	// W1 score is computed.
	MinPoints int
	// DistanceThresholdMAD is the minimum scaled W1 distance for a fire.
	// Default: 4.0. Under N(0, σ²) i.i.d. on both windows, expected W1 ≈
	// σ/√(πN) ≈ σ·0.073 at N=60, so a 4·MAD threshold corresponds to roughly
	// a 5·MAD raw shift on all of cur — strict, matches the Bonferroni-ish
	// stance of mannkendall's ZThreshold=5.0.
	DistanceThresholdMAD float64
	// MinDeviationMAD is the minimum |median(cur) - median(ref)| / MAD(ref)
	// effect-size gate. Default: 2.5 — a hair less strict than mannkendall's
	// MinSlopeMAD=3.0; W1 already incorporates dispersion change so this
	// secondary gate just enforces a directional shift.
	MinDeviationMAD float64
	// CooldownPoints is the number of points to skip after a fire before
	// re-evaluating. Default: 30 — matches mannkendall to bound emission
	// frequency on the same drift segment.
	CooldownPoints int
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[wsStateKey]*wsSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewWassersteinDetector returns a Wasserstein detector with default settings.
func NewWassersteinDetector() *WassersteinDetector {
	return &WassersteinDetector{
		WindowSize:           60,
		MinPoints:            60,
		DistanceThresholdMAD: 4.0,
		MinDeviationMAD:      2.5,
		CooldownPoints:       30,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[wsStateKey]*wsSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *WassersteinDetector) Name() string {
	return "wasserstein"
}

// Reset clears all per-series state for replay/reanalysis.
func (d *WassersteinDetector) Reset() {
	d.series = make(map[wsStateKey]*wsSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series window state for refs that storage has freed.
// Without this hook the per-series map keeps growing with the cumulative
// series count even after storage shrinks (see ScanMW/MannKendall for the
// same pattern).
func (d *WassersteinDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, wsStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration pattern is a 1:1 mirror of
// metrics_detector_mannkendall.go:142-215 — series cache, bulk status,
// replay-skip when nothing has changed, ForEachPoint to walk only new points
// since the last call.
func (d *WassersteinDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	// Bulk-fetch point counts and write generations in a single lock acquisition.
	refs := make([]observer.SeriesRef, len(d.cachedSeries))
	for i, meta := range d.cachedSeries {
		refs[i] = meta.Ref
	}
	bulkStatus := bulkSeriesStatus(storage, refs, dataTime)

	var allAnomalies []observer.Anomaly

	for i, meta := range d.cachedSeries {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			sk := wsStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &wsSeriesState{}
				d.series[sk] = state
			}

			// Replay-skip: no new data and no in-place writes.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			// Walk only the points strictly after the highest one we've
			// already appended. ForEachPoint's start parameter is exclusive,
			// so passing state.lastProcessedTime gives us exactly the new
			// points.
			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				d.appendPoint(state, p)

				// Decrement cooldown per ingested point regardless of whether
				// we score this tick. Mirrors mannkendall:193-195.
				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}

				if state.refCount >= d.MinPoints && state.curCount >= d.MinPoints && state.cooldownLeft == 0 {
					if anomaly, fired := d.scoreW1(state, seriesMeta, agg, p.Timestamp); fired {
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

// appendPoint pushes p into the per-series two-buffer state.
//
// Phase 1 (curBuf filling): while curCount < WindowSize, append to curBuf.
//
// Phase 2 (curBuf full): the oldest curBuf entry (curBuf[curHead]) slides
// into refBuf, and the new point P overwrites curBuf[curHead]; curHead
// advances modulo WindowSize. The slid-in point grows refBuf if it isn't yet
// full, otherwise it overwrites refBuf[refHead] and refHead advances.
func (d *WassersteinDetector) appendPoint(state *wsSeriesState, p observer.Point) {
	if state.curCount < d.WindowSize {
		state.curBuf = append(state.curBuf, p)
		state.curCount++
		// curHead stays 0 until curBuf wraps.
		return
	}
	// curBuf is full: slide its oldest entry into refBuf, then write p in
	// curBuf at curHead and advance.
	slid := state.curBuf[state.curHead]
	if state.refCount < d.WindowSize {
		state.refBuf = append(state.refBuf, slid)
		state.refCount++
		// refHead stays 0 until refBuf wraps.
	} else {
		state.refBuf[state.refHead] = slid
		state.refHead = (state.refHead + 1) % d.WindowSize
	}
	state.curBuf[state.curHead] = p
	state.curHead = (state.curHead + 1) % d.WindowSize
}

// windowSnapshot returns the contents of a ring buffer in chronological order.
// Allocates a fresh []float64 of size count; called only on scoring ticks
// gated by the dual MAD thresholds + cooldown so the allocation is rare.
//
// Mirrors metrics_detector_mannkendall.go:235-247.
func wassersteinSnapshot(buf []observer.Point, head, count, cap int) []float64 {
	out := make([]float64, count)
	if count < cap {
		// Buffer hasn't wrapped yet — entries are already in order at [0..count).
		for i := 0; i < count; i++ {
			out[i] = buf[i].Value
		}
		return out
	}
	for i := 0; i < cap; i++ {
		out[i] = buf[(head+i)%cap].Value
	}
	return out
}

// scoreW1 runs the Wasserstein-1 dual gate on the current ref/cur pair.
// Returns (anomaly, fired). Pure function over the ring snapshots.
func (d *WassersteinDetector) scoreW1(state *wsSeriesState, series *observer.Series, agg observer.Aggregate, dataTime int64) (observer.Anomaly, bool) {
	n := d.WindowSize
	if state.refCount < n || state.curCount < n {
		return observer.Anomaly{}, false
	}

	ref := wassersteinSnapshot(state.refBuf, state.refHead, state.refCount, d.WindowSize)
	cur := wassersteinSnapshot(state.curBuf, state.curHead, state.curCount, d.WindowSize)

	medRef := detectorMedian(ref)
	madRef := detectorMAD(ref, medRef, false)

	// Scale-stable denominator (mirrors mannkendall:298-301): when the ref
	// window is constant or near-constant, fall back to a percent-of-magnitude
	// scale so tiny absolute distances on a level-100 series don't produce
	// gigantic D values from a near-zero MAD.
	scaleDenom := madRef
	if scaleDenom < 1e-10 {
		scaleDenom = math.Max(math.Abs(medRef)*0.01, 1e-6)
	}

	// Sort copies of ref and cur ascending for the order-statistic alignment.
	refSorted := make([]float64, n)
	curSorted := make([]float64, n)
	copy(refSorted, ref)
	copy(curSorted, cur)
	sort.Float64s(refSorted)
	sort.Float64s(curSorted)

	// 1-D Wasserstein-1 distance in closed form for two equal-size empirical
	// distributions (Vallender 1974): the average over i of the gap between
	// the i-th order statistics. O(N) given the sorts above.
	w1 := 0.0
	for i := 0; i < n; i++ {
		w1 += math.Abs(refSorted[i] - curSorted[i])
	}
	w1 /= float64(n)

	d1 := w1 / scaleDenom
	if d1 < d.DistanceThresholdMAD {
		return observer.Anomaly{}, false
	}

	medCur := detectorMedian(cur)
	devMAD := math.Abs(medCur-medRef) / scaleDenom
	if devMAD < d.MinDeviationMAD {
		return observer.Anomaly{}, false
	}

	direction := "increasing"
	if medCur < medRef {
		direction = "decreasing"
	}

	// Score = D, clamped to a sane visual range so a single near-degenerate
	// series (e.g. madRef collapsing to 1e-6 fallback) cannot dominate the
	// downstream UI/correlator scoring.
	score := d1
	if score > 50 {
		score = 50
	}

	// Use the youngest point in the cur snapshot for SamplingIntervalSec /
	// CurrentValue rather than reconstructing observer.Point from the
	// chronological []float64. We still need a pointer-style buffer for the
	// median interval helper, so build it from cur in chronological order.
	chronological := make([]observer.Point, 0, 2*n)
	if state.refCount < d.WindowSize {
		chronological = append(chronological, state.refBuf[:state.refCount]...)
	} else {
		for i := 0; i < d.WindowSize; i++ {
			chronological = append(chronological, state.refBuf[(state.refHead+i)%d.WindowSize])
		}
	}
	if state.curCount < d.WindowSize {
		chronological = append(chronological, state.curBuf[:state.curCount]...)
	} else {
		for i := 0; i < d.WindowSize; i++ {
			chronological = append(chronological, state.curBuf[(state.curHead+i)%d.WindowSize])
		}
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "Wasserstein shift: " + seriesName,
		Description: fmt.Sprintf("%s distributional shift (W1=%.4f, D=%.2f·MAD, devMAD=%.2f, n=%d)",
			direction, w1, d1, devMAD, n),
		Timestamp:           dataTime,
		Score:               &score,
		SamplingIntervalSec: medianPointInterval(chronological),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medRef,
			BaselineMAD:    madRef,
			CurrentValue:   chronological[len(chronological)-1].Value,
			DeviationSigma: devMAD,
		},
	}
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *WassersteinDetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = 60
	}
	if d.MinPoints <= 0 {
		d.MinPoints = d.WindowSize
	}
	if d.MinPoints > d.WindowSize {
		// Scoring requires both buffers to be filled, so cap MinPoints.
		d.MinPoints = d.WindowSize
	}
	if d.DistanceThresholdMAD <= 0 {
		d.DistanceThresholdMAD = 4.0
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = 2.5
	}
	if d.CooldownPoints < 0 {
		d.CooldownPoints = 0
	}
	if d.CooldownPoints == 0 {
		d.CooldownPoints = 30
	}
	if d.series == nil {
		d.series = make(map[wsStateKey]*wsSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
