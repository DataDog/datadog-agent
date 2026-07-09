// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides the serverless metric agent for collecting and forwarding metrics.
package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tags bundles the three tag sets the ServerlessMetricAgent applies, one per
// Add*Metric helper. Each slice is in DogStatsD wire format: one
// `"key:value"` string per dimension. Any field may be nil if the caller
// doesn't intend to emit that kind of metric.
type Tags struct {
	Metric              []string // applied by AddLegacyEnhancedMetric
	EnhancedMetric      []string // applied by AddEnhancedMetric
	EnhancedUsageMetric []string // applied by AddEnhancedUsageMetric (includes high-cardinality instance tag)
}

// ServerlessMetricAgent submits serverless metric samples to the Fx-provided
// demultiplexer using the tag sets supplied at construction. Lifecycle is
// owned by Fx.
type ServerlessMetricAgent struct {
	Demux aggregator.Demultiplexer
	tags  Tags
}

// New constructs a ServerlessMetricAgent.
func New(demux aggregator.Demultiplexer, tags Tags) *ServerlessMetricAgent {
	return &ServerlessMetricAgent{Demux: demux, tags: tags}
}

// AddLegacyEnhancedMetric reports a metric value to the intake with all tags.
// This method should be removed in a future major serverless-init release.
// optional tags supplied as `key:value` strings through extraTags.
func (c *ServerlessMetricAgent) AddLegacyEnhancedMetric(name string, value float64, metricSource metrics.MetricSource, extraTags ...string) {
	c.sendMetricSample(name, value, metricSource, metrics.DistributionType, 0, c.tags.Metric, extraTags...)
}

// AddEnhancedMetric reports a metric value to the intake with the given timestamp and tags selected for enhanced metrics.
// optional tags supplied as `key:value` strings through extraTags.
func (c *ServerlessMetricAgent) AddEnhancedMetric(name string, value float64, metricSource metrics.MetricSource, timestamp float64, extraTags ...string) {
	c.sendMetricSample(name, value, metricSource, metrics.DistributionType, timestamp, c.tags.EnhancedMetric, extraTags...)
}

// AddEnhancedUsageMetric reports a metric value to the intake with the given timestamp and tags selected for enhanced usage metrics.
// optional tags supplied as `key:value` strings through extraTags.
func (c *ServerlessMetricAgent) AddEnhancedUsageMetric(name string, value float64, metricSource metrics.MetricSource, timestamp float64, extraTags ...string) {
	c.sendMetricSample(name, value, metricSource, metrics.GaugeType, timestamp, c.tags.EnhancedUsageMetric, extraTags...)
}

// Flush forces an immediate flush of aggregated samples to the serializer.
// Satisfied interface: cmd/serverless-init/lifecycle.Flusher, used by MicroVM
// to flush telemetry on-demand before a Firecracker snapshot (/suspend,
// /terminate), independent of the Fx-managed shutdown flush.
func (c *ServerlessMetricAgent) Flush() {
	if c.Demux == nil {
		return
	}
	c.Demux.ForceFlushToSerializer(time.Now(), true)
}

// sendMetricSample records a distribution metric sample using the agent's extra tags plus any
// optional tags supplied as `key:value` strings through extraTags.
func (c *ServerlessMetricAgent) sendMetricSample(name string, value float64, metricSource metrics.MetricSource, metricsType metrics.MetricType, timestamp float64, tags []string, extraTags ...string) {
	if c.Demux == nil {
		log.Debugf("Cannot add metric %s, the metric agent is not running", name)
		return
	}

	if timestamp == 0 {
		timestamp = float64(time.Now().UnixNano()) / float64(time.Second)
	}

	if len(extraTags) > 0 {
		tags = append(append([]string{}, tags...), extraTags...)
	}
	c.Demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metricsType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
		Source:     metricSource,
	})
}
