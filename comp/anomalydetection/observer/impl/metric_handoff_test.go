// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricHandoffSnapshotsReusableSampleFields(t *testing.T) {
	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "dogstatsd"}
	sample := &metricObs{
		name:      "requests",
		value:     42,
		host:      "host-a",
		tags:      []string{"service:web", "env:prod"},
		timestamp: 123,
	}

	require.False(t, h.ObserveMetricAndReportDrop(sample))

	// Aggregator MetricSamples are reusable as soon as ObserveMetric returns.
	// Mutating the scalar fields here must not change the queued observation.
	sample.name = "reused"
	sample.value = -1
	sample.host = "host-b"
	sample.timestamp = 456

	queued := <-ch
	require.True(t, queued.hasMetric)
	assert.Equal(t, "requests", queued.metric.name)
	assert.Equal(t, 42.0, queued.metric.value)
	assert.Equal(t, "host-a", queued.metric.host)
	assert.Equal(t, int64(123), queued.metric.timestamp)
	assert.Equal(t, []string{"service:web", "env:prod"}, queued.metric.tags.UnsafeToReadOnlySliceString())
}

func TestMetricHandoffDefersTagCanonicalization(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "check"}
	sample := &metricObs{
		name:      "requests",
		tags:      []string{"service:web", "env:prod", "service:web"},
		timestamp: 123,
	}

	require.False(t, h.ObserveMetricAndReportDrop(sample))
	queued := <-ch

	// The producer only captures the immutable CompositeTags view. Sorting and
	// the associated allocation happen in the consumer.
	assert.Equal(t, sample.tags, queued.metric.tags.UnsafeToReadOnlySliceString())
	decision := prepareMetricHandoff(queued.source, queued.metric, filter)
	require.NotNil(t, decision.metric)
	assert.Equal(t, []string{"env:prod", "service:web", "service:web"}, decision.metric.tags)
}
