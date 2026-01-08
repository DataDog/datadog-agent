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

// CUSUMDetector uses the Cumulative Sum (CUSUM) algorithm to detect when a
// metric shifts from its baseline. CUSUM is designed for detecting change points
// and naturally provides anomaly start and end times.
//
// Algorithm:
//
//	S[0] = 0
//	S[t] = max(0, S[t-1] + (x[t] - μ - k))
//
// Where μ is the baseline mean and k is the slack parameter (allowance for noise).
// An anomaly is detected when S[t] exceeds threshold h.
//
// Start time: When S first became positive (deviation started accumulating).
// End time: When S returned to 0 (deviation "paid back"), or end of series if ongoing.
type CUSUMDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	// Default: 5
	MinPoints int

	// BaselineFraction is the fraction of points to use for baseline estimation.
	// Default: 0.25 (first 25% of data)
	BaselineFraction float64

	// SlackFactor is multiplied by baseline stddev to get k (slack parameter).
	// Higher values make detection less sensitive to small shifts.
	// Default: 0.5
	SlackFactor float64

	// ThresholdFactor is multiplied by baseline stddev to get h (threshold).
	// Higher values require larger cumulative deviation to trigger.
	// Default: 4.0
	ThresholdFactor float64
}

// NewCUSUMDetector creates a CUSUMDetector with default settings.
func NewCUSUMDetector() *CUSUMDetector {
	return &CUSUMDetector{
		MinPoints:        5,
		BaselineFraction: 0.25,
		SlackFactor:      0.5,
		ThresholdFactor:  4.0,
	}
}

// Name returns the analysis name.
func (c *CUSUMDetector) Name() string {
	return "cusum_detector"
}

// Analyze runs CUSUM on the series and returns an anomaly if a shift is detected.
// The anomaly's TimeRange indicates when the shift started and the end of analyzed data.
func (c *CUSUMDetector) Analyze(series observer.Series) observer.TimeSeriesAnalysisResult {
	minPoints := c.MinPoints
	if minPoints <= 0 {
		minPoints = 5
	}
	baselineFrac := c.BaselineFraction
	if baselineFrac <= 0 {
		baselineFrac = 0.25
	}
	slackFactor := c.SlackFactor
	if slackFactor <= 0 {
		slackFactor = 0.5
	}
	thresholdFactor := c.ThresholdFactor
	if thresholdFactor <= 0 {
		thresholdFactor = 4.0
	}

	n := len(series.Points)
	if n < minPoints {
		return observer.TimeSeriesAnalysisResult{}
	}

	// Estimate baseline from first portion of data
	baselineEnd := int(float64(n) * baselineFrac)
	if baselineEnd < 2 {
		baselineEnd = 2
	}
	if baselineEnd >= n {
		baselineEnd = n - 1
	}

	baselinePoints := series.Points[:baselineEnd]
	baselineMean := mean(baselinePoints)
	baselineStddev := sampleStddev(baselinePoints, baselineMean)

	// Handle constant baseline (stddev ≈ 0)
	// Use a minimum stddev based on the mean to avoid division issues
	const epsilon = 1e-10
	if baselineStddev < epsilon {
		if baselineMean > epsilon {
			// Use 10% of mean as minimum stddev for relative change detection
			baselineStddev = baselineMean * 0.1
		} else {
			// Can't establish meaningful baseline
			return observer.TimeSeriesAnalysisResult{}
		}
	}

	// CUSUM parameters
	k := slackFactor * baselineStddev     // slack: ignore small deviations
	h := thresholdFactor * baselineStddev // threshold: trigger level

	// Run CUSUM and collect anomaly regions
	// An anomaly region starts when S first goes positive and ends when S returns to 0
	anomalies := runCUSUM(series, baselineMean, baselineStddev, k, h)

	return observer.TimeSeriesAnalysisResult{Anomalies: anomalies}
}

// anomalyRegion tracks a detected anomaly's boundaries.
type anomalyRegion struct {
	startIdx     int  // index where S first became positive
	endIdx       int  // index where S returned to 0, or -1 if ongoing
	crossedThreshold bool // true if S exceeded h at some point
}

// runCUSUM executes the CUSUM algorithm and returns detected anomalies.
// Uses CUSUM for start detection (when deviation began accumulating) and
// signal-level recovery detection for end (when values return to normal range).
func runCUSUM(series observer.Series, baselineMean, baselineStddev, k, h float64) []observer.AnomalyOutput {
	var S float64
	var regions []anomalyRegion
	var currentRegion *anomalyRegion

	// Recovery threshold: signal is "normal" if within 2σ of baseline
	recoveryThreshold := baselineMean + 2*baselineStddev

	for i, p := range series.Points {
		prevS := S
		S = math.Max(0, S+(p.Value-baselineMean-k))

		// Detect transitions
		if prevS == 0 && S > 0 {
			// Starting a new region
			currentRegion = &anomalyRegion{startIdx: i, endIdx: -1}
		}

		if currentRegion != nil {
			// Check if we crossed the threshold (confirmed anomaly)
			if S > h {
				currentRegion.crossedThreshold = true
			}

			// Check if region ended - two conditions:
			// 1. CUSUM returned to 0 (deviation fully "paid back")
			// 2. Signal returned to normal range AND we've crossed threshold
			recovered := S == 0 || (currentRegion.crossedThreshold && p.Value <= recoveryThreshold)

			if recovered && currentRegion.crossedThreshold {
				// Find the actual end: last point where value was elevated
				endIdx := i
				if p.Value <= recoveryThreshold {
					// Walk back to find last elevated point
					for j := i - 1; j >= currentRegion.startIdx; j-- {
						if series.Points[j].Value > recoveryThreshold {
							endIdx = j
							break
						}
					}
				}
				currentRegion.endIdx = endIdx
				regions = append(regions, *currentRegion)
				currentRegion = nil
				S = 0 // reset CUSUM for next potential region
			}
		}
	}

	// Handle ongoing region at end of series
	if currentRegion != nil && currentRegion.crossedThreshold {
		currentRegion.endIdx = len(series.Points) - 1
		regions = append(regions, *currentRegion)
	}

	// Convert regions to anomaly outputs
	// For now, report only the most recent anomaly (could report all)
	if len(regions) == 0 {
		return nil
	}

	// Take the last (most recent) region
	region := regions[len(regions)-1]
	startTs := series.Points[region.startIdx].Timestamp
	endTs := series.Points[region.endIdx].Timestamp

	// Calculate magnitude using points in the anomaly region
	anomalyMean := mean(series.Points[region.startIdx : region.endIdx+1])
	deviation := (anomalyMean - baselineMean) / baselineStddev

	return []observer.AnomalyOutput{{
		Source: series.Name,
		Title:  fmt.Sprintf("CUSUM shift detected: %s", series.Name),
		Description: fmt.Sprintf("%s shifted: %.2f → %.2f (%.1fσ above baseline)",
			series.Name, baselineMean, anomalyMean, deviation),
		Tags: series.Tags,
		TimeRange: observer.TimeRange{
			Start: startTs,
			End:   endTs,
		},
	}}
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
