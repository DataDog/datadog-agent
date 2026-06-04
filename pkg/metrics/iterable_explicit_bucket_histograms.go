// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// ExplicitBucketHistogramSeries holds a single named explicit-bucket histogram metric series.
// Each Points entry is a raw OTel HistogramDataPoint; per-point attributes remain inside the
// data point for lossless forwarding.  EnrichmentTags carries resource-level and scope-level
// attributes that should be merged at serialisation time.
type ExplicitBucketHistogramSeries struct {
	Name           string
	EnrichmentTags tagset.CompositeTags
	Host           string
	Interval       int64
	Points         []pmetric.HistogramDataPoint
	Source         MetricSource
}

// ExplicitBucketHistogramSink is the write side of an iterable explicit-bucket histogram stream.
type ExplicitBucketHistogramSink interface {
	Append(*ExplicitBucketHistogramSeries)
}

// ExplicitBucketHistogramSource is the read side consumed by the serializer.
type ExplicitBucketHistogramSource interface {
	MoveNext() bool
	Current() *ExplicitBucketHistogramSeries
	Count() uint64
	WaitForValue() bool
}

// IterableExplicitBucketHistograms is a specialisation of iterableMetrics for explicit-bucket histograms.
type IterableExplicitBucketHistograms struct {
	iterableMetrics
}

// NewIterableExplicitBucketHistograms creates a new instance of *IterableExplicitBucketHistograms.
//
// `callback` is called in the context of the sender's goroutine each time `Append` is called.
func NewIterableExplicitBucketHistograms(callback func(*ExplicitBucketHistogramSeries), chanSize int, bufferSize int) *IterableExplicitBucketHistograms {
	return &IterableExplicitBucketHistograms{
		iterableMetrics: *newIterableMetric(func(value interface{}) {
			callback(value.(*ExplicitBucketHistogramSeries))
		}, chanSize, bufferSize),
	}
}

// WaitForValue waits until a value is available for MoveNext() or until senderStopped() is called.
// Returns true if a value is available, false otherwise.
func (it *IterableExplicitBucketHistograms) WaitForValue() bool {
	return it.iterableMetrics.WaitForValue()
}

var _ ExplicitBucketHistogramSink = (*IterableExplicitBucketHistograms)(nil)

// Append appends an explicit-bucket histogram series.
func (it *IterableExplicitBucketHistograms) Append(s *ExplicitBucketHistogramSeries) {
	it.iterableMetrics.Append(s)
}

// Current returns the current explicit-bucket histogram series.
func (it *IterableExplicitBucketHistograms) Current() *ExplicitBucketHistogramSeries {
	return it.iterableMetrics.Current().(*ExplicitBucketHistogramSeries)
}
