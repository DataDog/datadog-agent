package metrics

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

type SimpleMapper struct {
	mapper        Mapper
	WithRemapping bool
	logger        *zap.Logger
}

func NewSimpleMapper(cfg translatorConfig, logger *zap.Logger) Mapper {
	d := NewDefaultMapper(nil, logger, cfg)
	return &SimpleMapper{
		mapper: d,
		logger: logger,
	}
}

func (m *SimpleMapper) MapNumberMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, dt DataType, slice pmetric.NumberDataPointSlice) {
	m.mapper.MapNumberMetrics(ctx, consumer, dims, dt, slice)
}

func (m *SimpleMapper) MapSummaryMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.SummaryDataPointSlice) {
	m.mapper.MapSummaryMetrics(ctx, consumer, dims, slice)
}

func (m *SimpleMapper) MapHistogramMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.HistogramDataPointSlice, delta bool) error {
	consumer.ConsumeExplicitBoundHistogram(ctx, dims, slice)
	return nil
}

func (m *SimpleMapper) MapExponentialHistogramMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.ExponentialHistogramDataPointSlice, delta bool) {
	consumer.ConsumeExponentialHistogram(ctx, dims, slice)
}
