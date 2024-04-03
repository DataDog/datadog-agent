// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// MetricType is the representation of an aggregator metric type
type MetricType int

// metric type constants enumeration
const (
	GaugeType MetricType = iota
	RateType
	CountType
	MonotonicCountType
	CounterType
	HistogramType
	HistorateType
	SetType
	DistributionType
	GaugeWithTimestampType
	CountWithTimestampType

	// NumMetricTypes is the number of metric types; must be the last item here
	NumMetricTypes
)

// DistributionMetricTypes contains the MetricTypes that are used for percentiles
var (
	DistributionMetricTypes = map[MetricType]struct{}{
		DistributionType: {},
	}
)

// EnrichTagsfn can be used to Enrich tags with origin detection tags.
type EnrichTagsfn func(tb tagset.TagsAccumulator, origin taggertypes.OriginInfo)

// String returns a string representation of MetricType
func (m MetricType) String() string {
	switch m {
	case GaugeType:
		return "Gauge"
	case RateType:
		return "Rate"
	case CountType:
		return "Count"
	case MonotonicCountType:
		return "MonotonicCount"
	case CounterType:
		return "Counter"
	case HistogramType:
		return "Histogram"
	case HistorateType:
		return "Historate"
	case SetType:
		return "Set"
	case DistributionType:
		return "Distribution"
	case GaugeWithTimestampType:
		return "GaugeWithTimestamp"
	case CountWithTimestampType:
		return "CountWithTimestamp"
	default:
		return ""
	}
}

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

// MetricSample represents a raw metric sample
type MetricSample struct {
	Name            string
	Value           float64
	RawValue        string
	Mtype           MetricType
	Tags            []string
	Host            string
	SampleRate      float64
	Timestamp       float64
	FlushFirstValue bool
	OriginInfo      taggertypes.OriginInfo
	ListenerID      string
	NoIndex         bool
	Source          MetricSource
}

// Implement the MetricSampleContext interface

// GetName returns the metric sample name
func (m *MetricSample) GetName() string {
	return m.Name
}

// GetHost returns the metric sample host
func (m *MetricSample) GetHost() string {
	return m.Host
}

// GetTags returns the metric sample tags
func (m *MetricSample) GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator, fn EnrichTagsfn) {
	metricBuffer.Append(m.Tags...)
	fn(taggerBuffer, m.OriginInfo)
}

// GetMetricType implements MetricSampleContext#GetMetricType.
func (m *MetricSample) GetMetricType() MetricType {
	return m.Mtype
}

// Copy returns a deep copy of the m MetricSample
func (m *MetricSample) Copy() *MetricSample {
	dst := &MetricSample{}
	*dst = *m
	dst.Tags = make([]string, len(m.Tags))
	copy(dst.Tags, m.Tags)
	return dst
}

// IsNoIndex returns true if the metric must not be indexed.
func (m *MetricSample) IsNoIndex() bool {
	return m.NoIndex
}

// GetSource returns the currently set MetricSource
func (m *MetricSample) GetSource() MetricSource {
	return m.Source
}
