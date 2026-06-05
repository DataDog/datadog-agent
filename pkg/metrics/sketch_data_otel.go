// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metrics

import (
	"math"

	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	_ ExplicitBoundProvider = (*ExplicitBoundHistogramPoint)(nil)
	_ ExponentialProvider   = (*ExponentialHistogramPoint)(nil)
)

// ExplicitBoundHistogramPoint wraps a pmetric.HistogramDataPoint
// and satisfies ExplicitBoundProvider.
type ExplicitBoundHistogramPoint struct {
	Point pmetric.HistogramDataPoint
}

func (h *ExplicitBoundHistogramPoint) ExplicitBounds() []float64 {
	return h.Point.ExplicitBounds().AsRaw()
}
func (h *ExplicitBoundHistogramPoint) BucketCounts() []uint64 {
	return h.Point.BucketCounts().AsRaw()
}
func (h *ExplicitBoundHistogramPoint) Count() uint64 { return h.Point.Count() }
func (h *ExplicitBoundHistogramPoint) HasSum() bool  { return h.Point.HasSum() }
func (h *ExplicitBoundHistogramPoint) Sum() float64  { return h.Point.Sum() }
func (h *ExplicitBoundHistogramPoint) HasMin() bool  { return h.Point.HasMin() }
func (h *ExplicitBoundHistogramPoint) Min() float64  { return h.Point.Min() }
func (h *ExplicitBoundHistogramPoint) HasMax() bool  { return h.Point.HasMax() }
func (h *ExplicitBoundHistogramPoint) Max() float64  { return h.Point.Max() }

func (h *ExplicitBoundHistogramPoint) SummaryValues() (min, max, sum float64) {
	if h.Point.HasMin() {
		min = h.Point.Min()
	}
	if h.Point.HasMax() {
		max = h.Point.Max()
	}
	if h.Point.HasSum() {
		sum = h.Point.Sum()
	}
	return min, max, sum
}

// ExponentialHistogramPoint wraps a pmetric.ExponentialHistogramDataPoint
// and satisfies ExponentialProvider.
type ExponentialHistogramPoint struct {
	Point pmetric.ExponentialHistogramDataPoint
}

func (h *ExponentialHistogramPoint) Scale() int32      { return h.Point.Scale() }
func (h *ExponentialHistogramPoint) ZeroCount() uint64 { return h.Point.ZeroCount() }
func (h *ExponentialHistogramPoint) PositiveOffset() int32 {
	return h.Point.Positive().Offset()
}
func (h *ExponentialHistogramPoint) PositiveBucketCounts() []uint64 {
	return h.Point.Positive().BucketCounts().AsRaw()
}
func (h *ExponentialHistogramPoint) NegativeOffset() int32 {
	return h.Point.Negative().Offset()
}
func (h *ExponentialHistogramPoint) NegativeBucketCounts() []uint64 {
	return h.Point.Negative().BucketCounts().AsRaw()
}
func (h *ExponentialHistogramPoint) Count() uint64 { return h.Point.Count() }
func (h *ExponentialHistogramPoint) HasSum() bool  { return h.Point.HasSum() }
func (h *ExponentialHistogramPoint) Sum() float64  { return h.Point.Sum() }
func (h *ExponentialHistogramPoint) HasMin() bool  { return h.Point.HasMin() }
func (h *ExponentialHistogramPoint) Min() float64  { return h.Point.Min() }
func (h *ExponentialHistogramPoint) HasMax() bool  { return h.Point.HasMax() }
func (h *ExponentialHistogramPoint) Max() float64  { return h.Point.Max() }

func (h *ExponentialHistogramPoint) SummaryValues() (min, max, sum float64) {
	min = math.Inf(1)
	if h.Point.HasMin() {
		min = h.Point.Min()
	}
	if h.Point.HasMax() {
		max = h.Point.Max()
	}
	if h.Point.HasSum() {
		sum = h.Point.Sum()
	}
	return min, max, sum
}
