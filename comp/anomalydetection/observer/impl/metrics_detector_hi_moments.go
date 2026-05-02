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

// hmGlitchScoreCap is the upper bound on the joint Mahalanobis score for a
// fire. Beyond this we drop the candidate as a sensor-glitch artefact rather
// than emit a runaway score — same convention as hlGlitchShiftCap=50,
// wassersteinDistance cap=50, and tukey_biweight tbGlitchZCap=50.
const hmGlitchScoreCap = 50.0

// himVarFloor prevents division-by-zero when computing g1/g2 on a window with
// vanishing M2 (e.g. an exactly-constant cur ring). Picked far below any
// realistic |M2/N| to avoid biasing live series.
const himVarFloor = 1e-20

// himZDenomFloor is the minimum sqrt(var_g1) / sqrt(var_g2) used when scaling
// g1 / g2 against the reference ring. Floors prevent over-amplification on
// near-stationary reference windows where var_g1/var_g2 collapse to ~0.
const himZDenomFloor = 1e-3

// hmStateKey identifies per-series state by ref and aggregation.
type hmStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// hmSeriesState holds streaming state per (series, aggregate) pair.
//
// Memory footprint per key:
//
//	ring         (60 * 8)                     =  480 B
//	g1Ring       (20 * 8)                     =  160 B
//	g2Ring       (20 * 8)                     =  160 B
//	scalars (mean/m2/m3/m4 + cursors + …)     ~  100 B
//	                                          -------
//	                                          ~ 900 B
//
// Cheaper than wasserstein (~2 KB) and tukey_biweight (~1.4 KB) and one of the
// lightest detectors in the catalog.
type hmSeriesState struct {
	// Cursor (mirrors mannkendall / wasserstein / hl_shift).
	lastProcessedCount int
	lastWriteGen       int64
	// lastProcessedTime is the highest point timestamp consumed so far. Used
	// as the exclusive lower bound for ForEachPoint so each point is appended
	// to the windows exactly once across replays/incremental advances.
	lastProcessedTime int64

	// ring is the chronological cur window. Once full, the oldest entry at
	// state.head is evicted into the Welford remove path before the new point
	// is welforded in. While filling, head stays at 0 and entries grow at
	// position count.
	ring  [himWindowSize]float64
	head  int
	count int

	// Welford running 4 moments over ring (count points).
	mean, m2, m3, m4 float64

	// Reference ring of (g1, g2) snapshots. One snapshot is taken every
	// himRefStride ticks once cur is full (= one snapshot per full window
	// turnover). Mean/var of the ring is recomputed on each scoring tick
	// (R=20 entries; cheap).
	g1Ring    [himRefLen]float64
	g2Ring    [himRefLen]float64
	gRingHead int
	gRingFill int
	// stride counts ticks since the last reference snapshot; when it hits
	// himRefStride a new (g1, g2) snapshot is pushed into the reference ring
	// and stride is reset to 0.
	stride int

	// seen counts every ingested point (used as the warmup gate against
	// himMinPoints) and is monotonic across calls. cooldownLeft decrements per
	// ingested point so cooldown expires regardless of whether scoring runs.
	seen         int
	cooldownLeft int
	lastFireTime int64
}

// HiMomentsDetector tracks streaming central moments through order 4 (Pébay
// 2008, "Formulas for Robust, One-Pass Parallel Computation of Covariances and
// Arbitrary-Order Statistical Moments") on a rolling current window plus a
// stable reference baseline of (skewness g1, excess kurtosis g2) snapshots.
//
// On each new point both the cur ring and its (mean, M2, M3, M4) are updated
// in O(1) via Welford add+remove recursions. Once per full window turnover
// (every WindowSize ticks) a fresh (g1, g2) snapshot is pushed into the
// reference ring. Once the reference ring is fully populated AND the warmup
// counter clears MinPoints, every tick computes z1=(g1-mean_g1)/std_g1 and
// z2=(g2-mean_g2)/std_g2 against the reference ring and emits an anomaly when
// both of:
//
//	score = sqrt(z1² + z2²) >= ZThreshold
//	max(|z1|, |z2|)         >= MagGate
//
// fire. The dual gate is the orthogonality contract that makes hi_moments
// non-redundant with biweight / H-L: g1 and g2 are translation-invariant, so a
// pure level shift never moves them and never fires this detector. mannkendall
// and wasserstein already cover that case; hi_moments isolates SHAPE changes
// (heavy-tail emergence, asymmetry flip, bimodal→unimodal collapse).
//
// Implements observer.Detector with explicit cursoring (modeled after
// MannKendall / Wasserstein / HLShift) and observer.SeriesRemover for
// compatibility with the catalog teardown contract validated by
// TestDefaultCatalog_DetectorTeardownContract.
type HiMomentsDetector struct {
	// WindowSize is the cur ring length and the reference-snapshot stride.
	// Default: 60 — matches mannkendall / wasserstein (~1 minute at 1 Hz).
	WindowSize int
	// ReferenceLen is the number of (g1, g2) snapshots in the reference ring.
	// Default: 20 — total warmup is N + R*N = 1260 ticks.
	ReferenceLen int
	// MinPoints is the warmup gate: no firing until seen >= MinPoints AND the
	// reference ring is full. Default: WindowSize + ReferenceLen*WindowSize.
	MinPoints int
	// ZThreshold is the joint sqrt(z1²+z2²) above which a fire is allowed.
	// Default: 5.0 — matches mannkendall / tukey_biweight.
	ZThreshold float64
	// MagGate is the marginal gate: max(|z1|, |z2|) must clear this. Without
	// it, two coincident sub-threshold moves could produce a fire on noise.
	// Default: 4.0.
	MagGate float64
	// CooldownPoints is the per-series suppression window after a fire.
	// Default: 60 — matches mannkendall / wasserstein / hl_shift / tukey_biweight.
	CooldownPoints int
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[hmStateKey]*hmSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// Compile-time fixed sizes; making them constants lets the ring + g1/g2 ring
// live as fixed-size arrays inside hmSeriesState (no slice header / heap
// indirection). If WindowSize / ReferenceLen are tuned via fields above, the
// fields are still authoritative for control flow — the constants are merely
// the storage upper bound.
const (
	himWindowSize = 60
	himRefLen     = 20
	himRefStride  = himWindowSize
	himMinPoints  = himWindowSize + himRefLen*himRefStride
)

// NewHiMomentsDetector returns a Hi-Moments detector with default settings.
// Parameterless to match the wasserstein / tukey_biweight / hl_shift factory
// pattern so the catalog entry's `func(any) any { return New<...>() }` shape
// works without per-detector config plumbing.
func NewHiMomentsDetector() *HiMomentsDetector {
	return &HiMomentsDetector{
		WindowSize:     himWindowSize,
		ReferenceLen:   himRefLen,
		MinPoints:      himMinPoints,
		ZThreshold:     5.0,
		MagGate:        4.0,
		CooldownPoints: 60,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[hmStateKey]*hmSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *HiMomentsDetector) Name() string { return "hi_moments" }

// Reset clears all per-series state for replay/reanalysis.
func (d *HiMomentsDetector) Reset() {
	d.series = make(map[hmStateKey]*hmSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series window state for refs that storage has freed.
// Without this hook the per-series map keeps growing with the cumulative
// series count even after storage shrinks (mirrors mannkendall / wasserstein /
// hl_shift / tukey_biweight).
func (d *HiMomentsDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, hmStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration shape is a structural
// copy of MannKendallDetector.Detect (metrics_detector_mannkendall.go:142-215)
// — cache series, bulk-fetch status, replay-skip when nothing has changed,
// then walk only the strictly-new points via ForEachPoint.
func (d *HiMomentsDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := hmStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &hmSeriesState{}
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
				d.appendPoint(state, p.Value)

				// Decrement cooldown per ingested point so it expires regardless
				// of whether we score this tick.
				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}

				// Once cur is full we accumulate ticks toward the next reference
				// snapshot. The first snapshot lands at tick WindowSize (the
				// moment the ring first fills), then again every WindowSize
				// ticks thereafter — i.e. one snapshot per full window
				// turnover.
				if state.count >= d.WindowSize {
					state.stride++
					if state.stride >= himRefStride {
						state.stride = 0
						g1, g2 := himG1G2(state)
						state.g1Ring[state.gRingHead] = g1
						state.g2Ring[state.gRingHead] = g2
						state.gRingHead = (state.gRingHead + 1) % d.ReferenceLen
						if state.gRingFill < d.ReferenceLen {
							state.gRingFill++
						}
					}
				}

				if state.seen >= d.MinPoints &&
					state.gRingFill >= d.ReferenceLen &&
					state.cooldownLeft == 0 {
					if anomaly, fired := d.scoreShape(state, seriesMeta, agg, p.Timestamp); fired {
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

// appendPoint advances the streaming state by exactly one point: when the cur
// ring is full, the oldest entry is welforded OUT (Pébay reverse) before the
// new point is welforded IN; while the ring is filling, only the IN path runs.
// Both paths are O(1) — 4 fused-multiply-adds each.
//
// The seen counter is incremented unconditionally; it gates warmup against
// MinPoints in Detect.
func (d *HiMomentsDetector) appendPoint(state *hmSeriesState, x float64) {
	if state.count < d.WindowSize {
		// Ring still filling. Append at position state.count (head stays 0).
		state.ring[state.count] = x
		state.count++
		himAdd(state, x, state.count-1) // n_pre = count-1
		state.seen++
		return
	}
	// Ring full: evict ring[head] via Welford remove, then write x there and
	// welford it in. head advances modulo WindowSize.
	oldest := state.ring[state.head]
	himRemove(state, oldest, state.count) // n_old = WindowSize
	state.ring[state.head] = x
	himAdd(state, x, state.count-1) // n_pre = WindowSize - 1 (after remove)
	state.head = (state.head + 1) % d.WindowSize
	// state.count unchanged at WindowSize.
	state.seen++
}

// himAdd applies Pébay 2008's one-pass ADD recursion for moments through order
// 4. n_pre is the count BEFORE this point is added (state already has count
// nominally tracking the post-add count, hence callers pass count-1).
//
// Update order: M4 first (uses old M2, M3), M3 next (uses old M2), M2 last,
// then mean. This is the canonical order — referenced moments must still be
// in their pre-update form.
func himAdd(state *hmSeriesState, x float64, nPre int) {
	nPost := nPre + 1
	delta := x - state.mean
	deltaN := delta / float64(nPost)
	deltaN2 := deltaN * deltaN
	term1 := delta * deltaN * float64(nPre) // = delta² * nPre/nPost

	// Pébay 2008 §3 eq 2.1-2.4.
	state.m4 += term1*deltaN2*float64(nPost*nPost-3*nPost+3) +
		6*deltaN2*state.m2 - 4*deltaN*state.m3
	state.m3 += term1*deltaN*float64(nPost-2) - 3*deltaN*state.m2
	state.m2 += term1
	state.mean += deltaN
}

// himRemove applies the Welford-style one-pass REMOVE recursion: given moments
// over nOld points (which include x), produce moments over nOld-1 points (x
// excluded). Derived as the algebraic inverse of himAdd: pre-add state is the
// state AFTER removing x.
//
// Verified by round-trip: himAdd ∘ himRemove (and the converse) returns to
// identity within 1e-9 on 1000 random points (TestHIMoments_AddRemoveWelfordIsExact).
//
// Update order: M2 first (uses post-update mean), M3 next (uses post-update
// M2), M4 last (uses post-update M2 and M3). This is the inverse of himAdd's
// order — a referenced moment must already be in its post-update form.
//
// NOTE on the formula: an earlier draft followed a published "remove" formula
// that referenced PRE-update M2 inside the M3 term; that produced a
// non-round-tripping update (e.g. removing 3 from {1,2,3} yielded M3=-2
// instead of 0). The version below is re-derived from the inverse of Pébay's
// add and verified numerically; see the test for the proof.
func himRemove(state *hmSeriesState, x float64, nOld int) {
	nNew := nOld - 1
	if nNew <= 0 {
		// Removing the last point: state collapses to identity.
		state.mean, state.m2, state.m3, state.m4 = 0, 0, 0, 0
		return
	}
	delta := x - state.mean
	deltaN := delta / float64(nNew)
	deltaN2 := deltaN * deltaN
	deltaN3 := deltaN2 * deltaN

	meanNew := (float64(nOld)*state.mean - x) / float64(nNew)

	// M2 first; uses meanNew (computed above, not yet stored).
	m2New := state.m2 - delta*(x-meanNew)

	// M3 uses M2_new (post-update).
	m3New := state.m3 -
		deltaN2*delta*float64(nOld)*float64(nOld-2) +
		3*deltaN*m2New

	// M4 uses M2_new and M3_new (both post-update).
	m4New := state.m4 -
		delta*deltaN3*float64(nOld)*float64(nOld*nOld-3*nOld+3) -
		6*deltaN2*m2New +
		4*deltaN*m3New

	state.m2 = m2New
	state.m3 = m3New
	state.m4 = m4New
	state.mean = meanNew
}

// himG1G2 returns (skewness g1, excess kurtosis g2) of the cur ring from its
// running moments. Pure function over (M2, M3, M4, count); allocates nothing.
//
//	var = M2 / N
//	std = sqrt(max(var, himVarFloor))
//	g1  = (M3 / N) / std³
//	g2  = (M4 / N) / var² - 3
func himG1G2(state *hmSeriesState) (g1, g2 float64) {
	n := float64(state.count)
	if n <= 0 {
		return 0, 0
	}
	variance := state.m2 / n
	if variance < himVarFloor {
		variance = himVarFloor
	}
	std := math.Sqrt(variance)
	g1 = (state.m3 / n) / (std * std * std)
	g2 = (state.m4/n)/(variance*variance) - 3
	return g1, g2
}

// scoreShape runs the joint (g1, g2) Mahalanobis-style gate against the
// reference ring's mean/var. Returns (anomaly, fired). Pure function over
// state — does not mutate it. Caller is responsible for setting cooldownLeft
// after a fire.
func (d *HiMomentsDetector) scoreShape(state *hmSeriesState, series *observer.Series, agg observer.Aggregate, dataTime int64) (observer.Anomaly, bool) {
	g1, g2 := himG1G2(state)

	// Reference ring stats. Sum then a second pass for variance keeps the
	// arithmetic obvious — R=20 entries makes both passes negligible
	// (<<microsecond, no allocation).
	r := state.gRingFill
	if r < 2 {
		// Cannot compute variance with <2 samples; engine should have gated
		// against this already, but be defensive.
		return observer.Anomaly{}, false
	}
	rF := float64(r)

	var sum1, sum2 float64
	for i := 0; i < r; i++ {
		sum1 += state.g1Ring[i]
		sum2 += state.g2Ring[i]
	}
	mean1 := sum1 / rF
	mean2 := sum2 / rF

	var var1, var2 float64
	for i := 0; i < r; i++ {
		d1 := state.g1Ring[i] - mean1
		d2 := state.g2Ring[i] - mean2
		var1 += d1 * d1
		var2 += d2 * d2
	}
	// Population variance (consistent with the moment-bag interpretation of
	// the ring — we treat the ring as the full reference, not a sample of a
	// larger one).
	var1 /= rF
	var2 /= rF

	std1 := math.Sqrt(var1)
	if std1 < himZDenomFloor {
		std1 = himZDenomFloor
	}
	std2 := math.Sqrt(var2)
	if std2 < himZDenomFloor {
		std2 = himZDenomFloor
	}

	z1 := (g1 - mean1) / std1
	z2 := (g2 - mean2) / std2
	score := math.Sqrt(z1*z1 + z2*z2)

	mag := math.Abs(z1)
	if math.Abs(z2) > mag {
		mag = math.Abs(z2)
	}

	// Glitch cap: an extreme score is almost certainly a sensor-glitch
	// artefact (NaN-converted 1e308, 1e-300 underflow yanking var to ~0).
	// Drop rather than emit a runaway score. Mirrors hl_shift / tukey_biweight.
	if score > hmGlitchScoreCap {
		return observer.Anomaly{}, false
	}

	// Primary gate: joint score above ZThreshold.
	if score < d.ZThreshold {
		return observer.Anomaly{}, false
	}
	// Marginal gate: at least one moment must be strongly off. Without this,
	// two coincident sub-threshold moves could combine into a fire on
	// borderline noise.
	if mag < d.MagGate {
		return observer.Anomaly{}, false
	}

	direction := "skew_up"
	switch {
	case math.Abs(z2) >= math.Abs(z1) && z2 >= 0:
		direction = "kurtosis_up"
	case math.Abs(z2) >= math.Abs(z1) && z2 < 0:
		direction = "kurtosis_down"
	case z1 >= 0:
		direction = "skew_up"
	case z1 < 0:
		direction = "skew_down"
	}

	// Score is the joint magnitude, already bounded above by hmGlitchScoreCap
	// (we returned without firing if it exceeded). Mirror the catalog idiom
	// of an explicit visual cap so a single near-degenerate ref ring cannot
	// dominate downstream UI/correlator scoring.
	visualScore := score
	if visualScore > hmGlitchScoreCap {
		visualScore = hmGlitchScoreCap
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "Shape shift: " + seriesName,
		Description: fmt.Sprintf("%s shape shift (g1=%.3f, g2=%.3f, z1=%.2f, z2=%.2f, score=%.2f, n=%d)",
			direction, g1, g2, z1, z2, score, state.count),
		Timestamp:           dataTime,
		Score:               &visualScore,
		SamplingIntervalSec: 0,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   mean1, // ref-ring mean of g1
			BaselineStddev: std1,  // ref-ring sigma of g1 (post-floor)
			CurrentValue:   g1,
			DeviationSigma: score,
		},
	}
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *HiMomentsDetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = himWindowSize
	}
	// WindowSize is bounded by the fixed-array storage; reject overrides that
	// would write past the ring.
	if d.WindowSize > himWindowSize {
		d.WindowSize = himWindowSize
	}
	if d.ReferenceLen <= 0 {
		d.ReferenceLen = himRefLen
	}
	if d.ReferenceLen > himRefLen {
		d.ReferenceLen = himRefLen
	}
	if d.MinPoints <= 0 {
		d.MinPoints = d.WindowSize + d.ReferenceLen*d.WindowSize
	}
	if d.ZThreshold <= 0 {
		d.ZThreshold = 5.0
	}
	if d.MagGate <= 0 {
		d.MagGate = 4.0
	}
	if d.CooldownPoints < 0 {
		d.CooldownPoints = 0
	}
	if d.CooldownPoints == 0 {
		d.CooldownPoints = 60
	}
	if d.series == nil {
		d.series = make(map[hmStateKey]*hmSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
