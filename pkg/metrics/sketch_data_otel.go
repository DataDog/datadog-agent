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
	_ SketchData = (*ExplicitBoundHistogramPoint)(nil)
	_ SketchData = (*ExponentialHistogramPoint)(nil)
)

// ExplicitBoundHistogramPoint wraps a pmetric.HistogramDataPoint
// to satisfy the SketchData interface.
type ExplicitBoundHistogramPoint struct {
	Point pmetric.HistogramDataPoint
}

// Kind returns SketchKindExplicitBound.
func (h *ExplicitBoundHistogramPoint) Kind() SketchKind { return SketchKindExplicitBound }

// Cols is not applicable; returns nil, nil.
func (h *ExplicitBoundHistogramPoint) Cols() ([]int32, []uint32) { return nil, nil }

// BasicStats is not applicable; returns zeros.
func (h *ExplicitBoundHistogramPoint) BasicStats() (int64, float64, float64, float64, float64) {
	return 0, 0, 0, 0, 0
}

// SummaryValues returns min, max, sum from the histogram data point.
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
// to satisfy the SketchData interface.
type ExponentialHistogramPoint struct {
	Point pmetric.ExponentialHistogramDataPoint
}

// Kind returns SketchKindExponential.
func (h *ExponentialHistogramPoint) Kind() SketchKind { return SketchKindExponential }

// Cols is not applicable; returns nil, nil.
func (h *ExponentialHistogramPoint) Cols() ([]int32, []uint32) { return nil, nil }

// BasicStats is not applicable; returns zeros.
func (h *ExponentialHistogramPoint) BasicStats() (int64, float64, float64, float64, float64) {
	return 0, 0, 0, 0, 0
}

// SummaryValues returns min, max, sum from the exponential histogram data point.
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
