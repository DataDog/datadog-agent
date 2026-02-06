// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import "sort"

// TracePercentiles contains percentile values for trace durations
type TracePercentiles struct {
	P50      int64    // 50th percentile (median)
	P95      int64    // 95th percentile
	P99      int64    // 99th percentile
	Services []string // Sorted list of unique services observed
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
		return &TracePercentiles{P50: 0, P95: 0, P99: 0, Services: nil}
	}

	// Extract durations
	durations := make([]int64, 0, len(traces))
	serviceSet := make(map[string]struct{})
	for _, trace := range traces {
		if trace != nil {
			duration := trace.duration
			if duration == 0 && len(trace.spans) > 0 {
				minStart := trace.spans[0].start
				maxEnd := trace.spans[0].start + trace.spans[0].duration
				for i := 1; i < len(trace.spans); i++ {
					span := trace.spans[i]
					start := span.start
					end := span.start + span.duration
					if start < minStart {
						minStart = start
					}
					if end > maxEnd {
						maxEnd = end
					}
				}
				if maxEnd > minStart {
					duration = maxEnd - minStart
				}
			}
			if duration > 0 {
				durations = append(durations, duration)
			}
			if trace.service != "" {
				serviceSet[trace.service] = struct{}{}
			}
			for _, span := range trace.spans {
				if span.service != "" {
					serviceSet[span.service] = struct{}{}
				}
			}
		}
	}

	if len(durations) == 0 {
		return &TracePercentiles{P50: 0, P95: 0, P99: 0, Services: nil}
	}

	services := make([]string, 0, len(serviceSet))
	for service := range serviceSet {
		services = append(services, service)
	}
	sort.Strings(services)

	// Sort durations in ascending order
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate percentiles
	return &TracePercentiles{
		P50:      percentile(durations, 50),
		P95:      percentile(durations, 95),
		P99:      percentile(durations, 99),
		Services: services,
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
