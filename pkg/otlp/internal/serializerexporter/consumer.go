// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var _ translator.Consumer = (*serializerConsumer)(nil)

type serializerConsumer struct {
	cardinality collectors.TagCardinality
	extraTags   []string
	series      metrics.Series
	sketches    metrics.SketchSeriesList
}

// enrichedTags of a given dimension.
// In the OTLP pipeline, 'contexts' are kept within the translator and function differently than DogStatsD/check metrics.
func (c *serializerConsumer) enrichedTags(dimensions *translator.Dimensions) []string {
	enrichedTags := make([]string, 0, len(c.extraTags)+len(dimensions.Tags()))
	enrichedTags = append(enrichedTags, c.extraTags...)
	enrichedTags = append(enrichedTags, dimensions.Tags()...)

	entityTags, err := tagger.Tag(dimensions.OriginID(), c.cardinality)
	if err != nil {
		log.Tracef("Cannot get tags for entity %s: %s", dimensions.OriginID(), err)
	} else {
		enrichedTags = append(enrichedTags, entityTags...)
	}

	globalTags, err := tagger.GlobalTags(c.cardinality)
	if err != nil {
		log.Trace(err.Error())
	} else {
		enrichedTags = append(enrichedTags, globalTags...)
	}

	return enrichedTags
}

func (c *serializerConsumer) ConsumeSketch(_ context.Context, dimensions *translator.Dimensions, ts uint64, qsketch *quantile.Sketch) {
	c.sketches = append(c.sketches, metrics.SketchSeries{
		Name:     dimensions.Name(),
		Tags:     tagset.CompositeTagsFromSlice(c.enrichedTags(dimensions)),
		Host:     dimensions.Host(),
		Interval: 1,
		Points: []metrics.SketchPoint{{
			Ts:     int64(ts / 1e9),
			Sketch: qsketch,
		}},
	})
}

func apiTypeFromTranslatorType(typ translator.MetricDataType) metrics.APIMetricType {
	switch typ {
	case translator.Count:
		return metrics.APICountType
	case translator.Gauge:
		return metrics.APIGaugeType
	}
	panic(fmt.Sprintf("unreachable: received non-count non-gauge type: %d", typ))
}

func (c *serializerConsumer) ConsumeTimeSeries(ctx context.Context, dimensions *translator.Dimensions, typ translator.MetricDataType, ts uint64, value float64) {
	c.series = append(c.series,
		&metrics.Serie{
			Name:     dimensions.Name(),
			Points:   []metrics.Point{{Ts: float64(ts / 1e9), Value: value}},
			Tags:     tagset.CompositeTagsFromSlice(c.enrichedTags(dimensions)),
			Host:     dimensions.Host(),
			MType:    apiTypeFromTranslatorType(typ),
			Interval: 1,
		},
	)
}

// addTelemetryMetric to know if an Agent is using OTLP metrics.
func (c *serializerConsumer) addTelemetryMetric(hostname string) {
	c.series = append(c.series, &metrics.Serie{
		Name:           "datadog.agent.otlp.metrics",
		Points:         []metrics.Point{{Value: 1, Ts: float64(time.Now().Unix())}},
		Tags:           tagset.CompositeTagsFromSlice([]string{}),
		Host:           hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	})
}

// flush all metrics and sketches in consumer.
func (c *serializerConsumer) flush(s serializer.MetricSerializer) error {
	if err := s.SendSketch(c.sketches); err != nil {
		return err
	}

	var err error
	metrics.StartIteration(
		metrics.NewIterableSeries(func(se *metrics.Serie) {}, 200, 4000),
		func(seriesSink metrics.SerieSink) {
			for _, serie := range c.series {
				seriesSink.Append(serie)
			}
		}, func(serieSource metrics.SerieSource) {
			err = s.SendIterableSeries(serieSource)
		})

	return err
}
