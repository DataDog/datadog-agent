// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// SimpleMapper is a cache-free mapper implementation that directly emits
// metric values without cumulative-to-delta conversion.
type SimpleMapper struct {
	cfg    translatorConfig
	logger *zap.Logger
}

// NewSimpleMapper creates a new SimpleMapper without a cache.
// This mapper emits raw values as Gauges instead of computing deltas.
func NewSimpleMapper(cfg translatorConfig, logger *zap.Logger) Mapper {
	return &SimpleMapper{
		cfg:    cfg,
		logger: logger,
	}
}

// MapNumberMetrics maps number datapoints to Datadog metrics.
func (m *SimpleMapper) MapNumberMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, dt DataType, slice pmetric.NumberDataPointSlice) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		if p.Flags().NoRecordedValue() {
			continue
		}

		pointDims := dims.WithAttributeMap(p.Attributes())
		var val float64
		switch p.ValueType() {
		case pmetric.NumberDataPointValueTypeDouble:
			val = p.DoubleValue()
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(p.IntValue())
		}

		// Calculate interval for Count type metrics (from OTLP delta sums)
		var interval int64
		if m.cfg.InferDeltaInterval && dt == Count {
			interval = inferDeltaInterval(uint64(p.StartTimestamp()), uint64(p.Timestamp()))
		}

		consumer.ConsumeTimeSeries(ctx, pointDims, dt, uint64(p.Timestamp()), interval, val)
	}
}

// MapSummaryMetrics maps summary datapoints to Datadog metrics.
// Since SimpleMapper doesn't use a cache, count and sum are emitted as Gauges
// with the current value, rather than as delta Counts.
func (m *SimpleMapper) MapSummaryMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.SummaryDataPointSlice) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		if p.Flags().NoRecordedValue() {
			continue
		}

		ts := uint64(p.Timestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		// Emit count as a Gauge (raw value, no delta conversion)
		countDims := pointDims.WithSuffix("count")
		consumer.ConsumeTimeSeries(ctx, countDims, Gauge, ts, 0, float64(p.Count()))

		// Emit sum as a Gauge (raw value, no delta conversion)
		sumDims := pointDims.WithSuffix("sum")
		consumer.ConsumeTimeSeries(ctx, sumDims, Gauge, ts, 0, p.Sum())

		// Emit quantiles if configured
		if m.cfg.Quantiles {
			baseQuantileDims := pointDims.WithSuffix("quantile")
			quantiles := p.QuantileValues()
			for j := 0; j < quantiles.Len(); j++ {
				q := quantiles.At(j)
				quantileDims := baseQuantileDims.AddTags(getQuantileTag(q.Quantile()))
				consumer.ConsumeTimeSeries(ctx, quantileDims, Gauge, ts, 0, q.Value())
			}
		}
	}
}

func (m *SimpleMapper) MapHistogramMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.HistogramDataPointSlice, _ bool) error {
	consumer.ConsumeExplicitBoundHistogram(ctx, dims, slice)
	return nil
}

func (m *SimpleMapper) MapExponentialHistogramMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.ExponentialHistogramDataPointSlice, _ bool) {
	consumer.ConsumeExponentialHistogram(ctx, dims, slice)
}
