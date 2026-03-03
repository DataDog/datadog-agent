// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logsagentexporter

import (
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

// These COAT metrics are for measuring the current volume of traffic for OTLP Log ingestion in the Agent/DDOT.
var (
	OTLPIngestAgentLogsEvents   coretelemetry.Counter
	OTLPIngestAgentLogsRequests coretelemetry.Counter
	OTLPIngestDDOTLogsEvents    coretelemetry.Counter
	OTLPIngestDDOTLogsRequests  coretelemetry.Counter
)

func init() {
	InitTelemetry(nil)
}

// InitTelemetry wires the OTLP logs ingestion counters with the provided telemetry component.
// Passing nil falls back to a noop component so callers can safely invoke the counters before initialization.
func InitTelemetry(tm coretelemetry.Component) {
	if tm == nil {
		tm = nooptelemetry.GetCompatComponent()
	}

	OTLPIngestAgentLogsEvents = tm.NewCounter(
		"runtime",
		"datadog_agent_otlp_logs_events",
		[]string{},
		"Counter metric of OTLP Log events in OTLP ingestion for the Datadog agent",
	)
	OTLPIngestAgentLogsRequests = tm.NewCounter(
		"runtime",
		"datadog_agent_otlp_logs_requests",
		[]string{},
		"Counter metric of OTLP Log requests in OTLP ingestion for the Datadog agent",
	)
	OTLPIngestDDOTLogsEvents = tm.NewCounter(
		"runtime",
		"ddot_otlp_logs_events",
		[]string{},
		"Counter metric of OTLP Log events in OTLP ingestion for DDOT",
	)
	OTLPIngestDDOTLogsRequests = tm.NewCounter(
		"runtime",
		"ddot_otlp_logs_requests",
		[]string{},
		"Counter metric of OTLP Log requests in OTLP ingestion for DDOT",
	)
}
