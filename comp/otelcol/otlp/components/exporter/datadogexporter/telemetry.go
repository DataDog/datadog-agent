// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogexporter

import (
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

// These metrics are for measuring the current volume of traffic for OTLP trace ingestion.
var (
	OTLPIngestDDOTTracesEvents   coretelemetry.Counter
	OTLPIngestDDOTTracesRequests coretelemetry.Counter
)

func init() {
	InitTelemetry(nil)
}

// InitTelemetry wires COAT counters for Datadog exporter OTLP trace ingestion.
// Passing nil falls back to a noop component so callers can safely invoke the counters before initialization.
func InitTelemetry(tm coretelemetry.Component) {
	if tm == nil {
		tm = nooptelemetry.GetCompatComponent()
	}

	OTLPIngestDDOTTracesEvents = tm.NewCounter(
		"runtime",
		"ddot_otlp_traces_events",
		[]string{},
		"Counter metric of OTLP Trace events in OTLP ingestion using DDOT",
	)
	OTLPIngestDDOTTracesRequests = tm.NewCounter(
		"runtime",
		"ddot_otlp_traces_requests",
		[]string{},
		"Counter metric of OTLP Trace requests in OTLP ingestion using DDOT",
	)
}
