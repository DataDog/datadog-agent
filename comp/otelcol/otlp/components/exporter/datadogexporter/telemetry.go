// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogexporter

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// These metrics are for measuring the current volume of traffic for OTLP trace ingestion.
var (
	OTLPIngestDDOTTracesEvents = telemetry.NewCounter(
		"runtime",
		"ddot_otlp_traces_events",
		[]string{},
		"Counter metric of OTLP Trace events in OTLP ingestion using DDOT",
	)
	OTLPIngestDDOTTracesRequests = telemetry.NewCounter(
		"runtime",
		"ddot_otlp_traces_requests",
		[]string{},
		"Counter metric of OTLP Trace requests in OTLP ingestion using DDOT",
	)
)
