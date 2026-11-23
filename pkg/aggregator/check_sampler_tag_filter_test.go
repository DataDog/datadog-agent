// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
)

func filterPodMatcher(t *testing.T, names ...string) metricname.TagMatcher {
	rules := map[string]filterlistimpl.MetricTagList{}
	for _, name := range names {
		rules[name] = filterlistimpl.MetricTagList{Tags: []string{"pod"}, Action: "exclude"}
	}
	return filterlistimpl.NewTagMatcher(rules, logmock.New(t))
}

func newTagFilterTestSampler(id string, table *filteredIdentityTable) *CheckSampler {
	return newCheckSampler(2, true, false, time.Hour, true, true, tags.NewStore("test"), checkid.ID(id), nooptagger.NewComponent(), table)
}

func podSample(name string, mtype metrics.MetricType, pod string, value, ts float64) *metrics.MetricSample {
	return &metrics.MetricSample{Name: name, Value: value, Mtype: mtype, Tags: []string{"env:prod", "pod:" + pod}, SampleRate: 1, Timestamp: ts}
}

func tagsOf(t tagsJoiner) []string {
	var out []string
	t.ForEach(func(tag string) { out = append(out, tag) })
	return out
}

type tagsJoiner interface{ ForEach(func(string)) }

// filteredTags returns the tags a context's output is filtered to, and whether it's filtered.
func filteredTags(cs *CheckSampler, key ckey.ContextKey) ([]string, bool) {
	ctx, ok := cs.contextResolver.resolver.get(key)
	if !ok {
		return nil, false
	}
	identity := cs.contextResolver.resolver.filteredIdentityOf(ctx)
	if identity == nil {
		return nil, false
	}
	return tagsOf(identity.Tags()), true
}

// samplerOutput is what a sampler committed since its last flush: what goes to the merger, and the rest.
type samplerOutput struct {
	mergeableSeries   []*metrics.Serie
	mergeableSketches []mergeableSketch
	series            metrics.Series
	sketches          metrics.SketchSeriesList
}

func flushSampler(cs *CheckSampler) samplerOutput {
	var out samplerOutput
	out.mergeableSeries, out.mergeableSketches = cs.flushMergeable()
	out.series, out.sketches = cs.flush()
	return out
}

func TestContextFitsIn64Bytes(t *testing.T) {
	// The filtered identity reference lives in Context's padding.
	assert.Equal(t, uintptr(64), unsafe.Sizeof(Context{}))
}

func TestCheckSamplerFiltersTagsAtOutput(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newTagFilterTestSampler("check:0", newFilteredIdentityTable())
	matcher := filterPodMatcher(t, "my.counter", "my.dist")
	flushMatcher := metricname.NewMatcher(nil, false)

	// Two sources in one instance filter to the same series.
	for _, pod := range []string{"a", "b"} {
		cs.addSample(podSample("my.counter", metrics.CounterType, pod, 1, 100.1), matcher)
		cs.addSample(podSample("my.dist", metrics.DistributionType, pod, 1, 100.1), matcher)
	}
	cs.commit(100.4, &flushMatcher)
	out := flushSampler(cs)

	assert.Empty(t, out.series)
	assert.Empty(t, out.sketches)
	require.Len(t, out.mergeableSeries, 2, "sources are aggregated apart, then merged after the sampler")
	assert.Equal(t, out.mergeableSeries[0].ContextKey, out.mergeableSeries[1].ContextKey)
	for _, s := range out.mergeableSeries {
		assert.ElementsMatch(t, []string{"env:prod"}, tagsOf(s.Tags))
	}

	require.Len(t, out.mergeableSketches, 2)
	assert.Equal(t, out.mergeableSketches[0].key, out.mergeableSketches[1].key)
	for _, s := range out.mergeableSketches {
		assert.ElementsMatch(t, []string{"env:prod"}, tagsOf(s.sketch.Tags))
		require.Len(t, s.sketch.Points, 1)
		assert.Equal(t, int64(100), s.sketch.Points[0].Ts)
	}
}

func TestFilteredIdentitiesAreSharedAndReleased(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newFilteredIdentityTable()
	matcher := filterPodMatcher(t, "my.counter")
	flushMatcher := metricname.NewMatcher(nil, false)
	samplers := []*CheckSampler{newTagFilterTestSampler("check:0", table), newTagFilterTestSampler("check:1", table)}

	for i, cs := range samplers {
		cs.addSample(podSample("my.counter", metrics.CounterType, string(rune('a'+i)), 1, 100), matcher)
	}
	require.Len(t, table.byKey, 1, "sources in different samplers share one identity")
	for _, ref := range table.byKey {
		assert.Equal(t, uint32(2), table.get(ref).refs)
	}

	// Without samples, contexts expire after two commits, and the identity with the last of them.
	for _, cs := range samplers {
		for commit := 0; commit < 3; commit++ {
			cs.commit(float64(101+commit), &flushMatcher)
		}
	}
	assert.Empty(t, table.byKey)
	assert.Len(t, table.free, len(table.identities))
}

func TestCheckSamplerRefiltersWhenRulesChange(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newTagFilterTestSampler("check:0", newFilteredIdentityTable())
	flushMatcher := metricname.NewMatcher(nil, false)
	key := cs.contextResolver.trackContext(podSample("my.dist", metrics.DistributionType, "a", 1, 100), filterlistimpl.NewNoopTagMatcher())

	_, filtered := filteredTags(cs, key)
	require.False(t, filtered)

	// A rule is added.
	cs.refilter(filterPodMatcher(t, "my.dist"))
	tags, filtered := filteredTags(cs, key)
	require.True(t, filtered)
	assert.ElementsMatch(t, []string{"env:prod"}, tags)

	// The rule is removed: the context's output is sent with its full tags, without the merger.
	cs.refilter(filterlistimpl.NewNoopTagMatcher())
	_, filtered = filteredTags(cs, key)
	require.False(t, filtered)
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100.1), filterlistimpl.NewNoopTagMatcher())
	cs.commit(100.3, &flushMatcher)
	_, mergeable := cs.flushMergeable()
	assert.Empty(t, mergeable)
	_, sketches := cs.flush()
	require.Len(t, sketches, 1)
	assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, tagsOf(sketches[0].Tags))
	assert.Equal(t, int64(100), sketches[0].Points[0].Ts)
	assert.Empty(t, cs.contextResolver.resolver.filteredIdentities.byKey)
}

func TestCheckSamplerDoesNotFilterSketchesStampedPerSecond(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newFilteredIdentityTable()
	cs := newCheckSampler(2, true, false, time.Hour, true, false, tags.NewStore("test"), checkid.ID("check:0"), nooptagger.NewComponent(), table)
	matcher := filterPodMatcher(t, "my.counter", "my.dist")
	flushMatcher := metricname.NewMatcher(nil, false)

	cs.addSample(podSample("my.counter", metrics.CounterType, "a", 1, 100.1), matcher)
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100.1), matcher)
	cs.commit(101.4, &flushMatcher)
	out := flushSampler(cs)

	require.Len(t, out.mergeableSeries, 1, "counters are still filtered and merged")
	assert.Empty(t, out.mergeableSketches)
	require.Len(t, out.sketches, 1)
	assert.Contains(t, tagsOf(out.sketches[0].Tags), "pod:a", "sketches stamped per second aren't filtered")
	assert.Equal(t, int64(100), out.sketches[0].Points[0].Ts)
	assert.Len(t, table.byKey, 2, "the distribution has an identity too, which sketches stamped per second don't use")
}

func TestCheckSamplerCountsFilteredTagsPerSample(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newTagFilterTestSampler("check:0", newFilteredIdentityTable())
	matcher := filterPodMatcher(t, "my.counter")

	before := tlmFilteredTags.Get()
	cs.addSample(podSample("my.counter", metrics.CounterType, "a", 1, 100), matcher)
	cs.addSample(podSample("my.counter", metrics.CounterType, "a", 2, 101), matcher)
	assert.Equal(t, before+2, tlmFilteredTags.Get(), "counts removed tags on every sample, including existing contexts")

	// Refiltering without new samples doesn't increment the counter.
	cs.refilter(matcher)
	assert.Equal(t, before+2, tlmFilteredTags.Get())
}
