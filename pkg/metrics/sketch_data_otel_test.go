// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build otlp

package metrics

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestExplicitBoundHistogramPoint(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
	dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
	dp.SetCount(11)
	dp.SetSum(42.0)
	dp.SetMin(0.5)
	dp.SetMax(9.5)

	h := &ExplicitBoundHistogramPoint{Point: dp}

	var _ ExplicitBoundProvider = h

	assert.Equal(t, []float64{1, 5, 10}, h.ExplicitBounds())
	assert.Equal(t, []uint64{1, 3, 5, 2}, h.BucketCounts())
	assert.Equal(t, uint64(11), h.Count())
	assert.True(t, h.HasSum())
	assert.Equal(t, 42.0, h.Sum())
	assert.True(t, h.HasMin())
	assert.Equal(t, 0.5, h.Min())
	assert.True(t, h.HasMax())
	assert.Equal(t, 9.5, h.Max())
}

func TestExplicitBoundHistogramPoint_NoOptionalFields(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	dp.ExplicitBounds().FromRaw([]float64{10})
	dp.BucketCounts().FromRaw([]uint64{3, 7})
	dp.SetCount(10)

	h := &ExplicitBoundHistogramPoint{Point: dp}

	assert.False(t, h.HasSum())
	assert.False(t, h.HasMin())
	assert.False(t, h.HasMax())
	assert.Equal(t, uint64(10), h.Count())
}

func TestExplicitBoundHistogramPoint_SummaryValues(t *testing.T) {
	t.Run("all_set", func(t *testing.T) {
		dp := pmetric.NewHistogramDataPoint()
		dp.SetMin(1.0)
		dp.SetMax(10.0)
		dp.SetSum(42.0)

		h := &ExplicitBoundHistogramPoint{Point: dp}
		min, max, sum := h.SummaryValues()
		assert.Equal(t, 1.0, min)
		assert.Equal(t, 10.0, max)
		assert.Equal(t, 42.0, sum)
	})

	t.Run("none_set", func(t *testing.T) {
		dp := pmetric.NewHistogramDataPoint()

		h := &ExplicitBoundHistogramPoint{Point: dp}
		min, max, sum := h.SummaryValues()
		assert.Equal(t, 0.0, min)
		assert.Equal(t, 0.0, max)
		assert.Equal(t, 0.0, sum)
	})

	t.Run("partial", func(t *testing.T) {
		dp := pmetric.NewHistogramDataPoint()
		dp.SetSum(100.0)

		h := &ExplicitBoundHistogramPoint{Point: dp}
		min, max, sum := h.SummaryValues()
		assert.Equal(t, 0.0, min)
		assert.Equal(t, 0.0, max)
		assert.Equal(t, 100.0, sum)
	})
}

func TestExponentialHistogramPoint(t *testing.T) {
	dp := pmetric.NewExponentialHistogramDataPoint()
	dp.SetScale(4)
	dp.SetZeroCount(5)
	dp.Positive().SetOffset(2)
	dp.Positive().BucketCounts().FromRaw([]uint64{10, 20, 30})
	dp.Negative().SetOffset(1)
	dp.Negative().BucketCounts().FromRaw([]uint64{7, 8})
	dp.SetCount(80)
	dp.SetSum(200.0)
	dp.SetMin(0.1)
	dp.SetMax(99.9)

	h := &ExponentialHistogramPoint{Point: dp}

	var _ ExponentialProvider = h

	assert.Equal(t, int32(4), h.Scale())
	assert.Equal(t, uint64(5), h.ZeroCount())
	assert.Equal(t, int32(2), h.PositiveOffset())
	assert.Equal(t, []uint64{10, 20, 30}, h.PositiveBucketCounts())
	assert.Equal(t, int32(1), h.NegativeOffset())
	assert.Equal(t, []uint64{7, 8}, h.NegativeBucketCounts())
	assert.Equal(t, uint64(80), h.Count())
	assert.True(t, h.HasSum())
	assert.Equal(t, 200.0, h.Sum())
	assert.True(t, h.HasMin())
	assert.Equal(t, 0.1, h.Min())
	assert.True(t, h.HasMax())
	assert.Equal(t, 99.9, h.Max())
}

func TestExponentialHistogramPoint_EmptyBuckets(t *testing.T) {
	dp := pmetric.NewExponentialHistogramDataPoint()
	dp.SetScale(0)
	dp.SetZeroCount(3)
	dp.SetCount(3)

	h := &ExponentialHistogramPoint{Point: dp}

	assert.Equal(t, int32(0), h.Scale())
	assert.Equal(t, uint64(3), h.ZeroCount())
	assert.Equal(t, int32(0), h.PositiveOffset())
	require.Empty(t, h.PositiveBucketCounts())
	assert.Equal(t, int32(0), h.NegativeOffset())
	require.Empty(t, h.NegativeBucketCounts())
	assert.False(t, h.HasSum())
	assert.False(t, h.HasMin())
	assert.False(t, h.HasMax())
}

func TestExponentialHistogramPoint_SummaryValues(t *testing.T) {
	t.Run("all_set", func(t *testing.T) {
		dp := pmetric.NewExponentialHistogramDataPoint()
		dp.SetMin(1.0)
		dp.SetMax(10.0)
		dp.SetSum(42.0)

		h := &ExponentialHistogramPoint{Point: dp}
		min, max, sum := h.SummaryValues()
		assert.Equal(t, 1.0, min)
		assert.Equal(t, 10.0, max)
		assert.Equal(t, 42.0, sum)
	})

	t.Run("none_set", func(t *testing.T) {
		dp := pmetric.NewExponentialHistogramDataPoint()

		h := &ExponentialHistogramPoint{Point: dp}
		min, max, sum := h.SummaryValues()
		assert.Equal(t, math.Inf(1), min, "ExponentialHistogramPoint defaults min to +Inf")
		assert.Equal(t, 0.0, max)
		assert.Equal(t, 0.0, sum)
	})

	t.Run("partial_sum_only", func(t *testing.T) {
		dp := pmetric.NewExponentialHistogramDataPoint()
		dp.SetSum(100.0)

		h := &ExponentialHistogramPoint{Point: dp}
		min, max, sum := h.SummaryValues()
		assert.Equal(t, math.Inf(1), min, "min defaults to +Inf when not set")
		assert.Equal(t, 0.0, max)
		assert.Equal(t, 100.0, sum)
	})
}
