// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

const (
	// common prefix used by all options
	optPrefix = "_"

	// OptStatsd designates a metric that should be emitted using statsd
	OptStatsd = "_statsd"

	// OptPrometheus designates a metric that should be emitted using prometheus
	OptPrometheus = "_prometheus"

	// OptPayloadTelemetry designates a metric that should be emitted as agent payload telemetry
	OptPayloadTelemetry = "_payload_telemetry"
)
