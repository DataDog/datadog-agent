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

// hlGlitchShiftCap is the upper bound on shiftMAD for a fire. Anything beyond
// is almost certainly a sensor glitch (NaN-converted 1e308, malformed counter
// reset) rather than a genuine level shift; we'd rather miss it than emit a
// runaway score. Mirrors tbGlitchZCap=50 used by tukey_biweight, mannkendall,
// and the Wasserstein D cap so all detectors share the same anti-runaway
// convention downstream.
const hlGlitchShiftCap = 50.0

// hlStateKey identifies per-series state by ref and aggregation.
type hlStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// hlSeriesState holds streaming state per (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	refBuf       (40 * 16)                    =  640 B
//	curBuf       (40 * 16)                    =  640 B
//	scalars                                   ~  100 B
//	                                          -------
//	                                          ~1.4 KB
//
// Same order as tukey_biweight (~1.4 KB) and ~30% lighter than wasserstein
// (~2 KB) thanks to N=40 vs N=60 windows.
type hlSeriesState struct {
	// Cursor (mirrors mannkendall / wasserstein / tukey_biweight).
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

	// ticksSinceScore counts new points ingested since the last scoring tick.
	// scoreShift runs only when this clears ScoreEvery, amortizing the O(N²)
	// pairwise scan across multiple ticks (mirrors tukey_biweight's IRLS
	// amortization).
	ticksSinceScore int

	// cooldownLeft is decremented on every ingested point so it expires
	// regardless of whether scoring runs on this tick.
	cooldownLeft int
	lastFireTime int64
}

// HLShiftDetector implements a streaming Hodges-Lehmann shift detector
// (Hodges & Lehmann 1963, "Estimates of location based on rank tests"). On
// each scoring tick it computes the median of all pairwise differences
//
//	delta_HL = median{ cur[j] - ref[i] : 0 <= i, j < N }
//
// over two sliding windows — the "Walsh differences" estimator. delta_HL is a
// 50%-breakdown robust estimate of the LEVEL SHIFT between cur and ref: a
// single arbitrarily large outlier in either window can shift at most one of
// the N values per row/column of the N×N pair matrix and therefore moves the
// median of N² entries by less than half its full magnitude. This is the
// structural property that distinguishes H-L from rank-only tests (scanmw,
// Mann-Whitney) and from Wasserstein-1 — the Walsh-difference median directly
// answers "by how much did the level move?" without conflating dispersion or
// shape change.
//
// The detector emits when |delta_HL| / MAD(ref) clears ShiftThresholdMAD AND
// the MAJORITY of cur values have moved in the same direction relative to
// median(ref). The second gate (ConfirmFraction) is the structural fix to the
// Wasserstein-1 over-firing failure mode from exp-0086/exp-0089: even an
// outlier-heavy window with no real bulk shift can drag W1 above its
// threshold, but the H-L shift estimator + same-sign confirmation requires
// the BULK of cur to have moved.
//
// Implements observer.Detector with explicit cursoring (modeled after
// MannKendall / Wasserstein) and observer.SeriesRemover for compatibility
// with the catalog teardown contract validated by
// TestDefaultCatalog_DetectorTeardownContract.
type HLShiftDetector struct {
	// WindowSize is the number of points per sliding window (ref and cur).
	// Default: 40 — smaller than mannkendall/wasserstein (N=60) because the
	// O(N²) pairwise scan grows quadratically; at N=40 a scoring tick costs
	// ~1600 diffs + ~17000 sort comparisons, comparable to mannkendall's
	// 1770-pair Theil-Sen scan amortized.
	WindowSize int
	// MinPoints is the minimum total fill before scoring runs. Default:
	// WindowSize. Both windows must be full (each at MinPoints) before any
	// shift score is computed.
	MinPoints int
	// ShiftThresholdMAD is the minimum |delta_HL| / MAD(ref) for a fire.
	// Default: 4.0 — matches wasserstein's DistanceThresholdMAD so the dual-
	// gate stance is comparable across the distributional-shift family.
	ShiftThresholdMAD float64
	// ConfirmFraction is the minimum fraction of cur values that must have
	// moved in the same direction as delta_HL relative to median(ref).
	// Default: 0.75 — the "majority moved" structural gate that rejects
	// single-tail outlier windows. With a single large cur outlier we get
	// 1/40 ≈ 0.025, well under 0.75; with a real shift we get N/N = 1.0.
	ConfirmFraction float64
	// ScoreEvery amortizes the O(N²) pairwise scan: scoreShift runs every
	// Nth point once both windows are full. Default: 4 — same value as
	// tukey_biweight's IRLS amortization.
	ScoreEvery int
	// CooldownPoints is the per-series suppression window after a fire.
	// Default: 30 — matches mannkendall / wasserstein / tukey_biweight to
	// bound emission frequency on the same drift segment.
	CooldownPoints int
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[hlStateKey]*hlSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewHLShiftDetector returns a Hodges-Lehmann shift detector with default
// settings. Parameterless to match the wasserstein/tukey_biweight factory
// pattern: catalog entries that wire `func(any) any { return New<...>() }`
// expect a no-arg constructor.
func NewHLShiftDetector() *HLShiftDetector {
	return &HLShiftDetector{
		WindowSize:        40,
		MinPoints:         40,
		ShiftThresholdMAD: 4.0,
		ConfirmFraction:   0.75,
		ScoreEvery:        4,
		CooldownPoints:    30,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[hlStateKey]*hlSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *HLShiftDetector) Name() string { return "hl_shift" }

// Reset clears all per-series state for replay/reanalysis.
func (d *HLShiftDetector) Reset() {
	d.series = make(map[hlStateKey]*hlSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series window state for refs that storage has freed.
// Without this hook the per-series map keeps growing with the cumulative
// series count even after storage shrinks (mirrors mannkendall / wasserstein /
// tukey_biweight).
func (d *HLShiftDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, hlStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration shape is a structural
// copy of WassersteinDetector.Detect (metrics_detector_wasserstein.go:167-238)
// — cache series, bulk-fetch status, replay-skip when nothing has changed,
// then walk only the strictly-new points via ForEachPoint.
func (d *HLShiftDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := hlStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &hlSeriesState{}
				d.series[sk] = state
			}

			// Replay-skip: no new data and no in-place writes.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				d.appendPoint(state, p)

				// Decrement cooldown per ingested point so it expires
				// regardless of whether we score this tick.
				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}
				state.ticksSinceScore++

				if state.refCount >= d.MinPoints && state.curCount >= d.MinPoints &&
					state.cooldownLeft == 0 && state.ticksSinceScore >= d.ScoreEvery {
					state.ticksSinceScore = 0
					if anomaly, fired := d.scoreShift(state, seriesMeta, agg, p.Timestamp); fired {
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
//
// Same two-buffer slide pattern as wasserstein
// (metrics_detector_wasserstein.go:248-268).
func (d *HLShiftDetector) appendPoint(state *hlSeriesState, p observer.Point) {
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

// hlSnapshot returns the contents of a ring buffer in chronological order as
// a fresh []float64. Allocated only on scoring ticks (gated by ScoreEvery +
// cooldown + both-windows-full) so allocation pressure is amortized.
//
// Mirrors wassersteinSnapshot (metrics_detector_wasserstein.go:275-288).
func hlSnapshot(buf []observer.Point, head, count, cap int) []float64 {
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

// scoreShift runs the H-L dual gate (MAD-scaled magnitude + same-sign
// confirmation) on the current ref/cur pair. Returns (anomaly, fired). Pure
// function over the ring snapshots — no mutation of state happens here.
func (d *HLShiftDetector) scoreShift(state *hlSeriesState, series *observer.Series, agg observer.Aggregate, dataTime int64) (observer.Anomaly, bool) {
	n := d.WindowSize
	if state.refCount < n || state.curCount < n {
		return observer.Anomaly{}, false
	}

	ref := hlSnapshot(state.refBuf, state.refHead, state.refCount, d.WindowSize)
	cur := hlSnapshot(state.curBuf, state.curHead, state.curCount, d.WindowSize)

	medRef := detectorMedian(ref)
	madRef := detectorMAD(ref, medRef, true)

	// Scale-stable denominator: when the ref window is constant or near-
	// constant, fall back to a percent-of-magnitude scale so tiny absolute
	// shifts on a level-100 series don't produce gigantic shiftMAD values
	// from a near-zero MAD. Mirrors wasserstein:308-311.
	scaleDenom := madRef
	if scaleDenom < 1e-10 {
		scaleDenom = math.Max(math.Abs(medRef)*0.01, 1e-6)
	}

	// Sort copies of ref and cur ascending. Sorting both lets us walk the
	// pairwise diff matrix by row (ref) over column-sorted cur, which keeps
	// the diff array's distribution coherent — though the median is
	// invariant under row/column ordering, presorting also gives marginally
	// better cache behaviour and matches the wasserstein:314-319 idiom.
	refSorted := make([]float64, n)
	curSorted := make([]float64, n)
	copy(refSorted, ref)
	copy(curSorted, cur)
	sort.Float64s(refSorted)
	sort.Float64s(curSorted)

	// Walsh differences: every pair (i, j) over the N×N grid produces
	// d_ij = cur_sorted[j] - ref_sorted[i]. The H-L shift estimator is the
	// median of these N² values. O(N²) fill + O(N² log N) sort.
	diffs := make([]float64, n*n)
	idx := 0
	for i := 0; i < n; i++ {
		ri := refSorted[i]
		for j := 0; j < n; j++ {
			diffs[idx] = curSorted[j] - ri
			idx++
		}
	}
	sort.Float64s(diffs)
	// detectorMedian copies the slice; we already sorted in place, so pull
	// the median directly to avoid a redundant allocation+sort on N²=1600
	// values per scoring tick.
	deltaHL := hlMedianSorted(diffs)

	shiftMAD := math.Abs(deltaHL) / scaleDenom

	// Glitch cap: shiftMAD beyond hlGlitchShiftCap is almost certainly a
	// sensor-glitch artefact (1e308 sentinel that snuck past the storage
	// guard, or a counter reset converted to a huge negative). Drop rather
	// than emit a runaway score.
	if shiftMAD > hlGlitchShiftCap {
		return observer.Anomaly{}, false
	}

	// Primary gate: MAD-scaled magnitude.
	if shiftMAD < d.ShiftThresholdMAD {
		return observer.Anomaly{}, false
	}

	// Confirmation gate: structural anti-overfire. Require a MAJORITY of
	// cur values to have moved in the same direction as delta_HL relative
	// to median(ref). With a single tail outlier in cur (the W1 over-fire
	// failure mode from exp-0086/exp-0089) only 1/N moves; with a real
	// level shift, ~N/N do. ConfirmFraction=0.75 sits comfortably between.
	dir := 1.0
	if deltaHL < 0 {
		dir = -1.0
	}
	confirmCount := 0
	for _, v := range cur {
		// sign test against medRef in the direction of delta_HL. Equal
		// values (v == medRef) don't count toward either side; the gate
		// requires strict same-sign movement, which is what we want from a
		// "majority moved" guarantee.
		if (v-medRef)*dir > 0 {
			confirmCount++
		}
	}
	confirmFrac := float64(confirmCount) / float64(n)
	if confirmFrac < d.ConfirmFraction {
		return observer.Anomaly{}, false
	}

	direction := "increasing"
	if deltaHL < 0 {
		direction = "decreasing"
	}

	// Score = shiftMAD, clamped to a sane visual range so a single near-
	// degenerate series cannot dominate the downstream UI/correlator
	// scoring. Glitch-cap above already filters > hlGlitchShiftCap; we keep
	// the explicit min here to mirror the wasserstein/mannkendall idiom.
	score := shiftMAD
	if score > hlGlitchShiftCap {
		score = hlGlitchShiftCap
	}

	// Concatenate ref+cur in chronological order for the median-interval
	// helper. Same shape as wasserstein:358-373.
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
		Title:        "H-L shift: " + seriesName,
		Description: fmt.Sprintf("%s level shift (delta_HL=%.4f, shiftMAD=%.2f, confirm=%d/%d, n=%d)",
			direction, deltaHL, shiftMAD, confirmCount, n, n),
		Timestamp:           dataTime,
		Score:               &score,
		SamplingIntervalSec: medianPointInterval(chronological),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medRef,
			BaselineMAD:    madRef,
			CurrentValue:   chronological[len(chronological)-1].Value,
			DeviationSigma: shiftMAD,
		},
	}
	return anomaly, true
}

// hlMedianSorted returns the median of an already-sorted []float64 without
// re-sorting. Used to avoid a redundant copy+sort on the N²=1600 diff array
// in scoreShift's hot path.
func hlMedianSorted(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *HLShiftDetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = 40
	}
	if d.MinPoints <= 0 {
		d.MinPoints = d.WindowSize
	}
	if d.MinPoints > d.WindowSize {
		// Scoring requires both buffers to be filled, so cap MinPoints.
		d.MinPoints = d.WindowSize
	}
	if d.ShiftThresholdMAD <= 0 {
		d.ShiftThresholdMAD = 4.0
	}
	if d.ConfirmFraction <= 0 {
		d.ConfirmFraction = 0.75
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
		d.series = make(map[hlStateKey]*hlSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
