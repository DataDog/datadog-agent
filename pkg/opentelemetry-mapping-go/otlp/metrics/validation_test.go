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

	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

func TestValidateHistogramDataPoint(t *testing.T) {
	// The limit is the agent sketch's own capacity, not a policy number: binLimit
	// bins of uint16 each.
	require.Equal(t, uint64(quantile.Default().MaxCount()), sketchMaxObservationCount)
	sketchMax := sketchMaxObservationCount

	tests := []struct {
		name     string
		setup    func(dp pmetric.HistogramDataPoint)
		reason   string
		badCount uint64
		drop     bool
	}{
		{
			name: "plausible point passes",
			setup: func(dp pmetric.HistogramDataPoint) {
				dp.SetCount(10)
				dp.ExplicitBounds().Append(1, 5)
				dp.BucketCounts().Append(2, 3, 5)
			},
		},
		{
			name: "no recorded value",
			setup: func(dp pmetric.HistogramDataPoint) {
				dp.SetCount(10)
				dp.SetFlags(dp.Flags().WithNoRecordedValue(true))
			},
			reason: dropReasonNoRecordedValue,
			drop:   true,
		},
		{
			name: "bucket count above limit",
			setup: func(dp pmetric.HistogramDataPoint) {
				dp.SetCount(sketchMax + 1)
				dp.ExplicitBounds().Append(1)
				dp.BucketCounts().Append(1, sketchMax+1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 1,
			drop:     true,
		},
		{
			name: "bucket count exactly at limit passes",
			setup: func(dp pmetric.HistogramDataPoint) {
				dp.SetCount(sketchMax)
				dp.ExplicitBounds().Append(1)
				dp.BucketCounts().Append(0, sketchMax)
			},
		},
		{
			name: "total count above limit, buckets below it",
			setup: func(dp pmetric.HistogramDataPoint) {
				// The per-bucket limit alone does not bound how many bins the whole
				// point produces, so the total is checked too.
				dp.SetCount(sketchMax + 2)
				dp.ExplicitBounds().Append(1)
				dp.BucketCounts().Append(sketchMax/2+1, sketchMax/2+1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 2,
			drop:     true,
		},
		{
			name: "zero count passes",
			setup: func(dp pmetric.HistogramDataPoint) {
				// An idle point still carries .count/.sum aggregates, and a cumulative
				// one primes the delta cache that the next non-empty point needs.
				dp.SetCount(0)
				dp.ExplicitBounds().Append(1)
				dp.BucketCounts().Append(0, 0)
			},
		},
		{
			name: "zero count with an oversized bucket is dropped",
			setup: func(dp pmetric.HistogramDataPoint) {
				// A count of 0 with a non-empty bucket is malformed, and the delta path
				// inserts bucket counts regardless of the total, so the guard must still
				// look at the buckets.
				dp.SetCount(0)
				dp.ExplicitBounds().Append(1)
				dp.BucketCounts().Append(0, sketchMax+1)
			},
			reason:   dropReasonBucketCountTooHigh,
			badCount: sketchMax + 1,
			drop:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := pmetric.NewHistogramDataPoint()
			tt.setup(dp)

			reason, badCount, drop := validateHistogramDataPoint(dp, sketchMaxObservationCount)

			assert.Equal(t, tt.drop, drop)
			assert.Equal(t, tt.reason, reason)
			assert.Equal(t, tt.badCount, badCount)
		})
	}
}

func TestValidateExpHistogramDataPoint(t *testing.T) {
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

func TestDefaultMapperDropsOversizedHistograms(t *testing.T) {
	cfg := translatorConfig{
		HistMode:                  HistogramModeDistributions,
		SendHistogramAggregations: true,
	}

	t.Run("explicit bounds histogram", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), cfg)

		slice := pmetric.NewHistogramDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetCount(sketchMaxObservationCount + 1)
		dp.SetSum(1)
		dp.ExplicitBounds().Append(1)
		dp.BucketCounts().Append(0, sketchMaxObservationCount+1)
		dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Empty(t, consumer.data.Metrics.Sketches)
		assert.Empty(t, consumer.data.Metrics.TimeSeries, "the aggregates of an implausible point are implausible too")

		require.Equal(t, 1, logs.Len())
		entry := logs.All()[0]
		assert.Contains(t, entry.Message, "bucket count too high")
		assert.Equal(t, "test.histogram", entry.ContextMap()["metric name"])
		assert.Equal(t, sketchMaxObservationCount+1, entry.ContextMap()["bucket count"])
		assert.Equal(t, sketchMaxObservationCount, entry.ContextMap()["limit"])
	})

	t.Run("exponential histogram", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), cfg)

		slice := pmetric.NewExponentialHistogramDataPointSlice()
		dp := slice.AppendEmpty()
		dp.SetCount(10)
		dp.SetSum(1)
		dp.Positive().BucketCounts().Append(sketchMaxObservationCount + 1)
		dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))

		consumer := newTestConsumer()
		m.MapExponentialHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.exp_histogram"}, slice, true)

		assert.Empty(t, consumer.data.Metrics.Sketches)
		assert.Empty(t, consumer.data.Metrics.TimeSeries)
		assert.Equal(t, 1, logs.Len())
	})

	t.Run("warning fires once per metric name", func(t *testing.T) {
		core, logs := observer.New(zapcore.WarnLevel)
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.New(core), cfg)

		slice := pmetric.NewHistogramDataPointSlice()
		for i := 0; i < 3; i++ {
			dp := slice.AppendEmpty()
			dp.SetCount(sketchMaxObservationCount + 1)
			dp.BucketCounts().Append(sketchMaxObservationCount + 1)
			dp.SetTimestamp(pcommon.Timestamp(1_000_000_000))
		}

		consumer := newTestConsumer()
		require.NoError(t, m.MapHistogramMetrics(context.Background(), &consumer, &Dimensions{name: "test.histogram"}, slice, true))

		assert.Equal(t, 1, logs.Len(), "three dropped points, one warning")
	})

	t.Run("exponential histogram with no recorded value", func(t *testing.T) {
		// This check did not exist on the exponential path before the guard.
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.NewNop(), cfg)

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

	t.Run("plausible histogram is untouched", func(t *testing.T) {
		m := newDefaultMapper(newTTLCache(1800, 3600), zap.NewNop(), cfg)

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
		assert.NotEmpty(t, consumer.data.Metrics.TimeSeries)
	})
}
