// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

var (
	// OTLPIngestAgentTracesEvents is COAT metric for tracking OTLP trace events (spans) in the agent.
	OTLPIngestAgentTracesEvents coretelemetry.Counter
	// OTLPIngestAgentTracesRequests is Coat metric for tracking OTLP trace requests in the agent.
	OTLPIngestAgentTracesRequests coretelemetry.Counter
)

func init() {
	// We default to a noop implementation, but override this for the trace agent binary in cmd/trace-agent/subcommands/run/command.go
	InitTelemetry(nil)
}

// InitTelemetry wires the COAT counters using the provided telemetry component.
// Passing nil falls back to a noop component so callers can safely invoke the counters before initialization.
func InitTelemetry(tm coretelemetry.Component) {
	if tm == nil {
		tm = nooptelemetry.GetCompatComponent()
	}

	OTLPIngestAgentTracesEvents = tm.NewCounter(
		"runtime",
		"datadog_agent_otlp_traces_events",
		[]string{},
		"Counter metric of OTLP Trace events in OTLP ingestion with the Datadog agent",
	)
	OTLPIngestAgentTracesRequests = tm.NewCounter(
		"runtime",
		"datadog_agent_otlp_traces_requests",
		[]string{},
		"Counter metric of OTLP Trace requests in OTLP ingestion with the Datadog agent",
	)
}
