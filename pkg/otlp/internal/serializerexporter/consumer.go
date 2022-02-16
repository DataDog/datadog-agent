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
	"github.com/DataDog/datadog-agent/pkg/metricsserializer"
	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

var _ translator.Consumer = (*serializerConsumer)(nil)

type serializerConsumer struct {
	series   metricsserializer.Series
	sketches metricsserializer.SketchSeriesList
}

func (c *serializerConsumer) ConsumeSketch(_ context.Context, dimensions *translator.Dimensions, ts uint64, qsketch *quantile.Sketch) {
	c.sketches = append(c.sketches, metrics.SketchSeries{
		Name:     dimensions.Name(),
		Tags:     dimensions.Tags(),
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
			Tags:     dimensions.Tags(),
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
		Tags:           []string{},
		Host:           hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	})
}

// enrichTags of series and sketches.
// This method should be called once after metrics have been mapped.
//
// In the OTLP pipeline, 'contexts' are kept within the translator, and,
// therefore, this works a little differently than for DogStatsD/check metrics.
func (c *serializerConsumer) enrichTags(cardinality string) {
	// TODO (AP-1328): Get origin from semantic conventions.
	const origin = ""
	const k8sOriginID = ""

	for i := range c.series {
		tb := tagset.NewHashlessTagsAccumulatorFromSlice(c.series[i].Tags)
		tagger.EnrichTags(tb, origin, k8sOriginID, cardinality)
		c.series[i].Tags = tb.Get()
	}

	for i := range c.sketches {
		tb := tagset.NewHashlessTagsAccumulatorFromSlice(c.sketches[i].Tags)
		tagger.EnrichTags(tb, origin, k8sOriginID, cardinality)
		c.sketches[i].Tags = tb.Get()
	}
}

// flush all metrics and sketches in consumer.
func (c *serializerConsumer) flush(s serializer.MetricSerializer) error {
	if err := s.SendSketch(c.sketches); err != nil {
		return err
	}
	return s.SendSeries(c.series)
}
