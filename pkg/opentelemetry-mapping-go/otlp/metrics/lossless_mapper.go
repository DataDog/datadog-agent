// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// lossLessMapper is a cache-free mapper implementation that directly emits
// metric values without cumulative-to-delta conversion and sends Histograms wihtout conversion.
//
//	This mapper emits raw values from OTLP cumulative monotonic Sums as Datadog Gauges,
//	instead of computing deltas and reporting them as Datadog Counts.
type lossLessMapper struct {
	cfg                  translatorConfig
	logger               *zap.Logger
	warnedRateAttrErrors sync.Map
}

// newLossLessMapper creates a new lossLessMapper without a cache.
// This mapper emits raw values from OTLP delta Sums as Datadog Counts,
// and OTLP cumulative values (like Summary count/sum) as Datadog Gauges
// instead of computing deltas.
func newLossLessMapper(cfg translatorConfig, logger *zap.Logger) mapper {
	return &lossLessMapper{
		cfg:    cfg,
		logger: logger,
	}
}

// MapNumberMetrics maps number datapoints to Datadog metrics.
func (m *lossLessMapper) MapNumberMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, dt DataType, slice pmetric.NumberDataPointSlice) {
	mapNumberMetrics(ctx, consumer, dims, dt, slice, m.logger, m.cfg.InferDeltaInterval, &m.warnedRateAttrErrors)
}

const (
	// deltaSumRateAttributeKey is the datapoint-level attribute that signals
	// a delta sum should be emitted as a Datadog rate instead of a count.
	deltaSumRateAttributeKey = "datadog.metric.as_type"
)

// rateAttrErrors tracks which one-shot errors have been emitted for a
// given metric name so that each error type fires at most once.
type rateAttrErrors struct {
	wrongType    atomic.Bool
	zeroInterval atomic.Bool
	unknownValue atomic.Bool
}

// mapNumberMetrics maps number datapoints into Datadog metrics.
// This is a shared implementation used by both defaultMapper and lossLessMapper.
func mapNumberMetrics(
	ctx context.Context,
	consumer Consumer,
	dims *Dimensions,
	dt DataType,
	slice pmetric.NumberDataPointSlice,
	logger *zap.Logger,
	inferInterval bool,
	warnedRateAttrErrors *sync.Map,
) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		if p.Flags().NoRecordedValue() {
			// No recorded value, skip.
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

		if isSkippable(logger, pointDims.name, val) {
			continue
		}

		// Calculate interval for Count type metrics (from OTLP delta sums)
		var interval int64
		if inferInterval && dt == Count {
			interval = inferDeltaInterval(uint64(p.StartTimestamp()), uint64(p.Timestamp()))
		}

		pointDt := dt

		// Check rate attribute and change metric type if necessary and convert value
		if asType, ok := p.Attributes().Get(deltaSumRateAttributeKey); ok {
			switch strings.ToLower(asType.Str()) {
			case "rate":
				//conversion is allowed only from delta-sum to Rate
				if pointDt == Count {
					pointDt = Rate

					if interval > 0 {
						val = val / float64(interval)
					} else {
						w, _ := warnedRateAttrErrors.LoadOrStore(pointDims.name, &rateAttrErrors{})
						if re := w.(*rateAttrErrors); !re.zeroInterval.Swap(true) {
							logger.Error("datadog.metric.as_type=rate on delta sum but interval is 0; value will not be divided by interval. "+
								"Enable the exporter.datadogexporter.InferIntervalForDeltaMetrics feature gate and ensure the metric has a valid StartTimestamp.",
								zap.String("metric name", pointDims.name),
							)
						}
					}

				} else {
					w, _ := warnedRateAttrErrors.LoadOrStore(pointDims.name, &rateAttrErrors{})
					if re := w.(*rateAttrErrors); !re.wrongType.Swap(true) {
						logger.Error("datadog.metric.as_type=rate is only supported on delta sum metrics, ignoring",
							zap.String("metric name", pointDims.name),
						)
					}
				}
			case "count", "gauge":
				// explicit no-op: the metric keeps its original type
			default:
				w, _ := warnedRateAttrErrors.LoadOrStore(pointDims.name, &rateAttrErrors{})
				if re := w.(*rateAttrErrors); !re.unknownValue.Swap(true) {
					logger.Error("unsupported datadog.metric.as_type value, ignoring; accepted values are \"rate\", \"count\", \"gauge\"",
						zap.String("metric name", pointDims.name),
						zap.String("value", asType.Str()),
					)
				}
			}
		}

		consumer.ConsumeTimeSeries(ctx, pointDims, pointDt, uint64(p.Timestamp()), interval, val)
	}
}

// MapSummaryMetrics maps summary datapoints to Datadog metrics.
// Since lossLessMapper doesn't use a cache, count and sum are emitted as Gauges
// with the current value, rather than as delta Counts.
func (m *lossLessMapper) MapSummaryMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.SummaryDataPointSlice) {
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
		if isSkippable(m.logger, sumDims.name, p.Sum()) {
			continue
		}
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

func (m *lossLessMapper) MapHistogramMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.HistogramDataPointSlice, _ bool) error {
	consumer.ConsumeExplicitBoundHistogram(ctx, dims, slice)
	return nil
}

func (m *lossLessMapper) MapExponentialHistogramMetrics(ctx context.Context, consumer Consumer, dims *Dimensions, slice pmetric.ExponentialHistogramDataPointSlice, _ bool) {
	consumer.ConsumeExponentialHistogram(ctx, dims, slice)
}
