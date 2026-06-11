// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync/atomic"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
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
	GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator, tagger tagger.Component)

	// GetMetricType returns the metric type for this metric.  This is used for telemetry.
	GetMetricType() MetricType

	// IsNoIndex returns true if the metric must not be indexed.
	IsNoIndex() bool

	// GetMetricSource returns the metric source for this metric. This is used to define the Origin
	GetSource() MetricSource
}

// UnitMilliseconds is the unit string for timing metrics, as defined by the Datadog API.
const UnitMilliseconds = "millisecond"

// DogStatsDCompactIdentityState is shared by the experimental DogStatsD
// compact-identity cache and downstream consumers that can acknowledge they no
// longer need full descriptor fields on every row. It is intentionally tiny and
// atomic because parser workers and columnar workers run on different
// goroutines.
type DogStatsDCompactIdentityState struct {
	columnarDescriptorKnown atomic.Uint32
	columnarDescriptorRefs  [4]atomic.Uint64
}

func dogstatsdColumnarMetricTypeBit(mtype MetricType) uint32 {
	if mtype < 0 || mtype >= 32 {
		return 0
	}
	return 1 << uint(mtype)
}

func dogstatsdColumnarMetricTypeRefIndex(mtype MetricType) int {
	switch mtype {
	case GaugeType:
		return 0
	case CounterType:
		return 1
	case CountType:
		return 2
	case SetType:
		return 3
	default:
		return -1
	}
}

// ColumnarDescriptorKnown reports whether the columnar-v3 consumer has already
// observed descriptor metadata for this compact identity and metric type.
func (s *DogStatsDCompactIdentityState) ColumnarDescriptorKnown(mtype MetricType) bool {
	bit := dogstatsdColumnarMetricTypeBit(mtype)
	return s != nil && bit != 0 && s.columnarDescriptorKnown.Load()&bit != 0
}

// MarkColumnarDescriptorKnown records that columnar-v3 can resolve this compact
// identity and metric type without receiving descriptor strings on subsequent
// rows.
func (s *DogStatsDCompactIdentityState) MarkColumnarDescriptorKnown(mtype MetricType) {
	if s == nil {
		return
	}
	bit := dogstatsdColumnarMetricTypeBit(mtype)
	if bit == 0 {
		return
	}
	for {
		old := s.columnarDescriptorKnown.Load()
		if old&bit != 0 || s.columnarDescriptorKnown.CompareAndSwap(old, old|bit) {
			return
		}
	}
}

// ColumnarDescriptorRef returns the columnar-v3 shard-local descriptor slot
// last acknowledged for this compact identity and metric type. The generation
// must be validated by the columnar shard before use.
func (s *DogStatsDCompactIdentityState) ColumnarDescriptorRef(mtype MetricType) (int, uint32, bool) {
	if s == nil {
		return 0, 0, false
	}
	idx := dogstatsdColumnarMetricTypeRefIndex(mtype)
	if idx < 0 {
		return 0, 0, false
	}
	packed := s.columnarDescriptorRefs[idx].Load()
	if packed == 0 {
		return 0, 0, false
	}
	descriptorID := int(uint32(packed)) - 1
	generation := uint32(packed >> 32)
	return descriptorID, generation, generation != 0
}

// MarkColumnarDescriptorRef records the columnar-v3 shard-local descriptor slot
// for this compact identity and metric type. The descriptor ID is packed as +1
// so zero remains the missing-reference sentinel.
func (s *DogStatsDCompactIdentityState) MarkColumnarDescriptorRef(mtype MetricType, descriptorID int, generation uint32) {
	if s == nil || descriptorID < 0 || generation == 0 {
		return
	}
	idx := dogstatsdColumnarMetricTypeRefIndex(mtype)
	if idx < 0 {
		return
	}
	packed := (uint64(generation) << 32) | uint64(uint32(descriptorID+1))
	s.columnarDescriptorRefs[idx].Store(packed)
	s.MarkColumnarDescriptorKnown(mtype)
}

// ClearColumnarDescriptorKnown clears the downstream descriptor acknowledgement
// for one metric type.
func (s *DogStatsDCompactIdentityState) ClearColumnarDescriptorKnown(mtype MetricType) {
	if s == nil {
		return
	}
	bit := dogstatsdColumnarMetricTypeBit(mtype)
	if bit == 0 {
		return
	}
	if idx := dogstatsdColumnarMetricTypeRefIndex(mtype); idx >= 0 {
		s.columnarDescriptorRefs[idx].Store(0)
	}
	for {
		old := s.columnarDescriptorKnown.Load()
		if old&bit == 0 || s.columnarDescriptorKnown.CompareAndSwap(old, old&^bit) {
			return
		}
	}
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
	Timestamp       float64 // Seconds since epoch (accepts fractional seconds)
	FlushFirstValue bool
	OriginInfo      taggertypes.OriginInfo
	ListenerID      string
	NoIndex         bool
	Source          MetricSource
	Unit            string

	// DogStatsDTagsetID is an experimental parser-local compact identifier for
	// an exact client-provided DogStatsD tagset. A value of 0 means no stable
	// compact tagset identity is available for this sample.
	DogStatsDTagsetID uint64
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
func (m *MetricSample) GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator, tagger tagger.Component) {
	metricBuffer.Append(m.Tags...)
	tagger.EnrichTags(taggerBuffer, m.OriginInfo)
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

// GetValue returns the metric sample value, satisfying observer.MetricView.
func (m *MetricSample) GetValue() float64 {
	return m.Value
}

// GetRawTags returns the metric sample tags, satisfying observer.MetricView.
// The caller must not retain the slice — it may be returned to a pool.
func (m *MetricSample) GetRawTags() []string {
	return m.Tags
}

// GetTimestampUnix returns the metric sample timestamp in Unix seconds, satisfying observer.MetricView.
// Returns 0 for un-timestamped samples (standard DogStatsD submissions).
func (m *MetricSample) GetTimestampUnix() int64 {
	return int64(m.Timestamp)
}

// GetSampleRate returns the metric sample rate, satisfying observer.MetricView.
func (m *MetricSample) GetSampleRate() float64 {
	return m.SampleRate
}
