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

// ExplicitBucketHistogramSource is the read side consumed by the serializer.
type ExplicitBucketHistogramSource interface {
	MoveNext() bool
	Current() *ExplicitBucketHistogramSeries
	Count() uint64
	WaitForValue() bool
}
