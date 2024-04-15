// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"math"
)

// SimpleHistogram tracks how many times something is happening.
type SimpleHistogram interface {
	telemetryComponent.SimpleHistogram
}

// NewSimpleHistogram creates a new SimpleHistogram with default options.
func NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram {
	return NewSimpleHistogramWithOpts(subsystem, name, help, buckets, DefaultOptions)
}

// NewSimpleHistogramWithOpts creates a new SimpleHistogram.
func NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram {
	return telemetryComponent.GetCompatComponent().NewSimpleHistogramWithOpts(subsystem, name, help, buckets, telemetryComponent.Options(opts))
}

// Function to calculate mean
func mean(data []telemetryComponent.Bucket) float64 {
	sum := 0.0
	for _, value := range data {
		sum += float64(value.Count)
	}
	return sum / float64(len(data))
}

// Function to calculate standard deviation
func stdDev(data []telemetryComponent.Bucket, mean float64) float64 {
	sum := 0.0
	for _, value := range data {
		sum += (float64(value.Count) - mean) * (float64(value.Count) - mean)
	}
	variance := sum / float64(len(data)-1)
	return math.Sqrt(variance)
}

// GetSkew gets the skew of the histogram with the given tags.
// Highly experimental.
func GetSkew(histogram Histogram, tags ...string) float64 {
	taggedHist := histogram.WithValues(tags...)
	hist := taggedHist.Get()
	if hist.Count == 0 {
		return 0
	}

	n := float64(hist.Count)
	m := mean(hist.Buckets)
	s := stdDev(hist.Buckets, m)
	sum := 0.0
	for _, value := range hist.Buckets {
		sum += math.Pow((float64(value.Count)-m)/s, 3)
	}

	return (n / ((n - 1) * (n - 2))) * sum
}
