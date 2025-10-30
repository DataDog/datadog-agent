// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// These metrics are for measuring the current volume of traffic customers are sending through OTLP collectors (Agent, DDOT)
var (
	OTLPIngestMetricsEvents = telemetry.NewCounter(
		"runtime",
		"datadog_agent_otlp_metrics_events",
		[]string{},
		"Counter metric of OTLP Metric events in OTLP ingestion",
	)
	OTLPIngestMetricsRequests = telemetry.NewCounter(
		"runtime",
		"datadog_agent_otlp_metrics_requests",
		[]string{},
		"Counter metric of OTLP Metric requests in OTLP ingestion",
	)
)
