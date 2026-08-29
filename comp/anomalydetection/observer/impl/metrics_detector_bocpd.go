// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultBOCPDWarmupPoints = 60
	defaultBOCPDMaxRunLength = 120
)

// bocpdStateKey uniquely identifies a (series, aggregation) pair for BOCPD state.
type bocpdStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// bocpdSeriesState holds per-series streaming BOCPD state.
type bocpdSeriesState struct {

	// Cursor tracking the last position advanced by Detect.
	lastProcessedTime  int64
	lastProcessedCount int   // PointCountUpTo(ref, dataTime) at last Detect
	lastWriteGen       int64 // WriteGeneration at last Detect; used to catch same-bucket merges

	initialized bool

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

	// Alert lifecycle.
	inAlert       bool
	alertStart    int64
	recoveryCount int // consecutive non-triggering points since last trigger
}

// BOCPDConfig holds configuration for the BOCPD detector.
type BOCPDConfig struct {
	// WarmupPoints is the number of initial points used for baseline estimation.
	// A longer warmup captures more of the metric's natural variability, reducing
	// false positives from normal fluctuation. Default: 60 (~1 minute at 1Hz).
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

	// MaxRunLength caps tracked run-length hypotheses and raw history.
	// It must be at least WarmupPoints. Default: 120.
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
		WarmupPoints:       defaultBOCPDWarmupPoints,
		Hazard:             0.05,
		CPThreshold:        0.6,
		ShortRunLength:     5,
		CPMassThreshold:    0.7,
		MaxRunLength:       defaultBOCPDMaxRunLength,
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
	ready  bool

	// per-(series, aggregation) state.
	series map[bocpdStateKey]*bocpdSeriesState

	// Cache discovered series refs across Detect calls. Refresh when seriesGen changes.
	// Holding []SeriesRef instead of []SeriesMeta avoids retaining the Tags []string
	// backing array for every series (~5.8 MiB at 50k series).
	cachedRefs []observer.SeriesRef
	cachedGen  uint64
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
	if config.MaxRunLength < config.WarmupPoints {
		pkglog.Warnf("[observer] BOCPD max_run_length=%d is below warmup_points=%d; using %d", config.MaxRunLength, config.WarmupPoints, config.WarmupPoints)
		config.MaxRunLength = config.WarmupPoints
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
	return "bocpd"
}

func (b *BOCPDDetector) Ready() bool { return b.ready }

// DetectorPointWindow implements observer.DetectorPointWindowRequirement.
func (b *BOCPDDetector) DetectorPointWindow() observer.DetectorPointWindow {
	return observer.DetectorPointWindow{MinPoints: b.config.WarmupPoints, MaxPoints: b.config.MaxRunLength}
}

// Detect implements Detector. It discovers series, reads only newly visible
// points, and updates per-series BOCPD posterior state incrementally.
//
// Correctness takes priority over positional cursoring: storage may insert
// points into existing history, so this detector gates incremental work on
// visible point counts rather than raw slice positions.
func (b *BOCPDDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	gen := storage.SeriesGeneration()
	if b.cachedRefs == nil || gen != b.cachedGen {
		b.cachedRefs = workloadSeriesRefs(storage, b.cachedRefs)
		b.cachedGen = gen
	}

	var allAnomalies []observer.Anomaly

	for _, ref := range b.cachedRefs {
		visibleCount := storage.PointCountUpTo(ref, dataTime)
		for _, agg := range b.config.Aggregations {
			if !supportsSeriesAggregate(storage, ref, agg) {
				continue
			}
			sk := bocpdStateKey{ref: ref, agg: agg}

			state, exists := b.series[sk]
			if !exists && visibleCount < b.config.WarmupPoints {
				continue
			}
			if !exists {
				state = &bocpdSeriesState{}
				b.initializeFromStorage(storage, ref, dataTime, agg, state)
				b.series[sk] = state
			}

			// Skip if no new data is visible at the current dataTime.
			// PointCountUpTo handles replay (data becomes visible as dataTime
			// advances) and the common live case. WriteGeneration catches
			// same-bucket merges and retention-churn scenarios where point
			// count stays constant but stored values changed.
			writeGen := storage.WriteGeneration(ref)
			if visibleCount <= state.lastProcessedCount && writeGen == state.lastWriteGen {
				continue
			}

			prevLen := len(allAnomalies)
			storage.ForEachPoint(ref, state.lastProcessedTime, dataTime, agg, func(series *observer.Series, p observer.Point) {
				anomaly := b.processPoint(state, p, series, agg, visibleCount >= b.config.WarmupPoints)
				if anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})
			// Set SourceRef on any anomalies produced in this iteration.
			for k := prevLen; k < len(allAnomalies); k++ {
				allAnomalies[k].SourceRef = &observer.QueryHandle{Ref: ref, Aggregate: agg}
			}
			// Always advance cursors regardless of whether ForEachPoint found points.
			state.lastProcessedCount = visibleCount
			state.lastWriteGen = writeGen
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// Reset clears all per-series state for replay/reanalysis.
func (b *BOCPDDetector) Reset() {
	b.series = make(map[bocpdStateKey]*bocpdSeriesState)
	b.cachedRefs = nil
	b.cachedGen = 0
	b.ready = false
}

// RemoveSeries drops posterior state for refs that storage has freed.
// Each (ref, agg) entry in the per-series map carries three float64 arrays
// of size MaxRunLength+1 (~2.9 KB at default config), so without this
// teardown the map grows with the cumulative number of series ever seen
// even after their storage payload is gone. Called by the engine right
// after timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (b *BOCPDDetector) RemoveSeries(refs []observer.SeriesRef) {
	if len(refs) == 0 || len(b.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range b.config.Aggregations {
			delete(b.series, bocpdStateKey{ref: ref, agg: agg})
		}
	}
	// Drop the cached series snapshot so the next Detect re-lists from
	// storage and we don't iterate over removed refs.
	b.cachedRefs = nil
	b.cachedGen = 0
}

// processPoint handles a single new observation for a series.
// Returns an anomaly pointer if this point triggers a new alert onset.
func (b *BOCPDDetector) processPoint(state *bocpdSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate, allowAlert bool) *observer.Anomaly {
	x := p.Value

	if !state.initialized {
		return nil
	}
	triggered, cpProb, shortRunMass := b.updatePosterior(state, x)
	if !allowAlert {
		state.inAlert = false
		state.recoveryCount = 0
		return nil
	}

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

// initializeFromStorage initializes the baseline and replays its samples from
// storage. The two passes avoid retaining raw warmup values in detector state.
func (b *BOCPDDetector) initializeFromStorage(storage observer.StorageReader, ref observer.SeriesRef, dataTime int64, agg observer.Aggregate, state *bocpdSeriesState) {
	count := 0
	mean, m2 := 0.0, 0.0
	storage.ForEachPoint(ref, 0, dataTime, agg, func(_ *observer.Series, p observer.Point) {
		if count >= b.config.WarmupPoints {
			return
		}
		count++
		delta := p.Value - mean
		mean += delta / float64(count)
		m2 += delta * (p.Value - mean)
		state.lastProcessedTime = p.Timestamp
	})
	if count < b.config.WarmupPoints {
		return
	}

	variance := m2 / float64(count-1) // sample variance (Bessel's correction)
	stddev := math.Sqrt(variance)

	if variance < b.config.MinVariance {
		variance = b.config.MinVariance
		stddev = math.Sqrt(variance)
	}

	state.baselineMean = mean
	state.baselineStddev = stddev
	state.obsVar = variance
	state.priorMean = mean
	state.priorPrecision = 1.0 / (variance * b.config.PriorVarianceScale)

	// Initialize posterior arrays.
	bufSize := b.config.MaxRunLength + 1
	state.runProbs = make([]float64, 1, bufSize)
	state.means = make([]float64, 1, bufSize)
	state.precisions = make([]float64, 1, bufSize)
	state.runProbs[0] = 1.0
	state.means[0] = state.priorMean
	state.precisions[0] = state.priorPrecision
	storage.ForEachPoint(ref, 0, state.lastProcessedTime, agg, func(_ *observer.Series, p observer.Point) {
		b.updatePosterior(state, p.Value)
	})

	state.initialized = true
	b.ready = true
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
	oldLen := len(state.runProbs)
	fullLen := oldLen + 1
	newLen := fullLen
	if newLen > b.config.MaxRunLength+1 {
		newLen = b.config.MaxRunLength + 1
	} else {
		state.runProbs = state.runProbs[:newLen]
		state.means = state.means[:newLen]
		state.precisions = state.precisions[:newLen]
	}

	// Update from high run lengths to low ones so each source hypothesis is
	// read before its successor overwrites the next slot. At the horizon, the
	// final successor is deliberately not retained, but remains part of the
	// full posterior used for trigger probabilities below.
	var cpMass float64
	var shortRunRawMass float64
	var discardedGrowthProb float64
	for r := oldLen - 1; r >= 0; r-- {
		pred := gaussianPDF(x, state.means[r], state.obsVar+1.0/state.precisions[r])
		growthProb := state.runProbs[r] * (1.0 - hazard) * pred
		if r+1 < newLen {
			state.runProbs[r+1] = growthProb
			state.means[r+1], state.precisions[r+1] = normalPosterior(state.means[r], state.precisions[r], x, state.obsVar)
		} else {
			discardedGrowthProb = growthProb
		}
		cpMass += state.runProbs[r] * pred
		if r+1 <= b.config.ShortRunLength {
			shortRunRawMass += growthProb
		}
	}
	cpRaw := hazard * cpMass
	state.runProbs[0] = cpRaw
	state.means[0], state.precisions[0] = normalPosterior(state.priorMean, state.priorPrecision, x, state.obsVar)

	// Normalize first over the full posterior (including a discarded horizon
	// tail), then normalize the retained posterior after truncation. Trigger
	// values intentionally use the full posterior distribution.
	fullTotal := cpRaw
	for r := 1; r < newLen; r++ {
		fullTotal += state.runProbs[r]
	}
	// The raw tail is not resident, but was included in cpMass above. It is
	// the only full-posterior probability missing from the retained slice.
	fullTotal += discardedGrowthProb

	var cpProb, shortRunMass float64
	if fullTotal <= 0 || math.IsNaN(fullTotal) || math.IsInf(fullTotal, 0) {
		uniform := 1.0 / float64(fullLen)
		cpProb = uniform
		shortRunMass = float64(min(b.config.ShortRunLength, fullLen-1)) * uniform
		for i := range state.runProbs {
			state.runProbs[i] = uniform
		}
	} else {
		cpProb = cpRaw / fullTotal
		shortRunMass = shortRunRawMass / fullTotal
		for i := range state.runProbs {
			state.runProbs[i] /= fullTotal
		}
	}
	normalizeProbs(state.runProbs)

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
