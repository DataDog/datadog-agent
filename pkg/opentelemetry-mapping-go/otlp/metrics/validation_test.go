// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

func TestValidateExpHistogramDataPoint(t *testing.T) {
	// The limit is the agent sketch's own capacity, not a policy number: binLimit
	// bins of uint16 each.
	require.Equal(t, uint64(quantile.Default().MaxCount()), sketchMaxObservationCount)
	sketchMax := sketchMaxObservationCount

	tests := []struct {
		name     string
		setup    func(dp pmetric.ExponentialHistogramDataPoint)
		reason   string
		badCount uint64
		drop     bool
	}{
		{
			name: "plausible point passes",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.SetZeroCount(1)
				dp.Positive().BucketCounts().Append(4, 5)
			},
		},
		{
			name: "no recorded value",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.SetFlags(dp.Flags().WithNoRecordedValue(true))
			},
			reason: dropReasonNoRecordedValue,
			drop:   true,
		},
		{
			name: "zero count bucket above limit",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.SetZeroCount(sketchMax + 1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 1,
			drop:     true,
		},
		{
			name: "positive bucket above limit",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.Positive().BucketCounts().Append(1, sketchMax+1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 1,
			drop:     true,
		},
		{
			name: "negative bucket above limit",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.Negative().BucketCounts().Append(1, sketchMax+1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 1,
			drop:     true,
		},
		{
			name: "bucket count exactly at limit passes",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(sketchMax)
				dp.Positive().BucketCounts().Append(sketchMax)
			},
		},
		{
			name: "total count above limit",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(sketchMax + 1)
				dp.Positive().BucketCounts().Append(1, 2)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 1,
			drop:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := pmetric.NewExponentialHistogramDataPoint()
			tt.setup(dp)

			reason, badCount, drop := validateExpHistogramDataPoint(dp, sketchMaxObservationCount)

			assert.Equal(t, tt.drop, drop)
			assert.Equal(t, tt.reason, reason)
			assert.Equal(t, tt.badCount, badCount)
		})
	}
}

// appendHistogramPoint adds a single-bound histogram point whose second bucket
// holds count observations.
func appendHistogramPoint(slice pmetric.HistogramDataPointSlice, ts uint64, count uint64, sum float64) {
	dp := slice.AppendEmpty()
	dp.SetCount(count)
	dp.SetSum(sum)
	dp.ExplicitBounds().Append(1)
	dp.BucketCounts().Append(0, count)
	dp.SetTimestamp(pcommon.Timestamp(ts))
}

// seriesTotal returns the sum of the time series whose name ends in suffix. It is
// a sum because a histogram emits one .bucket series per bucket.
func seriesTotal(t *testing.T, consumer *testConsumer, suffix string) float64 {
	t.Helper()

	found := false
	total := 0.0
	for _, series := range consumer.data.Metrics.TimeSeries {
		if strings.HasSuffix(series.Name, suffix) {
			found = true
			total += series.Value
		}
	}

	require.True(t, found, "no time series ending in %q, got %v", suffix, consumer.data.Metrics.TimeSeries)
	return total
}

func TestDefaultMapperSketchCapacity(t *testing.T) {
	distributions := translatorConfig{
		HistMode:                  HistogramModeDistributions,
		SendHistogramAggregations: true,
	}
	sketchMax := sketchMaxObservationCount

	t.Run("delta bucket above the limit drops the sketch", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendHistogramPoint(slice, 1_000_000_000, sketchMax+1, 1)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Empty(t, consumer.data.Metrics.Sketches)

		require.Equal(t, 1, logs.Len())
		entry := logs.All()[0]
		assert.Contains(t, entry.Message, "bucket count too high")
		assert.Equal(t, "test.histogram", entry.ContextMap()["metric name"])
		assert.Equal(t, sketchMax+1, entry.ContextMap()["bucket count"])
		assert.Equal(t, sketchMax, entry.ContextMap()["limit"])
	})

	t.Run("cumulative lifetime count above the limit is kept", func(t *testing.T) {
		// The regression this guards against: a cumulative point carries the counts of
		// the whole process lifetime, but only the delta reaches the sketch. Checking
		// the raw counts would silence the series forever once the lifetime total
		// passes the limit.
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendHistogramPoint(slice, 1_000_000_000, sketchMax+1_000, 1)
		appendHistogramPoint(slice, 2_000_000_000, sketchMax+1_010, 2)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, false))

		require.Len(t, consumer.data.Metrics.Sketches, 1, "the second point has a delta of 10")
		assert.Equal(t, int64(10), consumer.data.Metrics.Sketches[0].Summary.Cnt)
		assert.Equal(t, float64(10), seriesTotal(t, &consumer, ".count"))
		assert.Zero(t, logs.Len())
	})

	t.Run("cumulative delta above the limit drops only that point", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendHistogramPoint(slice, 1_000_000_000, 10, 1)
		appendHistogramPoint(slice, 2_000_000_000, 10+sketchMax+1, 2)
		appendHistogramPoint(slice, 3_000_000_000, 10+sketchMax+11, 3)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, false))

		require.Len(t, consumer.data.Metrics.Sketches, 1, "the third point is computed against the second one, not against a stale cache entry")
		assert.Equal(t, int64(10), consumer.data.Metrics.Sketches[0].Summary.Cnt)
		assert.Equal(t, 1, logs.Len())
	})

	t.Run("counters mode is untouched", func(t *testing.T) {
		// Buckets become plain Count series here, no sketch bins are allocated, so the
		// sketch limit must not apply.
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.NewNop(), translatorConfig{
			HistMode:                  HistogramModeCounters,
			SendHistogramAggregations: true,
		})

		slice := pmetric.NewHistogramDataPointSlice()
		appendHistogramPoint(slice, 1_000_000_000, sketchMax+1, 1)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Empty(t, consumer.data.Metrics.Sketches)
		assert.Equal(t, float64(sketchMax+1), seriesTotal(t, &consumer, ".bucket"))
		assert.Equal(t, float64(sketchMax+1), seriesTotal(t, &consumer, ".count"))
	})

	t.Run("nobuckets mode is untouched", func(t *testing.T) {
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.NewNop(), translatorConfig{
			HistMode:                  HistogramModeNoBuckets,
			SendHistogramAggregations: true,
		})

		slice := pmetric.NewHistogramDataPointSlice()
		appendHistogramPoint(slice, 1_000_000_000, sketchMax+1, 1)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Empty(t, consumer.data.Metrics.Sketches)
		assert.Equal(t, float64(sketchMax+1), seriesTotal(t, &consumer, ".count"))
	})

	t.Run("warning fires once per metric name", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		for i := uint64(1); i <= 3; i++ {
			appendHistogramPoint(slice, i*1_000_000_000, sketchMax+1, float64(i))
		}

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Empty(t, consumer.data.Metrics.Sketches)
		assert.Equal(t, 1, logs.Len(), "three dropped sketches, one warning")
	})

	t.Run("plausible histogram is untouched", func(t *testing.T) {
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.NewNop(), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetCount(10)
		dp.SetSum(100)
		dp.ExplicitBounds().Append(1, 5)
		dp.BucketCounts().Append(2, 3, 5)
		dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Len(t, consumer.data.Metrics.Sketches, 1)
		assert.Equal(t, float64(10), seriesTotal(t, &consumer, ".count"))
	})

	t.Run("exponential histogram above the limit is dropped", func(t *testing.T) {
		// Exponential points are validated at the top of the loop, so the whole point
		// goes, aggregates included: only delta points get here and their raw counts
		// are the ones inserted.
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewExponentialHistogramDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetCount(10)
		dp.SetSum(1)
		dp.Positive().BucketCounts().Append(sketchMax + 1)
		dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))

		consumer := newTestConsumer()
		m.MapExponentialHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.exp_histogram"}, slice, true)

		assert.Empty(t, consumer.data.Metrics.Sketches)
		assert.Empty(t, consumer.data.Metrics.TimeSeries)
		assert.Equal(t, 1, logs.Len())
	})

	t.Run("exponential histogram with no recorded value", func(t *testing.T) {
		// This check did not exist on the exponential path before the guard.
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.NewNop(), distributions)

		slice := pmetric.NewExponentialHistogramDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetCount(10)
		dp.SetSum(1)
		dp.Positive().BucketCounts().Append(4, 6)
		dp.SetFlags(dp.Flags().WithNoRecordedValue(true))
		dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))

		consumer := newTestConsumer()
		m.MapExponentialHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.exp_histogram"}, slice, true)

		assert.Empty(t, consumer.data.Metrics.Sketches)
		assert.Empty(t, consumer.data.Metrics.TimeSeries)
	})
}
