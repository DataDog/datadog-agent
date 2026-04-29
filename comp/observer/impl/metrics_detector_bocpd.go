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
// Sufficient statistics are Normal-Inverse-Gamma (NIG) parameters so that
// the per-hypothesis predictive is Student-t (robust to outliers).
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

	// Baseline (set once after warmup, used for AnomalyDebugInfo).
	baselineMean   float64
	baselineStddev float64

	// NIG prior parameters (fixed after warmup initialisation).
	priorMu    float64
	priorKappa float64
	priorAlpha float64
	priorBeta  float64

	// BOCPD posterior state: one NIG entry per run-length hypothesis.
	runProbs []float64
	mus      []float64
	kappas   []float64
	alphas   []float64
	betas    []float64

	// Pre-allocated swap buffers to avoid per-point allocation.
	newRunProbs []float64
	newMus      []float64
	newKappas   []float64
	newAlphas   []float64
	newBetas    []float64

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
	// Retained for backwards-compatibility; not used in the NIG formulation.
	// Default: 10.0
	PriorVarianceScale float64 `json:"prior_variance_scale"`

	// MinVariance is the floor for observation variance. When warmup data has
	// near-zero variance (e.g. constant series), this prevents pathologically
	// sharp PDFs that would flag any tiny fluctuation as anomalous. Default: 1.0
	MinVariance float64 `json:"min_variance"`

	// RecoveryPoints is how many consecutive non-triggering points are needed
	// to exit alert state. Default: 10
	RecoveryPoints int `json:"recovery_points"`

	// PriorKappa is the NIG prior pseudo-count on the mean. It controls how
	// diffuse the prior on mu is. Larger values shrink the predictive scale.
	// Default: 1.0
	PriorKappa float64 `json:"prior_kappa"`

	// DegreesOfFreedomFloor is the minimum degrees of freedom (ν = 2α) for the
	// Student-t predictive distribution. Values ≤ 2 give effectively uncapped
	// variance (very heavy tails). Must be > 0.
	// Default: 3.0
	DegreesOfFreedomFloor float64 `json:"degrees_of_freedom_floor"`

	// Aggregations to run detection on. Default: [Average, Count]
	Aggregations []observer.Aggregate `json:"-"`
}

// DefaultBOCPDConfig returns a BOCPDConfig with default values.
func DefaultBOCPDConfig() BOCPDConfig {
	return BOCPDConfig{
		WarmupPoints:          120,
		Hazard:                0.05,
		CPThreshold:           0.6,
		ShortRunLength:        5,
		CPMassThreshold:       0.7,
		MaxRunLength:          200,
		PriorVarianceScale:    10.0,
		MinVariance:           1.0,
		RecoveryPoints:        10,
		PriorKappa:            1.0,
		DegreesOfFreedomFloor: 3.0,
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
	if config.PriorKappa <= 0 {
		config.PriorKappa = defaults.PriorKappa
	}
	if config.DegreesOfFreedomFloor <= 0 {
		config.DegreesOfFreedomFloor = defaults.DegreesOfFreedomFloor
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

// initializeFromWarmup computes NIG prior parameters from warmup statistics and
// initialises the BOCPD posterior state.
func (b *BOCPDDetector) initializeFromWarmup(state *bocpdSeriesState) {
	variance := state.warmupM2 / float64(state.warmupCount-1) // sample variance (Bessel's correction)
	stddev := math.Sqrt(variance)

	if variance < b.config.MinVariance {
		variance = b.config.MinVariance
		stddev = math.Sqrt(variance)
	}

	state.baselineMean = state.warmupMean
	state.baselineStddev = stddev

	// NIG prior: parameterised so E[sigma^2] ≈ observed variance for large α.
	//   μ₀  = warmup sample mean
	//   κ₀  = PriorKappa (pseudo-count on the mean; ≥ 1)
	//   α₀  = warmupCount / 2   (shape; encodes "we saw N/2 variance samples")
	//   β₀  = variance * α₀    (rate; E[σ²] = β/(α-1) ≈ variance for large α)
	state.priorMu = state.warmupMean
	state.priorKappa = math.Max(b.config.PriorKappa, 1.0)
	state.priorAlpha = float64(state.warmupCount) / 2.0
	state.priorBeta = variance * state.priorAlpha

	// Allocate posterior arrays.
	bufSize := b.config.MaxRunLength + 2
	state.runProbs = make([]float64, 1, bufSize)
	state.mus = make([]float64, 1, bufSize)
	state.kappas = make([]float64, 1, bufSize)
	state.alphas = make([]float64, 1, bufSize)
	state.betas = make([]float64, 1, bufSize)
	state.runProbs[0] = 1.0
	state.mus[0] = state.priorMu
	state.kappas[0] = state.priorKappa
	state.alphas[0] = state.priorAlpha
	state.betas[0] = state.priorBeta

	state.newRunProbs = make([]float64, 0, bufSize)
	state.newMus = make([]float64, 0, bufSize)
	state.newKappas = make([]float64, 0, bufSize)
	state.newAlphas = make([]float64, 0, bufSize)
	state.newBetas = make([]float64, 0, bufSize)

	// Replay warmup points through the posterior to build up run-length
	// hypotheses. This ensures the detector has context when it starts
	// checking triggers on post-warmup points.
	for _, val := range state.warmupBuffer {
		b.updatePosterior(state, val)
	}
	state.warmupBuffer = nil // free memory

	state.initialized = true
}

// updatePosterior performs one step of the BOCPD recurrence using NIG
// sufficient statistics and a Student-t predictive distribution.
// Returns (triggered, cpProb, shortRunMass).
func (b *BOCPDDetector) updatePosterior(state *bocpdSeriesState, x float64) (bool, float64, float64) {
	hazard := b.config.Hazard
	dfFloor := b.config.DegreesOfFreedomFloor

	// Standard BOCPD recurrence (Adams & MacKay 2007) with Student-t predictive:
	// cpMass = hazard * sum_r(runProbs[r] * pred(x|r))
	newLen := len(state.runProbs) + 1
	state.newRunProbs = state.newRunProbs[:newLen]
	state.newMus = state.newMus[:newLen]
	state.newKappas = state.newKappas[:newLen]
	state.newAlphas = state.newAlphas[:newLen]
	state.newBetas = state.newBetas[:newLen]

	var cpMass float64
	for r := range state.runProbs {
		pred := studentTPredictivePDF(x, state.mus[r], state.kappas[r], state.alphas[r], state.betas[r], dfFloor)
		state.newRunProbs[r+1] = state.runProbs[r] * (1.0 - hazard) * pred
		cpMass += state.runProbs[r] * pred
	}
	state.newRunProbs[0] = hazard * cpMass

	normalizeProbs(state.newRunProbs)
	cpProb := state.newRunProbs[0]
	shortRunMass := shortRunLengthMass(state.newRunProbs, b.config.ShortRunLength)

	// NIG posterior update for each hypothesis.
	// Index 0: fresh segment — updated from the fixed prior.
	state.newMus[0], state.newKappas[0], state.newAlphas[0], state.newBetas[0] =
		nigUpdate(state.priorMu, state.priorKappa, state.priorAlpha, state.priorBeta, x)
	for r := range state.mus {
		state.newMus[r+1], state.newKappas[r+1], state.newAlphas[r+1], state.newBetas[r+1] =
			nigUpdate(state.mus[r], state.kappas[r], state.alphas[r], state.betas[r], x)
	}

	// Truncate to MaxRunLength.
	if newLen > b.config.MaxRunLength+1 {
		newLen = b.config.MaxRunLength + 1
		state.newRunProbs = state.newRunProbs[:newLen]
		state.newMus = state.newMus[:newLen]
		state.newKappas = state.newKappas[:newLen]
		state.newAlphas = state.newAlphas[:newLen]
		state.newBetas = state.newBetas[:newLen]
		normalizeProbs(state.newRunProbs)
	}

	// Swap buffers.
	state.runProbs, state.newRunProbs = state.newRunProbs, state.runProbs
	state.mus, state.newMus = state.newMus, state.mus
	state.kappas, state.newKappas = state.newKappas, state.kappas
	state.alphas, state.newAlphas = state.newAlphas, state.alphas
	state.betas, state.newBetas = state.newBetas, state.betas

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

// nigUpdate returns the NIG posterior parameters after observing a single point x.
// The update is the standard NIG conjugate update:
//
//	κₙ = κ + 1
//	μₙ = (κ·μ + x) / κₙ
//	αₙ = α + 0.5
//	βₙ = β + κ·(x−μ)² / (2·κₙ)
func nigUpdate(mu, kappa, alpha, beta, x float64) (muN, kappaN, alphaN, betaN float64) {
	kappaN = kappa + 1.0
	muN = (kappa*mu + x) / kappaN
	alphaN = alpha + 0.5
	betaN = beta + (kappa*(x-mu)*(x-mu))/(2.0*kappaN)
	return
}

// studentTPredictivePDF evaluates the NIG marginal predictive density at x.
// The predictive is a Student-t with:
//
//	ν     = max(2α, dfFloor)
//	scale² = β·(κ+1) / (α·κ)
//
// Using the log-space computation for numerical stability.
func studentTPredictivePDF(x, mu, kappa, alpha, beta, dfFloor float64) float64 {
	nu := 2 * alpha
	if nu < dfFloor {
		nu = dfFloor
	}
	scale2 := beta * (kappa + 1.0) / (alpha * kappa)
	if scale2 < 1e-18 {
		scale2 = 1e-18
	}
	z2 := (x - mu) * (x - mu) / scale2
	logG1, _ := math.Lgamma((nu + 1.0) / 2.0)
	logG2, _ := math.Lgamma(nu / 2.0)
	logPdf := logG1 - logG2 -
		0.5*math.Log(nu*math.Pi*scale2) -
		((nu+1.0)/2.0)*math.Log1p(z2/nu)
	return math.Exp(logPdf)
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
