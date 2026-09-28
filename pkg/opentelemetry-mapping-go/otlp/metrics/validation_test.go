// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"math"
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
		name   string
		setup  func(dp pmetric.ExponentialHistogramDataPoint)
		reason string
		// badCount is the number of observations counted when the limit was crossed.
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
			badCount: sketchMax + 2,
			drop:     true,
		},
		{
			name: "negative bucket above limit",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.Negative().BucketCounts().Append(1, sketchMax+1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 2,
			drop:     true,
		},
		{
			name: "zero count and buckets summing above limit",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.SetZeroCount(1)
				dp.Positive().BucketCounts().Append(sketchMax - 1)
				dp.Negative().BucketCounts().Append(1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 1,
			drop:     true,
		},
		{
			name: "zero count and buckets summing exactly to limit pass",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(sketchMax)
				dp.SetZeroCount(1)
				dp.Positive().BucketCounts().Append(sketchMax - 2)
				dp.Negative().BucketCounts().Append(1)
			},
		},
		{
			name: "sum past the uint64 range is reported saturated",
			setup: func(dp pmetric.ExponentialHistogramDataPoint) {
				dp.SetCount(10)
				dp.Positive().BucketCounts().Append(1, math.MaxUint64)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: math.MaxUint64,
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

// appendBucketsPoint adds a histogram point with one bucket per value in buckets,
// bounded at 1, 2, and so on. It declares count observations whatever the
// buckets hold, so that a test can make the two disagree.
func appendBucketsPoint(slice pmetric.HistogramDataPointSlice, ts, count uint64, buckets ...uint64) {
	dp := slice.AppendEmpty()
	dp.SetCount(count)
	dp.SetSum(1)
	for i := 1; i < len(buckets); i++ {
		dp.ExplicitBounds().Append(float64(i))
	}
	dp.BucketCounts().Append(buckets...)
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
		assert.Equal(t, float64(sketchMax+1), seriesTotal(t, &consumer, ".count"), "only the sketch is dropped")
		assert.Equal(t, float64(1), seriesTotal(t, &consumer, ".sum"))

		require.Equal(t, 1, logs.Len())
		entry := logs.All()[0]
		assert.Contains(t, entry.Message, "too many observations")
		assert.Equal(t, "test.histogram", entry.ContextMap()["metric name"])
		assert.Equal(t, sketchMax+1, entry.ContextMap()["count"])
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

	t.Run("cumulative count going down is not read as a huge count", func(t *testing.T) {
		// Without a start timestamp nothing else reveals the reset, and converting the
		// negative delta to uint64 gives an architecture-dependent count, close to 2^64
		// on amd64. A point without buckets would also carry it into the sketch, as the
		// single bucket built from its count.
		for _, tc := range []struct {
			name          string
			before, after []uint64
		}{
			{name: "with buckets", before: []uint64{0, 100}, after: []uint64{0, 10}},
			{name: "without buckets"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				core, logs := observer.New(zapcore.WarnLevel)
				m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

				slice := pmetric.NewHistogramDataPointSlice()
				appendBucketsPoint(slice, 1_000_000_000, 100, tc.before...)
				appendBucketsPoint(slice, 2_000_000_000, 10, tc.after...)

				consumer := newTestConsumer()
				require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, false))

				assert.Empty(t, consumer.data.Metrics.TimeSeries, "a reset has no delta to report")
				assert.Empty(t, consumer.data.Metrics.Sketches)
				assert.Zero(t, logs.Len())
			})
		}
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

// TestDefaultMapperSketchCapacityAcrossBuckets covers the limit on what all the
// buckets of a point put into the sketch together. The Count a point declares
// cannot stand in for it: OTLP requires Count to equal the sum of the buckets,
// but nothing enforces it, and it is the sum that decides how many bins the
// sketch allocates.
func TestDefaultMapperSketchCapacityAcrossBuckets(t *testing.T) {
	distributions := translatorConfig{
		HistMode:                  HistogramModeDistributions,
		SendHistogramAggregations: true,
	}
	sketchMax := sketchMaxObservationCount

	t.Run("explicit bounds buckets summing above the limit", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendBucketsPoint(slice, 1_000_000_000, 1, sketchMax, sketchMax, sketchMax)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Empty(t, consumer.data.Metrics.Sketches)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, 2*sketchMax, logs.All()[0].ContextMap()["count"],
			"the total when the limit was crossed, not the bucket that crossed it")
	})

	t.Run("a bucket above the limit is caught whatever Count says", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendBucketsPoint(slice, 1_000_000_000, 10, 0, sketchMax+1)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Empty(t, consumer.data.Metrics.Sketches)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, sketchMax+1, logs.All()[0].ContextMap()["count"])
	})

	t.Run("buckets summing exactly to the limit are kept", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendBucketsPoint(slice, 1_000_000_000, sketchMax, sketchMax/2, sketchMax-sketchMax/2)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		require.Len(t, consumer.data.Metrics.Sketches, 1)
		assert.Equal(t, int64(sketchMax), consumer.data.Metrics.Sketches[0].Summary.Cnt)
		assert.Zero(t, logs.Len())
	})

	t.Run("cumulative deltas summing above the limit drop only that point", func(t *testing.T) {
		// The declared Count does not move, so only the bucket deltas reveal the
		// observations the second point puts into the sketch.
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendBucketsPoint(slice, 1_000_000_000, 1, 0, 0)
		appendBucketsPoint(slice, 2_000_000_000, 1, sketchMax, sketchMax)
		appendBucketsPoint(slice, 3_000_000_000, 11, sketchMax+5, sketchMax+5)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, false))

		require.Len(t, consumer.data.Metrics.Sketches, 1, "the third point is computed against the second one, not against a stale cache entry")
		assert.Equal(t, int64(10), consumer.data.Metrics.Sketches[0].Summary.Cnt)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, 2*sketchMax, logs.All()[0].ContextMap()["count"])
	})

	t.Run("a negative cumulative delta does not make room for other buckets", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewHistogramDataPointSlice()
		appendBucketsPoint(slice, 1_000_000_000, 1, sketchMax, 0, 0)
		appendBucketsPoint(slice, 2_000_000_000, 1, 0, sketchMax, sketchMax)

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, false))

		assert.Empty(t, consumer.data.Metrics.Sketches)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, 2*sketchMax, logs.All()[0].ContextMap()["count"])
	})

	t.Run("exponential buckets summing above the limit", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), distributions)

		slice := pmetric.NewExponentialHistogramDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetCount(1)
		dp.SetSum(1)
		dp.Positive().BucketCounts().Append(sketchMax, sketchMax)
		dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))

		consumer := newTestConsumer()
		m.MapExponentialHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.exp_histogram"}, slice, true)

		assert.Empty(t, consumer.data.Metrics.Sketches)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, 2*sketchMax, logs.All()[0].ContextMap()["count"])
	})
}
