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
	filterlistdef "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
)

func stripPodMatcher(t *testing.T, names ...string) filterlistdef.TagMatcher {
	rules := map[string]filterlistimpl.MetricTagList{}
	for _, name := range names {
		rules[name] = filterlistimpl.MetricTagList{Tags: []string{"pod"}, Action: "exclude"}
	}
	return filterlistimpl.NewTagMatcher(rules, logmock.New(t))
}

func newStripTestSampler(id string, table *strippedIdentityTable) *CheckSampler {
	cs := newCheckSampler(2, true, false, time.Hour, true, tags.NewStore(true, "test"), checkid.ID(id), nooptagger.NewComponent())
	cs.shareStrippedIdentities(table)
	return cs
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

// strippedTags returns the tags a context's output is stripped to, and whether it's stripped.
func strippedTags(cs *CheckSampler, key ckey.ContextKey) ([]string, bool) {
	ctx, ok := cs.contextResolver.resolver.get(key)
	if !ok {
		return nil, false
	}
	identity, stripped := cs.contextResolver.resolver.strippedOf(ctx)
	if !stripped {
		return nil, false
	}
	return tagsOf(identity.Tags()), true
}

// samplerOutput is what a sampler committed since its last flush: what goes to the merger, and the rest.
type samplerOutput struct {
	mergedSeries   []*metrics.Serie
	mergedSketches []mergedSketch
	series         metrics.Series
	sketches       metrics.SketchSeriesList
}

func flushSampler(cs *CheckSampler) samplerOutput {
	var out samplerOutput
	out.mergedSeries, out.mergedSketches = cs.flushMerged()
	out.series, out.sketches = cs.flush()
	return out
}

func TestContextDoesNotGrow(t *testing.T) {
	// The stripped identity reference and the output flags live in Context's padding.
	assert.Equal(t, uintptr(64), unsafe.Sizeof(Context{}))
}

func TestCheckSamplerStripsFilteredOutput(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	matcher := stripPodMatcher(t, "my.counter", "my.dist")
	flushMatcher := metricname.NewMatcher(nil, false)

	// Two sources in one instance strip to the same series. Their samples are submitted in the second of the commit.
	for _, pod := range []string{"a", "b"} {
		cs.addSample(podSample("my.counter", metrics.CounterType, pod, 1, 100.1), matcher)
		cs.addSample(podSample("my.dist", metrics.DistributionType, pod, 1, 100.1), matcher)
	}
	cs.commit(100.4, &flushMatcher)
	out := flushSampler(cs)

	assert.Empty(t, out.series)
	assert.Empty(t, out.sketches)
	require.Len(t, out.mergedSeries, 2, "sources are aggregated apart, then merged after the sampler")
	assert.Equal(t, out.mergedSeries[0].ContextKey, out.mergedSeries[1].ContextKey)
	for _, s := range out.mergedSeries {
		assert.ElementsMatch(t, []string{"env:prod"}, tagsOf(s.Tags))
	}

	require.Len(t, out.mergedSketches, 2, "filtered sketches are released at the commit, not held for the next one")
	assert.Equal(t, out.mergedSketches[0].key, out.mergedSketches[1].key)
	for _, s := range out.mergedSketches {
		assert.ElementsMatch(t, []string{"env:prod"}, tagsOf(s.sketch.Tags))
		require.Len(t, s.sketch.Points, 1)
		assert.Equal(t, int64(100), s.sketch.Points[0].Ts)
	}
}

func TestCheckSamplerStampsFilteredSketchesAtCommit(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	matcher := stripPodMatcher(t, "my.dist")
	flushMatcher := metricname.NewMatcher(nil, false)

	// One run submits samples over several seconds.
	for _, ts := range []float64{97.5, 98.5, 99.1} {
		cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, ts), matcher)
	}
	cs.commit(99.3, &flushMatcher)
	_, sketches := cs.flushMerged()

	require.Len(t, sketches, 1)
	points := sketches[0].sketch.Points
	require.Len(t, points, 1, "the run's samples are merged into one point")
	assert.Equal(t, int64(99), points[0].Ts)
	assert.Equal(t, int64(3), points[0].Sketch.Basic.Cnt)
}

func TestCheckSamplerKeepsUnfilteredSketchesPerSecond(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	matcher := stripPodMatcher(t, "other.metric")
	flushMatcher := metricname.NewMatcher(nil, false)

	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 98.5), matcher)
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 99.1), matcher)
	cs.commit(99.3, &flushMatcher)
	out := flushSampler(cs)

	assert.Empty(t, out.mergedSketches)
	require.Len(t, out.sketches, 1)
	require.Len(t, out.sketches[0].Points, 1, "the commit second is held until the next commit, as without filtering")
	assert.Equal(t, int64(98), out.sketches[0].Points[0].Ts)
}

func TestStrippedIdentitiesAreSharedAndReleased(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newStrippedIdentityTable()
	matcher := stripPodMatcher(t, "my.counter")
	flushMatcher := metricname.NewMatcher(nil, false)
	samplers := []*CheckSampler{newStripTestSampler("check:0", table), newStripTestSampler("check:1", table)}

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

func TestCheckSamplerRestripsWhenRulesChange(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	flushMatcher := metricname.NewMatcher(nil, false)
	key := cs.contextResolver.trackContext(podSample("my.dist", metrics.DistributionType, "a", 1, 100), filterlistimpl.NewNoopTagMatcher())

	_, stripped := strippedTags(cs, key)
	require.False(t, stripped)

	// A rule is added.
	cs.restrip(stripPodMatcher(t, "my.dist"))
	tags, stripped := strippedTags(cs, key)
	require.True(t, stripped)
	assert.ElementsMatch(t, []string{"env:prod"}, tags)

	// The rule is removed: the context's output keeps its full tags, and its sketches are released per second again.
	cs.restrip(filterlistimpl.NewNoopTagMatcher())
	_, stripped = strippedTags(cs, key)
	require.False(t, stripped)
	assert.False(t, cs.releasesAtCommit(key))
	assert.Empty(t, cs.contextResolver.resolver.stripped.byKey)
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100.1), filterlistimpl.NewNoopTagMatcher())
	cs.commit(101.3, &flushMatcher)
	out := flushSampler(cs)
	assert.Empty(t, out.mergedSketches)
	require.Len(t, out.sketches, 1)
	assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, tagsOf(out.sketches[0].Tags))
	assert.Equal(t, int64(100), out.sketches[0].Points[0].Ts)
}

// Context keys don't include the metric type: samples of different types with the same name and tags share a
// context, whichever came first.
func TestCheckSamplerStripsOnlyMergeableOutputOfSharedContexts(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	flushMatcher := metricname.NewMatcher(nil, false)

	t.Run("histogram then distribution", func(t *testing.T) {
		cs := newStripTestSampler("check:0", newStrippedIdentityTable())
		matcher := stripPodMatcher(t, "my.metric")
		cs.addSample(podSample("my.metric", metrics.HistogramType, "a", 1, 100), matcher)
		cs.addSample(podSample("my.metric", metrics.DistributionType, "a", 1, 100), matcher)
		cs.commit(101, &flushMatcher)
		out := flushSampler(cs)

		require.Len(t, out.mergedSketches, 1, "distributions are stripped once they reach a context, whichever type created it")
		assert.ElementsMatch(t, []string{"env:prod"}, tagsOf(out.mergedSketches[0].sketch.Tags))
		assert.Empty(t, out.mergedSeries, "histogram statistics can't be merged")
		require.NotEmpty(t, out.series)
		for _, s := range out.series {
			assert.Contains(t, tagsOf(s.Tags), "pod:a")
		}
	})

	t.Run("distribution then explicit-timestamp count", func(t *testing.T) {
		cs := newStripTestSampler("check:0", newStrippedIdentityTable())
		matcher := stripPodMatcher(t, "my.metric")
		cs.addSample(podSample("my.metric", metrics.DistributionType, "a", 1, 100), matcher)
		cs.addSample(&metrics.MetricSample{Name: "my.metric", Mtype: metrics.CountWithTimestampType, Value: 5,
			Tags: []string{"env:prod", "pod:a"}, SampleRate: 1, Timestamp: 90}, matcher)
		cs.commit(101, &flushMatcher)
		out := flushSampler(cs)

		assert.Empty(t, out.mergedSeries, "explicit-timestamp metrics are never merged")
		require.Len(t, out.series, 1)
		assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, tagsOf(out.series[0].Tags))
		assert.Equal(t, []metrics.Point{{Ts: 90, Value: 5}}, out.series[0].Points)
		assert.Len(t, out.mergedSketches, 1)
	})

	t.Run("distribution then histogram", func(t *testing.T) {
		cs := newStripTestSampler("check:0", newStrippedIdentityTable())
		matcher := stripPodMatcher(t, "my.metric")
		cs.addSample(podSample("my.metric", metrics.DistributionType, "a", 1, 100), matcher)
		cs.addSample(podSample("my.metric", metrics.HistogramType, "a", 1, 100), matcher)
		cs.commit(101, &flushMatcher)
		out := flushSampler(cs)

		assert.Empty(t, out.mergedSeries, "histogram statistics can't be merged")
		require.NotEmpty(t, out.series)
		for _, s := range out.series {
			assert.Contains(t, tagsOf(s.Tags), "pod:a")
		}
		assert.Len(t, out.mergedSketches, 1)
	})

}
