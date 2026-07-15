// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbacksender

import telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"

var (
	tlmWriterAppendSamples = telemetryimpl.GetCompatComponent().NewCounter(
		"metric_lookback",
		"writer_append_samples",
		[]string{"check_name", "state"},
		"Number of lookback metric samples submitted to writer append calls by the shadow sender",
	)
	tlmWriterAppendDuration = telemetryimpl.GetCompatComponent().NewGauge(
		"metric_lookback",
		"writer_append_duration_seconds",
		[]string{"check_name", "state"},
		"Most recent duration in seconds of lookback writer append calls",
	)
	tlmUnsupportedDrops = telemetryimpl.GetCompatComponent().NewCounter(
		"metric_lookback",
		"unsupported_payloads_dropped",
		[]string{"method"},
		"Number of unsupported sender payloads dropped by lookback sender",
	)
)
