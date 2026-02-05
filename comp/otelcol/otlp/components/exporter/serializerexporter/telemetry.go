// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

// These metrics are for measuring the current volume of traffic for OTLP Metric ingestion.
var (
	OTLPIngestAgentMetricsEvents   coretelemetry.Counter
	OTLPIngestAgentMetricsRequests coretelemetry.Counter
	OTLPIngestDDOTMetricsEvents    coretelemetry.Counter
	OTLPIngestDDOTMetricsRequests  coretelemetry.Counter
)

func init() {
	InitTelemetry(nil)
}

// InitTelemetry wires the OTLP metrics ingestion counters with the provided telemetry component.
// Passing nil falls back to a noop component so callers can safely invoke the counters before initialization.
func InitTelemetry(tm coretelemetry.Component) {
	if tm == nil {
		tm = nooptelemetry.GetCompatComponent()
	}

	OTLPIngestAgentMetricsEvents = tm.NewCounter(
		"runtime",
		"datadog_agent_otlp_metrics_events",
		[]string{},
		"Counter metric of OTLP Metric events in OTLP ingestion for the Datadog agent",
	)
	OTLPIngestAgentMetricsRequests = tm.NewCounter(
		"runtime",
		"datadog_agent_otlp_metrics_requests",
		[]string{},
		"Counter metric of OTLP Metric requests in OTLP ingestion for the Datadog agent",
	)
	OTLPIngestDDOTMetricsEvents = tm.NewCounter(
		"runtime",
		"ddot_otlp_metrics_events",
		[]string{},
		"Counter metric of OTLP Metric events in OTLP ingestion for DDOT",
	)
	OTLPIngestDDOTMetricsRequests = tm.NewCounter(
		"runtime",
		"ddot_otlp_metrics_requests",
		[]string{},
		"Counter metric of OTLP Metric requests in OTLP ingestion for DDOT",
	)
}
