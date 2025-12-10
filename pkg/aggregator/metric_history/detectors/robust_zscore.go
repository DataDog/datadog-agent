// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package detectors

import (
	"fmt"
	"math"
	"sort"

	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
)

// RobustZScoreDetector detects anomalies using the Modified Z-Score algorithm.
//
// Unlike the standard Z-score which uses mean and standard deviation, the robust
// Z-score uses median and MAD (Median Absolute Deviation). This makes it resistant
// to outliers contaminating the baseline statistics.
//
// The formula is: M_i = 0.6745 * (x_i - median) / MAD
// where 0.6745 is the 75th percentile of the standard normal distribution,
// used to scale MAD to be comparable to standard deviation.
//
// Reference: https://www.itl.nist.gov/div898/handbook/eda/section3/eda35h.htm
type RobustZScoreDetector struct {
	// Threshold is the minimum |M| score to trigger an anomaly.
	// Common values: 3.0 (less sensitive) to 3.5 (more sensitive).
	// Default: 3.5
	Threshold float64

	// MinDataPoints is the minimum number of points required before detection.
	// Default: 10
	MinDataPoints int

	// WindowSize is the number of recent points to use for computing baseline.
	// If 0, uses all available points.
	// Default: 0 (use all)
	WindowSize int
}

// NewRobustZScoreDetector creates a new detector with default parameters.
func NewRobustZScoreDetector() *RobustZScoreDetector {
	return &RobustZScoreDetector{
		Threshold:     3.5,
		MinDataPoints: 10,
		WindowSize:    0,
	}
}

// Name returns the detector's unique identifier.
func (d *RobustZScoreDetector) Name() string {
	return "robust_zscore"
}

// Analyze examines a series and detects anomalies using robust Z-score.
func (d *RobustZScoreDetector) Analyze(key mh.SeriesKey, history *mh.MetricHistory) []mh.Anomaly {
	points := history.Recent.ToSlice()
	if len(points) < d.MinDataPoints {
		return nil
	}

	// Extract values (using deltas to handle monotonic counters)
	// Delta-first approach: analyze rate of change, not absolute values
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Stats.Mean()
	}

	// Compute deltas
	deltas := make([]float64, len(values)-1)
	for i := 0; i < len(values)-1; i++ {
		deltas[i] = values[i+1] - values[i]
	}

	if len(deltas) < d.MinDataPoints {
		return nil
	}

	// Apply window if configured
	windowDeltas := deltas
	if d.WindowSize > 0 && len(deltas) > d.WindowSize {
		windowDeltas = deltas[len(deltas)-d.WindowSize:]
	}

	// Compute median and MAD
	median := computeMedian(windowDeltas)
	mad := computeMAD(windowDeltas, median)

	// If MAD is 0, the data is nearly constant.
	// This can happen when most values are identical but there's one outlier.
	// In this case, use the mean absolute deviation as a fallback,
	// or a small floor if truly constant.
	if mad < 1e-10 {
		// Check if there's any actual variance using mean absolute deviation
		meanAbsDev := computeMeanAbsoluteDeviation(windowDeltas, median)
		if meanAbsDev < 1e-10 {
			// Truly constant data - no anomalies possible
			return nil
		}
		// Use mean absolute deviation scaled to approximate MAD
		// (MAD ≈ 0.6745 * stddev, mean abs dev ≈ 0.8 * stddev for normal dist)
		mad = meanAbsDev * 0.85
	}

	// Check the most recent delta for anomaly
	// We check the last few points to catch recent anomalies
	var anomalies []mh.Anomaly
	checkCount := min(3, len(deltas))

	for i := len(deltas) - checkCount; i < len(deltas); i++ {
		delta := deltas[i]
		// Modified Z-score formula: M = 0.6745 * (x - median) / MAD
		mScore := 0.6745 * (delta - median) / mad

		// Skip if the absolute delta is negligible (avoids false positives on
		// near-constant metrics like percentages where tiny floating point
		// differences can create high M-scores due to tiny MAD)
		absDelta := math.Abs(delta)
		absDeviation := math.Abs(delta - median)
		if absDelta < 0.01 && absDeviation < 0.01 {
			continue // delta too small to be meaningful
		}

		if math.Abs(mScore) >= d.Threshold {
			anomalyType := "spike"
			direction := "increase"
			if mScore < 0 {
				anomalyType = "drop"
				direction = "decrease"
			}

			// Map to corresponding point (delta[i] is between points[i] and points[i+1])
			timestamp := points[i+1].Timestamp
			severity := math.Min(math.Abs(mScore)/7.0, 1.0) // normalize to 0-1

			anomalies = append(anomalies, mh.Anomaly{
				SeriesKey:    key,
				DetectorName: d.Name(),
				Timestamp:    timestamp,
				Type:         anomalyType,
				Severity:     severity,
				Message: fmt.Sprintf("Anomalous %s detected: delta=%.2f, median=%.2f, MAD=%.2f, M-score=%.1f",
					direction, delta, median, mad, mScore),
			})
		}
	}

	return anomalies
}

// computeMedian returns the median of a slice of float64 values.
// The input slice is not modified.
func computeMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Make a copy to avoid modifying the original
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// computeMAD returns the Median Absolute Deviation from the median.
func computeMAD(values []float64, median float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Compute absolute deviations from median
	absDeviations := make([]float64, len(values))
	for i, v := range values {
		absDeviations[i] = math.Abs(v - median)
	}

	return computeMedian(absDeviations)
}

// computeMeanAbsoluteDeviation returns the mean of absolute deviations from median.
// This is used as a fallback when MAD is 0 but there's still variance.
func computeMeanAbsoluteDeviation(values []float64, median float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range values {
		sum += math.Abs(v - median)
	}

	return sum / float64(len(values))
}
