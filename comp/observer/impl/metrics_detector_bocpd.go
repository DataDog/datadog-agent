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

// bocpdSeriesState holds per-series streaming BOCPD state.
type bocpdSeriesState struct {
	key observer.SeriesKey
	agg observer.Aggregate

	// Cursor: how many points we've processed so far (count-based for safety).
	lastProcessedTime  int64
	lastProcessedCount int

	// Warmup: Welford online mean/variance accumulation.
	initialized    bool
	warmupCount    int
	warmupMean     float64
	warmupM2       float64     // sum of squared deviations (Welford)
	warmupBuffer   []float64   // buffered values for posterior replay after init

	// Baseline (set once after warmup).
	baselineMean   float64
	baselineStddev float64
	obsVar         float64
	priorMean      float64
	priorPrecision float64

	// BOCPD posterior state (persists across advances).
	runProbs   []float64
	means      []float64
	precisions []float64

	// Pre-allocated swap buffers to avoid per-point allocation.
	newRunProbs   []float64
	newMeans      []float64
	newPrecisions []float64

	// Alert lifecycle.
	inAlert        bool
	alertStart     int64
	recoveryCount  int // consecutive non-triggering points since last trigger
}

// BOCPDDetector detects changepoints using Bayesian Online Changepoint Detection.
// This is a streaming, stateful Detector implementation that maintains per-series
// posterior state and processes only newly visible points on each advance.
type BOCPDDetector struct {
	// WarmupPoints is the number of initial points used for baseline estimation.
	// A longer warmup captures more of the metric's natural variability, reducing
	// false positives from normal fluctuation. Default: 120 (~2 minutes at 1Hz).
	WarmupPoints int

	// Hazard is the constant changepoint hazard probability.
	// Default: 0.05
	Hazard float64

	// CPThreshold is the posterior P(changepoint at t) threshold to emit.
	// Default: 0.6
	CPThreshold float64

	// ShortRunLength is the run-length horizon k for short-run posterior mass P(r_t <= k).
	// Default: 5
	ShortRunLength int

	// CPMassThreshold is the threshold for short-run posterior mass P(r_t <= k).
	// Default: 0.7
	CPMassThreshold float64

	// MaxRunLength caps tracked run-length hypotheses for bounded compute.
	// Default: 200
	MaxRunLength int

	// PriorVarianceScale controls prior variance over the mean relative to observed variance.
	// Default: 10.0
	PriorVarianceScale float64

	// MinVariance is the floor for observation variance. When warmup data has
	// near-zero variance (e.g. constant series), this prevents pathologically
	// sharp PDFs that would flag any tiny fluctuation as anomalous. Default: 1.0
	MinVariance float64

	// RecoveryPoints is how many consecutive non-triggering points are needed
	// to exit alert state. Default: 10
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count]
	Aggregations []observer.Aggregate

	// per-series state keyed by "namespace|name|tags|agg"
	series map[string]*bocpdSeriesState
}

// NewBOCPDDetector creates a streaming BOCPD detector with sensible defaults.
func NewBOCPDDetector() *BOCPDDetector {
	return &BOCPDDetector{
		WarmupPoints:       120,
		Hazard:             0.05,
		CPThreshold:        0.6,
		ShortRunLength:     5,
		CPMassThreshold:    0.7,
		MaxRunLength:       200,
		PriorVarianceScale: 10.0,
		MinVariance:        1.0,
		RecoveryPoints:     10,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[string]*bocpdSeriesState),
	}
}

// Name returns the detector name.
func (b *BOCPDDetector) Name() string {
	return "bocpd_detector"
}

// Detect implements Detector. It discovers series, reads only new points,
// and updates per-series BOCPD posterior state incrementally.
func (b *BOCPDDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	b.ensureDefaults()

	seriesKeys := storage.ListSeries(observer.SeriesFilter{})

	var allAnomalies []observer.Anomaly
	var allTelemetry []observer.ObserverTelemetry

	for _, key := range seriesKeys {
		for _, agg := range b.Aggregations {
			stateKey := b.stateKey(key, agg)

			// Get or create per-series state.
			state, exists := b.series[stateKey]
			if !exists {
				state = b.newSeriesState(key, agg)
				b.series[stateKey] = state
			}

			// Check if there are new points.
			visibleCount := storage.PointCountUpTo(key, dataTime)
			if visibleCount <= state.lastProcessedCount {
				continue
			}

			// Read only new points since last processed time.
			series := storage.GetSeriesRange(key, state.lastProcessedTime, dataTime, agg)
			if series == nil || len(series.Points) == 0 {
				continue
			}

			// Process each new point incrementally.
			for _, p := range series.Points {
				anomaly, telemetry := b.processPoint(state, p, series)
				if anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
				allTelemetry = append(allTelemetry, telemetry...)
			}

			// Update cursor.
			state.lastProcessedTime = series.Points[len(series.Points)-1].Timestamp
			state.lastProcessedCount = visibleCount
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies, Telemetry: allTelemetry}
}

// Reset clears all per-series state for replay/reanalysis.
func (b *BOCPDDetector) Reset() {
	b.series = make(map[string]*bocpdSeriesState)
}

// processPoint handles a single new observation for a series.
// Returns an anomaly (if new alert onset) and telemetry for observability.
func (b *BOCPDDetector) processPoint(state *bocpdSeriesState, p observer.Point, series *observer.Series) (*observer.Anomaly, []observer.ObserverTelemetry) {
	x := p.Value

	// Phase 1: Warmup — accumulate baseline statistics.
	if !state.initialized {
		return b.warmupPoint(state, x), nil
	}

	// Phase 2: Online BOCPD posterior update.
	triggered, cpProb, shortRunMass := b.updatePosterior(state, x)

	// Emit telemetry for cpProb and shortRunMass at each initialized point.
	telemetry := []observer.ObserverTelemetry{
		{
			DetectorName: b.Name(),
			Metric: &metricObs{
				name:      "cp_prob",
				value:     cpProb,
				timestamp: p.Timestamp,
			},
		},
		{
			DetectorName: b.Name(),
			Metric: &metricObs{
				name:      "short_run_mass",
				value:     shortRunMass,
				timestamp: p.Timestamp,
			},
		},
	}

	// Phase 3: Alert lifecycle.
	if triggered {
		state.recoveryCount = 0
		if !state.inAlert {
			// New alert onset — emit anomaly.
			state.inAlert = true
			state.alertStart = p.Timestamp
			return b.makeAnomaly(state, p, series, cpProb, shortRunMass), telemetry
		}
		// Already in alert — suppress repeated emission.
		return nil, telemetry
	}

	// Not triggered on this point.
	if state.inAlert {
		state.recoveryCount++
		if state.recoveryCount >= b.RecoveryPoints {
			state.inAlert = false
			state.recoveryCount = 0
		}
	}
	return nil, telemetry
}

// warmupPoint accumulates a point during the warmup phase using Welford's algorithm.
func (b *BOCPDDetector) warmupPoint(state *bocpdSeriesState, x float64) *observer.Anomaly {
	state.warmupCount++
	state.warmupBuffer = append(state.warmupBuffer, x)
	delta := x - state.warmupMean
	state.warmupMean += delta / float64(state.warmupCount)
	delta2 := x - state.warmupMean
	state.warmupM2 += delta * delta2

	if state.warmupCount >= b.WarmupPoints {
		b.initializeFromWarmup(state)
	}
	return nil
}

// initializeFromWarmup computes baseline parameters and initializes BOCPD posterior state.
func (b *BOCPDDetector) initializeFromWarmup(state *bocpdSeriesState) {
	variance := state.warmupM2 / float64(state.warmupCount-1) // sample variance (Bessel's correction)
	stddev := math.Sqrt(variance)

	if variance < b.MinVariance {
		variance = b.MinVariance
		stddev = math.Sqrt(variance)
	}

	state.baselineMean = state.warmupMean
	state.baselineStddev = stddev
	state.obsVar = variance
	state.priorMean = state.warmupMean
	state.priorPrecision = 1.0 / (variance * b.PriorVarianceScale)

	// Initialize posterior arrays.
	bufSize := b.MaxRunLength + 2
	state.runProbs = make([]float64, 1, bufSize)
	state.means = make([]float64, 1, bufSize)
	state.precisions = make([]float64, 1, bufSize)
	state.runProbs[0] = 1.0
	state.means[0] = state.priorMean
	state.precisions[0] = state.priorPrecision

	state.newRunProbs = make([]float64, 0, bufSize)
	state.newMeans = make([]float64, 0, bufSize)
	state.newPrecisions = make([]float64, 0, bufSize)

	// Replay warmup points through the posterior to build up run-length
	// hypotheses. This ensures the detector has context when it starts
	// checking triggers on post-warmup points.
	for _, val := range state.warmupBuffer {
		b.updatePosterior(state, val)
	}
	state.warmupBuffer = nil // free memory

	state.initialized = true
}

// updatePosterior performs one step of the BOCPD recurrence.
// Returns (triggered, cpProb, shortRunMass).
func (b *BOCPDDetector) updatePosterior(state *bocpdSeriesState, x float64) (bool, float64, float64) {
	hazard := b.Hazard
	predPrior := gaussianPDF(x, state.priorMean, state.obsVar+1.0/state.priorPrecision)

	// Compute new run-length probabilities.
	newLen := len(state.runProbs) + 1
	state.newRunProbs = state.newRunProbs[:newLen]
	state.newRunProbs[0] = hazard * predPrior
	for r := range state.runProbs {
		pred := gaussianPDF(x, state.means[r], state.obsVar+1.0/state.precisions[r])
		state.newRunProbs[r+1] = state.runProbs[r] * (1.0 - hazard) * pred
	}

	normalizeProbs(state.newRunProbs)
	cpProb := state.newRunProbs[0]
	shortRunMass := shortRunLengthMass(state.newRunProbs, b.ShortRunLength)

	// Update posterior means and precisions.
	state.newMeans = state.newMeans[:newLen]
	state.newPrecisions = state.newPrecisions[:newLen]
	state.newMeans[0], state.newPrecisions[0] = normalPosterior(state.priorMean, state.priorPrecision, x, state.obsVar)
	for r := range state.means {
		state.newMeans[r+1], state.newPrecisions[r+1] = normalPosterior(state.means[r], state.precisions[r], x, state.obsVar)
	}

	// Truncate to MaxRunLength.
	if newLen > b.MaxRunLength+1 {
		newLen = b.MaxRunLength + 1
		state.newRunProbs = state.newRunProbs[:newLen]
		state.newMeans = state.newMeans[:newLen]
		state.newPrecisions = state.newPrecisions[:newLen]
		normalizeProbs(state.newRunProbs)
	}

	// Swap buffers.
	state.runProbs, state.newRunProbs = state.newRunProbs, state.runProbs
	state.means, state.newMeans = state.newMeans, state.means
	state.precisions, state.newPrecisions = state.newPrecisions, state.precisions

	// Check trigger conditions.
	// Short-run mass is only meaningful when there are run-length hypotheses
	// beyond the short-run window; otherwise all mass is trivially "short."
	triggeredByPeak := cpProb >= b.CPThreshold
	triggeredByShift := shortRunMass >= b.CPMassThreshold && len(state.runProbs) > b.ShortRunLength+1
	triggered := triggeredByPeak || triggeredByShift
	return triggered, cpProb, shortRunMass
}

// makeAnomaly constructs an Anomaly for a new alert onset.
func (b *BOCPDDetector) makeAnomaly(state *bocpdSeriesState, p observer.Point, series *observer.Series, cpProb, shortRunMass float64) *observer.Anomaly {
	seriesName := series.Name + ":" + aggSuffix(state.agg)
	deviation := (p.Value - state.baselineMean) / state.baselineStddev

	triggerType := "short-run posterior mass"
	triggerValue := shortRunMass
	triggerThreshold := b.CPMassThreshold
	if cpProb >= b.CPThreshold {
		triggerType = "changepoint probability"
		triggerValue = cpProb
		triggerThreshold = b.CPThreshold
	}

	return &observer.Anomaly{
		Type:           observer.AnomalyTypeMetric,
		Source:         observer.MetricName(seriesName),
		SourceSeriesID: observer.SeriesID(seriesKey(series.Namespace, seriesName, series.Tags)),
		DetectorName:   b.Name(),
		Title:          fmt.Sprintf("BOCPD changepoint detected: %s", seriesName),
		Description: fmt.Sprintf("%s %s %.2f exceeded threshold %.2f (cp=%.2f, short-run<=%d mass=%.2f)",
			seriesName, triggerType, triggerValue, triggerThreshold, cpProb, b.ShortRunLength, shortRunMass),
		Tags:      series.Tags,
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   state.baselineMean,
			BaselineStddev: state.baselineStddev,
			Threshold:      triggerThreshold,
			CurrentValue:   p.Value,
			DeviationSigma: deviation,
		},
	}
}

// stateKey returns a unique key for per-series state tracking.
func (b *BOCPDDetector) stateKey(key observer.SeriesKey, agg observer.Aggregate) string {
	return seriesKey(key.Namespace, key.Name, key.Tags) + "|" + aggSuffix(agg)
}

// newSeriesState creates a fresh per-series state entry.
func (b *BOCPDDetector) newSeriesState(key observer.SeriesKey, agg observer.Aggregate) *bocpdSeriesState {
	return &bocpdSeriesState{
		key: key,
		agg: agg,
	}
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (b *BOCPDDetector) ensureDefaults() {
	if b.WarmupPoints <= 0 {
		b.WarmupPoints = 120
	}
	if b.Hazard <= 0 || b.Hazard >= 1 {
		b.Hazard = 0.05
	}
	if b.CPThreshold <= 0 || b.CPThreshold >= 1 {
		b.CPThreshold = 0.6
	}
	if b.ShortRunLength <= 0 {
		b.ShortRunLength = 5
	}
	if b.CPMassThreshold <= 0 || b.CPMassThreshold >= 1 {
		b.CPMassThreshold = 0.7
	}
	if b.MaxRunLength <= 0 {
		b.MaxRunLength = 200
	}
	if b.PriorVarianceScale <= 0 {
		b.PriorVarianceScale = 10.0
	}
	if b.RecoveryPoints <= 0 {
		b.RecoveryPoints = 10
	}
	if b.series == nil {
		b.series = make(map[string]*bocpdSeriesState)
	}
	if len(b.Aggregations) == 0 {
		b.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}

func shortRunLengthMass(runProbs []float64, shortRunLength int) float64 {
	maxIdx := shortRunLength
	if maxIdx > len(runProbs)-1 {
		maxIdx = len(runProbs) - 1
	}
	var mass float64
	for i := 0; i <= maxIdx; i++ {
		mass += runProbs[i]
	}
	return mass
}

func normalPosterior(priorMean, priorPrecision, x, obsVar float64) (mean, precision float64) {
	obsPrecision := 1.0 / obsVar
	precision = priorPrecision + obsPrecision
	mean = (priorPrecision*priorMean + obsPrecision*x) / precision
	return mean, precision
}

func normalizeProbs(probs []float64) {
	var total float64
	for _, p := range probs {
		total += p
	}
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		uniform := 1.0 / float64(len(probs))
		for i := range probs {
			probs[i] = uniform
		}
		return
	}
	for i := range probs {
		probs[i] /= total
	}
}

func gaussianPDF(x, mean, variance float64) float64 {
	const minVariance = 1e-12
	if variance < minVariance {
		variance = minVariance
	}
	z := x - mean
	denom := math.Sqrt(2 * math.Pi * variance)
	return math.Exp(-(z*z)/(2*variance)) / denom
}
