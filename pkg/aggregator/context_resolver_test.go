// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Helper functions to run tests and benchmarks for context resolver, time and check samplers.
func testWithTagsStore(t *testing.T, test func(*testing.T, *tags.Store)) {
	t.Run("useStore=true", func(t *testing.T) { test(t, tags.NewStore(true, "test")) })
	t.Run("useStore=false", func(t *testing.T) { test(t, tags.NewStore(false, "test")) })
}

func benchWithTagsStore(t *testing.B, test func(*testing.B, *tags.Store)) {
	t.Run("useStore=true", func(t *testing.B) { test(t, tags.NewStore(true, "test")) })
	t.Run("useStore=false", func(t *testing.B) { test(t, tags.NewStore(false, "test")) })
}

func assertContext(t *testing.T, cx *Context, name string, tags []string, host string) {
	assert.Equal(t, cx.Name, name)
	assert.Equal(t, cx.Host, host)
	metrics.AssertCompositeTagsEqual(t, cx.Tags(), tagset.CompositeTagsFromSlice(tags))
}

func TestGenerateContextKey(t *testing.T) {
	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}

	contextKey := generateContextKey(&mSample)
	assert.Equal(t, ckey.ContextKey(0x8cdd8c0c59c767db), contextKey)
}

func testTrackContext(t *testing.T, store *tags.Store) {
	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz"},
		SampleRate: 1,
	}
	mSample3 := metrics.MetricSample{ // same as mSample2, with different Host
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.CountType,
		Tags:       []string{"foo", "bar", "baz"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}

	contextResolver := newContextResolver(nooptagger.NewTaggerClient(), store, "test")

	// Track the 2 contexts
	contextKey1 := contextResolver.trackContext(&mSample1, 0)
	contextKey2 := contextResolver.trackContext(&mSample2, 0)
	contextKey3 := contextResolver.trackContext(&mSample3, 0)

	// When we look up the 2 keys, they return the correct contexts
	context1 := contextResolver.contextsByKey[contextKey1].context
	assertContext(t, context1, mSample1.Name, mSample1.Tags, "")

	context2 := contextResolver.contextsByKey[contextKey2].context
	assertContext(t, context2, mSample2.Name, mSample2.Tags, "")

	context3 := contextResolver.contextsByKey[contextKey3].context
	assertContext(t, context3, mSample3.Name, mSample3.Tags, mSample3.Host)

	assert.Equal(t, uint64(2), contextResolver.countsByMtype[metrics.GaugeType])
	assert.Equal(t, uint64(1), contextResolver.countsByMtype[metrics.CountType])
	assert.Equal(t, uint64(0), contextResolver.countsByMtype[metrics.RateType])

	// If the struct changes it's ok to change these, but be careful if you notice that
	// the size increases a lot.
	assert.Equal(t, uint64(0x90), contextResolver.bytesByMtype[metrics.GaugeType])
	assert.Equal(t, uint64(0x48), contextResolver.bytesByMtype[metrics.CountType])
	assert.Equal(t, uint64(0), contextResolver.bytesByMtype[metrics.RateType])
	assert.Equal(t, uint64(0x2b), contextResolver.dataBytesByMtype[metrics.GaugeType])
	assert.Equal(t, uint64(0x26), contextResolver.dataBytesByMtype[metrics.CountType])
	assert.Equal(t, uint64(0), contextResolver.dataBytesByMtype[metrics.RateType])

	// Make sure we can update the telemetry as well
	contextResolver.updateMetrics(tlmDogstatsdContextsByMtype, tlmDogstatsdContextsBytesByMtype)

	unknownContextKey := ckey.ContextKey(0xffffffffffffffff)
	_, ok := contextResolver.contextsByKey[unknownContextKey]
	assert.False(t, ok)
}

func TestTrackContext(t *testing.T) {
	testWithTagsStore(t, testTrackContext)
}

func testExpireContexts(t *testing.T, store *tags.Store) {
	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz"},
		SampleRate: 1,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.counter.name",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo"},
		SampleRate: 1,
	}
	contextResolver := newTimestampContextResolver(nooptagger.NewTaggerClient(), store, "test", 2, 4)

	// Track the 2 contexts
	contextKey1 := contextResolver.trackContext(&mSample1, 4) // expires after 6
	contextKey2 := contextResolver.trackContext(&mSample2, 6) // expires after 8
	contextKey3 := contextResolver.trackContext(&mSample3, 6) // expires after 10

	// With an expireTimestap of 3, both contexts are still valid
	contextResolver.expireContexts(4)
	_, ok := contextResolver.resolver.contextsByKey[contextKey1]
	assert.True(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey2]
	assert.True(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey3]
	assert.True(t, ok)

	// With an expireTimestap of 8, context 1 is expired
	contextResolver.expireContexts(7)

	// context 1 is not tracked anymore, but context 2 still is
	_, ok = contextResolver.resolver.contextsByKey[contextKey1]
	assert.False(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey2]
	assert.True(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey3]
	assert.True(t, ok)

	contextResolver.expireContexts(9)
	_, ok = contextResolver.resolver.contextsByKey[contextKey1]
	assert.False(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey2]
	assert.False(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey3]
	assert.True(t, ok)

	contextResolver.expireContexts(11)
	_, ok = contextResolver.resolver.contextsByKey[contextKey1]
	assert.False(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey2]
	assert.False(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey3]
	assert.False(t, ok)
}

func TestExpireContexts(t *testing.T) {
	testWithTagsStore(t, testExpireContexts)
}

func testCountBasedExpireContexts(t *testing.T, store *tags.Store) {
	mSample1 := metrics.MetricSample{Name: "my.metric.name1"}
	mSample2 := metrics.MetricSample{Name: "my.metric.name2"}
	mSample3 := metrics.MetricSample{Name: "my.metric.name3"}
	contextResolver := newCountBasedContextResolver(2, store, nooptagger.NewTaggerClient(), "test")

	contextKey1 := contextResolver.trackContext(&mSample1)
	contextKey2 := contextResolver.trackContext(&mSample2)
	require.Len(t, contextResolver.expireContexts(), 0)

	contextKey3 := contextResolver.trackContext(&mSample3)
	contextResolver.trackContext(&mSample2)
	require.Len(t, contextResolver.expireContexts(), 0)

	expiredContextKeys := contextResolver.expireContexts()
	require.ElementsMatch(t, expiredContextKeys, []ckey.ContextKey{contextKey1})

	expiredContextKeys = contextResolver.expireContexts()
	require.ElementsMatch(t, expiredContextKeys, []ckey.ContextKey{contextKey2, contextKey3})

	require.Len(t, contextResolver.expireContexts(), 0)
	require.Len(t, contextResolver.resolver.contextsByKey, 0)
}

func TestCountBasedExpireContexts(t *testing.T) {
	testWithTagsStore(t, testCountBasedExpireContexts)
}

func testTagDeduplication(t *testing.T, store *tags.Store) {
	resolver := newContextResolver(nooptagger.NewTaggerClient(), store, "test")

	ckey := resolver.trackContext(&metrics.MetricSample{
		Name: "foo",
		Tags: []string{"bar", "bar"},
	}, 0)

	assert.Equal(t, resolver.contextsByKey[ckey].context.Tags().Len(), 1)
	metrics.AssertCompositeTagsEqual(t, resolver.contextsByKey[ckey].context.Tags(), tagset.CompositeTagsFromSlice([]string{"bar"}))
}

func TestTagDeduplication(t *testing.T) {
	testWithTagsStore(t, testTagDeduplication)
}

type mockSink []*metrics.Serie

func (s *mockSink) Append(ms *metrics.Serie) {
	*s = append(*s, ms)
}

type mockSample struct {
	name       string
	taggerTags []string
	metricTags []string
}

func (s *mockSample) GetName() string                   { return s.name }
func (s *mockSample) GetHost() string                   { return "noop" }
func (s *mockSample) GetMetricType() metrics.MetricType { return metrics.GaugeType }
func (s *mockSample) IsNoIndex() bool                   { return false }
func (s *mockSample) GetSource() metrics.MetricSource   { return metrics.MetricSourceUnknown }
func (s *mockSample) GetTags(tb, mb tagset.TagsAccumulator, _ metrics.EnrichTagsfn) {
	tb.Append(s.taggerTags...)
	mb.Append(s.metricTags...)
}

func TestOriginTelemetry(t *testing.T) {
	r := newContextResolver(nooptagger.NewTaggerClient(), tags.NewStore(true, "test"), "test")
	r.trackContext(&mockSample{"foo", []string{"foo"}, []string{"ook"}}, 0)
	r.trackContext(&mockSample{"foo", []string{"foo"}, []string{"eek"}}, 0)
	r.trackContext(&mockSample{"foo", []string{"bar"}, []string{"ook"}}, 0)
	r.trackContext(&mockSample{"bar", []string{"bar"}, []string{}}, 0)
	r.trackContext(&mockSample{"bar", []string{"baz"}, []string{}}, 0)
	sink := mockSink{}
	ts := 1672835152.0
	r.sendOriginTelemetry(ts, &sink, "test", []string{"test"})

	assert.ElementsMatch(t, sink, []*metrics.Serie{{
		Name:   "datadog.agent.aggregator.dogstatsd_contexts_by_origin",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test"}, []string{"foo"}),
		MType:  metrics.APIGaugeType,
		Points: []metrics.Point{{Ts: ts, Value: 2.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_contexts_by_origin",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test"}, []string{"bar"}),
		MType:  metrics.APIGaugeType,
		Points: []metrics.Point{{Ts: ts, Value: 2.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_contexts_by_origin",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test"}, []string{"baz"}),
		MType:  metrics.APIGaugeType,
		Points: []metrics.Point{{Ts: ts, Value: 1.0}},
	}})
}
