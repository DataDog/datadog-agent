// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// RobustZScoreDetector uses the Modified Z-Score algorithm to detect anomalies.
// Unlike standard z-score which uses mean and stddev, this uses median and MAD
// (Median Absolute Deviation), making it robust to outliers.
//
// Algorithm:
//
//	MAD = median(|x - median(x)|)
//	Modified Z-Score = 0.6745 * (x - median) / MAD
//
// The constant 0.6745 makes MAD consistent with stddev for normal distributions.
// Points with |Modified Z-Score| > threshold are flagged as anomalies.
type RobustZScoreDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	// Default: 10
	MinPoints int

	// BaselineFraction is the fraction of points to use for baseline estimation.
	// Default: 0.25 (first 25% of data)
	BaselineFraction float64

	// Threshold is the modified z-score threshold for anomaly detection.
	// Common values: 3.0 (less sensitive), 3.5 (moderate), 5.0 (very selective)
	// Default: 3.5
	Threshold float64
}

// NewRobustZScoreDetector creates a RobustZScoreDetector with default settings.
func NewRobustZScoreDetector() *RobustZScoreDetector {
	return &RobustZScoreDetector{
		MinPoints:        10,
		BaselineFraction: 0.25,
		Threshold:        3.5,
	}
}

// Name returns the analysis name.
func (r *RobustZScoreDetector) Name() string {
	return "robust_zscore"
}

// Analyze runs robust z-score detection on the series and returns anomalies.
func (r *RobustZScoreDetector) Analyze(series observer.Series) observer.TimeSeriesAnalysisResult {
	minPoints := r.MinPoints
	if minPoints <= 0 {
		minPoints = 10
	}
	baselineFrac := r.BaselineFraction
	if baselineFrac <= 0 {
		baselineFrac = 0.25
	}
	threshold := r.Threshold
	if threshold <= 0 {
		threshold = 3.5
	}

	n := len(series.Points)
	if n < minPoints {
		return observer.TimeSeriesAnalysisResult{}
	}

	// Estimate baseline from first portion of data
	baselineEnd := int(float64(n) * baselineFrac)
	if baselineEnd < 3 {
		baselineEnd = 3
	}
	if baselineEnd >= n {
		baselineEnd = n - 1
	}

	baselineValues := make([]float64, baselineEnd)
	for i := 0; i < baselineEnd; i++ {
		baselineValues[i] = series.Points[i].Value
	}

	baselineMedian := median(baselineValues)
	baselineMAD := mad(baselineValues, baselineMedian)

	// Handle constant baseline (MAD ≈ 0)
	const epsilon = 1e-10
	if baselineMAD < epsilon {
		if math.Abs(baselineMedian) > epsilon {
			// Use 10% of median as minimum MAD
			baselineMAD = math.Abs(baselineMedian) * 0.1
		} else {
			// Can't establish meaningful baseline
			return observer.TimeSeriesAnalysisResult{}
		}
	}

	// Scan for anomalies after the baseline period
	var anomalies []observer.AnomalyOutput
	const k = 0.6745 // consistency constant for normal distribution

	for i := baselineEnd; i < n; i++ {
		p := series.Points[i]
		modifiedZScore := k * (p.Value - baselineMedian) / baselineMAD

		if math.Abs(modifiedZScore) > threshold {
			direction := "above"
			if modifiedZScore < 0 {
				direction = "below"
			}

			debugInfo := &observer.AnomalyDebugInfo{
				BaselineStart:  series.Points[0].Timestamp,
				BaselineEnd:    series.Points[baselineEnd-1].Timestamp,
				BaselineMedian: baselineMedian,
				BaselineMAD:    baselineMAD,
				Threshold:      threshold,
				CurrentValue:   p.Value,
				DeviationSigma: modifiedZScore,
			}

			anomaly := observer.AnomalyOutput{
				Source: series.Name,
				Title:  "Robust Z-Score anomaly: " + series.Name,
				Description: fmt.Sprintf("%s value %.2f is %.1fσ %s median baseline of %.2f",
					series.Name, p.Value, math.Abs(modifiedZScore), direction, baselineMedian),
				Tags:      series.Tags,
				Timestamp: p.Timestamp,
				DebugInfo: debugInfo,
			}
			anomalies = append(anomalies, anomaly)

			// Only report the first anomaly per series (like CUSUM)
			break
		}
	}

	return observer.TimeSeriesAnalysisResult{Anomalies: anomalies}
}

// median calculates the median of a slice of float64 values.
// Returns 0 for empty slices.
func median(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}

	// Sort a copy to avoid mutating the original
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// mad calculates the Median Absolute Deviation.
// MAD = median(|x - median(x)|)
func mad(values []float64, med float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}

	deviations := make([]float64, n)
	for i, v := range values {
		deviations[i] = math.Abs(v - med)
	}

	return median(deviations)
}
