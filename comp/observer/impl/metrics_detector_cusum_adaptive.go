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

// AdaptiveCUSUMDetector is an improved CUSUM that automatically finds the most
// stable baseline window using sliding-window variance minimization, uses robust
// statistics (median + MAD), and adapts the threshold based on signal noise.
//
// Key improvements over standard CUSUM:
//   - Auto-baseline: slides a window across the series and picks the lowest-MAD
//     window, so disruption at the start doesn't contaminate the baseline.
//   - Robust statistics: median + scaled MAD (MAD * 1.4826 for normal consistency)
//     instead of mean + stddev, so outliers don't inflate the baseline.
//   - Minimum deviation gate: the crossing point must also show a minimum
//     instantaneous deviation from baseline to avoid slow-drift false positives.
type AdaptiveCUSUMDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	MinPoints int

	// BaselineWindowFraction is the fraction of series length used as the
	// sliding baseline window size.
	BaselineWindowFraction float64

	// SlackFactor is multiplied by the robust sigma estimate to get k (slack).
	SlackFactor float64

	// ThresholdFactor is multiplied by the robust sigma estimate to get h (threshold).
	ThresholdFactor float64

	// MinDeviationAtCrossing is the minimum |x - median| / sigma required at
	// the threshold crossing point to confirm the anomaly.
	MinDeviationAtCrossing float64

	// MinThresholdFactor sets a floor on the threshold relative to the median,
	// ensuring constant or near-constant baselines still have a meaningful threshold.
	MinThresholdFactor float64
}

// NewAdaptiveCUSUMDetector creates an AdaptiveCUSUMDetector with default settings.
func NewAdaptiveCUSUMDetector() *AdaptiveCUSUMDetector {
	return &AdaptiveCUSUMDetector{
		MinPoints:              10,
		BaselineWindowFraction: 0.25,
		SlackFactor:            0.5,
		ThresholdFactor:        5.0,
		MinDeviationAtCrossing: 2.5,
		MinThresholdFactor:     0.10,
	}
}

// Name returns the detector name.
func (d *AdaptiveCUSUMDetector) Name() string {
	return "cusum_adaptive_detector"
}

// Detect runs adaptive CUSUM on the series and returns an anomaly if a shift is detected.
func (d *AdaptiveCUSUMDetector) Detect(series observer.Series) observer.MetricsDetectionResult {
	minPoints := d.MinPoints
	if minPoints <= 0 {
		minPoints = 10
	}
	baselineWindowFrac := d.BaselineWindowFraction
	if baselineWindowFrac <= 0 {
		baselineWindowFrac = 0.25
	}
	slackFactor := d.SlackFactor
	if slackFactor <= 0 {
		slackFactor = 0.5
	}
	thresholdFactor := d.ThresholdFactor
	if thresholdFactor <= 0 {
		thresholdFactor = 5.0
	}
	minDeviation := d.MinDeviationAtCrossing
	if minDeviation <= 0 {
		minDeviation = 2.5
	}
	minThresholdFactor := d.MinThresholdFactor
	if minThresholdFactor <= 0 {
		minThresholdFactor = 0.10
	}

	n := len(series.Points)
	if n < minPoints {
		return observer.MetricsDetectionResult{}
	}

	// Find the most stable baseline window via sliding window MAD minimization
	windowSize := int(float64(n) * baselineWindowFrac)
	if windowSize < 3 {
		windowSize = 3
	}
	if windowSize >= n {
		windowSize = n - 1
	}

	bestStart := 0
	bestMAD := math.Inf(1)

	for start := 0; start+windowSize <= n; start++ {
		windowPoints := series.Points[start : start+windowSize]
		med := medianOfPoints(windowPoints)
		mad := madOfPoints(windowPoints, med)
		if mad < bestMAD {
			bestMAD = mad
			bestStart = start
		}
	}

	baselineEndIdx := bestStart + windowSize
	baselinePoints := series.Points[bestStart:baselineEndIdx]

	// Robust baseline statistics: median + MAD
	baselineMedian := medianOfPoints(baselinePoints)
	baselineMAD := madOfPoints(baselinePoints, baselineMedian)

	// Scale MAD to be a consistent estimator of stddev for normal distributions
	const madScaleFactor = 1.4826
	robustSigma := baselineMAD * madScaleFactor

	// Also compute mean + stddev for debug info
	baselineMean := mean(baselinePoints)
	baselineStddev := sampleStddev(baselinePoints, baselineMean)

	const epsilon = 1e-10

	// Handle near-constant baseline
	if robustSigma < epsilon {
		if math.Abs(baselineMedian) > epsilon {
			robustSigma = math.Abs(baselineMedian) * minThresholdFactor
		} else {
			return observer.MetricsDetectionResult{}
		}
	}

	// CUSUM parameters
	k := slackFactor * robustSigma
	h := thresholdFactor * robustSigma

	// Build debug info
	debugInfo := &observer.AnomalyDebugInfo{
		BaselineStart:  series.Points[bestStart].Timestamp,
		BaselineEnd:    series.Points[baselineEndIdx-1].Timestamp,
		BaselineMean:   baselineMean,
		BaselineMedian: baselineMedian,
		BaselineStddev: baselineStddev,
		BaselineMAD:    baselineMAD,
		Threshold:      h,
		SlackParam:     k,
	}

	// Run CUSUM from the beginning of the series, but skip detections within
	// the baseline window (those points are known-stable by construction).
	anomaly := runAdaptiveCUSUM(series, bestStart, baselineEndIdx, baselineMedian, robustSigma, k, h, minDeviation, debugInfo)
	if anomaly == nil {
		return observer.MetricsDetectionResult{}
	}

	return observer.MetricsDetectionResult{Anomalies: []observer.Anomaly{*anomaly}}
}

// runAdaptiveCUSUM executes a two-sided CUSUM over the full series.
// Detections within [baselineStart, baselineEnd) are suppressed since
// that window is known to be stable.
// Returns the first valid threshold crossing.
func runAdaptiveCUSUM(series observer.Series, baselineStart, baselineEnd int, baselineMedian, robustSigma, k, h, minDeviation float64, debugInfo *observer.AnomalyDebugInfo) *observer.Anomaly {
	var sHigh, sLow float64
	cusumValues := make([]float64, 0, len(series.Points))

	for i, p := range series.Points {
		// Upper CUSUM: detect increases
		sHigh = math.Max(0, sHigh+(p.Value-baselineMedian-k))
		// Lower CUSUM: detect decreases
		sLow = math.Max(0, sLow+(baselineMedian-p.Value-k))

		// Track the dominant CUSUM value
		if sHigh >= sLow {
			cusumValues = append(cusumValues, sHigh)
		} else {
			cusumValues = append(cusumValues, -sLow)
		}

		// Skip detections within the baseline window
		if i >= baselineStart && i < baselineEnd {
			continue
		}

		// Check for upward shift
		if sHigh > h {
			deviation := (p.Value - baselineMedian) / robustSigma
			if deviation < minDeviation {
				continue // accumulated drift without clear shift at this point
			}
			debugInfo.CurrentValue = p.Value
			debugInfo.DeviationSigma = deviation
			if len(cusumValues) > 50 {
				debugInfo.CUSUMValues = cusumValues[len(cusumValues)-50:]
			} else {
				debugInfo.CUSUMValues = cusumValues
			}
			return &observer.Anomaly{
				Source: observer.MetricName(series.Name),
				Title:  fmt.Sprintf("Adaptive CUSUM shift detected: %s", series.Name),
				Description: fmt.Sprintf("%s shifted to %.2f (%.1fσ above baseline median of %.2f)",
					series.Name, p.Value, deviation, baselineMedian),
				Tags:      series.Tags,
				Timestamp: series.Points[i].Timestamp,
				DebugInfo: debugInfo,
			}
		}

		// Check for downward shift
		if sLow > h {
			deviation := (baselineMedian - p.Value) / robustSigma
			if deviation < minDeviation {
				continue
			}
			debugInfo.CurrentValue = p.Value
			debugInfo.DeviationSigma = -deviation
			if len(cusumValues) > 50 {
				debugInfo.CUSUMValues = cusumValues[len(cusumValues)-50:]
			} else {
				debugInfo.CUSUMValues = cusumValues
			}
			return &observer.Anomaly{
				Source: observer.MetricName(series.Name),
				Title:  fmt.Sprintf("Adaptive CUSUM shift detected: %s", series.Name),
				Description: fmt.Sprintf("%s shifted to %.2f (%.1fσ below baseline median of %.2f)",
					series.Name, p.Value, deviation, baselineMedian),
				Tags:      series.Tags,
				Timestamp: series.Points[i].Timestamp,
				DebugInfo: debugInfo,
			}
		}
	}

	return nil
}

// medianOfPoints computes the median value of a slice of Points.
func medianOfPoints(points []observer.Point) float64 {
	n := len(points)
	if n == 0 {
		return 0
	}
	values := make([]float64, n)
	for i, p := range points {
		values[i] = p.Value
	}
	sort.Float64s(values)
	if n%2 == 0 {
		return (values[n/2-1] + values[n/2]) / 2
	}
	return values[n/2]
}

// madOfPoints computes the Median Absolute Deviation of a slice of Points.
// MAD = median(|x_i - median(x)|)
func madOfPoints(points []observer.Point, median float64) float64 {
	n := len(points)
	if n == 0 {
		return 0
	}
	deviations := make([]float64, n)
	for i, p := range points {
		deviations[i] = math.Abs(p.Value - median)
	}
	sort.Float64s(deviations)
	if n%2 == 0 {
		return (deviations[n/2-1] + deviations[n/2]) / 2
	}
	return deviations[n/2]
}
