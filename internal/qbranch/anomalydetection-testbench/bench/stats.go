// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"math"
	"sort"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
)

const (
	telemetryDetectorProcessingTimeNs  = "observer.detector.processing_time_ns"
	telemetryTbInputLogsCount          = "observer.input_logs.count"
	telemetryTbInputMetricsCount       = "observer.input_metrics.count"
	telemetryTbInputMetricsCardinality = "observer.input_metrics.cardinality"
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
	// InputMetricsCount is the number of accepted parquet metric samples. It
	// excludes virtual metrics derived from logs and filtered/dropped samples.
	InputMetricsCount int64 `json:"input_metrics_count"`
	// InputMetricsCardinality is the number of unique accepted parquet metric
	// series (name + tag combinations), excluding log-derived series. It is
	// estimated with fixed memory above 65,536 unique series.
	InputMetricsCardinality int `json:"input_metrics_cardinality"`
	// InputLogsCount is the number of raw log entries present in the scenario.
	InputLogsCount int `json:"input_logs_count"`
	// InputAnomaliesCount is the total number of anomalies produced by detectors,
	// which is the input volume processed by correlators.
	InputAnomaliesCount int `json:"input_anomalies_count"`
}

// enrichDetectorStatsKind sets the Kind field on each DetectorProcessingStats entry
// using the catalog entries to map component names to kinds.
func enrichDetectorStatsKind(stats map[string]DetectorProcessingStats, entries []observerimpl.CatalogEntry) {
	nameToKind := make(map[string]string, len(entries))
	for _, e := range entries {
		nameToKind[e.Name] = e.Kind
	}
	for name, s := range stats {
		if kind, ok := nameToKind[name]; ok {
			s.Kind = kind
			stats[name] = s
		}
	}
}

// sumStoredTelemetryCounter returns the total value of a telemetry counter metric
// by summing all matching telemetry series from a StateView.
func sumStoredTelemetryCounter(sv observerimpl.StateView, name string) int {
	total := 0.0
	series := sv.ListSeries(observerdef.SeriesFilter{Namespace: observerdef.TelemetryNamespace})
	maxTs := sv.MaxTimestamp()
	for _, m := range series {
		if m.Name != name {
			continue
		}
		s := sv.GetSeriesRange(m.Ref, 0, maxTs, observerdef.AggregateSum)
		if s == nil {
			continue
		}
		for _, p := range s.Points {
			total += p.Value
		}
	}
	return int(total)
}

// detectorNameFromTags extracts the detector name from a "detector:xxx" tag.
func detectorNameFromTags(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, "detector:") {
			return strings.TrimPrefix(t, "detector:")
		}
	}
	return ""
}

// computeDetectorProcessingStatsFromStateView groups telemetry samples for
// telemetryDetectorProcessingTimeNs by detector name and computes
// avg / median / p99 for each.
func computeDetectorProcessingStatsFromStateView(sv observerimpl.StateView) map[string]DetectorProcessingStats {
	byDetector := make(map[string][]float64)

	series := sv.ListSeries(observerdef.SeriesFilter{Namespace: observerdef.TelemetryNamespace})
	maxTs := sv.MaxTimestamp()

	for _, m := range series {
		if m.Name != telemetryDetectorProcessingTimeNs {
			continue
		}

		avgSeries := sv.GetSeriesRange(m.Ref, 0, maxTs, observerdef.AggregateAverage)
		countSeries := sv.GetSeriesRange(m.Ref, 0, maxTs, observerdef.AggregateCount)
		if avgSeries == nil || countSeries == nil {
			continue
		}

		name := detectorNameFromTags(m.Tags)
		if name == "" {
			continue
		}

		n := len(avgSeries.Points)
		if len(countSeries.Points) < n {
			n = len(countSeries.Points)
		}
		for i := 0; i < n; i++ {
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
