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
	Name string `json:"name"`
	// Kind is the component kind: "detector", "correlator", or "extractor".
	Kind     string  `json:"kind"`
	Count    int     `json:"count"`
	AvgNs    float64 `json:"avg_ns"`
	MedianNs float64 `json:"median_ns"`
	P99Ns    float64 `json:"p99_ns"`
	TotalNs  float64 `json:"total_ns"` // sum of all individual call durations
}

// ReplayStats aggregates all statistics produced during a replay run.
type ReplayStats struct {
	// DetectorStats holds per-detector processing-time statistics keyed by detector name.
	DetectorStats map[string]DetectorProcessingStats `json:"detector_stats,omitempty"`
	// InputMetricsCount is the total number of metric data points (samples) in the scenario.
	InputMetricsCount int `json:"input_metrics_count"`
	// InputMetricsCardinality is the number of unique metric series (name + tag combinations).
	InputMetricsCardinality int `json:"input_metrics_cardinality"`
	// InputLogsCount is the number of raw log entries present in the scenario.
	InputLogsCount int `json:"input_logs_count"`
	// InputAnomaliesCount is the total number of anomalies produced by detectors,
	// which is the input volume processed by correlators.
	InputAnomaliesCount int `json:"input_anomalies_count"`
}

// enrichDetectorStatsKind sets the Kind field on each DetectorProcessingStats entry.
// It builds a reverse map from instance.Name() → kind, because a component's runtime
// Name() (e.g. "bocpd_detector") may differ from its catalog key (e.g. "bocpd").
func enrichDetectorStatsKind(stats map[string]DetectorProcessingStats, components map[string]*componentInstance) {
	kindStr := map[componentKind]string{
		componentDetector:   "detector",
		componentCorrelator: "correlator",
		componentExtractor:  "extractor",
	}
	type namer interface{ Name() string }
	nameToKind := make(map[string]string, len(components))
	for _, ci := range components {
		if n, ok := ci.instance.(namer); ok {
			nameToKind[n.Name()] = kindStr[ci.entry.kind]
		}
	}
	for name, s := range stats {
		if kind, ok := nameToKind[name]; ok {
			s.Kind = kind
			stats[name] = s
		}
	}
}

// sumTelemetryCounter returns the total value of all counter telemetry events
// matching the given metric name.
func sumTelemetryCounter(telemetry []observerdef.ObserverTelemetry, name string) int {
	total := 0.0
	for _, t := range telemetry {
		if t.Kind != observerdef.MetricKindCounter || t.Metric == nil {
			continue
		}
		if t.Metric.GetName() == name {
			total += t.Metric.GetValue()
		}
	}
	return int(total)
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
