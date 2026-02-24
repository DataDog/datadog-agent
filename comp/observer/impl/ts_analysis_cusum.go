// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// CUSUMDetector uses the Cumulative Sum (CUSUM) algorithm to detect when a
// metric shifts from its baseline. CUSUM is designed for detecting change points.
//
// Algorithm:
//
//	S[0] = 0
//	S[t] = max(0, S[t-1] + (x[t] - μ - k))
//
// Where μ is the baseline mean and k is the slack parameter (allowance for noise).
// An anomaly is emitted when S[t] first exceeds threshold h, representing the
// point of change detection.
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

	// SkipCountMetrics skips :count metrics which can be noisy from container scaling.
	// Default: true
	SkipCountMetrics bool
}

// NewCUSUMDetector creates a CUSUMDetector with default settings.
func NewCUSUMDetector() *CUSUMDetector {
	return &CUSUMDetector{
		MinPoints:        5,
		BaselineFraction: 0.25,
		SlackFactor:      0.5,
		ThresholdFactor:  4.0,
		SkipCountMetrics: true,
	}
}

// NewCUSUMDetectorWithConfig creates a CUSUMDetector with custom settings.
func NewCUSUMDetectorWithConfig(skipCountMetrics bool) *CUSUMDetector {
	d := NewCUSUMDetector()
	d.SkipCountMetrics = skipCountMetrics
	return d
}

// Name returns the analysis name.
func (c *CUSUMDetector) Name() string {
	return "cusum_detector"
}

// Analyze runs CUSUM on the series and returns an anomaly if a shift is detected.
// The anomaly's Timestamp indicates when the shift was first detected (threshold crossing).
func (c *CUSUMDetector) Analyze(series observer.Series) observer.TimeSeriesAnalysisResult {
	// Skip :count metrics - these are cardinality counts that create noise
	// when container counts change (e.g., 1 -> 2 pods)
	if c.SkipCountMetrics && strings.HasSuffix(series.Name, ":count") {
		return observer.TimeSeriesAnalysisResult{}
	}

	isVirtual := strings.HasPrefix(series.Name, "_.")

	if isVirtual {
		fmt.Printf("[cc] CUSUMDetector: running on %s\n", series.Name)
	}

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

	// Build debug info
	debugInfo := &observer.AnomalyDebugInfo{
		BaselineStart:  series.Points[0].Timestamp,
		BaselineEnd:    series.Points[baselineEnd-1].Timestamp,
		BaselineMean:   baselineMean,
		BaselineStddev: baselineStddev,
		Threshold:      h,
		SlackParam:     k,
	}

	// Run CUSUM and detect threshold crossing
	anomaly := runCUSUM(series, baselineMean, baselineStddev, k, h, debugInfo)
	if anomaly == nil {
		return observer.TimeSeriesAnalysisResult{}
	}

	if isVirtual {
		fmt.Printf("[cc] CUSUMDetector: Anomaly detected: %s\n", series.Name)
	}

	return observer.TimeSeriesAnalysisResult{Anomalies: []observer.AnomalyOutput{*anomaly}}
}

// runCUSUM executes a two-sided CUSUM algorithm and returns an anomaly at the first threshold crossing.
// It tracks both upward shifts (S_high) and downward shifts (S_low).
// Returns nil if no threshold crossing is detected.
func runCUSUM(series observer.Series, baselineMean, baselineStddev, k, h float64, debugInfo *observer.AnomalyDebugInfo) *observer.AnomalyOutput {
	var sHigh, sLow float64
	cusumValues := make([]float64, 0, len(series.Points))

	for i, p := range series.Points {
		// Upper CUSUM: detect increases
		sHigh = math.Max(0, sHigh+(p.Value-baselineMean-k))
		// Lower CUSUM: detect decreases
		sLow = math.Max(0, sLow+(baselineMean-p.Value-k))

		// Track the dominant CUSUM value (whichever is larger)
		if sHigh >= sLow {
			cusumValues = append(cusumValues, sHigh)
		} else {
			cusumValues = append(cusumValues, -sLow) // negative for downward
		}

		// Check for upward shift
		if sHigh > h {
			deviation := (p.Value - baselineMean) / baselineStddev
			debugInfo.CurrentValue = p.Value
			debugInfo.DeviationSigma = deviation
			// Keep last 50 CUSUM values for visualization
			if len(cusumValues) > 50 {
				debugInfo.CUSUMValues = cusumValues[len(cusumValues)-50:]
			} else {
				debugInfo.CUSUMValues = cusumValues
			}
			return &observer.AnomalyOutput{
				Source: observer.MetricName(series.Name),
				Title:  fmt.Sprintf("CUSUM shift detected: %s", series.Name),
				Description: fmt.Sprintf("%s shifted to %.2f (%.1fσ above baseline of %.2f)",
					series.Name, p.Value, deviation, baselineMean),
				Tags:      series.Tags,
				Timestamp: series.Points[i].Timestamp,
				DebugInfo: debugInfo,
			}
		}

		// Check for downward shift
		if sLow > h {
			deviation := (baselineMean - p.Value) / baselineStddev
			debugInfo.CurrentValue = p.Value
			debugInfo.DeviationSigma = -deviation // negative for below baseline
			if len(cusumValues) > 50 {
				debugInfo.CUSUMValues = cusumValues[len(cusumValues)-50:]
			} else {
				debugInfo.CUSUMValues = cusumValues
			}
			return &observer.AnomalyOutput{
				Source: observer.MetricName(series.Name),
				Title:  fmt.Sprintf("CUSUM shift detected: %s", series.Name),
				Description: fmt.Sprintf("%s shifted to %.2f (%.1fσ below baseline of %.2f)",
					series.Name, p.Value, deviation, baselineMean),
				Tags:      series.Tags,
				Timestamp: series.Points[i].Timestamp,
				DebugInfo: debugInfo,
			}
		}
	}

	return nil
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
