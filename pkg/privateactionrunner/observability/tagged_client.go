// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package observability

import (
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// NewTaggedMetricsClient wraps a statsd client so a fixed set of tags is appended to
// every emitted metric. Used to stamp the runner's identity tags (runner_id,
// runner_version, modes) on PAR metrics so executions from multiple Private Action
// Runners are distinguishable on dashboards/alerts. Returns c unchanged if tags is empty.
func NewTaggedMetricsClient(c statsd.ClientInterface, tags []string) statsd.ClientInterface {
	if len(tags) == 0 {
		return c
	}
	return &taggedMetricsClient{ClientInterface: c, tags: tags}
}

// taggedMetricsClient embeds the wrapped client (so non-metric methods pass through)
// and overrides the metric-emitting methods to append the fixed tags.
type taggedMetricsClient struct {
	statsd.ClientInterface
	tags []string
}

var _ statsd.ClientInterface = (*taggedMetricsClient)(nil)

// withTags returns per-call tags + the fixed tags, without mutating the caller's slice.
func (t *taggedMetricsClient) withTags(tags []string) []string {
	out := make([]string, 0, len(tags)+len(t.tags))
	out = append(out, tags...)
	out = append(out, t.tags...)
	return out
}

func (t *taggedMetricsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	return t.ClientInterface.Gauge(name, value, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) Count(name string, value int64, tags []string, rate float64) error {
	return t.ClientInterface.Count(name, value, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) Histogram(name string, value float64, tags []string, rate float64) error {
	return t.ClientInterface.Histogram(name, value, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) Distribution(name string, value float64, tags []string, rate float64) error {
	return t.ClientInterface.Distribution(name, value, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return t.ClientInterface.Timing(name, value, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	return t.ClientInterface.TimeInMilliseconds(name, value, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) Incr(name string, tags []string, rate float64) error {
	return t.ClientInterface.Incr(name, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) Decr(name string, tags []string, rate float64) error {
	return t.ClientInterface.Decr(name, t.withTags(tags), rate)
}

func (t *taggedMetricsClient) Set(name string, value string, tags []string, rate float64) error {
	return t.ClientInterface.Set(name, value, t.withTags(tags), rate)
}
