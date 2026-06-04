// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// ExponentialHistogramSeries holds a single named exponential histogram metric series.
// Each Points entry is a raw OTel ExponentialHistogramDataPoint; per-point attributes remain
// inside the data point for lossless forwarding.  EnrichmentTags carries resource-level and
// scope-level attributes that should be merged at serialisation time.
type ExponentialHistogramSeries struct {
	Name           string
	EnrichmentTags tagset.CompositeTags
	Host           string
	Interval       int64
	Points         []pmetric.ExponentialHistogramDataPoint
	Source         MetricSource
}

// ExponentialHistogramSink is the write side of an iterable exponential histogram stream.
type ExponentialHistogramSink interface {
	Append(*ExponentialHistogramSeries)
}

// ExponentialHistogramSource is the read side consumed by the serializer.
type ExponentialHistogramSource interface {
	MoveNext() bool
	Current() *ExponentialHistogramSeries
	Count() uint64
	WaitForValue() bool
}

// IterableExponentialHistograms is a specialisation of iterableMetrics for exponential histograms.
type IterableExponentialHistograms struct {
	iterableMetrics
}

// NewIterableExponentialHistograms creates a new instance of *IterableExponentialHistograms.
//
// `callback` is called in the context of the sender's goroutine each time `Append` is called.
func NewIterableExponentialHistograms(callback func(*ExponentialHistogramSeries), chanSize int, bufferSize int) *IterableExponentialHistograms {
	return &IterableExponentialHistograms{
		iterableMetrics: *newIterableMetric(func(value interface{}) {
			callback(value.(*ExponentialHistogramSeries))
		}, chanSize, bufferSize),
	}
}

// WaitForValue waits until a value is available for MoveNext() or until senderStopped() is called.
// Returns true if a value is available, false otherwise.
func (it *IterableExponentialHistograms) WaitForValue() bool {
	return it.iterableMetrics.WaitForValue()
}

var _ ExponentialHistogramSink = (*IterableExponentialHistograms)(nil)

// Append appends an exponential histogram series.
func (it *IterableExponentialHistograms) Append(s *ExponentialHistogramSeries) {
	it.iterableMetrics.Append(s)
}

// Current returns the current exponential histogram series.
func (it *IterableExponentialHistograms) Current() *ExponentialHistogramSeries {
	return it.iterableMetrics.Current().(*ExponentialHistogramSeries)
}
