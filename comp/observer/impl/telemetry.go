// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Observer
	telemetryDetectorProcessingTimeNs = "observer.detector.processing_time_ns"
	// telemetryObsChannelDropped counts observations silently dropped because
	// the internal observation channel was full. Tagged by source so per-pipeline
	// pressure is visible (e.g. "system-checks-hf" vs "all-metrics").
	telemetryObsChannelDropped = "observer.channel.dropped"
	// Only in testbench
	telemetryTbInputMetricsCount       = "observer.input_metrics.count"
	telemetryTbInputMetricsCardinality = "observer.input_metrics.cardinality"
	telemetryTbInputLogsCount          = "observer.input_logs.count"

	// RRCF detector
	telemetryRRCFScore     = "observer.rrcf.score"
	telemetryRRCFThreshold = "observer.rrcf.threshold"

	// Log pattern extractor — counter: delta (new clusters) per processed log
	telemetryLogPatternExtractorPatternCount = "observer.log_pattern_extractor.pattern_count"
)

// This is used to:
// 1. Enumerate all the telemetry metrics that are emitted by the observer
// 2. Send them to the backend if we are running in observer mode (not testbench)
type telemetryHandler struct {
	telemetryGauges   map[string]telemetry.Gauge
	telemetryCounters map[string]telemetry.Counter
}

func newTelemetryHandler(telemetryComp telemetry.Component) *telemetryHandler {
	gauges := make(map[string]telemetry.Gauge)
	counters := make(map[string]telemetry.Counter)

	// Gauges
	gauges[telemetryDetectorProcessingTimeNs] = telemetryComp.NewGauge(
		"observer",
		telemetryDetectorProcessingTimeNs,
		[]string{"detector"},
		"Per detector processing time in nanoseconds (doesn't include storage time)",
	)
	gauges[telemetryRRCFScore] = telemetryComp.NewGauge(
		"observer",
		telemetryRRCFScore,
		[]string{"detector"},
		"RRCF CoDisp score per scored shingle",
	)
	gauges[telemetryRRCFThreshold] = telemetryComp.NewGauge(
		"observer",
		telemetryRRCFThreshold,
		[]string{"detector"},
		"RRCF dynamic anomaly detection threshold (post-warmup)",
	)

	// Counters
	counters[telemetryTbInputMetricsCount] = telemetryComp.NewCounter(
		"observer",
		telemetryTbInputMetricsCount,
		[]string{},
		"Total number of metrics processed by the observer",
	)
	counters[telemetryTbInputLogsCount] = telemetryComp.NewCounter(
		"observer",
		telemetryTbInputLogsCount,
		[]string{},
		"Total number of logs processed by the observer",
	)
	counters[telemetryTbInputMetricsCardinality] = telemetryComp.NewCounter(
		"observer",
		telemetryTbInputMetricsCardinality,
		[]string{},
		"Total number of unique metrics processed by the observer (metrics with different tags are counted as different metrics)",
	)
	counters[telemetryLogPatternExtractorPatternCount] = telemetryComp.NewCounter(
		"observer",
		telemetryLogPatternExtractorPatternCount,
		[]string{"detector"},
		"Log pattern extractor new clusters added per processed log",
	)
	counters[telemetryObsChannelDropped] = telemetryComp.NewCounter(
		"observer",
		telemetryObsChannelDropped,
		[]string{"source"},
		"Observations dropped because the internal channel was full, tagged by source handle",
	)

	return &telemetryHandler{
		telemetryGauges:   gauges,
		telemetryCounters: counters,
	}
}

// handleTelemetry handles the telemetry events by sending them to the telemetry backend.
// Note 1: we don't forward the exact timestamp for each telemetry event but this is not a problem
// because we use this only in the observer with realtime data
// Note 2: we don't send logs to the backend
func (h *telemetryHandler) handleTelemetry(events []observerdef.ObserverTelemetry) {
	for _, event := range events {
		if event.Metric == nil {
			continue
		}
		name := event.Metric.GetName()
		tags := event.Metric.GetRawTags()
		value := event.Metric.GetValue()

		switch event.Kind {
		case observerdef.MetricKindCounter:
			counter, ok := h.telemetryCounters[name]
			if ok {
				counter.Add(value, tags...)
				continue
			}
			pkglog.Warnf("[observer] telemetry counter not found: %s", name)
		default:
			gauge, ok := h.telemetryGauges[name]
			if ok {
				gauge.Set(value, tags...)
				continue
			}
			pkglog.Warnf("[observer] telemetry gauge not found: %s", name)
		}
	}
}

// isMetricRegistered checks if a metric is registered in the telemetry handler
func (h *telemetryHandler) isMetricRegistered(metricName string) bool {
	if _, ok := h.telemetryGauges[metricName]; ok {
		return true
	}
	_, ok := h.telemetryCounters[metricName]
	return ok
}

// isCounterMetric reports whether the storage metric name (no :agg suffix) is a registered counter.
func (h *telemetryHandler) isCounterMetric(name string) bool {
	if h == nil {
		return false
	}
	_, ok := h.telemetryCounters[name]
	return ok
}

// detectorNameFromTags returns the value of the first "detector:" tag, for ObserverTelemetry.DetectorName.
func detectorNameFromTags(tags []string) string {
	const prefix = "detector:"
	for _, t := range tags {
		if len(t) > len(prefix) && t[:len(prefix)] == prefix {
			return t[len(prefix):]
		}
	}
	return ""
}

// newTelemetryGauge creates gauge telemetry for the given metric name, tags, value, and data time (unix seconds).
// Warning: use a `telemetryName` that is defined in this file.
func newTelemetryGauge(tags []string, telemetryName string, value float64, dataTimeSec int64) observerdef.ObserverTelemetry {
	tagsCopy := copyTags(tags)
	return observerdef.ObserverTelemetry{
		DetectorName: detectorNameFromTags(tagsCopy),
		Kind:         observerdef.MetricKindGauge,
		Metric: &metricObs{
			name:      telemetryName,
			value:     value,
			tags:      tagsCopy,
			timestamp: dataTimeSec,
		},
	}
}

// newTelemetryCounter creates counter telemetry: the value is added to the named counter (must be registered in newTelemetryHandler).
func newTelemetryCounter(tags []string, telemetryName string, value float64, dataTimeSec int64) observerdef.ObserverTelemetry {
	tagsCopy := copyTags(tags)
	return observerdef.ObserverTelemetry{
		DetectorName: detectorNameFromTags(tagsCopy),
		Kind:         observerdef.MetricKindCounter,
		Metric: &metricObs{
			name:      telemetryName,
			value:     value,
			tags:      tagsCopy,
			timestamp: dataTimeSec,
		},
	}
}
