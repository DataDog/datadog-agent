// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
)

// DistributionMetricTypes contains the MetricTypes that are used for percentiles
var (
	DistributionMetricTypes = map[MetricType]struct{}{
		DistributionType: {},
	}

	// we use to pull tagger metrics in dogstatsd. Pulling it later in the
	// pipeline improve memory allocation. We kept the old name to be
	// backward compatible and because origin detection only affect
	// dogstatsd metrics.
	tlmUDPOriginDetectionError = telemetry.NewCounter("dogstatsd", "udp_origin_detection_error",
		nil, "Dogstatsd UDP origin detection error count")
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
	default:
		return ""
	}
}

// MetricSampleContext allows to access a sample context data
type MetricSampleContext interface {
	GetName() string
	GetHost() string
	GetTags(*util.TagsBuilder)
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
	OriginID        string
	K8sOriginID     string
	Cardinality     string
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

func findOriginTags(origin string, cardinality collectors.TagCardinality, tb *util.TagsBuilder) {
	if origin != packets.NoOrigin {
		if err := tagger.TagBuilder(origin, cardinality, tb); err != nil {
			log.Errorf(err.Error())
		}
	}
}

func addOrchestratorTags(cardinality collectors.TagCardinality, tb *util.TagsBuilder) {
	// Include orchestrator scope tags if the cardinality is set to orchestrator
	if cardinality == collectors.OrchestratorCardinality {
		if err := tagger.OrchestratorScopeTagBuilder(tb); err != nil {
			log.Error(err.Error())
		}
	}
}

// EnrichTags expend a tag list with origin detection tags
func EnrichTags(tb *util.TagsBuilder, originID string, k8sOriginID string, cardinality string) {
	taggerCard := taggerCardinality(cardinality)

	findOriginTags(originID, taggerCard, tb)
	addOrchestratorTags(taggerCard, tb)

	if k8sOriginID != "" {
		if err := tagger.TagBuilder(k8sOriginID, taggerCard, tb); err != nil {
			tlmUDPOriginDetectionError.Inc()
			log.Tracef("Cannot get tags for entity %s: %s", k8sOriginID, err)
		}
	}

	tb.SortUniq()
}

// GetTags returns the metric sample tags
func (m *MetricSample) GetTags(tb *util.TagsBuilder) {
	tb.Append(m.Tags...)
	EnrichTags(tb, m.OriginID, m.K8sOriginID, m.Cardinality)
}

// Copy returns a deep copy of the m MetricSample
func (m *MetricSample) Copy() *MetricSample {
	dst := &MetricSample{}
	*dst = *m
	dst.Tags = make([]string, len(m.Tags))
	copy(dst.Tags, m.Tags)
	return dst
}

// taggerCardinality converts tagger cardinality string to collectors.TagCardinality
// It defaults to DogstatsdCardinality if the string is empty or unknown
func taggerCardinality(cardinality string) collectors.TagCardinality {
	if cardinality == "" {
		return tagger.DogstatsdCardinality
	}

	taggerCardinality, err := collectors.StringToTagCardinality(cardinality)
	if err != nil {
		log.Tracef("Couldn't convert cardinality tag: %w", err)
		return tagger.DogstatsdCardinality
	}

	return taggerCardinality
}
