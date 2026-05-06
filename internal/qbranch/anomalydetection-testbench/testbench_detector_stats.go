// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"math"
	"sort"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	tboutput "github.com/DataDog/datadog-agent/internal/qbranch/anomalydetection-testbench/output"
)

// enrichDetectorStatsKind sets the Kind field on each DetectorProcessingStats entry.
// It builds a reverse map from instance.Name() → kind, because a component's runtime
// Name() (e.g. "bocpd_detector") may differ from its catalog key (e.g. "bocpd").
func enrichDetectorStatsKind(stats map[string]tboutput.DetectorProcessingStats, components map[string]*observerimpl.ComponentInstance) {
	kindStr := map[observerimpl.ComponentKind]string{
		observerimpl.ComponentDetector:   "detector",
		observerimpl.ComponentCorrelator: "correlator",
		observerimpl.ComponentExtractor:  "extractor",
	}
	type namer interface{ Name() string }
	nameToKind := make(map[string]string, len(components))
	for _, ci := range components {
		if n, ok := ci.Instance().(namer); ok {
			nameToKind[n.Name()] = kindStr[ci.Kind()]
		}
	}
	for name, s := range stats {
		if kind, ok := nameToKind[name]; ok {
			s.Kind = kind
			stats[name] = s
		}
	}
}

// sumStoredTelemetryCounter returns the total value of a telemetry counter metric
// by summing all matching telemetry series in storage.
func sumStoredTelemetryCounter(storage *observerimpl.TimeSeriesStorage, name string) int {
	total := 0.0
	for _, m := range storage.ListSeriesMetadata(observerdef.TelemetryNamespace) {
		if m.Name != name {
			continue
		}
		s := storage.GetSeriesByNumericID(m.Ref, observerimpl.AggregateSum)
		if s == nil {
			continue
		}
		for _, p := range s.Points {
			total += p.Value
		}
	}
	return int(total)
}

// computeDetectorProcessingStats groups telemetry samples for
// TelemetryDetectorProcessingTimeNs by detector name and computes
// avg / median / p99 for each.
func computeDetectorProcessingStatsFromStorage(storage *observerimpl.TimeSeriesStorage) map[string]tboutput.DetectorProcessingStats {
	byDetector := make(map[string][]float64)

	for _, m := range storage.ListSeriesMetadata(observerdef.TelemetryNamespace) {
		if m.Name != observerimpl.TelemetryDetectorProcessingTimeNs {
			continue
		}

		avgSeries := storage.GetSeriesByNumericID(m.Ref, observerimpl.AggregateAverage)
		countSeries := storage.GetSeriesByNumericID(m.Ref, observerimpl.AggregateCount)
		if avgSeries == nil || countSeries == nil {
			continue
		}

		name := observerimpl.DetectorNameFromTags(m.Tags)
		if name == "" {
			continue
		}

		n := len(avgSeries.Points)
		if len(countSeries.Points) < n {
			n = len(countSeries.Points)
		}
		for i := 0; i < n; i++ {
			// Storage keeps avg+count per bucket. Rehydrate bucket observations
			// as repeated avg values to compute summary percentiles consistently.
			sampleCount := int(math.Round(countSeries.Points[i].Value))
			if sampleCount <= 0 {
				continue
			}
			v := avgSeries.Points[i].Value
			for j := 0; j < sampleCount; j++ {
				byDetector[name] = append(byDetector[name], v)
			}
		}
	}

	result := make(map[string]tboutput.DetectorProcessingStats, len(byDetector))
	for name, values := range byDetector {
		result[name] = statsFromSamples(name, values)
	}
	return result
}

func statsFromSamples(name string, values []float64) tboutput.DetectorProcessingStats {
	if len(values) == 0 {
		return tboutput.DetectorProcessingStats{Name: name}
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	avg := sum / float64(len(values))

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	return tboutput.DetectorProcessingStats{
		Name:     name,
		Count:    len(values),
		AvgNs:    avg,
		MedianNs: interpolatedPercentile(sorted, 50),
		P99Ns:    interpolatedPercentile(sorted, 99),
		TotalNs:  sum,
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
