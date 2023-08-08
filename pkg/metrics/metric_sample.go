// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics/model"
	"github.com/DataDog/datadog-agent/pkg/tagger"
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

// MetricSampleContext allows to access a sample context data
type (
	MetricSampleContext model.MetricSampleContext
	MetricSample        model.MetricSample
)

// GetTags returns the metric sample tags
func (m *MetricSample) GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator) {
	metricBuffer.Append(m.Tags...)
	tagger.EnrichTags(taggerBuffer, m.OriginFromUDS, m.OriginFromClient, m.Cardinality)
}
