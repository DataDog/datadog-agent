// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type precheckOnlyMetricView struct {
	name string
	host string
}

func (m *precheckOnlyMetricView) GetName() string { return m.name }
func (m *precheckOnlyMetricView) GetHost() string { return m.host }
func (*precheckOnlyMetricView) GetValue() float64 { panic("value must not be read") }
func (*precheckOnlyMetricView) GetTags() tagset.CompositeTags {
	panic("tags must not be read")
}
func (*precheckOnlyMetricView) GetTimestampUnix() int64 { panic("timestamp must not be read") }
func (*precheckOnlyMetricView) GetSampleRate() float64  { panic("sample rate must not be read") }

func TestMetricHandoffSnapshotsReusableSampleFields(t *testing.T) {
	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "dogstatsd"}
	sample := &metricObs{
		name:      "requests",
		value:     42,
		host:      "host-a",
		tags:      tagset.CompositeTagsFromSlice([]string{"service:web", "env:prod"}),
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

func TestMetricHandoffRetainsImmutableCompositeTags(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "check", filter: filter}
	sample := &metricObs{
		name:      "requests",
		tags:      tagset.CompositeTagsFromSlice([]string{"service:web", "env:prod", "service:web"}),
		timestamp: 123,
	}

	require.False(t, h.ObserveMetricAndReportDrop(sample))
	queued := <-ch

	// Both stages retain the immutable view without flattening or sorting it.
	assert.Equal(t, sample.tags, queued.metric.tags)
	decision := prepareMetricHandoff(queued.source, queued.metric, filter, nil)
	require.NotNil(t, decision.metric)
	assert.Equal(t, sample.tags, decision.metric.tags)
	assert.Equal(t, storageKeyForContextKey(queued.source, queued.metric.contextKey), decision.metric.storageKey)
}

func TestMetricHandoffDerivesContextKeyFromResolvedIdentity(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "dogstatsd", filter: filter}
	sample := &metricObs{
		name:      "requests",
		host:      "host-a",
		tags:      tagset.CompositeTagsFromSlice([]string{"service:web", "env:prod", "service:web"}),
		timestamp: 123,
	}

	require.False(t, h.observeMetricAndReportDrop(sample, 0))
	queued := <-ch
	require.Zero(t, queued.metric.contextKey)
	wantStorageKey := storageKeyForContextKey(queued.source, testContextKeyFor(sample))
	keyGenerator := ckey.NewSliceKeyGenerator()

	decision := prepareMetricHandoff(queued.source, queued.metric, filter, keyGenerator)
	require.NotNil(t, decision.metric)
	assert.Equal(t, wantStorageKey, decision.metric.storageKey)

	filter.publishMutedSnapshot(map[uint64]struct{}{wantStorageKey: {}})
	assert.Nil(t, prepareMetricHandoff(queued.source, queued.metric, filter, keyGenerator).metric)
}

func TestMetricHandoffRejectsNameOnlyRuleBeforeSnapshot(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:        excludeAtMatch,
		Name:        "drop_system_cpu",
		NamePattern: "system.cpu.*",
	}})
	require.NoError(t, err)

	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "dogstatsd", filter: filter}

	require.False(t, h.observeMetricAndReportDrop(&precheckOnlyMetricView{
		name: "system.cpu.user",
		host: "host-a",
	}, 0))
	assert.Len(t, ch, 0)
}

func TestMetricHandoffDefersTagDependentRule(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:        excludeAtMatch,
		Name:        "drop_dev_requests",
		NamePattern: "requests",
		Tags:        []string{"env:dev"},
	}})
	require.NoError(t, err)

	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "dogstatsd", filter: filter}
	sample := &metricObs{
		name:      "requests",
		tags:      tagset.CompositeTagsFromSlice([]string{"service:web", "env:dev"}),
		timestamp: 123,
	}

	require.False(t, h.observeMetricAndReportDrop(sample, 0))
	queued := <-ch
	require.True(t, queued.metric.precheck.needsTags)
	assert.Nil(t, prepareMetricHandoff(queued.source, queued.metric, filter, nil).metric)
}
