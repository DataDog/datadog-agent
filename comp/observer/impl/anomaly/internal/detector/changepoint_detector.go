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

// ChangePointDetector detects persistent regime changes in a global score
type ChangePointDetector struct {
	// Hyperparameters
	method             string  // 'page-hinkley', 'two-window', or 'cusum'
	baselineWindowSize int     // size of baseline/reference window (in points)
	recentWindowSize   int     // size of recent window (in points)
	cusumThreshold     float64 // threshold for CUSUM statistic
	cusumDrift         float64 // drift parameter for CUSUM
	twoWindowThreshold float64 // threshold for two-window test statistic
	persistence        int     // change must persist for N points
	suppressMinutes    int     // cooldown period after alert
	aggregateSignals   bool    // whether to aggregate signals into global score

	// State for Page-Hinkley
	phSum         float64 // cumulative sum for Page-Hinkley (upward shifts)
	phMinSum      float64 // minimum cumulative sum
	phSumDown     float64 // cumulative sum for downward shifts
	phMinSumDown  float64 // minimum cumulative sum for downward
	phMean        float64 // baseline mean
	phInitialized bool

	// State for general use
	globalScoreHistory []float64 // history of global scores
	baselineHistory    []float64 // longer baseline history
	lastAlertTime      time.Time // timestamp of last alert
	lastChangePoint    int       // index of last detected change point
	numSignals         int       // number of signals
	initialized        bool
}

// NewChangePointDetector creates a new change-point detector with default hyperparameters
func NewChangePointDetector() *ChangePointDetector {
	return NewChangePointDetectorWithParams(
		"page-hinkley", // method
		288,            // baseline window = 24 hours
		6,              // recent window = 30 minutes
		2.5,            // CUSUM threshold (lowered from 5.0 for earlier detection)
		0.01,           // CUSUM drift
		3.0,            // two-window threshold (robust test)
		2,              // persistence
		30,             // suppress_minutes
		true,           // aggregate signals
	)
}

// NewChangePointDetectorWithParams creates a change-point detector with custom parameters
func NewChangePointDetectorWithParams(method string, baselineWindowSize, recentWindowSize int,
	cusumThreshold, cusumDrift, twoWindowThreshold float64, persistence, suppressMinutes int, aggregateSignals bool) *ChangePointDetector {

	return &ChangePointDetector{
		method:             method,
		baselineWindowSize: baselineWindowSize,
		recentWindowSize:   recentWindowSize,
		cusumThreshold:     cusumThreshold,
		cusumDrift:         cusumDrift,
		twoWindowThreshold: twoWindowThreshold,
		persistence:        persistence,
		suppressMinutes:    suppressMinutes,
		aggregateSignals:   aggregateSignals,
		phInitialized:      false,
		initialized:        false,
	}
}

// Name returns the detector name
func (d *ChangePointDetector) Name() string {
	return "ChangePoint-" + d.method
}

// HigherIsAnomalous returns true since higher scores indicate anomalies
func (d *ChangePointDetector) HigherIsAnomalous() bool {
	return true
}

// ComputeScore processes telemetry results and returns anomaly score
func (d *ChangePointDetector) ComputeScore(result TelemetryResult) (float64, error) {
	// Initialize on first call
	if !d.initialized {
		signals := result.ToArray()
		d.numSignals = len(signals)
		d.globalScoreHistory = make([]float64, 0, d.baselineWindowSize)
		d.baselineHistory = make([]float64, 0, d.baselineWindowSize)
		d.initialized = true
	}

	// Compute global score by aggregating signals
	signals := result.ToArray()
	var globalScore float64
	if d.aggregateSignals {
		// Aggregate signals: use minimum (worst) signal instead of mean
		// Since 1.0=normal and 0.0=anomalous, the minimum catches degradation
		// Mean would dilute the signal when some metrics drop while others stay stable
		if len(signals) > 0 {
			globalScore = signals[0]
			for _, s := range signals {
				if s < globalScore {
					globalScore = s
				}
			}
		}
	} else {
		// Use the first signal as the global score (useful if wrapping another detector)
		if len(signals) > 0 {
			globalScore = signals[0]
		}
	}

	// Add to history
	d.globalScoreHistory = append(d.globalScoreHistory, globalScore)
	d.baselineHistory = append(d.baselineHistory, globalScore)

	// Trim histories
	if len(d.globalScoreHistory) > d.baselineWindowSize*2 {
		d.globalScoreHistory = d.globalScoreHistory[1:]
	}
	if len(d.baselineHistory) > d.baselineWindowSize {
		d.baselineHistory = d.baselineHistory[1:]
	}

	// Need minimum data before detecting
	if len(d.baselineHistory) < d.baselineWindowSize/2 {
		return 0.0, nil
	}

	// Detect change point based on method
	var changeDetected bool
	var changeScore float64

	switch d.method {
	case "page-hinkley":
		changeDetected, changeScore = d.detectPageHinkley(globalScore)
	case "two-window":
		changeDetected, changeScore = d.detectTwoWindow()
	case "cusum":
		changeDetected, changeScore = d.detectCUSUM(globalScore)
	default:
		changeDetected, changeScore = d.detectPageHinkley(globalScore)
	}

	// Check if we should alert
	now := time.Now()
	suppressDuration := time.Duration(d.suppressMinutes) * time.Minute
	timeSinceLastAlert := now.Sub(d.lastAlertTime)

	if changeDetected && timeSinceLastAlert > suppressDuration {
		d.lastAlertTime = now
		d.lastChangePoint = len(d.globalScoreHistory)
		return changeScore, nil
	}

	// Return change score for continuous monitoring (not normalized)
	// This allows visualization of the detector's change detection signal
	return changeScore, nil
}

// detectPageHinkley detects changes using the Page-Hinkley algorithm
func (d *ChangePointDetector) detectPageHinkley(currentScore float64) (bool, float64) {
	if !d.phInitialized {
		// Initialize baseline mean
		d.phMean = computeMedian(d.baselineHistory)
		d.phSum = 0.0
		d.phMinSum = 0.0
		d.phSumDown = 0.0
		d.phMinSumDown = 0.0
		d.phInitialized = true
	}

	// Update cumulative sum for upward shifts
	// S_t = S_{t-1} + (x_t - mean - delta)
	delta := d.cusumDrift
	d.phSum += (currentScore - d.phMean - delta)

	// Track minimum
	if d.phSum < d.phMinSum {
		d.phMinSum = d.phSum
	}

	// Update cumulative sum for downward shifts
	// Since 1.0=normal and 0.0=anomalous, downward shifts are the primary concern
	d.phSumDown += (d.phMean - currentScore - delta)

	// Track minimum for downward
	if d.phSumDown < d.phMinSumDown {
		d.phMinSumDown = d.phSumDown
	}

	// Compute test statistics
	Mt := d.phSum - d.phMinSum
	MtDown := d.phSumDown - d.phMinSumDown

	// Compute baseline std for threshold scaling
	_, baselineStd := computeMeanStd(d.baselineHistory)
	if baselineStd < 1e-10 {
		baselineStd = 1.0
	}

	threshold := d.cusumThreshold * baselineStd

	// Detect upward change if M_t > threshold
	if Mt > threshold {
		// Reset upward tracking
		d.phSum = 0.0
		d.phMinSum = 0.0
		d.phMean = currentScore // update mean to new regime
		return true, Mt / baselineStd
	}

	// Detect downward change if MtDown > threshold
	if MtDown > threshold {
		// Reset downward tracking
		d.phSumDown = 0.0
		d.phMinSumDown = 0.0
		d.phMean = currentScore // update mean to new regime
		return true, MtDown / baselineStd
	}

	// Return the larger of the two statistics for monitoring
	maxStat := Mt
	if MtDown > maxStat {
		maxStat = MtDown
	}
	return false, maxStat / baselineStd
}

// detectTwoWindow detects changes using a two-window robust test
func (d *ChangePointDetector) detectTwoWindow() (bool, float64) {
	histLen := len(d.globalScoreHistory)

	// Need enough data for both windows
	if histLen < d.recentWindowSize+d.baselineWindowSize {
		return false, 0.0
	}

	// Get recent window (last N points)
	recentStart := histLen - d.recentWindowSize
	recentWindow := d.globalScoreHistory[recentStart:]

	// Get reference window (baseline, excluding recent)
	refEnd := recentStart
	refStart := refEnd - d.baselineWindowSize
	if refStart < 0 {
		refStart = 0
	}
	refWindow := d.globalScoreHistory[refStart:refEnd]

	if len(refWindow) == 0 {
		return false, 0.0
	}

	// Compute robust statistics
	recentMedian := computeMedian(recentWindow)
	refMedian := computeMedian(refWindow)
	refMAD := computeMAD(refWindow, refMedian)

	// Robust scale
	scale := 1.4826 * refMAD
	if scale < 1e-10 {
		scale = 1e-10
	}

	// Test statistic: robust z-score for difference in medians
	// T = (median_recent - median_ref) / (scale / sqrt(n_recent))
	T := (recentMedian - refMedian) / (scale / math.Sqrt(float64(d.recentWindowSize)))

	// Detect change if T exceeds threshold
	changeDetected := math.Abs(T) > d.twoWindowThreshold

	return changeDetected, math.Abs(T)
}

// detectCUSUM detects changes using the CUSUM algorithm
func (d *ChangePointDetector) detectCUSUM(currentScore float64) (bool, float64) {
	// CUSUM maintains two cumulative sums: one for upward shifts, one for downward
	// For simplicity, we'll track upward shifts (scores increasing)

	if !d.phInitialized {
		// Initialize baseline mean
		d.phMean = computeMedian(d.baselineHistory)
		d.phSum = 0.0
		d.phInitialized = true
	}

	// CUSUM for upward shift
	// S_t = max(0, S_{t-1} + (x_t - mean - k))
	// where k is the slack parameter (allowable drift)
	k := d.cusumDrift
	d.phSum = math.Max(0, d.phSum+(currentScore-d.phMean-k))

	// Compute baseline std for threshold scaling
	_, baselineStd := computeMeanStd(d.baselineHistory)
	if baselineStd < 1e-10 {
		baselineStd = 1.0
	}

	threshold := d.cusumThreshold * baselineStd

	// Detect change if S_t > threshold
	if d.phSum > threshold {
		// Reset
		d.phSum = 0.0
		d.phMean = currentScore // update mean to new regime
		return true, d.phSum / baselineStd
	}

	return false, d.phSum / baselineStd
}

// Reset resets the detector state (useful after acknowledging a change)
func (d *ChangePointDetector) Reset() {
	d.phSum = 0.0
	d.phMinSum = 0.0
	d.phSumDown = 0.0
	d.phMinSumDown = 0.0
	d.phInitialized = false
	d.globalScoreHistory = make([]float64, 0, d.baselineWindowSize)
	d.baselineHistory = make([]float64, 0, d.baselineWindowSize)
}
