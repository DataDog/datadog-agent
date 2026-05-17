// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

var (
	tlmDogstatsdPipelineFlushDuration = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_pipeline", "flush_duration_ns",
		[]string{"phase"}, "Cumulative DogStatsD aggregation/serialization flush duration by phase, in nanoseconds")
	tlmDogstatsdPipelineFlushes = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_pipeline", "flushes",
		[]string{"phase"}, "Count of DogStatsD aggregation/serialization flush phase observations")
	tlmDogstatsdPipelineItems = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_pipeline", "items",
		[]string{"kind"}, "Count of DogStatsD aggregation/serialization items observed during flush")
)

func recordDogstatsdPipelineDuration(phase string, duration time.Duration) {
	tlmDogstatsdPipelineFlushDuration.Add(float64(duration.Nanoseconds()), phase)
	tlmDogstatsdPipelineFlushes.Inc(phase)
}

func observeDogstatsdPipelineSerie(serie *metrics.Serie) {
	if serie == nil {
		return
	}
	tlmDogstatsdPipelineItems.Inc("series")
	tlmDogstatsdPipelineItems.Add(float64(len(serie.Points)), "series_points")
	tlmDogstatsdPipelineItems.Add(float64(serie.Tags.Len()), "series_tags")
}

func observeDogstatsdPipelineSerieRow(row *metrics.SerieRow) {
	if row == nil {
		return
	}
	tlmDogstatsdPipelineItems.Inc("series_rows")
	tlmDogstatsdPipelineItems.Add(float64(len(row.Points)), "series_row_points")
	tlmDogstatsdPipelineItems.Add(float64(row.Tags.Len()), "series_row_tags")
}

func observeDogstatsdPipelineSketch(sketch *metrics.SketchSeries) {
	if sketch == nil {
		return
	}
	tlmDogstatsdPipelineItems.Inc("sketch_series")
	tlmDogstatsdPipelineItems.Add(float64(len(sketch.Points)), "sketch_points")
	tlmDogstatsdPipelineItems.Add(float64(sketch.Tags.Len()), "sketch_tags")
}
