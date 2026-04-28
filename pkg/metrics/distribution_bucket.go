// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// DistributionBucket represents an explicit bucket that should be inserted into
// a distribution sketch as a weighted value, without interpolation.
type DistributionBucket struct {
	Name            string
	Count           int64
	LowerBound      float64
	UpperBound      float64
	Monotonic       bool
	Tags            []string
	Host            string
	Timestamp       float64
	FlushFirstValue bool
	Source          MetricSource
}

// GetName returns the bucket name.
func (m *DistributionBucket) GetName() string {
	return m.Name
}

// GetHost returns the bucket host.
func (m *DistributionBucket) GetHost() string {
	return m.Host
}

// GetTags returns the bucket tags.
func (m *DistributionBucket) GetTags(_, metricBuffer tagset.TagsAccumulator, _ tagger.Component) {
	// DistributionBucket currently only comes from checks, so returning the
	// metric tags directly is enough for context tracking.
	metricBuffer.Append(m.Tags...)
}

// GetMetricType implements MetricSampleContext#GetMetricType.
func (m *DistributionBucket) GetMetricType() MetricType {
	return DistributionType
}

// IsNoIndex returns if the metric must not be indexed.
func (m *DistributionBucket) IsNoIndex() bool {
	return false
}

// GetSource returns the currently set MetricSource.
func (m *DistributionBucket) GetSource() MetricSource {
	return m.Source
}
