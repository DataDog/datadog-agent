// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// HistogramBucket represents a prometheus/openmetrics histogram bucket
type HistogramBucket struct {
	Name            string
	Value           int64
	LowerBound      float64
	UpperBound      float64
	Monotonic       bool
	Tags            []string
	Host            string
	Timestamp       float64
	FlushFirstValue bool
	Source          MetricSource
}

// Implement the MetricSampleContext interface

// GetName returns the bucket name
func (m *HistogramBucket) GetName() string {
	return m.Name
}

// GetHost returns the bucket host
func (m *HistogramBucket) GetHost() string {
	return m.Host
}

// GetTags returns the bucket tags.
func (m *HistogramBucket) GetTags(_, metricBuffer tagset.TagsAccumulator, _ tagger.Component) {
	// Other 'GetTags' methods for metrics support origin detections. Since
	// HistogramBucket only come, for now, from checks we can simply return
	// tags.
	metricBuffer.Append(m.Tags...)
}

// GetMetricType implements MetricSampleContext#GetMetricType.
func (m *HistogramBucket) GetMetricType() MetricType {
	return HistogramType
}

// IsNoIndex returns if the metric must not be indexed.
func (m *HistogramBucket) IsNoIndex() bool {
	return false
}

// GetSource returns the currently set MetricSource
func (m *HistogramBucket) GetSource() MetricSource {
	return m.Source
}
