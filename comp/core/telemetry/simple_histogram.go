// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

// HistogramValue is a struct representing the internal histogram state
type HistogramValue struct {
	Count   uint64
	Sum     float64
	Buckets []Bucket
}

// Bucket is a struct representing the internal bucket state
type Bucket struct {
	UpperBound float64
	Count      uint64
}

// SimpleHistogram tracks how many times something is happening.
type SimpleHistogram interface {
	// Observe the value to the Histogram value.
	Observe(value float64)

	// Get gets the current histogram values
	Get() HistogramValue
}
