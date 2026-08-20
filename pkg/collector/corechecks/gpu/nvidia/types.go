// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package nvidia holds the logic to collect metrics from the NVIDIA Management Library (NVML).
package nvidia

import (
	"fmt"
	"slices"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

// MetricPriority represents the priority level of a metric
type MetricPriority int

const (
	// Low priority is the default priority level (0)
	Low MetricPriority = 0
	// MediumLow priority level (5)
	MediumLow MetricPriority = 5
	// Medium priority level (10)
	Medium MetricPriority = 10
	// High priority level (20)
	High MetricPriority = 20
)

// CollectorName is the name of the nvml sub-collectors
type CollectorName string

// Sample represents a single data point emitted by a collector. Can be a metric, event, histogram point...
type Sample interface {
	// Priority is the priority of the sample, indicating which sample to keep in case of duplicates. Low (default) is the lowest priority.
	Priority() MetricPriority

	// Key is the unique identifier for the sample, it is used to deduplicate samples with the same key. It should include the sample type
	Key() string

	// AssociatedWorkloads returns the workloads that are associated with the sample.
	AssociatedWorkloads() []workloadmeta.EntityID

	// Clone returns a copy of the sample that can be enriched independently.
	Clone() Sample

	// AppendTags appends tags to the sample.
	AppendTags([]string)

	// Emit emits the sample to the sender.
	Emit(namespace string, sender sender.Sender, timestamp time.Time) error
}

type baseSample struct {
	priority            MetricPriority
	associatedWorkloads []workloadmeta.EntityID
	tags                []string
}

func (s *baseSample) Priority() MetricPriority {
	return s.priority
}

func (s *baseSample) AssociatedWorkloads() []workloadmeta.EntityID {
	return slices.Clone(s.associatedWorkloads)
}

func (s *baseSample) clone() baseSample {
	sample := *s
	sample.tags = slices.Clone(s.tags)
	sample.associatedWorkloads = slices.Clone(s.associatedWorkloads)
	return sample
}

func (s *baseSample) AppendTags(tags []string) {
	s.tags = append(s.tags, tags...)
}

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	baseSample
	Name                string               // Name holds the name of the metric.
	Value               float64              // Value holds the value of the metric.
	Type                ddmetrics.MetricType // Type holds the type of the metric.
	RateCalculationMode RateCalculationMode  // RateCalculationMode is the mode of rate calculation for the metric.
}

// NewMetric creates a metric sample with its common sample metadata.
func NewMetric(name string, value float64, metricType ddmetrics.MetricType, priority MetricPriority, tags []string, associatedWorkloads []workloadmeta.EntityID) *Metric {
	return &Metric{
		baseSample: baseSample{
			priority:            priority,
			tags:                slices.Clone(tags),
			associatedWorkloads: slices.Clone(associatedWorkloads),
		},
		Name:  name,
		Value: value,
		Type:  metricType,
	}
}

var _ Sample = (*Metric)(nil)

func (m *Metric) Key() string {
	return "__metric__:" + m.Name
}

func (m *Metric) Clone() Sample {
	metric := *m
	metric.baseSample = m.baseSample.clone()
	return &metric
}

func (m *Metric) Emit(namespace string, snd sender.Sender, timestamp time.Time) error {
	metricTimestamp := float64(timestamp.UnixNano()) / float64(time.Second)
	switch m.Type {
	case ddmetrics.GaugeType:
		return snd.GaugeWithTimestamp(namespace+m.Name, m.Value, "", m.tags, metricTimestamp)
	case ddmetrics.CountType:
		return snd.CountWithTimestamp(namespace+m.Name, m.Value, "", m.tags, metricTimestamp)
	default:
		return fmt.Errorf("unsupported metric type %s", m.Type)
	}
}

// HistogramSample carries histogram bucket data.
type HistogramSample struct {
	baseSample
	Name            string
	Value           int64
	Bounds          [2]float64
	Monotonic       bool
	FlushFirstValue bool
}

// NewHistogramSample creates a histogram bucket sample with its common sample metadata.
func NewHistogramSample(name string, value int64, bounds [2]float64, monotonic, flushFirstValue bool, priority MetricPriority, tags []string, associatedWorkloads []workloadmeta.EntityID) *HistogramSample {
	return &HistogramSample{
		baseSample: baseSample{
			priority:            priority,
			tags:                slices.Clone(tags),
			associatedWorkloads: slices.Clone(associatedWorkloads),
		},
		Name:            name,
		Value:           value,
		Bounds:          bounds,
		Monotonic:       monotonic,
		FlushFirstValue: flushFirstValue,
	}
}

var _ Sample = (*HistogramSample)(nil)

func (h *HistogramSample) Key() string {
	return "__histogram__:" + h.Name
}

func (h *HistogramSample) Clone() Sample {
	sample := *h
	sample.baseSample = h.baseSample.clone()
	return &sample
}

func (h *HistogramSample) Emit(namespace string, snd sender.Sender, _ time.Time) error {
	snd.HistogramBucket(namespace+h.Name, h.Value, h.Bounds[0], h.Bounds[1], h.Monotonic, "", h.tags, h.FlushFirstValue)

	return nil
}

// Collector collects samples from a specific NVML subsystem and device.
type Collector interface {
	// Collect collects samples from the given NVML device. Samples should only
	// contain sample-specific tags; the GPU check adds device and workload tags.
	Collect() ([]Sample, error)

	// Name returns the name of the subsystem
	Name() CollectorName

	// Device returns the device this collector is collecting metrics from.
	Device() ddnvml.Device
}
