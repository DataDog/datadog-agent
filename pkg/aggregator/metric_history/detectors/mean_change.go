// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package detectors

import (
	"fmt"
	"math"

	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
)

// MeanChangeDetector detects significant changes in the rate of change of a metric
// using a delta-first approach with a sliding window. It transforms raw values to
// deltas before analysis, which avoids false positives from monotonic counters while
// still detecting spikes in gauges and rate changes in counters.
type MeanChangeDetector struct {
	// Threshold is the minimum change in mean (as multiple of stddev) to trigger.
	Threshold float64 // default: 2.0

	// RecentWindowSize is the number of most recent points to compare.
	RecentWindowSize int // default: 5

	// BaselineWindowSize is the number of historical points to use as baseline.
	BaselineWindowSize int // default: 10
}

// NewMeanChangeDetector creates a new mean change detector with default parameters.
func NewMeanChangeDetector() *MeanChangeDetector {
	return &MeanChangeDetector{
		Threshold:          2.0,
		RecentWindowSize:   5,
		BaselineWindowSize: 10,
	}
}

// Name returns the detector's unique identifier.
func (d *MeanChangeDetector) Name() string {
	return "mean_change"
}

// Analyze examines a series and detects significant changes in the rate of change
// using a delta-first approach with a sliding window.
//
// This method transforms values to deltas (rate of change) before analysis, which:
//   - Naturally handles monotonic counters (constant rate = low variance in deltas)
//   - Detects spikes in both gauges and counters (sudden rate changes)
//   - Avoids false positives from normal counter behavior
func (d *MeanChangeDetector) Analyze(key mh.SeriesKey, history *mh.MetricHistory) []mh.Anomaly {
	// Get the mean values from the Recent tier
	points := history.Recent.ToSlice()
	// We need +1 extra point to compute deltas
	minRequired := d.BaselineWindowSize + d.RecentWindowSize + 1
	if len(points) < minRequired {
		return nil // not enough data
	}

	// Extract means
	means := make([]float64, len(points))
	for i, p := range points {
		means[i] = p.Stats.Mean()
	}

	// Compute deltas (rate of change) from consecutive values
	// deltas[i] = means[i+1] - means[i]
	// This transforms monotonic counters to their rate, which should be stable
	deltas := make([]float64, len(means)-1)
	for i := 0; i < len(means)-1; i++ {
		deltas[i] = means[i+1] - means[i]
	}

	// Sliding window approach on deltas: compare recent window against baseline window
	// Baseline: the deltas before the recent window
	// Recent: the last N deltas
	recentStart := len(deltas) - d.RecentWindowSize
	baselineEnd := recentStart
	baselineStart := baselineEnd - d.BaselineWindowSize

	// Ensure we have valid indices
	if baselineStart < 0 {
		baselineStart = 0
	}

	baselineMean, baselineStd := meanAndStd(deltas[baselineStart:baselineEnd])
	recentMean, recentStd := meanAndStd(deltas[recentStart:])

	// Also compute mean of absolute deltas to detect spikes that return to baseline
	// (e.g., load goes 2->10->2, deltas are [+8, -8], mean=0 but |mean|=8)
	baselineAbsMean := meanAbs(deltas[baselineStart:baselineEnd])
	recentAbsMean := meanAbs(deltas[recentStart:])

	// Check if the change is significant
	// If baseline has zero variance, the metric is perfectly stable.
	// We use a minimum stddev floor based on the magnitude of the baseline mean,
	// or a small absolute value, whichever is larger.
	minStd := math.Max(math.Abs(baselineMean)*0.1, 0.001)
	if baselineStd < minStd {
		baselineStd = minStd
	}

	// Calculate change score for mean shift
	meanChange := math.Abs(recentMean-baselineMean) / baselineStd

	// Calculate change score for volatility increase (detects spikes that return to baseline)
	// Use a floor for baselineAbsMean to avoid division issues with perfectly stable metrics
	absFloor := math.Max(baselineAbsMean, 0.001)
	volatilityChange := recentAbsMean / absFloor

	// Skip alerting if the absolute change is negligible
	absChange := math.Abs(recentMean - baselineMean)
	relChange := absChange / math.Max(math.Abs(baselineMean), 0.001)
	negligibleMeanChange := relChange < 0.01 && absChange < 0.001
	negligibleVolatilityChange := recentAbsMean < 0.001 || volatilityChange < 2.0

	if negligibleMeanChange && negligibleVolatilityChange {
		return nil // change is too small to be meaningful
	}

	// Use the timestamp of the first point in the recent window
	// Note: recentStart is in delta space, which corresponds to points[recentStart+1]
	timestamp := points[recentStart+1].Timestamp

	// Check for mean shift (sustained change)
	if meanChange >= d.Threshold && !negligibleMeanChange {
		changeType := "increase"
		if recentMean < baselineMean {
			changeType = "decrease"
		}

		return []mh.Anomaly{{
			SeriesKey:    key,
			DetectorName: d.Name(),
			Timestamp:    timestamp,
			Type:         "changepoint",
			Severity:     math.Min(meanChange/5.0, 1.0),
			Message: fmt.Sprintf("Significant rate of change %s detected: %.2f/interval -> %.2f/interval (%.1f stddev)",
				changeType, baselineMean, recentMean, meanChange),
		}}
	}

	// Check for volatility spike (transient spike that returns to baseline)
	// This catches cases where mean delta is ~0 but there was significant movement
	if volatilityChange >= d.Threshold && recentAbsMean > baselineAbsMean+baselineStd*d.Threshold {
		return []mh.Anomaly{{
			SeriesKey:    key,
			DetectorName: d.Name(),
			Timestamp:    timestamp,
			Type:         "spike",
			Severity:     math.Min(volatilityChange/5.0, 1.0),
			Message: fmt.Sprintf("Volatility spike detected: avg |delta| %.2f/interval -> %.2f/interval (%.1fx increase, stddev %.2f -> %.2f)",
				baselineAbsMean, recentAbsMean, volatilityChange, baselineStd, recentStd),
		}}
	}

	return nil
}

// meanAbs calculates the mean of absolute values.
func meanAbs(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += math.Abs(v)
	}
	return sum / float64(len(values))
}

// meanAndStd calculates mean and standard deviation of values.
func meanAndStd(values []float64) (mean, std float64) {
	if len(values) == 0 {
		return 0, 0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))

	// Calculate standard deviation
	sumSquaredDiff := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}
	std = math.Sqrt(sumSquaredDiff / float64(len(values)))

	return mean, std
}
