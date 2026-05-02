// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"math/rand"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ESN tunables. The reservoir geometry is fixed at compile time so the global
// reservoir matrix can live as a flat array on the detector struct without
// per-series allocation. Adjust with a recorded justification — these
// constants together control startup latency, false-positive discipline, and
// memory footprint per (series, aggregate) pair.
const (
	esnReservoirSize  = 20
	esnSpectralRadius = 0.9
	esnInputScale     = 0.5
	esnLeakRate       = 0.3 // leaky-integrator update factor

	esnSparsity = 0.2 // fraction of nonzero W_res entries

	esnLMSAlpha = 0.02 // online readout learning rate (LMS)
	esnRegL2    = 1e-3 // ridge penalty applied during the LMS update

	esnWarmupPoints = 80 // no firing or scoring during warmup
	esnTrainPoints  = 80 // additional adapt-only points after warmup

	esnZThreshold = 6.0 // |standardized innovation| floor (strict)
	esnEffectMAD  = 4.0 // |innovation|/MAD floor (effect-size gate)

	esnCooldownPoints = 30

	esnProjectionSeed = int64(0xE5) // deterministic reservoir seed
	esnInnovDecay     = 0.02        // EWMA discount for innovation mean/var

	esnInnRingSize = 128 // size of the innovation ring for MAD
)

// esnStateKey identifies per-series state by ref and aggregation.
type esnStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// esnSeriesState is the streaming state kept per (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	x        (20 * 8)               =  160 B
//	w        (20 * 8)                =  160 B
//	innRing  (128 * 8)               = 1024 B
//	scalars                          ~  100 B
//	                                   -------
//	                                  ~1.4 KB
type esnSeriesState struct {
	// Cursor (mirrors loda / mannkendall).
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// Reservoir state x and linear readout weights w. Both are per-series:
	// the reservoir matrix is global and frozen, only the projection of the
	// reservoir onto the predicted scalar is learned per series.
	x [esnReservoirSize]float64
	w [esnReservoirSize]float64

	// EWMA mean and variance of the prediction innovation e_t = y_t - y_hat_t.
	// EWMA (rather than Welford) is used to keep the rolling stats from being
	// permanently inflated by a single large spike: the innovation distribution
	// can drift after a regime change, and an unbounded sum of squared
	// deviations would otherwise depress future z scores indefinitely.
	innMean float64
	innVar  float64
	innN    int

	// Ring of recent innovations used for the MAD gate. detectorMAD operates
	// on a snapshot, so we keep a simple round-robin buffer.
	innRing [esnInnRingSize]float64
	innHead int
	innFill int

	// Predict-then-update bookkeeping. The detector predicts y_t from the
	// reservoir state observed at t-1; on the first ever tick we have no
	// previous state to run a prediction against, so we skip scoring.
	prevValue float64
	hasPrev   bool

	// Tick counters and cooldown tracking.
	seen         int
	cooldownLeft int
	lastFireTime int64
}

// ESNDetector is a streaming Echo State Network anomaly detector
// (Jaeger 2001; Lukoševičius & Jaeger 2009).
//
// At each tick it runs a 1-step-ahead nonlinear prediction of the current
// value from the reservoir state observed at the previous tick, computes the
// prediction innovation, and fires when the standardized innovation clears
// both a strict z-score floor and an effect-size MAD floor. Cooldown
// suppresses adjacent re-fires on the same regime.
//
// The reservoir matrix and input weights are GLOBAL (shared across all
// series, fixed by esnProjectionSeed) so two processes evaluating the same
// stream agree on every byte; only the linear readout weights w and the
// reservoir state x are per series. This keeps memory tight while still
// giving each series its own predictor.
//
// Implements observer.Detector, observer.SeriesRemover, and Reset(); the
// iteration shape (cached series + bulk status + ForEachPoint cursor)
// mirrors loda / mannkendall.
type ESNDetector struct {
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Global reservoir: input weights and recurrent matrix. Built once at
	// construction with a fixed seed so the detector is deterministic.
	wIn  [esnReservoirSize]float64
	wRes [esnReservoirSize][esnReservoirSize]float64

	// Per-series state keyed by ref+agg.
	series map[esnStateKey]*esnSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewESNDetector returns an ESNDetector with default settings and a
// deterministic reservoir matrix (seeded from esnProjectionSeed).
func NewESNDetector() *ESNDetector {
	d := &ESNDetector{
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[esnStateKey]*esnSeriesState),
	}
	d.buildReservoir()
	return d
}

// Name returns the detector name as registered in the catalog.
func (d *ESNDetector) Name() string { return "esn" }

// Reset clears all per-series state for replay/reanalysis. The reservoir
// matrix is left intact — it is constant by design.
func (d *ESNDetector) Reset() {
	d.series = make(map[esnStateKey]*esnSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Without this hook the per-series map would grow unbounded with the
// cumulative number of series ever observed even after storage shrinks
// (mirrors loda / mannkendall).
func (d *ESNDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, esnStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration pattern mirrors loda
// (metrics_detector_loda.go:193-247): cache series on SeriesGeneration,
// bulk-fetch status, replay-skip when nothing changed, then process only
// the strictly-new points via ForEachPoint.
func (d *ESNDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := esnStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &esnSeriesState{}
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

// processPoint runs the full per-tick ESN pipeline for a single new point
// and returns (anomaly, fired). The cooldown counter is decremented on every
// tick (anomaly or not) so it expires reliably even during long no-score
// stretches. Order is strictly predict-then-update: the prediction uses the
// reservoir state observed at t-1 (state.x BEFORE this tick), which is what
// makes the innovation a genuine 1-step-ahead error rather than a residual.
func (d *ESNDetector) processPoint(state *esnSeriesState, series *observer.Series, agg observer.Aggregate, p observer.Point) (observer.Anomaly, bool) {
	y := p.Value

	// Step 1: predict y_hat from the reservoir state observed at t-1 (i.e.
	// state.x BEFORE this tick's update). The first ever tick has no past
	// state, so y_hat is bound to 0 / the readout's initial zero weights;
	// step 3 guards against scoring on that pathological first innovation.
	var yHat float64
	for i := 0; i < esnReservoirSize; i++ {
		yHat += state.w[i] * state.x[i]
	}

	// Step 2: compute the innovation and run the EWMA / ring / LMS updates,
	// but only once we have a previous value — otherwise the very first tick
	// would inject a pseudo-innovation of (y - 0) into the rolling stats and
	// permanently bias them.
	var (
		innovation float64
		hadInnov   bool
	)
	if state.hasPrev {
		innovation = y - yHat
		hadInnov = true

		// EWMA-style mean / variance update on the innovation. Welford was an
		// option but the constant esnInnovDecay explicitly defines the
		// rolling-window discount, and a Welford accumulator never forgets:
		// a single regime change would inflate variance forever and gate the
		// detector from ever firing again.
		if state.innN == 0 {
			state.innMean = innovation
			state.innVar = 0
		} else {
			delta := innovation - state.innMean
			state.innMean = (1-esnInnovDecay)*state.innMean + esnInnovDecay*innovation
			state.innVar = (1-esnInnovDecay)*state.innVar + esnInnovDecay*delta*delta
		}
		state.innN++

		// Push to MAD ring.
		state.innRing[state.innHead] = innovation
		state.innHead = (state.innHead + 1) % esnInnRingSize
		if state.innFill < esnInnRingSize {
			state.innFill++
		}

		// Online LMS readout update on the state used for prediction (the
		// PRE-update reservoir state). Ridge term shrinks weights toward zero
		// so unused reservoir directions don't drift unboundedly during long
		// stable phases.
		for i := 0; i < esnReservoirSize; i++ {
			state.w[i] += esnLMSAlpha * (innovation*state.x[i] - esnRegL2*state.w[i])
		}
	}

	// Step 3: leaky-integrator reservoir update with the current input y. We
	// always update the reservoir (even when scoring is suppressed) so the
	// state evolves with the stream during warmup.
	var pre [esnReservoirSize]float64
	for i := 0; i < esnReservoirSize; i++ {
		pi := d.wIn[i] * y
		row := &d.wRes[i]
		for j := 0; j < esnReservoirSize; j++ {
			pi += row[j] * state.x[j]
		}
		pre[i] = pi
	}
	for i := 0; i < esnReservoirSize; i++ {
		state.x[i] = (1-esnLeakRate)*state.x[i] + esnLeakRate*math.Tanh(pre[i])
	}

	state.prevValue = y
	state.hasPrev = true
	state.seen++

	// Cooldown decrements per ingested point so it expires regardless of
	// whether scoring runs on this tick.
	if state.cooldownLeft > 0 {
		state.cooldownLeft--
	}

	// Step 4: warmup + adapt phases — never fire while still calibrating.
	if state.seen <= esnWarmupPoints+esnTrainPoints {
		return observer.Anomaly{}, false
	}
	if !hadInnov || state.innN < 2 {
		return observer.Anomaly{}, false
	}

	// Step 5: standardize the innovation against the EWMA mean / var.
	std := math.Sqrt(state.innVar + 1e-12)
	z := (innovation - state.innMean) / std
	zAbs := math.Abs(z)

	// Step 6: MAD over the innovation ring snapshot. We center on the ring's
	// own median (rather than the EWMA mean) to make MAD a pure dispersion
	// measure, then divide |innovation| by MAD for the effect-size gate.
	snap := make([]float64, state.innFill)
	copy(snap, state.innRing[:state.innFill])
	med := detectorMedian(snap)
	mad := detectorMAD(snap, med, false)
	if mad < 1e-9 {
		mad = 1e-9
	}
	effect := math.Abs(innovation) / mad

	if zAbs < esnZThreshold || effect < esnEffectMAD {
		return observer.Anomaly{}, false
	}
	if state.cooldownLeft > 0 {
		return observer.Anomaly{}, false
	}

	// Step 7: emit. Score is |z| capped at 50 to keep downstream UI / scoring
	// sane in the face of pathological numerical edges (the same cap used by
	// mannkendall).
	score := zAbs
	if score > 50 {
		score = 50
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "ESN innovation: " + seriesName,
		Description: fmt.Sprintf("%s ESN innovation z=%.2f, e=%.4f, mad=%.4f (mean=%.4f, var=%.4f)",
			seriesName, z, innovation, mad, state.innMean, state.innVar),
		Timestamp: p.Timestamp,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   state.innMean,
			BaselineStddev: std,
			BaselineMedian: med,
			BaselineMAD:    mad,
			CurrentValue:   y,
			DeviationSigma: zAbs,
		},
	}

	state.cooldownLeft = esnCooldownPoints
	state.lastFireTime = p.Timestamp
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields and rebuilds the global
// reservoir if a detector was constructed via &ESNDetector{} without using
// the constructor. Mirrors loda's lazy initialization pattern.
func (d *ESNDetector) ensureDefaults() {
	if d.series == nil {
		d.series = make(map[esnStateKey]*esnSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
	// Detect a zeroed reservoir (struct-literal construction) and rebuild.
	allZero := true
	for i := 0; i < esnReservoirSize && allZero; i++ {
		if d.wIn[i] != 0 {
			allZero = false
		}
	}
	if allZero {
		d.buildReservoir()
	}
}

// buildReservoir populates wIn and wRes deterministically.
//
// wIn entries are uniform in [-esnInputScale, +esnInputScale]; wRes entries
// are uniform in [-1, 1] but only with probability esnSparsity (the rest are
// zero). After population the recurrent matrix is rescaled so its spectral
// radius equals esnSpectralRadius — power iteration for 10 rounds is the
// standard approach (Lukoševičius 2012). If the iteration's estimate
// degenerates (rho ≈ 0 from a pathological RNG draw) we fall back to a
// deterministic scalar scaling derived from the expected operator norm.
func (d *ESNDetector) buildReservoir() {
	rng := rand.New(rand.NewSource(esnProjectionSeed)) //nolint:gosec // deterministic seed for repeatability

	// Input weights — scaled uniform.
	for i := 0; i < esnReservoirSize; i++ {
		d.wIn[i] = (rng.Float64()*2 - 1) * esnInputScale
	}

	// Recurrent matrix — sparse uniform [-1, 1].
	for i := 0; i < esnReservoirSize; i++ {
		for j := 0; j < esnReservoirSize; j++ {
			if rng.Float64() >= esnSparsity {
				d.wRes[i][j] = 0
				continue
			}
			d.wRes[i][j] = rng.Float64()*2 - 1
		}
	}

	// Power iteration for the spectral radius rho.
	var v [esnReservoirSize]float64
	for i := 0; i < esnReservoirSize; i++ {
		v[i] = rng.Float64()*2 - 1
	}
	rho := 0.0
	for iter := 0; iter < 10; iter++ {
		var w [esnReservoirSize]float64
		for i := 0; i < esnReservoirSize; i++ {
			var s float64
			for j := 0; j < esnReservoirSize; j++ {
				s += d.wRes[i][j] * v[j]
			}
			w[i] = s
		}
		var norm float64
		for i := 0; i < esnReservoirSize; i++ {
			norm += w[i] * w[i]
		}
		norm = math.Sqrt(norm)
		if norm < 1e-12 {
			rho = 0
			break
		}
		rho = norm
		for i := 0; i < esnReservoirSize; i++ {
			v[i] = w[i] / norm
		}
	}

	var scale float64
	if rho > 1e-12 {
		scale = esnSpectralRadius / rho
	} else {
		// Fallback: the expected operator-2 norm of an N×N sparse uniform
		// matrix with sparsity p is ~sqrt(N*p/3). Solve for the scaling that
		// targets esnSpectralRadius. This branch should be unreachable with
		// the fixed seed but keeps the constructor terminating in
		// pathological RNG states.
		denom := math.Sqrt(float64(esnReservoirSize) * esnSparsity / 3.0)
		if denom < 1e-12 {
			denom = 1
		}
		scale = esnSpectralRadius / denom
	}
	for i := 0; i < esnReservoirSize; i++ {
		for j := 0; j < esnReservoirSize; j++ {
			d.wRes[i][j] *= scale
		}
	}
}
