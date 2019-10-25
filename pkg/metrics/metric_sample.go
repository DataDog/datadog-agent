// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

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
	// NOTE: DistributionType is in development and is NOT supported
	DistributionType
)

// DistributionMetricTypes contains the MetricTypes that are used for percentiles
var DistributionMetricTypes = map[MetricType]struct{}{
	DistributionType: {},
}

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
	default:
		return ""
	}
}

// MetricSampleContext allows to access a sample context data
type MetricSampleContext interface {
	GetName() string
	GetHost() string
	GetTags() []string
}

// MetricSample represents a raw metric sample
type MetricSample struct {
	Name       string
	Value      float64
	RawValue   string
	Mtype      MetricType
	Tags       []string
	Host       string
	SampleRate float64
	Timestamp  float64
}

// MetricSampleValue holds the value of a metric sample
type MetricSampleValue struct {
	Value      float64
	RawValue   string
	SampleRate float64
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
func (m *MetricSample) GetTags() []string {
	return m.Tags
}

// Copy returns a deep copy of the m MetricSample
func (m *MetricSample) Copy() *MetricSample {
	dst := &MetricSample{}
	*dst = *m
	dst.Tags = make([]string, len(m.Tags))
	copy(dst.Tags, m.Tags)
	return dst
}
