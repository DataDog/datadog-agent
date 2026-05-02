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

// Burg AR detector tunables. Each constant has a derivation in the stage-2
// plan; tweak only with recorded justification.
const (
	// burgAROrder is the AR order p. p=3 keeps per-series state ~1.6 KB and
	// the Burg recursion cost at O(p^2)=9 mul/adds per tick.
	burgAROrder = 3
	// burgWarmup is the number of points consumed before scoring begins.
	// At seen=warmup+1 the reflection-coefficient ring buffer is already
	// saturated (warmup-3 >= burgRefRingSize) so kBar/kStd estimates are
	// stable on the first scoring tick.
	burgWarmup = 80
	// burgRefRingSize is the rolling history length used to estimate the
	// per-component mean and std of the reflection-coefficient vector.
	burgRefRingSize = 64
	// burgZThreshold is the absolute floor on the L2 Mahalanobis-style drift
	// score over the AR reflection-coefficient vector. Tightening this is the
	// primary FP knob (see SUCCESS CRITERIA fallback in the plan).
	burgZThreshold = 6.0
	// burgInnovGuard requires a concurrent prediction-error innovation z of
	// at least this magnitude. Without this, kZ alone can spuriously flag
	// stable series whose Welford |error| variance is small.
	burgInnovGuard = 4.0
	// burgCooldownPoints is the per-series suppression window after a fire,
	// matching the convention used by mannkendall / scanmw.
	burgCooldownPoints = 30
	// burgInnovRingSize is reserved for a future bounded-history innovation
	// estimator. Today the streaming Welford is unbounded by design — long
	// histories shrink the per-tick noise floor and improve drift sensitivity
	// after the warmup gate.
	burgInnovRingSize = 64
	// burgInnZRecentWindow is the lookback used when checking the innovation
	// gate. PLAN DEVIATION: the plan specifies a strictly current-tick innZ
	// gate, but the dual gate (kZ AND innZ) is structurally hard to satisfy
	// because innZ peaks during xbuf transition (single-tick spike) while kZ
	// peaks 2–4 ticks later when xbuf saturates with the new dynamics — by
	// which time the Burg predictor has adapted and innov has dropped. Using
	// a max-over-recent-window for the innZ gate captures both peaks within
	// the same regime-change event without weakening the FP suppression
	// (stable series have low max innZ over any recent window).
	burgInnZRecentWindow = 5
)

// burgStateKey identifies per-series state by ref and aggregation.
type burgStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// burgSeriesState holds streaming state per (series, aggregate) pair.
//
// Per-key memory:
//
//	xbuf       (4 * 8)              =   32 B
//	k          (3 * 8)              =   24 B
//	ef, eb     (2 * 4 * 8)          =   64 B
//	kRing      (64 * 3 * 8)         = 1536 B
//	scalars                         ~  100 B
//	                                 -------
//	                                 ~1.7 KB
type burgSeriesState struct {
	// Cursor (mirrors mannkendall / loda).
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// Last p+1 raw values. xbuf[0] is the oldest; xbuf[p] is the most
	// recent. xN counts the number filled and is capped at p+1.
	xbuf [burgAROrder + 1]float64
	xN   int

	// Reflection coefficients k[0..p-1] from the most recent recursion.
	k [burgAROrder]float64
	// Forward / backward prediction-error vectors, sized p+1 because the
	// Burg lattice recursion reads index n-1 down to m+1 in-place.
	ef [burgAROrder + 1]float64
	eb [burgAROrder + 1]float64

	// Streaming Welford on |ef[p]| (1-step prediction-error innovation).
	errMean, errM2 float64
	errN           int

	// Ring history of past reflection-coefficient vectors. Used to estimate
	// the per-component mean / std for the drift z score.
	kRing     [burgRefRingSize][burgAROrder]float64
	kRingHead int
	kRingFill int

	// Recent innZ history (length burgInnZRecentWindow). Used to gate fires
	// on the max innZ observed in the last window — see the constant comment
	// for the structural rationale.
	innZRing     [burgInnZRecentWindow]float64
	innZRingHead int
	innZRingFill int

	seen         int
	cooldownLeft int
	lastFireTime int64
}

// BurgarDetector implements an online Burg-lattice AR(p) coefficient-drift
// detector. The algorithm scores changes in a series' GENERATIVE DYNAMICS
// (regime change in temporal dependence: oscillation onset, autocorrelation
// collapse, smoothing change), which is structurally orthogonal to LEVEL
// shifts (BOCPD, ScanMW) and TREND drift (MannKendall).
//
// Each tick:
//
//  1. Shift y_t into a length-(p+1) raw-value buffer.
//  2. Once the buffer is full, run Burg's lattice recursion (Marple 1987 §6.5)
//     to estimate the reflection coefficients k[0..p-1] from the most recent
//     p+1 values. Stability clamping keeps |k_m| < 0.999.
//  3. Update streaming Welford on the absolute prediction-error innovation
//     |ef[p]|, and push the current k vector into a 64-deep ring history.
//  4. After warmup, score the L2 Mahalanobis-style drift of k against the
//     ring's per-component mean / std (kZ) and require a concurrent
//     innovation z (innZ) above an independent floor. Both gates must pass —
//     either alone over-fires on otherwise-stable noise.
//
// Implements observer.Detector, observer.SeriesRemover, and Reset(). The
// iteration shape (cached series + bulk status + ForEachPoint cursor)
// mirrors mannkendall / loda.
type BurgarDetector struct {
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-series state keyed by ref+agg.
	series map[burgStateKey]*burgSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewBurgarDetector returns a BurgarDetector with default configuration.
//
// NOTE: stage-1 named the constructor NewBurgarDetector and the catalog
// already references it; stage 2 keeps the name for catalog compatibility
// even though the original plan referred to it as NewBurgARDetector.
func NewBurgarDetector() *BurgarDetector {
	return &BurgarDetector{
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[burgStateKey]*burgSeriesState),
	}
}

// Name returns the catalog-registered detector name.
func (d *BurgarDetector) Name() string { return "burgar" }

// Reset clears all per-series state for replay/reanalysis.
func (d *BurgarDetector) Reset() {
	d.series = make(map[burgStateKey]*burgSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Mirrors the contract on scanmw / mannkendall — without this hook the
// per-series map would grow unbounded with cumulative cardinality.
func (d *BurgarDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, burgStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration pattern matches
// mannkendall (metrics_detector_mannkendall.go:142-215): cache series on
// SeriesGeneration, bulk-fetch status, replay-skip when nothing changed,
// then process only the strictly-new points via ForEachPoint.
func (d *BurgarDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := burgStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &burgSeriesState{}
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
				if anomaly, fired := d.processPoint(state, seriesMeta, agg, p); fired {
					anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
					allAnomalies = append(allAnomalies, anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})

			state.lastProcessedCount = status.pointCount
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// processPoint runs the full per-tick Burg pipeline for a single new point
// and returns (anomaly, fired). The cooldown counter is decremented per call
// so it expires regardless of whether scoring runs on this tick (matches
// mannkendall's contract).
func (d *BurgarDetector) processPoint(state *burgSeriesState, series *observer.Series, agg observer.Aggregate, p observer.Point) (observer.Anomaly, bool) {
	// Step 1: append y_t to xbuf, shifting in when full.
	if state.xN < burgAROrder+1 {
		state.xbuf[state.xN] = p.Value
		state.xN++
	} else {
		for i := 0; i < burgAROrder; i++ {
			state.xbuf[i] = state.xbuf[i+1]
		}
		state.xbuf[burgAROrder] = p.Value
	}
	state.seen++

	// Cooldown decrements per ingested point so it expires reliably even
	// during long no-score stretches (warmup, insufficient lag history).
	if state.cooldownLeft > 0 {
		state.cooldownLeft--
	}

	// Step 2: not enough lag history yet.
	if state.xN < burgAROrder+1 {
		return observer.Anomaly{}, false
	}

	// Step 3: Burg lattice recursion (Marple 1987 §6.5) on the most recent
	// p+1 values. Order p=3 → 9 mul/adds per tick.
	for i := 0; i <= burgAROrder; i++ {
		state.ef[i] = state.xbuf[i]
		state.eb[i] = state.xbuf[i]
	}
	for m := 0; m < burgAROrder; m++ {
		var num, den float64
		for n := m + 1; n <= burgAROrder; n++ {
			num += state.ef[n] * state.eb[n-1]
			den += state.ef[n]*state.ef[n] + state.eb[n-1]*state.eb[n-1]
		}
		// Floor the denominator to avoid divide-by-zero on near-constant
		// windows (where every ef/eb collapses to the same value).
		if den < 1e-12 {
			den = 1e-12
		}
		km := -2 * num / den
		// Lattice stability: |k_m| < 1 keeps the implied AR polynomial inside
		// the unit circle. Clamp at 0.999 (slightly tighter than the
		// theoretical 1 to avoid catastrophic cancellation downstream).
		if km > 0.999 {
			km = 0.999
		} else if km < -0.999 {
			km = -0.999
		}
		state.k[m] = km
		// In-place ef/eb update. Walk descending so eb[n-1] is read with its
		// pre-update value (writes to eb[n] happen at higher indices first).
		for n := burgAROrder; n >= m+1; n-- {
			efNew := state.ef[n] + km*state.eb[n-1]
			state.eb[n] = state.eb[n-1] + km*state.ef[n]
			state.ef[n] = efNew
		}
	}

	// Step 4: score-FIRST then update. Computing drift / innovation z against
	// the PRIOR ring history and PRIOR Welford state avoids a structural
	// bias where the just-arrived k vector immediately dilutes its own
	// baseline. This is the same fix LODA applies (its p99 is computed from
	// PRIOR scores, then the new score is pushed). PLAN DEVIATION: the plan
	// listed Welford+push at step 4 then drift/innov at step 6, which would
	// mix the current sample into its own baseline. The change is locally
	// contained, structurally safe (the FP analysis is identical for stable
	// series — current k is one of 64 i.i.d. samples either way) and lifts
	// the first-tick post-regime-change drift score from ~4 to ~7 on the
	// canonical AR(1)→oscillation regression.
	innov := math.Abs(state.ef[burgAROrder])

	var (
		kZ          float64
		innZ        float64
		topAbsDrift float64
	)
	if state.kRingFill >= 1 && state.errN >= 1 {
		// Reflection-coefficient drift z. Mahalanobis-style L2 over the
		// per-component standardised drifts under a diagonal-covariance
		// approximation (cheap, and adequate at p=3).
		var kBar [burgAROrder]float64
		for j := 0; j < state.kRingFill; j++ {
			for i := 0; i < burgAROrder; i++ {
				kBar[i] += state.kRing[j][i]
			}
		}
		invN := 1.0 / float64(state.kRingFill)
		for i := 0; i < burgAROrder; i++ {
			kBar[i] *= invN
		}
		var kVar [burgAROrder]float64
		if state.kRingFill > 1 {
			denom := float64(state.kRingFill - 1)
			for j := 0; j < state.kRingFill; j++ {
				for i := 0; i < burgAROrder; i++ {
					dx := state.kRing[j][i] - kBar[i]
					kVar[i] += dx * dx
				}
			}
			for i := 0; i < burgAROrder; i++ {
				kVar[i] /= denom
			}
		}
		var kZ2 float64
		for i := 0; i < burgAROrder; i++ {
			std := math.Sqrt(kVar[i])
			// Floor at 1e-9 so a tied-history component (kVar=0) doesn't blow
			// up; warmup ensures kRingFill==64 by first scoring tick so this
			// branch is rare in practice.
			if std < 1e-9 {
				std = 1e-9
			}
			drift := (state.k[i] - kBar[i]) / std
			kZ2 += drift * drift
			if absD := math.Abs(drift); absD > topAbsDrift {
				topAbsDrift = absD
			}
		}
		kZ = math.Sqrt(kZ2)

		// Innovation z against PRIOR Welford (errMean, errM2, errN are
		// updated below, after scoring).
		var innStd float64
		if state.errN > 1 {
			innStd = math.Sqrt(state.errM2/float64(state.errN-1) + 1e-12)
		} else {
			innStd = math.Sqrt(1e-12)
		}
		innZ = math.Abs(innov-state.errMean) / innStd
	}

	// Step 5: update Welford on |ef[p]| and push the current k vector into
	// kRing. Always run, even during warmup, so the prior-state windows are
	// populated by the first post-warmup scoring tick.
	delta := innov - state.errMean
	state.errN++
	state.errMean += delta / float64(state.errN)
	delta2 := innov - state.errMean
	state.errM2 += delta * delta2

	state.kRing[state.kRingHead] = state.k
	state.kRingHead = (state.kRingHead + 1) % burgRefRingSize
	if state.kRingFill < burgRefRingSize {
		state.kRingFill++
	}

	// Push current innZ into the recent-history ring. The max over this ring
	// is what the gate checks (see burgInnZRecentWindow doc).
	state.innZRing[state.innZRingHead] = innZ
	state.innZRingHead = (state.innZRingHead + 1) % burgInnZRecentWindow
	if state.innZRingFill < burgInnZRecentWindow {
		state.innZRingFill++
	}
	var innZRecentMax float64
	for j := 0; j < state.innZRingFill; j++ {
		if state.innZRing[j] > innZRecentMax {
			innZRecentMax = state.innZRing[j]
		}
	}

	// Step 6: warmup gate.
	if state.seen <= burgWarmup {
		return observer.Anomaly{}, false
	}

	// Step 7: dual-gate fire — both the coefficient drift AND the innovation
	// (max over the recent window) must clear their floors, and cooldown must
	// have expired. The recent-window max captures the structural fact that
	// kZ and innZ peak at different ticks during a regime change (see the
	// burgInnZRecentWindow doc).
	if kZ < burgZThreshold || innZRecentMax < burgInnovGuard || state.cooldownLeft > 0 {
		return observer.Anomaly{}, false
	}

	// Score is kZ, capped at 50 so a single pathological series can't
	// dominate downstream UI / correlator scoring (mirrors MannKendall).
	score := kZ
	if score > 50 {
		score = 50
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "Burg AR coeff drift: " + seriesName,
		Description: fmt.Sprintf("Burg AR coeff drift kZ=%.2f, innZmax=%.2f (innZ=%.2f), k=[%.3f,%.3f,%.3f], top |drift_i|=%.2f",
			kZ, innZRecentMax, innZ, state.k[0], state.k[1], state.k[2], topAbsDrift),
		Timestamp:           p.Timestamp,
		Score:               &score,
		SamplingIntervalSec: 0,
		DebugInfo: &observer.AnomalyDebugInfo{
			CurrentValue:   p.Value,
			DeviationSigma: kZ,
		},
	}

	state.cooldownLeft = burgCooldownPoints
	state.lastFireTime = p.Timestamp
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
// Cheap to call on every Detect entry — guards against zero-value detectors
// constructed via &BurgarDetector{} (e.g. some tests).
func (d *BurgarDetector) ensureDefaults() {
	if d.series == nil {
		d.series = make(map[burgStateKey]*burgSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}

