// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

var (
	tlmSeriesPipelineDuration = telemetryimpl.GetCompatComponent().NewCounter("serializer", "series_pipeline_duration_ns",
		[]string{"phase"}, "Cumulative metrics series serialization duration by phase, in nanoseconds")
	tlmSeriesPipelineEvents = telemetryimpl.GetCompatComponent().NewCounter("serializer", "series_pipeline_events",
		[]string{"phase"}, "Count of metrics series serialization phase observations")
)

func recordSeriesPipelineDuration(phase string, duration time.Duration) {
	tlmSeriesPipelineDuration.Add(float64(duration.Nanoseconds()), phase)
	tlmSeriesPipelineEvents.Inc(phase)
}
