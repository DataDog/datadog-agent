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

// ExponentialHistogramSource is the read side consumed by the serializer.
type ExponentialHistogramSource interface {
	MoveNext() bool
	Current() *ExponentialHistogramSeries
	Count() uint64
	WaitForValue() bool
}
