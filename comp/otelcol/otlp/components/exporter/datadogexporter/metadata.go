package datadogexporter

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var Type = component.MustNewType("datadog")

const (
	LogsStability    = component.StabilityLevelBeta
	TracesStability  = component.StabilityLevelBeta
	MetricsStability = component.StabilityLevelBeta
)

func Meter(settings component.TelemetrySettings) metric.Meter {
	return settings.MeterProvider.Meter("otel-agent/datadog")
}

func Tracer(settings component.TelemetrySettings) trace.Tracer {
	return settings.TracerProvider.Tracer("otel-agent/datadog")
}
