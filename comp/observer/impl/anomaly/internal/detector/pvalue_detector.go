// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package detector provides anomaly detection algorithms and scoring mechanisms.
package detector

import (
	"math"
	"time"
)

// PValueDetector combines per-signal p-values using Fisher or Stouffer method
type PValueDetector struct {
	// Hyperparameters
	windowSize        int     // W: number of historical points per signal
	combineMethod     string  // 'fisher' or 'stouffer'
	combinedThreshold float64 // threshold for combined p-value or Z-score
	persistence       int     // must satisfy condition for N consecutive points
	suppressMinutes   int     // cooldown period after alert
	useWeights        bool    // whether to use weighted Stouffer
	zThreshold        float64 // Z-score threshold for Stouffer method

	// State
	windows         [][]float64 // rolling windows per signal [M][W]
	weights         []float64   // weights for Stouffer (inverse of stddev)
	combinedHistory []float64   // history of combined scores
	lastAlertTime   time.Time   // timestamp of last alert
	numSignals      int         // M: number of signals
	initialized     bool
}

// NewPValueDetector creates a new p-value combination detector with default hyperparameters
func NewPValueDetector() *PValueDetector {
	return NewPValueDetectorWithParams(
		12*24,      // W = 2 days of 5-min points (576)
		"stouffer", // combine method
		3.3,        // Z threshold (p ~ 0.0005)
		2,          // persistence
		30,         // suppress_minutes
		true,       // use weights
	)
}

// NewPValueDetectorWithParams creates a p-value detector with custom parameters
func NewPValueDetectorWithParams(windowSize int, combineMethod string, threshold float64, persistence int, suppressMinutes int, useWeights bool) *PValueDetector {
	return &PValueDetector{
		windowSize:        windowSize,
		combineMethod:     combineMethod,
		combinedThreshold: threshold,
		persistence:       persistence,
		suppressMinutes:   suppressMinutes,
		useWeights:        useWeights,
		zThreshold:        threshold,
		initialized:       false,
	}
}

// Name returns the detector name
func (d *PValueDetector) Name() string {
	if d.combineMethod == "fisher" {
		return "PValue-Fisher"
	}
	return "PValue-Stouffer"
}

// HigherIsAnomalous returns true since higher scores indicate anomalies
func (d *PValueDetector) HigherIsAnomalous() bool {
	return true
}

// ComputeScore processes telemetry results and returns anomaly score
func (d *PValueDetector) ComputeScore(result TelemetryResult) (float64, error) {
	// Initialize on first call
	signals := result.ToArray()

	if !d.initialized {
		d.numSignals = len(signals)
		d.windows = make([][]float64, d.numSignals)
		d.weights = make([]float64, d.numSignals)
		d.combinedHistory = make([]float64, 0, 10)

		for i := 0; i < d.numSignals; i++ {
			d.windows[i] = make([]float64, 0, d.windowSize)
			d.weights[i] = 1.0 // default uniform weights
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

	// Update weights based on signal variability (inverse of stddev)
	if d.useWeights {
		for s := 0; s < d.numSignals; s++ {
			if len(d.windows[s]) >= 10 {
				_, stddev := computeMeanStd(d.windows[s])
				if stddev > 1e-10 {
					d.weights[s] = 1.0 / stddev
				} else {
					d.weights[s] = 1.0
				}
			}
		}
	}

	// Compute p-values for current signals
	pValues := d.computePValues(signals)

	// Combine p-values
	var combinedScore float64
	if d.combineMethod == "fisher" {
		combinedScore = d.combineFisher(pValues)
	} else {
		combinedScore = d.combineStouffer(pValues)
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
		if d.combinedHistory[i] > d.zThreshold {
			persistenceCount++
		}
	}

	// Check if we should alert
	now := time.Now()
	suppressDuration := time.Duration(d.suppressMinutes) * time.Minute
	timeSinceLastAlert := now.Sub(d.lastAlertTime)

	if persistenceCount >= d.persistence && timeSinceLastAlert > suppressDuration {
		d.lastAlertTime = now
		return combinedScore, nil // Return the actual score when alerting
	}

	// Return normalized score (0 to 1 range for non-alert monitoring)
	return combinedScore / 10.0, nil
}

// computePValues computes empirical p-values for current signals
func (d *PValueDetector) computePValues(currentSignals []float64) []float64 {
	pValues := make([]float64, d.numSignals)

	for s := 0; s < d.numSignals; s++ {
		if len(d.windows[s]) == 0 {
			pValues[s] = 0.5
			continue
		}

		// Compute empirical left-tail p-value
		// Since 1.0=normal and 0.0=anomalous, detect when current value is LOW
		rank := 0
		for _, v := range d.windows[s] {
			if v >= currentSignals[s] {
				rank++
			}
		}

		n := len(d.windows[s])
		// Use smoothing: (rank + 1) / (n + 1)
		// Small p-value when current is low (anomalous)
		pValues[s] = float64(rank+1) / float64(n+1)

		// Ensure p-value is not exactly 0 (causes issues with log)
		if pValues[s] < 1e-12 {
			pValues[s] = 1e-12
		}
		if pValues[s] > 1.0 {
			pValues[s] = 1.0
		}
	}

	return pValues
}

// combineFisher combines p-values using Fisher's method
// Returns the combined statistic (not the final p-value)
func (d *PValueDetector) combineFisher(pValues []float64) float64 {
	// F = -2 * sum(ln(p_i))
	// Under independence, F ~ chi2 with 2*M degrees of freedom
	F := 0.0
	for _, p := range pValues {
		F += -2.0 * math.Log(p)
	}

	// For simplicity, return F as the score
	// Higher F means more anomalous
	// Could convert to p-value using chi2 CDF, but F is monotonic
	return F
}

// combineStouffer combines p-values using Stouffer's Z method
func (d *PValueDetector) combineStouffer(pValues []float64) float64 {
	// Z = sum(w_i * Phi^-1(1 - p_i)) / sqrt(sum(w_i^2))
	// where Phi^-1 is the inverse standard normal CDF

	numerator := 0.0
	denominator := 0.0

	for s, p := range pValues {
		// Convert p-value to Z-score using inverse normal CDF
		// Z = Phi^-1(1 - p)
		z := inverseNormalCDF(1.0 - p)

		weight := d.weights[s]
		numerator += weight * z
		denominator += weight * weight
	}

	if denominator < 1e-10 {
		return 0.0
	}

	Z := numerator / math.Sqrt(denominator)
	return Z
}

// inverseNormalCDF approximates the inverse normal CDF (probit function)
// Uses the rational approximation by Peter J. Acklam
func inverseNormalCDF(p float64) float64 {
	if p <= 0 {
		return -8.0 // cap at reasonable value
	}
	if p >= 1 {
		return 8.0
	}

	// Coefficients for rational approximation
	const (
		a1 = -3.969683028665376e+01
		a2 = 2.209460984245205e+02
		a3 = -2.759285104469687e+02
		a4 = 1.383577518672690e+02
		a5 = -3.066479806614716e+01
		a6 = 2.506628277459239e+00

		b1 = -5.447609879822406e+01
		b2 = 1.615858368580409e+02
		b3 = -1.556989798598866e+02
		b4 = 6.680131188771972e+01
		b5 = -1.328068155288572e+01

		c1 = -7.784894002430293e-03
		c2 = -3.223964580411365e-01
		c3 = -2.400758277161838e+00
		c4 = -2.549732539343734e+00
		c5 = 4.374664141464968e+00
		c6 = 2.938163982698783e+00

		d1 = 7.784695709041462e-03
		d2 = 3.224671290700398e-01
		d3 = 2.445134137142996e+00
		d4 = 3.754408661907416e+00

		pLow  = 0.02425
		pHigh = 1 - pLow
	)

	var q, r, x float64

	if p < pLow {
		// Rational approximation for lower region
		q = math.Sqrt(-2 * math.Log(p))
		x = (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	} else if p <= pHigh {
		// Rational approximation for central region
		q = p - 0.5
		r = q * q
		x = (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
			(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
	} else {
		// Rational approximation for upper region
		q = math.Sqrt(-2 * math.Log(1-p))
		x = -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}

	return x
}
