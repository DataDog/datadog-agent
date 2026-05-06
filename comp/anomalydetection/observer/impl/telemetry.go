// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Observer
	TelemetryDetectorProcessingTimeNs = "observer.detector.processing_time_ns"
	// telemetryObsChannelDropped counts observations silently dropped because
	// the internal observation channel was full. Tagged by source so per-pipeline
	// pressure is visible (e.g. "system-checks-hf" vs "all-metrics").
	telemetryObsChannelDropped = "observer.channel.dropped"
	// Only in testbench
	TelemetryTbInputMetricsCount       = "observer.input_metrics.count"
	TelemetryTbInputMetricsCardinality = "observer.input_metrics.cardinality"
	TelemetryTbInputLogsCount          = "observer.input_logs.count"

	// RRCF detector
	telemetryRRCFScore     = "observer.rrcf.score"
	telemetryRRCFThreshold = "observer.rrcf.threshold"

	// Log pattern extractor — counter: delta (new clusters) per processed log
	telemetryLogPatternExtractorPatternCount = "observer.log_pattern_extractor.pattern_count"
)

// This is used to:
// 1. Enumerate all the telemetry metrics that are emitted by the observer
// 2. Send them to the backend if we are running in observer mode (not testbench)
type TelemetryHandler struct {
	telemetryGauges   map[string]telemetry.Gauge
	telemetryCounters map[string]telemetry.Counter
}

func NewTelemetryHandler(telemetryComp telemetry.Component) *TelemetryHandler {
	gauges := make(map[string]telemetry.Gauge)
	counters := make(map[string]telemetry.Counter)

	// Gauges
	gauges[TelemetryDetectorProcessingTimeNs] = telemetryComp.NewGauge(
		"observer",
		TelemetryDetectorProcessingTimeNs,
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
	counters[TelemetryTbInputMetricsCount] = telemetryComp.NewCounter(
		"observer",
		TelemetryTbInputMetricsCount,
		[]string{},
		"Total number of metrics processed by the observer",
	)
	counters[TelemetryTbInputLogsCount] = telemetryComp.NewCounter(
		"observer",
		TelemetryTbInputLogsCount,
		[]string{},
		"Total number of logs processed by the observer",
	)
	counters[TelemetryTbInputMetricsCardinality] = telemetryComp.NewCounter(
		"observer",
		TelemetryTbInputMetricsCardinality,
		[]string{},
		"Total number of unique metrics processed by the observer (metrics with different tags are counted as different metrics)",
	)
	counters[telemetryLogPatternExtractorPatternCount] = telemetryComp.NewCounter(
		"observer",
		telemetryLogPatternExtractorPatternCount,
		[]string{"detector"},
		"Log pattern extractor number of patterns (clusters) that are active (not garbage collected)",
	)
	counters[telemetryObsChannelDropped] = telemetryComp.NewCounter(
		"observer",
		telemetryObsChannelDropped,
		[]string{"source"},
		"Observations dropped because the internal channel was full, tagged by source handle",
	)

	return &TelemetryHandler{
		telemetryGauges:   gauges,
		telemetryCounters: counters,
	}
}

// HandleTelemetry handles the telemetry events by sending them to the telemetry backend.
// Note 1: we don't forward the exact timestamp for each telemetry event but this is not a problem
// because we use this only in the observer with realtime data
// Note 2: we don't send logs to the backend
func (h *TelemetryHandler) HandleTelemetry(events []observerdef.ObserverTelemetry) {
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

// IsMetricRegistered checks if a metric is registered in the telemetry handler
func (h *TelemetryHandler) IsMetricRegistered(metricName string) bool {
	if _, ok := h.telemetryGauges[metricName]; ok {
		return true
	}
	_, ok := h.telemetryCounters[metricName]
	return ok
}

// IsCounterMetric reports whether the storage metric name (no :agg suffix) is a registered counter.
func (h *TelemetryHandler) IsCounterMetric(name string) bool {
	if h == nil {
		return false
	}
	_, ok := h.telemetryCounters[name]
	return ok
}

// DetectorNameFromTags returns the value of the first "detector:" tag, for ObserverTelemetry.DetectorName.
func DetectorNameFromTags(tags []string) string {
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
		DetectorName: DetectorNameFromTags(tagsCopy),
		Kind:         observerdef.MetricKindGauge,
		Metric: &metricObs{
			name:      telemetryName,
			value:     value,
			tags:      tagsCopy,
			timestamp: dataTimeSec,
		},
	}
}

// NewTelemetryCounter creates counter telemetry: the value is added to the named counter (must be registered in NewTelemetryHandler).
func NewTelemetryCounter(tags []string, telemetryName string, value float64, dataTimeSec int64) observerdef.ObserverTelemetry {
	tagsCopy := copyTags(tags)
	return observerdef.ObserverTelemetry{
		DetectorName: DetectorNameFromTags(tagsCopy),
		Kind:         observerdef.MetricKindCounter,
		Metric: &metricObs{
			name:      telemetryName,
			value:     value,
			tags:      tagsCopy,
			timestamp: dataTimeSec,
		},
	}
}
