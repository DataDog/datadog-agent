// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package detector provides anomaly detection algorithms and scoring mechanisms.
package detector

import (
	"math"
	"sort"
	"time"
)

// EVTDetector implements Extreme Value Theory using Peaks-Over-Threshold
type EVTDetector struct {
	// Hyperparameters
	windowSize          int     // number of historical points per signal
	thresholdPercentile float64 // percentile for POT threshold (e.g., 0.975)
	minExceedances      int     // minimum exceedances to fit GPD
	refitDays           int     // refit GPD every N days
	combinedThreshold   float64 // combined Z-score threshold
	persistence         int     // must satisfy condition for N consecutive points
	suppressMinutes     int     // cooldown period after alert
	useStofferMethod    bool    // true for Stouffer, false for Fisher

	// State
	windows         [][]float64 // rolling windows per signal [M][W]
	gpdParams       []GPDParams // GPD parameters per signal
	combinedHistory []float64   // history of combined scores
	lastRefitTime   time.Time   // timestamp of last GPD refit
	lastAlertTime   time.Time   // timestamp of last alert
	numSignals      int         // number of signals
	initialized     bool
}

// GPDParams holds the parameters of a Generalized Pareto Distribution
type GPDParams struct {
	threshold float64 // u: threshold value
	xi        float64 // shape parameter
	beta      float64 // scale parameter
	pU        float64 // P(X > u): probability of exceeding threshold
	valid     bool    // whether params are valid
}

// NewEVTDetector creates a new EVT detector with default hyperparameters
func NewEVTDetector() *EVTDetector {
	return NewEVTDetectorWithParams(
		30*288, // 30 days of 5-min samples
		0.90,   // 90th percentile threshold (lowered for better sensitivity and faster fitting)
		10,     // min exceedances (lowered from 50 to 10 for faster GPD fitting)
		1,      // refit daily
		2.5,    // Z threshold (lowered from 3.3 for better detection)
		2,      // persistence
		30,     // suppress_minutes
		true,   // use Stouffer
	)
}

// NewEVTDetectorWithParams creates an EVT detector with custom parameters
func NewEVTDetectorWithParams(windowSize int, thresholdPercentile float64, minExceedances int,
	refitDays int, combinedThreshold float64, persistence int, suppressMinutes int, useStouffer bool) *EVTDetector {

	return &EVTDetector{
		windowSize:          windowSize,
		thresholdPercentile: thresholdPercentile,
		minExceedances:      minExceedances,
		refitDays:           refitDays,
		combinedThreshold:   combinedThreshold,
		persistence:         persistence,
		suppressMinutes:     suppressMinutes,
		useStofferMethod:    useStouffer,
		initialized:         false,
	}
}

// Name returns the detector name
func (d *EVTDetector) Name() string {
	return "EVT-POT"
}

// HigherIsAnomalous returns true since higher scores indicate anomalies
func (d *EVTDetector) HigherIsAnomalous() bool {
	return true
}

// ComputeScore processes telemetry results and returns anomaly score
func (d *EVTDetector) ComputeScore(result TelemetryResult) (float64, error) {
	signals := result.ToArray()

	// Transform signals: since 1.0=normal and 0.0=anomalous, invert so low values become high
	// EVT-POT detects high values (right tail), so we transform to make anomalies detectable
	transformedSignals := make([]float64, len(signals))
	for i := range signals {
		transformedSignals[i] = 1.0 - signals[i]
	}
	signals = transformedSignals

	// Initialize on first call
	if !d.initialized {
		d.numSignals = len(signals)
		d.windows = make([][]float64, d.numSignals)
		d.gpdParams = make([]GPDParams, d.numSignals)
		d.combinedHistory = make([]float64, 0, 10)

		for i := 0; i < d.numSignals; i++ {
			d.windows[i] = make([]float64, 0, d.windowSize)
		}

		d.initialized = true
	}

	// Update windows
	for s := 0; s < d.numSignals; s++ {
		d.windows[s] = append(d.windows[s], signals[s])

		// Trim window if too large
		if len(d.windows[s]) > d.windowSize {
			d.windows[s] = d.windows[s][1:]
		}
	}

	// Check if we need to refit GPD parameters
	now := time.Now()
	refitInterval := time.Duration(d.refitDays) * 24 * time.Hour
	timeSinceRefit := now.Sub(d.lastRefitTime)

	if timeSinceRefit > refitInterval || !d.gpdParams[0].valid {
		d.fitAllGPDs()
		d.lastRefitTime = now
	}

	// Compute p-values for current signals using EVT
	pValues := d.computeEVTPValues(signals)

	// Combine p-values
	var combinedScore float64
	if d.useStofferMethod {
		combinedScore = d.combineStouffer(pValues)
	} else {
		combinedScore = d.combineFisher(pValues)
	}

	// Store in history
	d.combinedHistory = append(d.combinedHistory, combinedScore)
	if len(d.combinedHistory) > 10 {
		d.combinedHistory = d.combinedHistory[1:]
	}

	// Check persistence
	persistenceCount := 0
	histLen := len(d.combinedHistory)
	checkLen := d.persistence
	if histLen < checkLen {
		checkLen = histLen
	}

	for i := histLen - checkLen; i < histLen; i++ {
		if d.combinedHistory[i] > d.combinedThreshold {
			persistenceCount++
		}
	}

	// Check if we should alert
	suppressDuration := time.Duration(d.suppressMinutes) * time.Minute
	timeSinceLastAlert := now.Sub(d.lastAlertTime)

	if persistenceCount >= d.persistence && timeSinceLastAlert > suppressDuration {
		d.lastAlertTime = now
		return combinedScore, nil
	}

	// Return combined score for continuous monitoring (not normalized)
	// This allows visualization of the detector's confidence level
	return combinedScore, nil
}

// fitAllGPDs fits GPD parameters for all signals
func (d *EVTDetector) fitAllGPDs() {
	for s := 0; s < d.numSignals; s++ {
		d.gpdParams[s] = d.fitGPD(d.windows[s])
	}
}

// fitGPD fits a GPD to exceedances above threshold
func (d *EVTDetector) fitGPD(window []float64) GPDParams {
	if len(window) < d.minExceedances*2 {
		return GPDParams{valid: false}
	}

	// Compute threshold
	sorted := make([]float64, len(window))
	copy(sorted, window)
	sort.Float64s(sorted)

	thresholdIdx := int(d.thresholdPercentile * float64(len(sorted)))
	if thresholdIdx >= len(sorted) {
		thresholdIdx = len(sorted) - 1
	}
	threshold := sorted[thresholdIdx]

	// Collect exceedances
	exceedances := make([]float64, 0)
	for _, v := range window {
		if v > threshold {
			exceedances = append(exceedances, v-threshold)
		}
	}

	if len(exceedances) < d.minExceedances {
		return GPDParams{valid: false}
	}

	// Fit GPD using method of moments (simpler than MLE)
	xi, beta := d.fitGPDMethodOfMoments(exceedances)

	// Probability of exceeding threshold
	pU := float64(len(exceedances)) / float64(len(window))

	return GPDParams{
		threshold: threshold,
		xi:        xi,
		beta:      beta,
		pU:        pU,
		valid:     true,
	}
}

// fitGPDMethodOfMoments fits GPD parameters using method of moments
func (d *EVTDetector) fitGPDMethodOfMoments(exceedances []float64) (float64, float64) {
	// Compute mean and variance
	mean := 0.0
	for _, e := range exceedances {
		mean += e
	}
	mean /= float64(len(exceedances))

	variance := 0.0
	for _, e := range exceedances {
		diff := e - mean
		variance += diff * diff
	}
	variance /= float64(len(exceedances))

	// Method of moments estimates
	// E[Y] = beta / (1 - xi)
	// Var[Y] = beta^2 / ((1-xi)^2 * (1-2*xi))
	// Solving: xi = 0.5 * (1 - mean^2/variance)
	//         beta = 0.5 * mean * (mean^2/variance + 1)

	if variance < 1e-10 || mean < 1e-10 {
		return 0.1, mean // fallback to exponential-like
	}

	ratio := mean * mean / variance
	xi := 0.5 * (1.0 - ratio)

	// Constrain xi to reasonable range [-0.5, 0.5]
	if xi < -0.5 {
		xi = -0.5
	}
	if xi > 0.5 {
		xi = 0.5
	}

	beta := mean / (1.0 - xi)
	if beta < 1e-10 {
		beta = mean
	}

	return xi, beta
}

// computeEVTPValues computes tail probabilities using EVT
func (d *EVTDetector) computeEVTPValues(currentSignals []float64) []float64 {
	pValues := make([]float64, d.numSignals)

	for s := 0; s < d.numSignals; s++ {
		if !d.gpdParams[s].valid || len(d.windows[s]) < 10 {
			// Fallback: use simple empirical p-value if GPD not yet fit
			// This allows detection even before we have enough data for GPD
			if len(d.windows[s]) > 0 {
				rank := 0
				x := currentSignals[s]
				for _, v := range d.windows[s] {
					if v <= x {
						rank++
					}
				}
				n := len(d.windows[s])
				pValues[s] = float64(n-rank+1) / float64(n+1)
			} else {
				pValues[s] = 0.5 // neutral if no data
			}
			continue
		}

		params := d.gpdParams[s]
		x := currentSignals[s]

		if x <= params.threshold {
			// Below threshold: use empirical p-value
			rank := 0
			for _, v := range d.windows[s] {
				if v <= x {
					rank++
				}
			}
			n := len(d.windows[s])
			pValues[s] = float64(n-rank+1) / float64(n+1)
		} else {
			// Above threshold: use GPD tail probability
			y := x - params.threshold

			// GPD survival function: S(y) = (1 + xi*y/beta)^(-1/xi)
			var sf float64
			if math.Abs(params.xi) < 1e-6 {
				// Exponential case (xi ~ 0)
				sf = math.Exp(-y / params.beta)
			} else {
				arg := 1.0 + params.xi*y/params.beta
				if arg <= 0 {
					sf = 0.0
				} else {
					sf = math.Pow(arg, -1.0/params.xi)
				}
			}

			// Overall tail probability: P(X > x) = P(X > u) * P(X > x | X > u)
			pValues[s] = params.pU * sf
		}

		// Ensure p-value is in valid range
		if pValues[s] < 1e-12 {
			pValues[s] = 1e-12
		}
		if pValues[s] > 1.0 {
			pValues[s] = 1.0
		}
	}

	return pValues
}

// combineStouffer combines p-values using Stouffer's Z method
func (d *EVTDetector) combineStouffer(pValues []float64) float64 {
	Z := 0.0
	for _, p := range pValues {
		z := inverseNormalCDF(1.0 - p)
		Z += z
	}
	Z /= math.Sqrt(float64(len(pValues)))
	return Z
}

// combineFisher combines p-values using Fisher's method
func (d *EVTDetector) combineFisher(pValues []float64) float64 {
	F := 0.0
	for _, p := range pValues {
		F += -2.0 * math.Log(p)
	}
	// Return F directly (higher = more anomalous)
	return F
}
