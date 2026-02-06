// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import "sort"

// TracePercentiles contains percentile values for trace durations
type TracePercentiles struct {
	P50 int64 // 50th percentile (median)
	P95 int64 // 95th percentile
	P99 int64 // 99th percentile
}

// TracePercentileCalculator calculates percentiles from trace durations
type TracePercentileCalculator struct{}

// NewTracePercentileCalculator creates a new TracePercentileCalculator
func NewTracePercentileCalculator() *TracePercentileCalculator {
	return &TracePercentileCalculator{}
}

// CalculatePercentiles takes a slice of traces and returns P50, P95, and P99 percentiles
func (c *TracePercentileCalculator) CalculatePercentiles(traces []*traceObs) *TracePercentiles {
	if len(traces) == 0 {
		return &TracePercentiles{P50: 0, P95: 0, P99: 0}
	}

	// Extract durations
	durations := make([]int64, 0, len(traces))
	for _, trace := range traces {
		if trace != nil {
			durations = append(durations, trace.duration)
		}
	}

	if len(durations) == 0 {
		return &TracePercentiles{P50: 0, P95: 0, P99: 0}
	}

	// Sort durations in ascending order
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate percentiles
	return &TracePercentiles{
		P50: percentile(durations, 50),
		P95: percentile(durations, 95),
		P99: percentile(durations, 99),
	}
}

// percentile calculates the percentile value from a sorted slice
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}

	// Calculate index using linear interpolation
	index := (p / 100.0) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	// Linear interpolation between lower and upper values
	weight := index - float64(lower)
	value := float64(sorted[lower])*(1.0-weight) + float64(sorted[upper])*weight
	return int64(value)
}
