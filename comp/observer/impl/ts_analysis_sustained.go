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

// SustainedElevationDetector detects when a metric stays elevated above baseline
// for a sustained period. It compares the recent mean to the baseline mean and
// fires an anomaly if the recent mean exceeds baseline by more than Threshold stddevs.
type SustainedElevationDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	// Default: 5
	MinPoints int
	// Threshold is the number of standard deviations above baseline mean
	// that recent mean must exceed to trigger an anomaly.
	// Default: 2.0
	Threshold float64
}

// NewSustainedElevationDetector creates a SustainedElevationDetector with default settings.
func NewSustainedElevationDetector() *SustainedElevationDetector {
	return &SustainedElevationDetector{
		MinPoints: 5,
		Threshold: 2.0,
	}
}

// Name returns the analysis name.
func (s *SustainedElevationDetector) Name() string {
	return "sustained_elevation_detector"
}

// Analyze checks if a series shows sustained elevation above baseline.
func (s *SustainedElevationDetector) Analyze(series observer.Series) observer.TimeSeriesAnalysisResult {
	minPoints := s.MinPoints
	if minPoints <= 0 {
		minPoints = 5
	}
	threshold := s.Threshold
	if threshold <= 0 {
		threshold = 2.0
	}

	if len(series.Points) < minPoints {
		return observer.TimeSeriesAnalysisResult{}
	}

	// Split series: first half = baseline, second half = recent
	mid := len(series.Points) / 2
	baselinePoints := series.Points[:mid]
	recentPoints := series.Points[mid:]

	// Calculate baseline mean
	baselineMean := mean(baselinePoints)

	// Calculate baseline stddev (sample stddev)
	baselineStddev := sampleStddev(baselinePoints, baselineMean)

	// Calculate recent mean
	recentMean := mean(recentPoints)

	// If stddev is 0 (constant baseline), skip detection
	// Use a small epsilon to avoid floating point issues
	const epsilon = 1e-10
	if baselineStddev < epsilon {
		return observer.TimeSeriesAnalysisResult{}
	}

	// Check if recent mean exceeds baseline mean + threshold * stddev
	elevationThreshold := baselineMean + threshold*baselineStddev
	if recentMean > elevationThreshold {
		return observer.TimeSeriesAnalysisResult{
			Anomalies: []observer.AnomalyOutput{{
				Source:      series.Name,
				Title:       fmt.Sprintf("Sustained elevation: %s", series.Name),
				Description: fmt.Sprintf("%s elevated: recent avg %.2f vs baseline %.2f (>%.2f stddev)", series.Name, recentMean, baselineMean, threshold),
				Tags:        series.Tags,
			}},
		}
	}

	return observer.TimeSeriesAnalysisResult{}
}

// mean calculates the arithmetic mean of points.
func mean(points []observer.Point) float64 {
	if len(points) == 0 {
		return 0
	}
	var sum float64
	for _, p := range points {
		sum += p.Value
	}
	return sum / float64(len(points))
}

// sampleStddev calculates the sample standard deviation.
// Uses Bessel's correction: sqrt(sum((x-mean)^2) / (n-1))
func sampleStddev(points []observer.Point, mean float64) float64 {
	n := len(points)
	if n < 2 {
		return 0
	}
	var sumSquares float64
	for _, p := range points {
		diff := p.Value - mean
		sumSquares += diff * diff
	}
	return math.Sqrt(sumSquares / float64(n-1))
}
