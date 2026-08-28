// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"testing"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserverTelemetry_NoopsDoNotPanic(_ *testing.T) {
	tel := newObserverTelemetry(noopsimpl.GetCompatComponent())
	tel.recordObservationAccepted("logs", "containers")
	tel.recordObservationDropped("logs", "containers")
	tel.recordRRCFScore("rrcf", 0.7)
	tel.recordRRCFThreshold("rrcf", 0.9)
	tel.setLogPatternCount(1)
	tel.recordLogAccepted("internal", 256)
	tel.recordMetricAccepted("dogstatsd")
	tel.recordFilteredMetric("dogstatsd")
	tel.recordInvalidMetricValueDropped("non_finite")
	tel.incrementLogsInFlight("internal")
	tel.decrementLogsInFlight("internal")
	tel.initLogsInFlight()
	tel.setSeriesCount(42)
	tel.recordStorageSeriesEvicted("capacity", 3)
	tel.recordStorageCapacityHit()
	tel.recordAdvanceSkipped("input")
	tel.recordInputRateLimiterDropped("internal", "high")
	tel.recordDetectorEmission("bocpd", "medium")
	tel.scorerSeverity.Set(2, "anomaly_scorer")
}

func TestObserverTelemetry_EmitsNewMetrics(t *testing.T) {
	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	tel := newObserverTelemetry(telComp)
	tel.recordLogAccepted("kubelet", 128)
	tel.recordMetricAccepted("dogstatsd")
	tel.recordObservationDropped("logs", "containers")
	tel.recordInvalidMetricValueDropped("non_finite")
	tel.recordInvalidMetricValueDropped("non_finite")
	tel.recordInvalidMetricValueDropped("extreme")
	tel.recordInputRateLimiterDropped("internal", "high")
	tel.recordDetectorEmission("bocpd", "medium")
	tel.recordDetectorEmission("bocpd", "medium")
	tel.setLogPatternCount(3)
	tel.scorerSeverity.Set(2, "anomaly_scorer")

	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": "kubelet"}).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryObservationsAccepted, map[string]string{"kind": "metrics", "source": "dogstatsd"}).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryObservationsDropped, map[string]string{"kind": "logs", "source": "containers"}).GetCounter().GetValue())
	assert.Equal(t, 128.0, observerMetric(t, telComp, telemetryLogsAcceptedBytes, map[string]string{"source": "kubelet"}).GetCounter().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryInvalidMetricValuesDropped, map[string]string{"reason": "non_finite"}).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryInvalidMetricValuesDropped, map[string]string{"reason": "extreme"}).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryLogsInputRateLimiterDropped, map[string]string{"source": "internal", "priority": "high"}).GetCounter().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryDetectorEmissions, map[string]string{"detector": "bocpd", "severity": "medium"}).GetCounter().GetValue())
	assert.Equal(t, 3.0, observerMetric(t, telComp, telemetryLogPatternExtractorPatternCount, nil).GetGauge().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryScorerSeverity, map[string]string{"scorer": "anomaly_scorer"}).GetGauge().GetValue())
}

func observerMetric(t *testing.T, telemetryComp telemetry.Component, metricName string, wantLabels map[string]string) *dto.Metric {
	t.Helper()

	metricFamilies, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	fullMetricName := "observer__" + metricName
	for _, family := range metricFamilies {
		if family.GetName() != fullMetricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if len(labels) != len(wantLabels) {
				continue
			}
			matches := true
			for name, value := range wantLabels {
				if labels[name] != value {
					matches = false
					break
				}
			}
			if matches {
				return metric
			}
		}
	}

	t.Fatalf("metric %q with labels %v not found", fullMetricName, wantLabels)
	return nil
}

func TestClassifyLogSource(t *testing.T) {
	require.Equal(t, "internal", classifyLogSource("agent_logs", nil))
	require.Equal(t, "kubelet", classifyLogSource("logs", []string{"source:kubelet", "service:kubelet"}))
	require.Equal(t, "containers", classifyLogSource("logs", []string{"source:docker"}))
}
