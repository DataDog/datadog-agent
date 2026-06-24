// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"

var (
	tlmShadowEnqueueDelays = telemetryimpl.GetCompatComponent().NewCounter(
		"metric_lookback",
		"shadow_enqueue_delays",
		[]string{"check_name"},
		"Number of shadow check ticks delayed while enqueueing to the shadow runner",
	)
	tlmShadowEnqueueDelayDuration = telemetryimpl.GetCompatComponent().NewGauge(
		"metric_lookback",
		"shadow_enqueue_delay_duration",
		[]string{"check_name"},
		"Duration in seconds spent enqueueing shadow checks to the shadow runner",
	)
)
