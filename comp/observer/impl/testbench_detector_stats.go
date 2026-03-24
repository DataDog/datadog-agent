// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// DetectorProcessingStats holds aggregate processing-time statistics for a single
// detector (or extractor / correlator) across all advance calls during a replay.
// Times are in nanoseconds.
type DetectorProcessingStats struct {
	Name     string  `json:"name"`
	Count    int     `json:"count"`
	AvgNs    float64 `json:"avg_ns"`
	MedianNs float64 `json:"median_ns"`
	P99Ns    float64 `json:"p99_ns"`
}

// computeDetectorProcessingStats groups telemetry samples for
// telemetryDetectorProcessingTimeNs by detector name and computes
// avg / median / p99 for each.
func computeDetectorProcessingStats(telemetry []observerdef.ObserverTelemetry) map[string]DetectorProcessingStats {
	byDetector := make(map[string][]float64)

	for _, t := range telemetry {
		if t.Metric == nil || t.Metric.GetName() != telemetryDetectorProcessingTimeNs {
			continue
		}
		name := t.DetectorName
		if name == "" {
			continue
		}
		byDetector[name] = append(byDetector[name], t.Metric.GetValue())
	}

	result := make(map[string]DetectorProcessingStats, len(byDetector))
	for name, values := range byDetector {
		result[name] = statsFromSamples(name, values)
	}
	return result
}

func statsFromSamples(name string, values []float64) DetectorProcessingStats {
	if len(values) == 0 {
		return DetectorProcessingStats{Name: name}
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	avg := sum / float64(len(values))

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	return DetectorProcessingStats{
		Name:     name,
		Count:    len(values),
		AvgNs:    avg,
		MedianNs: interpolatedPercentile(sorted, 50),
		P99Ns:    interpolatedPercentile(sorted, 99),
	}
}

// interpolatedPercentile computes a percentile via linear interpolation on a
// sorted slice. p must be in [0, 100].
func interpolatedPercentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	idx := (p / 100.0) * float64(n-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
