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
	// RRCF detector
	telemetryRRCFScore     = "observer.rrcf.score"
	telemetryRRCFThreshold = "observer.rrcf.threshold"

	// Log pattern extractor
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
	gauges[telemetryLogPatternExtractorPatternCount] = telemetryComp.NewGauge(
		"observer",
		telemetryLogPatternExtractorPatternCount,
		[]string{"detector"},
		"Log pattern extractor pattern count",
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

// newTelemetryGauge creates a new telemetry gauge for the given telemetry name, detector name, and data time.
// Warning: use a `telemetryName` that is defined in this file.
func newTelemetryGauge(detectorName string, telemetryName string, value float64, dataTimeSec int64) observerdef.ObserverTelemetry {
	return observerdef.ObserverTelemetry{
		DetectorName: detectorName,
		Kind:         observerdef.MetricKindGauge,
		Metric: &metricObs{
			name:      telemetryName,
			value:     value,
			tags:      []string{"detector:" + detectorName},
			timestamp: dataTimeSec,
		},
	}
}

// newTelemetryCounter creates counter telemetry: the value is added to the named counter (must be registered in newTelemetryHandler).
func newTelemetryCounter(detectorName string, telemetryName string, value float64, dataTimeSec int64) observerdef.ObserverTelemetry {
	return observerdef.ObserverTelemetry{
		DetectorName: detectorName,
		Kind:         observerdef.MetricKindCounter,
		Metric: &metricObs{
			name:      telemetryName,
			value:     value,
			tags:      []string{"detector:" + detectorName},
			timestamp: dataTimeSec,
		},
	}
}
