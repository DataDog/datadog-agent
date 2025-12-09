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

// MeanChangeDetector detects significant changes in the mean value of a metric.
type MeanChangeDetector struct {
	// Threshold is the minimum change in mean (as multiple of stddev) to trigger.
	Threshold float64 // default: 2.0

	// MinSegmentSize is the minimum number of points before and after the change.
	MinSegmentSize int // default: 5
}

// NewMeanChangeDetector creates a new mean change detector with default parameters.
func NewMeanChangeDetector() *MeanChangeDetector {
	return &MeanChangeDetector{
		Threshold:      2.0,
		MinSegmentSize: 5,
	}
}

// Name returns the detector's unique identifier.
func (d *MeanChangeDetector) Name() string {
	return "mean_change"
}

// Analyze examines a series and detects significant changes in the mean value.
func (d *MeanChangeDetector) Analyze(key mh.SeriesKey, history *mh.MetricHistory) []mh.Anomaly {
	// Get the mean values from the Recent tier
	points := history.Recent.ToSlice()
	if len(points) < d.MinSegmentSize*2 {
		return nil // not enough data
	}

	// Extract means
	means := make([]float64, len(points))
	for i, p := range points {
		means[i] = p.Stats.Mean()
	}

	// Simple approach: compare first half mean to second half mean
	midpoint := len(means) / 2
	firstHalfMean, firstHalfStd := meanAndStd(means[:midpoint])
	secondHalfMean, _ := meanAndStd(means[midpoint:])

	// Check if the change is significant
	if firstHalfStd == 0 {
		firstHalfStd = 1 // avoid division by zero
	}

	change := math.Abs(secondHalfMean-firstHalfMean) / firstHalfStd

	if change >= d.Threshold {
		changeType := "increase"
		if secondHalfMean < firstHalfMean {
			changeType = "decrease"
		}

		return []mh.Anomaly{{
			SeriesKey:    key,
			DetectorName: d.Name(),
			Timestamp:    points[midpoint].Timestamp,
			Type:         "changepoint",
			Severity:     math.Min(change/5.0, 1.0), // normalize to 0-1
			Message: fmt.Sprintf("Significant mean %s detected: %.2f -> %.2f (%.1f stddev)",
				changeType, firstHalfMean, secondHalfMean, change),
		}}
	}

	return nil
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
