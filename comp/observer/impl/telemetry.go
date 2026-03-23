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
)

// This is used to:
// 1. Enumerate all the telemetry metrics that are emitted by the observer
// 2. Send them to the backend if we are running in observer mode (not testbench)
type telemetryHandler struct {
	telemetryGauges map[string]telemetry.Gauge
}

func newTelemetryHandler(telemetryComp telemetry.Component) *telemetryHandler {
	gauges := make(map[string]telemetry.Gauge)

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

	return &telemetryHandler{
		telemetryGauges: gauges,
	}
}

// handleTelemetry handles the telemetry events by sending them to the telemetry backend.
// Note 1: we don't forward the exact timestamp for each telemetry event but this is not a problem
// because we use this only in the observer with realtime data
// Note 2: we don't send logs to the backend
func (h *telemetryHandler) handleTelemetry(events []observerdef.ObserverTelemetry) {
	for _, event := range events {
		if event.Metric != nil {
			gauge, isGauge := h.telemetryGauges[event.Metric.GetName()]
			if isGauge {
				gauge.Set(event.Metric.GetValue(), event.Metric.GetRawTags()...)
				continue
			}

			pkglog.Warnf("[observer] telemetry gauge not found: %s", event.Metric.GetName())
		}
	}
}

// isMetricRegistered checks if a metric is registered in the telemetry handler
func (h *telemetryHandler) isMetricRegistered(metricName string) bool {
	_, isRegistered := h.telemetryGauges[metricName]

	return isRegistered
}

// newTelemetryGauge creates a new telemetry gauge for the given telemetry name, detector name, and data time.
// Warning: use a `telemetryName` that is defined in this file.
func newTelemetryGauge(detectorName string, telemetryName string, value float64, dataTime int64) observerdef.ObserverTelemetry {
	return observerdef.ObserverTelemetry{
		DetectorName: detectorName,
		Metric: &metricObs{
			name:      telemetryName,
			value:     value,
			tags:      []string{"detector:" + detectorName},
			timestamp: dataTime,
		},
	}
}
