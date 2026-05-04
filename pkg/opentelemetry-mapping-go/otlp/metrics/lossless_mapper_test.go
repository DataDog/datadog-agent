// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLossLessMapperMapNumberMetrics(t *testing.T) {
	tests := []struct {
		name               string
		dataType           DataType
		inferDeltaInterval bool
		setupSlice         func(slice pmetric.NumberDataPointSlice)
		expectedTimeSeries []TestTimeSeries
	}{
		{
			name:     "gauge with int value",
			dataType: Gauge,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(100)
				dp.SetTimestamp(pcommon.Timestamp(1000000000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Gauge,
					Timestamp:      1000000000,
					Value:          100,
				},
			},
		},
		{
			name:     "gauge with double value",
			dataType: Gauge,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetDoubleValue(123.45)
				dp.SetTimestamp(pcommon.Timestamp(2000000000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Gauge,
					Timestamp:      2000000000,
					Value:          123.45,
				},
			},
		},
		{
			name:     "count with multiple datapoints",
			dataType: Count,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp1 := slice.AppendEmpty()
				dp1.SetIntValue(10)
				dp1.SetTimestamp(pcommon.Timestamp(1000000000))

				dp2 := slice.AppendEmpty()
				dp2.SetIntValue(20)
				dp2.SetTimestamp(pcommon.Timestamp(2000000000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Count,
					Timestamp:      1000000000,
					Value:          10,
				},
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Count,
					Timestamp:      2000000000,
					Value:          20,
				},
			},
		},
		{
			name:     "skips NoRecordedValue flag",
			dataType: Gauge,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp1 := slice.AppendEmpty()
				dp1.SetIntValue(100)
				dp1.SetTimestamp(pcommon.Timestamp(1000000000))
				dp1.SetFlags(dp1.Flags().WithNoRecordedValue(true))

				dp2 := slice.AppendEmpty()
				dp2.SetIntValue(200)
				dp2.SetTimestamp(pcommon.Timestamp(2000000000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Gauge,
					Timestamp:      2000000000,
					Value:          200,
				},
			},
		},
		{
			name:               "count with interval inference",
			dataType:           Count,
			inferDeltaInterval: true,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(50)
				// Start: 1000000000 ns = 1 second
				// End: 11000000000 ns = 11 seconds
				// Diff: 10 seconds
				dp.SetStartTimestamp(pcommon.Timestamp(1000000000))
				dp.SetTimestamp(pcommon.Timestamp(11000000000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Count,
					Timestamp:      11000000000,
					Interval:       10, // 10 seconds difference
					Value:          50,
				},
			},
		},
		{
			name:     "with attributes",
			dataType: Gauge,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(42)
				dp.SetTimestamp(pcommon.Timestamp(1000000000))
				dp.Attributes().PutStr("env", "prod")
				dp.Attributes().PutStr("service", "api")
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{
						Name: "test.metric",
						Tags: []string{"env:prod", "service:api"},
					},
					Type:      Gauge,
					Timestamp: 1000000000,
					Value:     42,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := translatorConfig{
				InferDeltaInterval: tt.inferDeltaInterval,
			}
			mapper := newLossLessMapper(cfg, zap.NewNop())

			slice := pmetric.NewNumberDataPointSlice()
			tt.setupSlice(slice)

			dims := &Dimensions{name: "test.metric"}
			consumer := newTestConsumer()

			mapper.MapNumberMetrics(context.Background(), &consumer, dims, tt.dataType, slice)

			require.Len(t, consumer.data.Metrics.TimeSeries, len(tt.expectedTimeSeries))
			for i, expected := range tt.expectedTimeSeries {
				actual := consumer.data.Metrics.TimeSeries[i]
				assert.Equal(t, expected.Name, actual.Name)
				assert.Equal(t, expected.Type, actual.Type)
				assert.Equal(t, expected.Timestamp, actual.Timestamp)
				assert.Equal(t, expected.Value, actual.Value)
				assert.Equal(t, expected.Interval, actual.Interval)
				if expected.Tags != nil {
					assert.ElementsMatch(t, expected.Tags, actual.Tags)
				}
			}
		})
	}
}

func TestLossLessMapperMapNumberMetrics_DeltaSumRateAttribute(t *testing.T) {
	tests := []struct {
		name               string
		inferDeltaInterval bool
		setupSlice         func(slice pmetric.NumberDataPointSlice)
		expectedTimeSeries []TestTimeSeries
	}{
		{
			name:               "delta sum with as_type=rate and interval inference",
			inferDeltaInterval: true,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(200)
				dp.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
				dp.SetTimestamp(pcommon.Timestamp(11_000_000_000))
				dp.Attributes().PutStr("datadog.metric.as_type", "rate")
				dp.Attributes().PutStr("env", "prod")
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{
						Name: "test.metric",
						Tags: []string{"env:prod", "datadog.metric.as_type:rate"},
					},
					Type:      Rate,
					Timestamp: 11_000_000_000,
					Interval:  10,
					Value:     20, // 200 / 10
				},
			},
		},
		{
			name:               "delta sum with as_type=rate without interval inference",
			inferDeltaInterval: false,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(200)
				dp.SetTimestamp(pcommon.Timestamp(11_000_000_000))
				dp.Attributes().PutStr("datadog.metric.as_type", "rate")
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Rate,
					Timestamp:      11_000_000_000,
					Interval:       0,
					Value:          200, // no division when interval is 0
				},
			},
		},
		{
			name:               "delta sum without as_type attribute stays count",
			inferDeltaInterval: true,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(50)
				dp.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
				dp.SetTimestamp(pcommon.Timestamp(11_000_000_000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Count,
					Timestamp:      11_000_000_000,
					Interval:       10,
					Value:          50,
				},
			},
		},
		{
			name:               "as_type attribute is kept in tags",
			inferDeltaInterval: true,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(100)
				dp.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
				dp.SetTimestamp(pcommon.Timestamp(6_000_000_000))
				dp.Attributes().PutStr("datadog.metric.as_type", "rate")
				dp.Attributes().PutStr("service", "web")
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{
						Name: "test.metric",
						Tags: []string{"service:web", "datadog.metric.as_type:rate"},
					},
					Type:      Rate,
					Timestamp: 6_000_000_000,
					Interval:  5,
					Value:     20, // 100 / 5
				},
			},
		},
		{
			name:               "as_type=rate on gauge is ignored",
			inferDeltaInterval: false,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(42)
				dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))
				dp.Attributes().PutStr("datadog.metric.as_type", "rate")
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric"},
					Type:           Gauge,
					Timestamp:      1_000_000_000,
					Value:          42,
				},
			},
		},
		{
			name:               "unknown as_type value is ignored",
			inferDeltaInterval: true,
			setupSlice: func(slice pmetric.NumberDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetIntValue(100)
				dp.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
				dp.SetTimestamp(pcommon.Timestamp(11_000_000_000))
				dp.Attributes().PutStr("datadog.metric.as_type", "histogram")
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{
						Name: "test.metric",
						Tags: []string{"datadog.metric.as_type:histogram"},
					},
					Type:      Count,
					Timestamp: 11_000_000_000,
					Interval:  10,
					Value:     100,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := translatorConfig{
				InferDeltaInterval: tt.inferDeltaInterval,
			}
			mapper := newLossLessMapper(cfg, zap.NewNop())

			slice := pmetric.NewNumberDataPointSlice()
			tt.setupSlice(slice)

			dims := &Dimensions{name: "test.metric"}
			consumer := newTestConsumer()

			// Use Gauge for the "as_type=rate on gauge" test case, Count for all others
			dt := Count
			if tt.name == "as_type=rate on gauge is ignored" {
				dt = Gauge
			}

			mapper.MapNumberMetrics(context.Background(), &consumer, dims, dt, slice)

			require.Len(t, consumer.data.Metrics.TimeSeries, len(tt.expectedTimeSeries))
			for i, expected := range tt.expectedTimeSeries {
				actual := consumer.data.Metrics.TimeSeries[i]
				assert.Equal(t, expected.Name, actual.Name)
				assert.Equal(t, expected.Type, actual.Type)
				assert.Equal(t, expected.Timestamp, actual.Timestamp)
				assert.Equal(t, expected.Value, actual.Value)
				assert.Equal(t, expected.Interval, actual.Interval)
				if expected.Tags != nil {
					assert.ElementsMatch(t, expected.Tags, actual.Tags)
				}
			}
		})
	}
}

func TestMapNumberMetrics_RateAttributeWarnings(t *testing.T) {
	t.Run("unknown as_type value logs error", func(t *testing.T) {
		core, logs := observer.New(zapcore.ErrorLevel)
		logger := zap.New(core)
		cfg := translatorConfig{
			InferDeltaInterval: true,
		}
		mapper := newLossLessMapper(cfg, logger)

		slice := pmetric.NewNumberDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetIntValue(100)
		dp.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
		dp.SetTimestamp(pcommon.Timestamp(11_000_000_000))
		dp.Attributes().PutStr("datadog.metric.as_type", "histogram")

		dims := &Dimensions{name: "test.metric"}
		consumer := newTestConsumer()
		mapper.MapNumberMetrics(context.Background(), &consumer, dims, Count, slice)

		require.Equal(t, 1, logs.Len())
		entry := logs.All()[0]
		assert.Contains(t, entry.Message, "unsupported datadog.metric.as_type value")
		assert.Equal(t, "test.metric", entry.ContextMap()["metric name"])
		assert.Equal(t, "histogram", entry.ContextMap()["value"])
	})

	t.Run("zero interval logs warning when as_type=rate", func(t *testing.T) {
		core, logs := observer.New(zapcore.ErrorLevel)
		logger := zap.New(core)
		cfg := translatorConfig{
			InferDeltaInterval: false,
		}
		mapper := newLossLessMapper(cfg, logger)

		slice := pmetric.NewNumberDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetIntValue(200)
		dp.SetTimestamp(pcommon.Timestamp(11_000_000_000))
		dp.Attributes().PutStr("datadog.metric.as_type", "rate")

		dims := &Dimensions{name: "test.metric"}
		consumer := newTestConsumer()
		mapper.MapNumberMetrics(context.Background(), &consumer, dims, Count, slice)

		require.Len(t, consumer.data.Metrics.TimeSeries, 1)
		ts := consumer.data.Metrics.TimeSeries[0]
		assert.Equal(t, Rate, ts.Type)
		assert.Equal(t, float64(200), ts.Value, "value should not be divided when interval is 0")

		require.Equal(t, 1, logs.Len())
		entry := logs.All()[0]
		assert.Contains(t, entry.Message, "interval is 0")
		assert.Equal(t, "test.metric", entry.ContextMap()["metric name"])
	})

	t.Run("one-shot error fires only once for repeated misuse", func(t *testing.T) {
		core, logs := observer.New(zapcore.ErrorLevel)
		logger := zap.New(core)
		cfg := translatorConfig{}
		mapper := newLossLessMapper(cfg, logger)

		slice := pmetric.NewNumberDataPointSlice()
		for i := 0; i < 3; i++ {
			dp := slice.AppendEmpty()
			dp.SetIntValue(int64(i + 1))
			dp.SetTimestamp(pcommon.Timestamp(uint64(i+1) * 1_000_000_000))
			dp.Attributes().PutStr("datadog.metric.as_type", "rate")
		}

		dims := &Dimensions{name: "test.metric"}
		consumer := newTestConsumer()
		mapper.MapNumberMetrics(context.Background(), &consumer, dims, Gauge, slice)

		require.Len(t, consumer.data.Metrics.TimeSeries, 3, "all datapoints should still be emitted")
		for _, ts := range consumer.data.Metrics.TimeSeries {
			assert.Equal(t, Gauge, ts.Type, "type should remain Gauge")
		}

		assert.Equal(t, 1, logs.Len(),
			"wrongType error should be logged exactly once despite 3 datapoints")
		assert.Contains(t, logs.All()[0].Message, "only supported on delta sum metrics")
	})
}

func TestLossLessMapperMapSummaryMetrics(t *testing.T) {
	tests := []struct {
		name               string
		quantiles          bool
		setupSlice         func(slice pmetric.SummaryDataPointSlice)
		expectedTimeSeries []TestTimeSeries
	}{
		{
			name:      "basic summary without quantiles",
			quantiles: false,
			setupSlice: func(slice pmetric.SummaryDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetCount(100)
				dp.SetSum(500.5)
				dp.SetTimestamp(pcommon.Timestamp(1000000000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric.count"},
					Type:           Gauge,
					Timestamp:      1000000000,
					Value:          100,
				},
				{
					TestDimensions: TestDimensions{Name: "test.metric.sum"},
					Type:           Gauge,
					Timestamp:      1000000000,
					Value:          500.5,
				},
			},
		},
		{
			name:      "summary with quantiles",
			quantiles: true,
			setupSlice: func(slice pmetric.SummaryDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetCount(100)
				dp.SetSum(500.5)
				dp.SetTimestamp(pcommon.Timestamp(1000000000))

				q1 := dp.QuantileValues().AppendEmpty()
				q1.SetQuantile(0.5)
				q1.SetValue(5.0)

				q2 := dp.QuantileValues().AppendEmpty()
				q2.SetQuantile(0.99)
				q2.SetValue(9.9)
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric.count"},
					Type:           Gauge,
					Timestamp:      1000000000,
					Value:          100,
				},
				{
					TestDimensions: TestDimensions{Name: "test.metric.sum"},
					Type:           Gauge,
					Timestamp:      1000000000,
					Value:          500.5,
				},
				{
					TestDimensions: TestDimensions{
						Name: "test.metric.quantile",
						Tags: []string{"quantile:0.5"},
					},
					Type:      Gauge,
					Timestamp: 1000000000,
					Value:     5.0,
				},
				{
					TestDimensions: TestDimensions{
						Name: "test.metric.quantile",
						Tags: []string{"quantile:0.99"},
					},
					Type:      Gauge,
					Timestamp: 1000000000,
					Value:     9.9,
				},
			},
		},
		{
			name:      "skips NoRecordedValue flag",
			quantiles: false,
			setupSlice: func(slice pmetric.SummaryDataPointSlice) {
				dp1 := slice.AppendEmpty()
				dp1.SetCount(100)
				dp1.SetSum(500.5)
				dp1.SetTimestamp(pcommon.Timestamp(1000000000))
				dp1.SetFlags(dp1.Flags().WithNoRecordedValue(true))

				dp2 := slice.AppendEmpty()
				dp2.SetCount(200)
				dp2.SetSum(1000.0)
				dp2.SetTimestamp(pcommon.Timestamp(2000000000))
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{Name: "test.metric.count"},
					Type:           Gauge,
					Timestamp:      2000000000,
					Value:          200,
				},
				{
					TestDimensions: TestDimensions{Name: "test.metric.sum"},
					Type:           Gauge,
					Timestamp:      2000000000,
					Value:          1000.0,
				},
			},
		},
		{
			name:      "summary with attributes",
			quantiles: false,
			setupSlice: func(slice pmetric.SummaryDataPointSlice) {
				dp := slice.AppendEmpty()
				dp.SetCount(50)
				dp.SetSum(250.0)
				dp.SetTimestamp(pcommon.Timestamp(1000000000))
				dp.Attributes().PutStr("env", "staging")
			},
			expectedTimeSeries: []TestTimeSeries{
				{
					TestDimensions: TestDimensions{
						Name: "test.metric.count",
						Tags: []string{"env:staging"},
					},
					Type:      Gauge,
					Timestamp: 1000000000,
					Value:     50,
				},
				{
					TestDimensions: TestDimensions{
						Name: "test.metric.sum",
						Tags: []string{"env:staging"},
					},
					Type:      Gauge,
					Timestamp: 1000000000,
					Value:     250.0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := translatorConfig{
				Quantiles: tt.quantiles,
			}
			mapper := newLossLessMapper(cfg, zap.NewNop())

			slice := pmetric.NewSummaryDataPointSlice()
			tt.setupSlice(slice)

			dims := &Dimensions{name: "test.metric"}
			consumer := newTestConsumer()

			mapper.MapSummaryMetrics(context.Background(), &consumer, dims, slice)

			require.Len(t, consumer.data.Metrics.TimeSeries, len(tt.expectedTimeSeries))
			for i, expected := range tt.expectedTimeSeries {
				actual := consumer.data.Metrics.TimeSeries[i]
				assert.Equal(t, expected.Name, actual.Name)
				assert.Equal(t, expected.Type, actual.Type)
				assert.Equal(t, expected.Timestamp, actual.Timestamp)
				assert.Equal(t, expected.Value, actual.Value)
				if expected.Tags != nil {
					assert.ElementsMatch(t, expected.Tags, actual.Tags)
				}
			}
		})
	}
}

func TestLossLessMapperMapHistogramMetrics(t *testing.T) {
	cfg := translatorConfig{}
	mapper := newLossLessMapper(cfg, zap.NewNop())

	slice := pmetric.NewHistogramDataPointSlice()
	dp := slice.AppendEmpty()
	dp.SetCount(10)
	dp.SetSum(100.0)
	dp.ExplicitBounds().Append(1, 5, 10)
	dp.BucketCounts().Append(2, 3, 4, 1)
	dp.SetTimestamp(pcommon.Timestamp(1000000000))

	dims := &Dimensions{name: "test.histogram"}
	consumer := &histogramTestConsumer{}

	err := mapper.MapHistogramMetrics(context.Background(), consumer, dims, slice, true)
	require.NoError(t, err)

	// Verify ConsumeExplicitBoundHistogram was called
	assert.True(t, consumer.explicitBoundCalled)
	assert.Equal(t, "test.histogram", consumer.dims.Name())
	assert.Equal(t, 1, consumer.slice.Len())
}

func TestLossLessMapperMapExponentialHistogramMetrics(t *testing.T) {
	cfg := translatorConfig{}
	mapper := newLossLessMapper(cfg, zap.NewNop())

	slice := pmetric.NewExponentialHistogramDataPointSlice()
	dp := slice.AppendEmpty()
	dp.SetCount(10)
	dp.SetSum(100.0)
	dp.SetScale(2)
	dp.SetTimestamp(pcommon.Timestamp(1000000000))

	dims := &Dimensions{name: "test.exp_histogram"}
	consumer := &histogramTestConsumer{}

	mapper.MapExponentialHistogramMetrics(context.Background(), consumer, dims, slice, true)

	// Verify ConsumeExponentialHistogram was called
	assert.True(t, consumer.exponentialCalled)
	assert.Equal(t, "test.exp_histogram", consumer.expDims.Name())
	assert.Equal(t, 1, consumer.expSlice.Len())
}

// histogramTestConsumer is a test consumer that tracks histogram method calls
type histogramTestConsumer struct {
	testConsumer
	explicitBoundCalled bool
	exponentialCalled   bool
	dims                *Dimensions
	slice               pmetric.HistogramDataPointSlice
	expDims             *Dimensions
	expSlice            pmetric.ExponentialHistogramDataPointSlice
}

func (h *histogramTestConsumer) ConsumeExplicitBoundHistogram(
	_ context.Context,
	dims *Dimensions,
	slice pmetric.HistogramDataPointSlice,
) {
	h.explicitBoundCalled = true
	h.dims = dims
	h.slice = slice
}

func (h *histogramTestConsumer) ConsumeExponentialHistogram(
	_ context.Context,
	dims *Dimensions,
	slice pmetric.ExponentialHistogramDataPointSlice,
) {
	h.exponentialCalled = true
	h.expDims = dims
	h.expSlice = slice
}
