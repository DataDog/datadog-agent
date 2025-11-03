// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logsagentexporter

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// These metrics are for measuring the current volume of traffic for OTLP Log ingestion.
var (
	OTLPIngestAgentLogsEvents = telemetry.NewCounter(
		"runtime",
		"datadog_agent_otlp_logs_events",
		[]string{},
		"Counter metric of OTLP Log events in OTLP ingestion for the Datadog agent",
	)
	OTLPIngestAgentLogsRequests = telemetry.NewCounter(
		"runtime",
		"datadog_agent_otlp_logs_requests",
		[]string{},
		"Counter metric of OTLP Log requests in OTLP ingestion for the Datadog agent",
	)
	OTLPIngestDDOTLogsEvents = telemetry.NewCounter(
		"runtime",
		"ddot_otlp_log_events",
		[]string{},
		"Counter metric of OTLP Log events in OTLP ingestion for DDOT",
	)
	OTLPIngestDDOTLogsRequests = telemetry.NewCounter(
		"runtime",
		"ddot_otlp_log_requests",
		[]string{},
		"Counter metric of OTLP Log requests in OTLP ingestion for DDOT",
	)
)
