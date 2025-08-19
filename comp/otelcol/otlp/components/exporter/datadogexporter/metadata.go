// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package datadogexporter

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Type is the type of the exporter
var Type = component.MustNewType("datadog")

const (
	// LogsStability is the stability level of the logs datatype.
	LogsStability = component.StabilityLevelBeta
	// TracesStability is the stability level of the traces datatype.
	TracesStability = component.StabilityLevelBeta
	// MetricsStability is the stability level of the metrics datatype.
	MetricsStability = component.StabilityLevelBeta
)

// Meter returns a new metric.Meter for the exporter.
func Meter(settings component.TelemetrySettings) metric.Meter {
	return settings.MeterProvider.Meter("otel-agent/datadog")
}

// Tracer returns a new trace.Tracer for the exporter.
func Tracer(settings component.TelemetrySettings) trace.Tracer {
	return settings.TracerProvider.Tracer("otel-agent/datadog")
}
