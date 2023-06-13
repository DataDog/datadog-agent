// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/limiter"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags_limiter"
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

	contextResolver := newContextResolver(store, nil, nil)

	// Track the 2 contexts
	contextKey1, _ := contextResolver.trackContext(&mSample1)
	contextKey2, _ := contextResolver.trackContext(&mSample2)
	contextKey3, _ := contextResolver.trackContext(&mSample3)

	// When we look up the 2 keys, they return the correct contexts
	context1 := contextResolver.contextsByKey[contextKey1]
	assertContext(t, context1, mSample1.Name, mSample1.Tags, "")

	context2 := contextResolver.contextsByKey[contextKey2]
	assertContext(t, context2, mSample2.Name, mSample2.Tags, "")

	context3 := contextResolver.contextsByKey[contextKey3]
	assertContext(t, context3, mSample3.Name, mSample3.Tags, mSample3.Host)

	assert.Equal(t, uint64(2), contextResolver.countsByMtype[metrics.GaugeType])
	assert.Equal(t, uint64(1), contextResolver.countsByMtype[metrics.CountType])
	assert.Equal(t, uint64(0), contextResolver.countsByMtype[metrics.RateType])

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
	contextResolver := newTimestampContextResolver(store, nil, nil)

	// Track the 2 contexts
	contextKey1, _ := contextResolver.trackContext(&mSample1, 4)
	contextKey2, _ := contextResolver.trackContext(&mSample2, 6)

	// With an expireTimestap of 3, both contexts are still valid
	contextResolver.expireContexts(3, nil)
	_, ok1 := contextResolver.resolver.contextsByKey[contextKey1]
	_, ok2 := contextResolver.resolver.contextsByKey[contextKey2]
	assert.True(t, ok1)
	assert.True(t, ok2)

	// With an expireTimestap of 5, context 1 is expired
	contextResolver.expireContexts(5, nil)

	// context 1 is not tracked anymore, but context 2 still is
	_, ok := contextResolver.resolver.contextsByKey[contextKey1]
	assert.False(t, ok)
	_, ok = contextResolver.resolver.contextsByKey[contextKey2]
	assert.True(t, ok)
}

func TestExpireContexts(t *testing.T) {
	testWithTagsStore(t, testExpireContexts)
}

func testExpireContextsWithKeep(t *testing.T, store *tags.Store) {
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
	contextResolver := newTimestampContextResolver(store, nil, nil)

	// Track the 2 contexts
	contextKey1, _ := contextResolver.trackContext(&mSample1, 4)
	contextKey2, _ := contextResolver.trackContext(&mSample2, 7)

	keeperCalled := 0
	keep := true
	keeper := func(k ckey.ContextKey) bool {
		keeperCalled++
		assert.Equal(t, k, contextKey1)
		return keep
	}

	// With an expireTimestap of 3, both contexts are still valid
	contextResolver.expireContexts(3, keeper)
	_, ok1 := contextResolver.resolver.contextsByKey[contextKey1]
	_, ok2 := contextResolver.resolver.contextsByKey[contextKey2]
	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, keeperCalled, 0)

	// With an expireTimestap of 5, context 1 is expired, but we explicitly keep it
	contextResolver.expireContexts(5, keeper)
	assert.Equal(t, keeperCalled, 1)

	// both contexts are still tracked
	_, ok1 = contextResolver.resolver.contextsByKey[contextKey1]
	_, ok2 = contextResolver.resolver.contextsByKey[contextKey2]
	assert.True(t, ok1)
	assert.True(t, ok2)

	// With an expireTimestap of 6, context 1 is expired, and we don't keep it this time
	keep = false
	contextResolver.expireContexts(6, keeper)
	assert.Equal(t, keeperCalled, 2)

	// context 1 is not tracked anymore
	_, ok1 = contextResolver.resolver.contextsByKey[contextKey1]
	_, ok2 = contextResolver.resolver.contextsByKey[contextKey2]
	assert.False(t, ok1)
	assert.True(t, ok2)
}

func TestExpireContextsWithKeep(t *testing.T) {
	testWithTagsStore(t, testExpireContextsWithKeep)
}

func testCountBasedExpireContexts(t *testing.T, store *tags.Store) {
	mSample1 := metrics.MetricSample{Name: "my.metric.name1"}
	mSample2 := metrics.MetricSample{Name: "my.metric.name2"}
	mSample3 := metrics.MetricSample{Name: "my.metric.name3"}
	contextResolver := newCountBasedContextResolver(2, store)

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
	resolver := newContextResolver(store, nil, nil)

	ckey, _ := resolver.trackContext(&metrics.MetricSample{
		Name: "foo",
		Tags: []string{"bar", "bar"},
	})

	assert.Equal(t, resolver.contextsByKey[ckey].Tags().Len(), 1)
	metrics.AssertCompositeTagsEqual(t, resolver.contextsByKey[ckey].Tags(), tagset.CompositeTagsFromSlice([]string{"bar"}))
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
func (s *mockSample) GetTags(tb, mb tagset.TagsAccumulator) {
	tb.Append(s.taggerTags...)
	mb.Append(s.metricTags...)
}

func TestOriginTelemetry(t *testing.T) {
	r := newContextResolver(tags.NewStore(true, "test"), nil, nil)
	r.trackContext(&mockSample{"foo", []string{"foo"}, []string{"ook"}})
	r.trackContext(&mockSample{"foo", []string{"foo"}, []string{"eek"}})
	r.trackContext(&mockSample{"foo", []string{"bar"}, []string{"ook"}})
	r.trackContext(&mockSample{"bar", []string{"bar"}, []string{}})
	r.trackContext(&mockSample{"bar", []string{"baz"}, []string{}})
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

func TestLimiterTelemetry(t *testing.T) {
	l := limiter.New(2, "pod", []string{"pod", "srv"})
	tl := tags_limiter.New(4)
	r := newContextResolver(tags.NewStore(true, "test"), l, tl)
	r.trackContext(&mockSample{"foo", []string{"pod:foo", "srv:foo"}, []string{"pod:bar"}})
	r.trackContext(&mockSample{"foo", []string{"pod:foo", "srv:foo"}, []string{"srv:bar"}})
	r.trackContext(&mockSample{"bar", []string{"pod:foo", "srv:foo"}, []string{"srv:bar"}})
	r.trackContext(&mockSample{"foo", []string{"pod:bar"}, []string{"srv:foo"}})
	r.trackContext(&mockSample{"bar", []string{"pod:bar"}, []string{"srv:bar"}})
	r.trackContext(&mockSample{"bar", []string{"pod:baz"}, []string{}})
	r.trackContext(&mockSample{"bar", []string{"pod:baz"}, []string{"1", "2", "3", "4", "5"}})
	sink := mockSink{}
	ts := 1672835152.0
	r.sendLimiterTelemetry(ts, &sink, "test", []string{"test"})

	assert.Subset(t, sink, []*metrics.Serie{{
		Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.current",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test"}, []string{"pod:foo", "srv:foo"}),
		MType:  metrics.APIGaugeType,
		Points: []metrics.Point{{Ts: ts, Value: 2.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_samples_dropped",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test", "reason:too_many_contexts"}, []string{"pod:foo", "srv:foo"}),
		MType:  metrics.APICountType,
		Points: []metrics.Point{{Ts: ts, Value: 1.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.current",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test"}, []string{"pod:bar"}),
		MType:  metrics.APIGaugeType,
		Points: []metrics.Point{{Ts: ts, Value: 2.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_samples_dropped",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test", "reason:too_many_contexts"}, []string{"pod:bar"}),
		MType:  metrics.APICountType,
		Points: []metrics.Point{{Ts: ts, Value: 0.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.current",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test"}, []string{"pod:baz"}),
		MType:  metrics.APIGaugeType,
		Points: []metrics.Point{{Ts: ts, Value: 1.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_samples_dropped",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test", "reason:too_many_contexts"}, []string{"pod:baz"}),
		MType:  metrics.APICountType,
		Points: []metrics.Point{{Ts: ts, Value: 0.0}},
	}, {
		Name:   "datadog.agent.aggregator.dogstatsd_samples_dropped",
		Host:   "test",
		Tags:   tagset.NewCompositeTags([]string{"test", "reason:too_many_tags"}, []string{"pod:baz"}),
		MType:  metrics.APICountType,
		Points: []metrics.Point{{Ts: ts, Value: 1.0}},
	}})
}

func TestTimestampContextResolverLimit(t *testing.T) {
	store := tags.NewStore(true, "")
	limiter := limiter.New(1, "pod", []string{})
	r := newTimestampContextResolver(store, limiter, nil)

	r.trackContext(&mockSample{"foo", []string{"pod:foo", "srv:foo"}, []string{"pod:bar"}}, 42)
	r.trackContext(&mockSample{"foo", []string{"pod:foo", "srv:foo"}, []string{"srv:bar"}}, 42)

	assert.Len(t, r.resolver.contextsByKey, 1)
	assert.Len(t, r.lastSeenByKey, 1)
}
