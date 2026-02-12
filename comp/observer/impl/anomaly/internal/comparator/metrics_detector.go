// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package comparator provides telemetry comparison and anomaly detection logic.
package comparator

import (
	"math"
)

// MetricsDetector detects anomalies in custom metrics using exponential smoothing
// and bidirectional deviation detection. Works for all metric types (latencies,
// counts, sizes, etc.) by detecting ANY significant change from baseline.
type MetricsDetector struct {
	// Per-metric state: map from metric name to detector state
	metricStates map[string]*metricState
	alpha        float64 // smoothing factor
	threshold    float64 // Z-score threshold
}

// metricState tracks the state for a single metric
type metricState struct {
	ewma        float64 // exponentially weighted moving average (baseline)
	ewmStd      float64 // exponentially weighted moving std deviation (variability)
	initialized bool    // whether this metric has been seen before
}

// NewMetricsDetector creates a new bidirectional metrics detector
// alpha: smoothing factor (0.1 = more smoothing, 0.3 = more responsive)
// threshold: Z-score threshold (2.5-3.0, higher = less sensitive)
func NewMetricsDetector(alpha float64, threshold float64) *MetricsDetector {
	return &MetricsDetector{
		metricStates: make(map[string]*metricState),
		alpha:        alpha,
		threshold:    threshold,
	}
}

// ComputeScore computes an aggregated anomaly score across all metrics
// Processes a map of metric_name -> value and returns a single similarity score
// Returns:
//   - 1.0 = all metrics normal
//   - 0.0 = severe anomalies detected
func (d *MetricsDetector) ComputeScore(metrics map[string]float64) float64 {
	if len(metrics) == 0 {
		return 1.0 // No metrics = no anomaly
	}

	totalScore := 0.0
	count := 0

	// Compute score for each metric
	for metricName, currentValue := range metrics {
		score := d.computeMetricScore(metricName, currentValue)
		totalScore += score
		count++
	}

	// Return average similarity score
	if count > 0 {
		return totalScore / float64(count)
	}
	return 1.0
}

// computeMetricScore computes similarity score for a single metric
func (d *MetricsDetector) computeMetricScore(metricName string, currentValue float64) float64 {
	// Get or create state for this metric
	state, exists := d.metricStates[metricName]
	if !exists {
		state = &metricState{
			initialized: false,
		}
		d.metricStates[metricName] = state
	}

	// First time seeing this metric - initialize baseline
	if !state.initialized {
		state.ewma = currentValue
		state.ewmStd = 0.0
		state.initialized = true
		return 1.0 // First point is normal
	}

	// Handle zero or near-zero baseline
	if state.ewma < 1e-6 {
		// For metrics near zero, use absolute deviation
		absoluteDeviation := math.Abs(currentValue - state.ewma)

		// Update EWMA
		state.ewma = d.alpha*currentValue + (1-d.alpha)*state.ewma

		// Update EWMA of absolute deviation
		state.ewmStd = d.alpha*absoluteDeviation + (1-d.alpha)*state.ewmStd

		// Compute Z-score
		zScore := 0.0
		if state.ewmStd > 1e-6 {
			zScore = absoluteDeviation / state.ewmStd
		} else if absoluteDeviation > 0.01 {
			// No historical variability, but current deviation exists
			zScore = absoluteDeviation / 0.01
		}

		// Convert to similarity score
		similarity := 1.0 / (1.0 + zScore/d.threshold)
		return similarity
	}

	// Compute BIDIRECTIONAL relative deviation (works for all metric types)
	// Detects both increases AND decreases from baseline
	relativeDeviation := math.Abs(currentValue-state.ewma) / state.ewma

	// Update EWMA baseline
	state.ewma = d.alpha*currentValue + (1-d.alpha)*state.ewma

	// Update EWMA of typical variability (tracking relative deviations)
	state.ewmStd = d.alpha*relativeDeviation + (1-d.alpha)*state.ewmStd

	// Compute Z-score: how many "standard deviations" away from normal
	zScore := 0.0
	if state.ewmStd > 1e-6 {
		zScore = relativeDeviation / state.ewmStd
	} else {
		// No historical variability - use 5% as baseline
		if relativeDeviation > 0.05 {
			zScore = relativeDeviation / 0.05
		}
	}

	// Convert Z-score to similarity score [0, 1]
	// - zScore = 0 → similarity = 1.0 (normal)
	// - zScore = threshold → similarity = 0.5 (borderline)
	// - zScore >> threshold → similarity → 0 (anomalous)
	similarity := 1.0 / (1.0 + zScore/d.threshold)

	return similarity
}

// Reset clears all metric states
func (d *MetricsDetector) Reset() {
	d.metricStates = make(map[string]*metricState)
}

// GetMetricBaseline returns the current baseline for a metric (for debugging)
func (d *MetricsDetector) GetMetricBaseline(metricName string) (float64, bool) {
	state, exists := d.metricStates[metricName]
	if !exists || !state.initialized {
		return 0, false
	}
	return state.ewma, true
}

// GetMetricVariability returns the typical variability for a metric (for debugging)
func (d *MetricsDetector) GetMetricVariability(metricName string) (float64, bool) {
	state, exists := d.metricStates[metricName]
	if !exists || !state.initialized {
		return 0, false
	}
	return state.ewmStd, true
}
