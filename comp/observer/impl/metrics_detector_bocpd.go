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

// bocpdStateKey uniquely identifies a (series, aggregation) pair for BOCPD state.
type bocpdStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// bocpdSeriesState holds per-series streaming BOCPD state.
type bocpdSeriesState struct {

	// Cursor: how many points we've processed so far (count-based for safety).
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64 // storage writeGeneration at last Detect

	// Warmup: Welford online mean/variance accumulation.
	initialized  bool
	warmupCount  int
	warmupMean   float64
	warmupM2     float64   // sum of squared deviations (Welford)
	warmupBuffer []float64 // buffered values for posterior replay after init

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
	inAlert       bool
	alertStart    int64
	recoveryCount int // consecutive non-triggering points since last trigger
}

// BOCPDConfig holds configuration for the BOCPD detector.
type BOCPDConfig struct {
	// WarmupPoints is the number of initial points used for baseline estimation.
	// A longer warmup captures more of the metric's natural variability, reducing
	// false positives from normal fluctuation. Default: 120 (~2 minutes at 1Hz).
	WarmupPoints int `json:"warmup_points"`

	// Hazard is the constant changepoint hazard probability.
	// Default: 0.05
	Hazard float64 `json:"hazard"`

	// CPThreshold is the posterior P(changepoint at t) threshold to emit.
	// Default: 0.6
	CPThreshold float64 `json:"cp_threshold"`

	// ShortRunLength is the run-length horizon k for short-run posterior mass P(r_t <= k).
	// Default: 5
	ShortRunLength int `json:"short_run_length"`

	// CPMassThreshold is the threshold for short-run posterior mass P(r_t <= k).
	// Default: 0.7
	CPMassThreshold float64 `json:"cp_mass_threshold"`

	// MaxRunLength caps tracked run-length hypotheses for bounded compute.
	// Default: 200
	MaxRunLength int `json:"max_run_length"`

	// PriorVarianceScale controls prior variance over the mean relative to observed variance.
	// Default: 10.0
	PriorVarianceScale float64 `json:"prior_variance_scale"`

	// MinVariance is the floor for observation variance. When warmup data has
	// near-zero variance (e.g. constant series), this prevents pathologically
	// sharp PDFs that would flag any tiny fluctuation as anomalous. Default: 1.0
	MinVariance float64 `json:"min_variance"`

	// RecoveryPoints is how many consecutive non-triggering points are needed
	// to exit alert state. Default: 10
	RecoveryPoints int `json:"recovery_points"`

	// Aggregations to run detection on. Default: [Average, Count]
	Aggregations []observer.Aggregate `json:"-"`
}

// DefaultBOCPDConfig returns a BOCPDConfig with default values.
func DefaultBOCPDConfig() BOCPDConfig {
	return BOCPDConfig{
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
	}
}

// BOCPDDetector detects changepoints using Bayesian Online Changepoint Detection.
// This is a streaming, stateful Detector implementation that maintains per-series
// posterior state and processes only newly visible points on each advance.
type BOCPDDetector struct {
	config BOCPDConfig

	// per-(series, aggregation) state.
	series map[bocpdStateKey]*bocpdSeriesState

	// Cache the discovered series list across Detect calls. Refresh when storage
	// reports that new series were added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewBOCPDDetector creates a streaming BOCPD detector with the given config.
// Zero-valued fields are filled from DefaultBOCPDConfig().
func NewBOCPDDetector(config BOCPDConfig) *BOCPDDetector {
	defaults := DefaultBOCPDConfig()
	// Warmup needs at least 2 points for Bessel's correction (n-1 denominator).
	if config.WarmupPoints < 2 {
		config.WarmupPoints = defaults.WarmupPoints
	}
	if config.Hazard <= 0 || config.Hazard >= 1 {
		config.Hazard = defaults.Hazard
	}
	if config.CPThreshold <= 0 || config.CPThreshold >= 1 {
		config.CPThreshold = defaults.CPThreshold
	}
	if config.ShortRunLength <= 0 {
		config.ShortRunLength = defaults.ShortRunLength
	}
	if config.CPMassThreshold <= 0 || config.CPMassThreshold >= 1 {
		config.CPMassThreshold = defaults.CPMassThreshold
	}
	if config.MaxRunLength <= 0 {
		config.MaxRunLength = defaults.MaxRunLength
	}
	if config.PriorVarianceScale <= 0 {
		config.PriorVarianceScale = defaults.PriorVarianceScale
	}
	if config.MinVariance <= 0 {
		config.MinVariance = defaults.MinVariance
	}
	if config.RecoveryPoints <= 0 {
		config.RecoveryPoints = defaults.RecoveryPoints
	}
	if len(config.Aggregations) == 0 {
		config.Aggregations = defaults.Aggregations
	}
	return &BOCPDDetector{
		config: config,
		series: make(map[bocpdStateKey]*bocpdSeriesState),
	}
}

// Name returns the detector name.
func (b *BOCPDDetector) Name() string {
	return "bocpd_detector"
}

// Detect implements Detector. It discovers series, reads only newly visible
// points, and updates per-series BOCPD posterior state incrementally.
//
// Correctness takes priority over positional cursoring: storage may insert
// points into existing history, so this detector gates incremental work on
// visible point counts rather than raw slice positions.
func (b *BOCPDDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	gen := storage.SeriesGeneration()
	if b.cachedSeries == nil || gen != b.cachedGen {
		b.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		b.cachedGen = gen
	}

	var allAnomalies []observer.Anomaly

	for _, meta := range b.cachedSeries {
		for _, agg := range b.config.Aggregations {
			sk := bocpdStateKey{ref: meta.Ref, agg: agg}

			state, exists := b.series[sk]
			if !exists {
				state = &bocpdSeriesState{}
				b.series[sk] = state
			}

			visibleCount := storage.PointCountUpTo(meta.Ref, dataTime)
			currentGen := storage.WriteGeneration(meta.Ref)
			mergeOccurred := visibleCount == state.lastProcessedCount && currentGen != state.lastWriteGen
			if visibleCount <= state.lastProcessedCount && !mergeOccurred {
				continue
			}

			startTime := state.lastProcessedTime
			if mergeOccurred {
				startTime = state.lastProcessedTime - 1
				if startTime < 0 {
					startTime = 0
				}
			}

			pointsSeen := false
			prevLen := len(allAnomalies)
			storage.ForEachPoint(meta.Ref, startTime, dataTime, agg, func(series *observer.Series, p observer.Point) {
				pointsSeen = true
				anomaly := b.processPoint(state, p, series, agg)
				if anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})
			// Set SourceRef on any anomalies produced in this iteration.
			for k := prevLen; k < len(allAnomalies); k++ {
				allAnomalies[k].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}

			if !pointsSeen && mergeOccurred {
				state.lastWriteGen = currentGen
				continue
			}
			if pointsSeen {
				state.lastProcessedCount = visibleCount
				state.lastWriteGen = currentGen
			}
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// Reset clears all per-series state for replay/reanalysis.
func (b *BOCPDDetector) Reset() {
	b.series = make(map[bocpdStateKey]*bocpdSeriesState)
	b.cachedSeries = nil
	b.cachedGen = 0
}

// processPoint handles a single new observation for a series.
// Returns an anomaly pointer if this point triggers a new alert onset.
func (b *BOCPDDetector) processPoint(state *bocpdSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	if !state.initialized {
		return b.warmupPoint(state, x)
	}

	triggered, cpProb, shortRunMass := b.updatePosterior(state, x)

	if triggered {
		state.recoveryCount = 0
		if !state.inAlert {
			state.inAlert = true
			state.alertStart = p.Timestamp
			return b.makeAnomaly(state, p, series, agg, cpProb, shortRunMass)
		}
		return nil
	}

	if state.inAlert {
		state.recoveryCount++
		if state.recoveryCount >= b.config.RecoveryPoints {
			state.inAlert = false
			state.recoveryCount = 0
		}
	}
	return nil
}

// warmupPoint accumulates a point during the warmup phase using Welford's algorithm.
func (b *BOCPDDetector) warmupPoint(state *bocpdSeriesState, x float64) *observer.Anomaly {
	state.warmupCount++
	state.warmupBuffer = append(state.warmupBuffer, x)
	delta := x - state.warmupMean
	state.warmupMean += delta / float64(state.warmupCount)
	delta2 := x - state.warmupMean
	state.warmupM2 += delta * delta2

	if state.warmupCount >= b.config.WarmupPoints {
		b.initializeFromWarmup(state)
	}
	return nil
}

// initializeFromWarmup computes baseline parameters and initializes BOCPD posterior state.
func (b *BOCPDDetector) initializeFromWarmup(state *bocpdSeriesState) {
	variance := state.warmupM2 / float64(state.warmupCount-1) // sample variance (Bessel's correction)
	stddev := math.Sqrt(variance)

	if variance < b.config.MinVariance {
		variance = b.config.MinVariance
		stddev = math.Sqrt(variance)
	}

	state.baselineMean = state.warmupMean
	state.baselineStddev = stddev
	state.obsVar = variance
	state.priorMean = state.warmupMean
	state.priorPrecision = 1.0 / (variance * b.config.PriorVarianceScale)

	// Initialize posterior arrays.
	bufSize := b.config.MaxRunLength + 2
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
	hazard := b.config.Hazard

	// Standard BOCPD recurrence (Adams & MacKay 2007):
	// cpMass = hazard * sum_r(runProbs[r] * pred(x|r))
	// This weighs the observation against all run-length hypotheses so the
	// detector can catch cascading shifts, not just the first deviation from
	// the warmup baseline.
	newLen := len(state.runProbs) + 1
	state.newRunProbs = state.newRunProbs[:newLen]
	var cpMass float64
	for r := range state.runProbs {
		pred := gaussianPDF(x, state.means[r], state.obsVar+1.0/state.precisions[r])
		state.newRunProbs[r+1] = state.runProbs[r] * (1.0 - hazard) * pred
		cpMass += state.runProbs[r] * pred
	}
	state.newRunProbs[0] = hazard * cpMass

	normalizeProbs(state.newRunProbs)
	cpProb := state.newRunProbs[0]
	shortRunMass := shortRunLengthMass(state.newRunProbs, b.config.ShortRunLength)

	// Update posterior means and precisions.
	state.newMeans = state.newMeans[:newLen]
	state.newPrecisions = state.newPrecisions[:newLen]
	state.newMeans[0], state.newPrecisions[0] = normalPosterior(state.priorMean, state.priorPrecision, x, state.obsVar)
	for r := range state.means {
		state.newMeans[r+1], state.newPrecisions[r+1] = normalPosterior(state.means[r], state.precisions[r], x, state.obsVar)
	}

	// Truncate to MaxRunLength.
	if newLen > b.config.MaxRunLength+1 {
		newLen = b.config.MaxRunLength + 1
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
	triggeredByPeak := cpProb >= b.config.CPThreshold
	triggeredByShift := shortRunMass >= b.config.CPMassThreshold && len(state.runProbs) > b.config.ShortRunLength+1
	triggered := triggeredByPeak || triggeredByShift
	return triggered, cpProb, shortRunMass
}

// makeAnomaly constructs an Anomaly for a new alert onset.
func (b *BOCPDDetector) makeAnomaly(state *bocpdSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate, cpProb, shortRunMass float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	deviation := (p.Value - state.baselineMean) / state.baselineStddev

	triggerType := "short-run posterior mass"
	triggerValue := shortRunMass
	triggerThreshold := b.config.CPMassThreshold
	if cpProb >= b.config.CPThreshold {
		triggerType = "changepoint probability"
		triggerValue = cpProb
		triggerThreshold = b.config.CPThreshold
	}

	displayName := source.String()
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: b.Name(),
		Title:        "BOCPD changepoint detected: " + displayName,
		Description: fmt.Sprintf("%s %s %.2f exceeded threshold %.2f (cp=%.2f, short-run<=%d mass=%.2f)",
			displayName, triggerType, triggerValue, triggerThreshold, cpProb, b.config.ShortRunLength, shortRunMass),
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

func shortRunLengthMass(runProbs []float64, shortRunLength int) float64 {
	maxIdx := shortRunLength
	if maxIdx > len(runProbs)-1 {
		maxIdx = len(runProbs) - 1
	}
	var mass float64
	// Start from index 1: index 0 is cpProb (changepoint probability),
	// which is tested separately via CPThreshold. Including it here
	// makes the two trigger conditions non-independent.
	for i := 1; i <= maxIdx; i++ {
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
