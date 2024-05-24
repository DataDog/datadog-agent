// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	// Type for tag enrichment processor.
	Type = component.MustNewType("infraattributes")
)

const (
	// TracesStability - stability level for traces.
	TracesStability = component.StabilityLevelAlpha
	// MetricsStability - stability level for metrics.
	MetricsStability = component.StabilityLevelAlpha
	// LogsStability - stability level for logs.
	LogsStability = component.StabilityLevelAlpha
)

// Meter for tag enrichement.
func Meter(settings component.TelemetrySettings) metric.Meter {
	return settings.MeterProvider.Meter("otelcol/infraattributes")
}

// Tracer for tag enrichment.
func Tracer(settings component.TelemetrySettings) trace.Tracer {
	return settings.TracerProvider.Tracer("otelcol/infraattributes")
}
