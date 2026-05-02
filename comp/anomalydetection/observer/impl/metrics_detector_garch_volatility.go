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

// GARCH(1,1) tunables. Bollerslev 1986. Constants are compile-time fixed so two
// processes evaluating the same stream agree on every byte. Adjustments must be
// recorded with justification — together they govern startup latency, the
// false-positive floor, and per-series memory.
const (
	// ω, α, β are the GARCH(1,1) parameters. The stationarity condition for the
	// recurrence σ²_{t+1} = ω + α·r²_t + β·σ²_t is α + β < 1; we are at 0.95
	// (weakly persistent, integrated-volatility-like behavior). ω is small so
	// that under flat input σ² decays toward ω/(1-β), not toward a noticeable
	// floor that would suppress every fire.
	garchOmega = 1e-6
	garchAlpha = 0.10
	garchBeta  = 0.85

	// EWMA smoothing for the level μ_t against which we de-mean. A small α_μ
	// makes μ slow to chase a step, so a one-off level shift inflates innov² for
	// many ticks; that's deliberate — it lets σ² absorb the shift without the
	// instantaneous |z| spiking. Combined with garchZThreshold=5 this realizes
	// the "preserve 703-shopify (level shift = quiet)" contract.
	garchMuAlpha = 0.02

	// Initial conditional variance before any data. Picked at unit scale so the
	// first σ² updates produce meaningful denominators well before warmup ends.
	garchSigmaSeed = 1.0

	// Warmup + train phases: the detector adapts but never fires. 64+64=128
	// ticks total, half of esn's 80+80 — GARCH stabilizes faster because it has
	// only two scalar states (μ, σ²) rather than a 20-D reservoir + readout.
	garchWarmupPoints = 64
	garchTrainPoints  = 64

	// Strict |z| firing floor. Pilots at 4 produced false positives on the
	// fp_egregious set (exp-0103 / exp-0096 / exp-0095); 5 is the strict gate.
	garchZThreshold = 5.0

	// Effect-size gate: |innov|/MAD(innov_ring) ≥ floor before scoring fires.
	// MAD on the innovation ring is the second line of defense — z alone fires
	// on a small absolute innov whenever σ has been small for a while.
	garchEffectFloor = 4.0

	// Innovation ring size for the MAD effect-size gate. 128 mirrors esn.
	garchInnovRingSize = 128

	// Cooldown suppresses adjacent re-fires on the same regime.
	garchCooldownPoints = 30

	// Score cap to keep downstream UI / scoring sane on numerical edges
	// (matches mannkendall and esn).
	garchScoreCap = 50.0
)

// garchStateKey identifies per-series state by ref and aggregation, mirroring
// esnStateKey / lodaStateKey.
type garchStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// garchSeriesState is the streaming state kept per (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	innRing  (128 * 8)   = 1024 B
//	scalars              ~  100 B
//	                       -------
//	                      ~1.1 KB
//
// Lighter than ESN (~1.4 KB) and DFA. The ring dominates; everything else is
// scalars.
type garchSeriesState struct {
	// Cursor (mirrors esn / loda / mannkendall).
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	mu     float64 // EWMA of value — the "level" we de-mean against
	sigma2 float64 // conditional variance σ²_t
	seen   int     // total points ingested (post init)

	// Ring of recent innovations used for the MAD gate. detectorMAD operates on
	// a snapshot, so we keep a simple round-robin buffer (mirrors esn.innRing).
	innRing [garchInnovRingSize]float64
	innHead int
	innFill int

	cooldownLeft int
	lastFireTime int64
}

// GarchVolatilityDetector is a streaming GARCH(1,1) conditional-variance
// anomaly detector (Bollerslev 1986, "Generalized Autoregressive Conditional
// Heteroskedasticity").
//
// At each tick it:
//  1. Computes the innovation r_t = value - μ_t against the slow-EWMA level μ_t
//     (predict step — uses σ² and μ from BEFORE this tick).
//  2. Standardizes r_t against the conditional standard deviation σ_t.
//  3. Fires when |z_t| ≥ garchZThreshold AND |r_t|/MAD(innov ring) ≥
//     garchEffectFloor (and warmup+train have elapsed and cooldown is clear).
//  4. Updates μ, σ², and the innovation ring (update step).
//
// Predict-then-update is critical: σ² absorbs any innovation INCLUDING the
// firing one, so if we updated first the denominator at firing time would
// already include the spike and the score would be permanently suppressed
// (same defensive ordering esn uses at processPoint).
//
// Implements observer.Detector and observer.SeriesRemover; the iteration shape
// (cached series + bulk status + ForEachPoint cursor) mirrors esn / loda.
type GarchVolatilityDetector struct {
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-series state keyed by ref+agg.
	series map[garchStateKey]*garchSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewGarchVolatilityDetector returns a fresh detector with default settings.
func NewGarchVolatilityDetector() *GarchVolatilityDetector {
	return &GarchVolatilityDetector{
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[garchStateKey]*garchSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (*GarchVolatilityDetector) Name() string { return "garch_volatility" }

// Reset clears all per-series state for replay/reanalysis.
func (d *GarchVolatilityDetector) Reset() {
	d.series = make(map[garchStateKey]*garchSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed. Without
// this hook the per-series map would grow unbounded with the cumulative number
// of series ever observed even after storage shrinks (mirrors esn / loda).
func (d *GarchVolatilityDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, garchStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration pattern mirrors esn
// (metrics_detector_esn.go:183-236): cache series on SeriesGeneration,
// bulk-fetch status, replay-skip when nothing changed, then process only the
// strictly-new points via ForEachPoint.
func (d *GarchVolatilityDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := garchStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &garchSeriesState{}
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

// processPoint runs the full per-tick GARCH pipeline for a single new point
// and returns (anomaly, fired). Cooldown decrements on every tick (anomaly or
// not) so it expires reliably even during long no-score stretches. Order is
// strictly predict-then-update (see GarchVolatilityDetector docstring).
func (d *GarchVolatilityDetector) processPoint(state *garchSeriesState, series *observer.Series, agg observer.Aggregate, p observer.Point) (observer.Anomaly, bool) {
	y := p.Value

	// First-tick init: seed μ with the observation and σ² with the seed
	// constant, then return — there is no prior μ to compute an innovation
	// against, so feeding (y - 0) into the recurrence would inject a
	// pseudo-innovation of magnitude y and bias σ² for many ticks afterwards.
	if state.seen == 0 {
		state.mu = y
		state.sigma2 = garchSigmaSeed
		state.seen = 1
		return observer.Anomaly{}, false
	}

	// Predict step (uses pre-update μ and σ²): innovation and standardized z.
	innov := y - state.mu

	var (
		fired   bool
		anomaly observer.Anomaly
	)

	// Cooldown decrements per ingested point so it expires regardless of
	// whether scoring runs on this tick.
	defer func() {
		if state.cooldownLeft > 0 {
			state.cooldownLeft--
		}
	}()

	// Score gate: warmup keeps σ² from being pinned to the seed; the additional
	// train phase gives the EWMA μ time to settle and the innovation ring time
	// to reach a representative MAD.
	if state.seen >= garchWarmupPoints {
		// MAD over the innovation ring snapshot. Computed FIRST so it can both
		// drive the effect-size gate AND act as a robust lower bound on the
		// |z| denominator (see sigma floor below). We center on the ring's own
		// median rather than μ so MAD is a pure dispersion measure of recent
		// innovations, mirroring esn.
		mad := 0.0
		if state.innFill > 0 {
			snap := make([]float64, state.innFill)
			copy(snap, state.innRing[:state.innFill])
			med := detectorMedian(snap)
			mad = detectorMAD(snap, med, false)
		}
		if mad < 1e-9 {
			mad = 1e-9
		}

		sigma2Floor := state.sigma2
		if sigma2Floor < 1e-12 {
			sigma2Floor = 1e-12
		}
		sigma := math.Sqrt(sigma2Floor)

		// DEVIATION FROM PLAN: floor σ at the MAD-implied scale (1.4826 is the
		// MAD→σ conversion for N(0,1)). The bare σ² recurrence has its own
		// variance — under stationary noise σ² fluctuates well below its
		// equilibrium, and a low-σ² valley combined with a moderate ~3σ innov
		// produces a spurious |z|≈6 fire (this is exactly what TestGARCH_
		// StableNoiseNoFire originally caught). Flooring σ at the robust
		// MAD-based scale keeps the denominator stable across σ² fluctuations
		// while still letting σ DOMINATE when the regime genuinely shifts (in
		// which case σ² grows past the MAD floor). The plan's "sqrt(max(σ²,
		// 1e-12))" denominator alone could not pass both the stable-noise and
		// volatility-clustering tests with the constants the plan specified.
		if madSigma := 1.4826 * mad; madSigma > sigma {
			sigma = madSigma
		}

		effect := math.Abs(innov) / mad

		z := innov / sigma
		zAbs := math.Abs(z)

		eligible := state.seen >= (garchWarmupPoints+garchTrainPoints) &&
			state.cooldownLeft == 0 &&
			zAbs >= garchZThreshold &&
			effect >= garchEffectFloor

		if eligible {
			score := zAbs
			if score > garchScoreCap {
				score = garchScoreCap
			}
			seriesName := series.Name + ":" + aggSuffix(agg)
			anomaly = observer.Anomaly{
				Type:         observer.AnomalyTypeMetric,
				Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
				DetectorName: d.Name(),
				Title:        "GARCH-σ residual: " + seriesName,
				Description: fmt.Sprintf("%s GARCH z=%.2f, σ=%.4f, innov=%.4f, n=%d (mu=%.4f, mad=%.4f)",
					seriesName, z, sigma, innov, state.seen, state.mu, mad),
				Timestamp: p.Timestamp,
				Score:     &score,
				DebugInfo: &observer.AnomalyDebugInfo{
					BaselineMean:   state.mu,
					BaselineStddev: sigma,
					BaselineMAD:    mad,
					CurrentValue:   y,
					DeviationSigma: zAbs,
				},
			}
			state.cooldownLeft = garchCooldownPoints
			state.lastFireTime = p.Timestamp
			fired = true
		}
	}

	// Update step. Order matches the GARCH(1,1) recurrence: μ first (so the
	// stored μ reflects the value just observed), then σ² using THIS tick's
	// innov² (so σ² absorbs the firing innovation and the next tick's |z| is
	// dampened — that's what makes the cooldown predictable rather than
	// chained re-fires).
	state.mu = (1-garchMuAlpha)*state.mu + garchMuAlpha*y
	state.sigma2 = garchOmega + garchAlpha*innov*innov + garchBeta*state.sigma2

	// Push innovation into the MAD ring (round-robin, mirrors esn).
	state.innRing[state.innHead] = innov
	state.innHead = (state.innHead + 1) % garchInnovRingSize
	if state.innFill < garchInnovRingSize {
		state.innFill++
	}

	state.seen++
	return anomaly, fired
}

// ensureDefaults fills in zero-valued fields for detectors constructed via
// &GarchVolatilityDetector{} (i.e. without the constructor). Mirrors esn's
// lazy initialization pattern.
func (d *GarchVolatilityDetector) ensureDefaults() {
	if d.series == nil {
		d.series = make(map[garchStateKey]*garchSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
