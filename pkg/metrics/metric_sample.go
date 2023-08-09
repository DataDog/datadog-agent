// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics/model"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// MetricType is the representation of an aggregator metric type
type MetricType = model.MetricType

// metric type constants enumeration
const (
	GaugeType          = model.GaugeType
	RateType           = model.RateType
	CountType          = model.CountType
	MonotonicCountType = model.MonotonicCountType
	CounterType        = model.CounterType
	HistogramType      = model.HistogramType
	HistorateType      = model.HistorateType
	SetType            = model.SetType
	DistributionType   = model.DistributionType

	// NumMetricTypes is the number of metric types; must be the last item here
	NumMetricTypes = model.NumMetricTypes
)

// DistributionMetricTypes contains the MetricTypes that are used for percentiles
var (
	DistributionMetricTypes = model.DistributionMetricTypes
)

type EnrichTagsfn = model.EnrichTagsfn

// MetricSampleContext allows to access a sample context data
type MetricSampleContext interface {
	GetName() string
	GetHost() string

	// GetTags extracts metric tags for context tracking.
	//
	// Implementations should call `Append` or `AppendHashed` on the provided accumulators.
	// Tags from origin detection should be appended to taggerBuffer. Client-provided tags
	// should be appended to the metricBuffer.
	GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator, fn EnrichTagsfn)

	// GetMetricType returns the metric type for this metric.  This is used for telemetry.
	GetMetricType() MetricType

	// IsNoIndex returns true if the metric must not be indexed.
	IsNoIndex() bool

	// GetMetricSource returns the metric source for this metric. This is used to define the Origin
	GetSource() MetricSource
}

type MetricSample = model.MetricSample
