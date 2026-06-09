// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package serializerexporter

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	otlpmetrics "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func (c *serializerConsumer) ConsumeExplicitBoundHistogram(_ context.Context, dimensions *otlpmetrics.Dimensions, ts uint64, interval int64, point pmetric.HistogramDataPoint, _ bool) {
	msrc, ok := metricOriginsMappings[dimensions.OriginProductDetail()]
	if !ok {
		msrc = metrics.MetricSourceOpenTelemetryCollectorUnknown
	}
	c.sketches = append(c.sketches, &metrics.SketchSeries{
		Name:     dimensions.Name(),
		Tags:     tagset.CompositeTagsFromSlice(enrichTags(c.extraTags, dimensions)),
		Host:     dimensions.Host(),
		Interval: interval,
		Points: []metrics.SketchPoint{{
			Ts:     int64(ts / 1e9),
			Sketch: &metrics.ExplicitBoundHistogramPoint{Point: point},
		}},
		Source: msrc,
	})
}

func (c *serializerConsumer) ConsumeExponentialHistogram(_ context.Context, dimensions *otlpmetrics.Dimensions, ts uint64, interval int64, point pmetric.ExponentialHistogramDataPoint) {
	msrc, ok := metricOriginsMappings[dimensions.OriginProductDetail()]
	if !ok {
		msrc = metrics.MetricSourceOpenTelemetryCollectorUnknown
	}
	c.sketches = append(c.sketches, &metrics.SketchSeries{
		Name:     dimensions.Name(),
		Tags:     tagset.CompositeTagsFromSlice(enrichTags(c.extraTags, dimensions)),
		Host:     dimensions.Host(),
		Interval: interval,
		Points: []metrics.SketchPoint{{
			Ts:     int64(ts / 1e9),
			Sketch: &metrics.ExponentialHistogramPoint{Point: point},
		}},
		Source: msrc,
	})
}
