// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package datadogrumreceiver

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel/trace"
)

// Type is the type of the exporter
var Type = component.MustNewType("datadogrum")

const (
	// LogsStability is the stability level of the logs datatype.
	LogsStability = component.StabilityLevelBeta
	// TracesStability is the stability level of the traces datatype.
	TracesStability = component.StabilityLevelBeta
)

// Tracer returns a new trace.Tracer for the exporter.
func Tracer(settings component.TelemetrySettings) trace.Tracer {
	return settings.TracerProvider.Tracer("otel-agent/datadogrum")
}
