package serializerexporter

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

var _ translator.Consumer = (*serializerConsumer)(nil)

type serializerConsumer struct {
	series   metrics.Series
	sketches metrics.SketchSeriesList
}

func (c *serializerConsumer) ConsumeSketch(_ context.Context, name string, ts uint64, qsketch *quantile.Sketch, tags []string, host string) {
	c.sketches = append(c.sketches, metrics.SketchSeries{
		Name:     name,
		Tags:     tags,
		Host:     host,
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

func (c *serializerConsumer) ConsumeTimeSeries(ctx context.Context, name string, typ translator.MetricDataType, ts uint64, value float64, tags []string, host string) {
	c.series = append(c.series,
		&metrics.Serie{
			Name:     name,
			Points:   []metrics.Point{{Ts: float64(ts / 1e9), Value: value}},
			Tags:     tags,
			Host:     host,
			MType:    apiTypeFromTranslatorType(typ),
			Interval: 1,
		},
	)
}

// flush all metrics and sketches in consumer.
func (c *serializerConsumer) flush(s serializer.MetricSerializer) error {
	if err := s.SendSketch(c.sketches); err != nil {
		return err
	}
	return s.SendSeries(c.series)
}
