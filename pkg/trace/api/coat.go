// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	OTLPIngestAgentTracesEvents = telemetry.NewCounter(
		"runtime",
		"datadog_agent_otlp_traces_events",
		[]string{},
		"Counter metric of OTLP Trace events in OTLP ingestion with the Datadog agent",
	)
	OTLPIngestAgentTracesRequests = telemetry.NewCounter(
		"runtime",
		"datadog_agent_otlp_traces_requests",
		[]string{},
		"Counter metric of OTLP Trace requests in OTLP ingestion with the Datadog agent",
	)
)
