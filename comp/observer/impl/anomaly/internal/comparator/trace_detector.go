// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package comparator

import (
	"math"
)

// TraceDetector detects anomalies in trace latency metrics using exponential smoothing
// and adaptive thresholding. This approach is specifically designed for latency metrics
// where relative changes matter more than absolute values.
type TraceDetector struct {
	ewma        float64 // exponentially weighted moving average
	ewmStd      float64 // exponentially weighted moving std deviation
	alpha       float64 // smoothing factor (0.1-0.3)
	threshold   float64 // Z-score threshold for anomaly detection
	initialized bool    // whether detector has been initialized
}

// NewTraceDetector creates a new trace anomaly detector
// alpha: smoothing factor (0.1 = more smoothing, 0.3 = more responsive)
// threshold: Z-score threshold (2.5-3.0, higher = less sensitive)
func NewTraceDetector(alpha float64, threshold float64) *TraceDetector {
	return &TraceDetector{
		alpha:       alpha,
		threshold:   threshold,
		initialized: false,
	}
}

// ComputeScore computes an anomaly score for the current trace latency value
// Returns a similarity score where:
//   - 1.0 = normal (similar to baseline)
//   - 0.0 = very anomalous (significant deviation from baseline)
//
// The algorithm:
// 1. Maintains EWMA of values to track baseline
// 2. Computes relative deviation: |current - baseline| / baseline
// 3. Maintains EWMA of relative deviations to track typical variability
// 4. Computes Z-score: how many "standard deviations" away from normal
// 5. Converts Z-score to similarity score using threshold normalization
func (d *TraceDetector) ComputeScore(currentValue float64) float64 {
	// Handle first data point - initialize baseline
	if !d.initialized {
		d.ewma = currentValue
		d.ewmStd = 0.0
		d.initialized = true
		return 1.0 // First point is considered normal
	}

	// Handle zero or near-zero baseline (avoid division by zero)
	if d.ewma < 1e-6 {
		// For very small baselines, use absolute deviation instead of relative
		absoluteDeviation := math.Abs(currentValue - d.ewma)

		// Update EWMA with current value
		d.ewma = d.alpha*currentValue + (1-d.alpha)*d.ewma

		// Update EWMA of absolute deviation
		d.ewmStd = d.alpha*absoluteDeviation + (1-d.alpha)*d.ewmStd

		// Compute Z-score using absolute deviation
		zScore := 0.0
		if d.ewmStd > 1e-6 {
			zScore = absoluteDeviation / d.ewmStd
		}

		// Convert to similarity score
		similarity := 1.0 / (1.0 + zScore/d.threshold)
		return similarity
	}

	// Compute relative deviation (percentage change from baseline)
	// This makes the detector scale-invariant and works for all latency ranges
	relativeDeviation := math.Abs(currentValue-d.ewma) / d.ewma

	// Update EWMA of the value (baseline)
	d.ewma = d.alpha*currentValue + (1-d.alpha)*d.ewma

	// Update EWMA of relative deviation (typical variability)
	d.ewmStd = d.alpha*relativeDeviation + (1-d.alpha)*d.ewmStd

	// Compute Z-score: how many "std deviations" is this deviation?
	zScore := 0.0
	if d.ewmStd > 1e-6 {
		zScore = relativeDeviation / d.ewmStd
	} else {
		// No variability in history - any deviation is significant
		if relativeDeviation > 0.01 { // More than 1% change
			zScore = relativeDeviation / 0.01
		}
	}

	// Convert Z-score to similarity score [0, 1]
	// - zScore = 0 → similarity = 1.0 (normal)
	// - zScore = threshold → similarity = 0.5 (borderline)
	// - zScore >> threshold → similarity → 0 (anomalous)
	similarity := 1.0 / (1.0 + zScore/d.threshold)

	return similarity
}

// Reset clears the detector's history
func (d *TraceDetector) Reset() {
	d.ewma = 0.0
	d.ewmStd = 0.0
	d.initialized = false
}

// GetBaseline returns the current baseline (EWMA)
func (d *TraceDetector) GetBaseline() float64 {
	return d.ewma
}

// GetVariability returns the current typical variability (EWMA of std dev)
func (d *TraceDetector) GetVariability() float64 {
	return d.ewmStd
}
