// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import "github.com/DataDog/datadog-agent/pkg/util"

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
func (m *HistogramBucket) GetTags(tb *util.TagsBuilder) {
	// Other 'GetTags' methods for metrics support origin detections. Since
	// HistogramBucket only come, for now, from checks we can simply return
	// tags.
	tb.Append(m.Tags...)
}
