// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"slices"
	"sort"
	"strings"
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
	cs.markStrippedMetrics()
	tags, stripped := strippedTags(cs, key)
	require.True(t, stripped)
	assert.ElementsMatch(t, []string{"env:prod"}, tags)

	// The rule is removed: the context stops being stripped, but the metric's sketches stay stamped at commit and go
	// to the merger, with their full tags, so that none can land in a second already sent for the series, and two
	// commits in one second are merged.
	cs.restrip(filterlistimpl.NewNoopTagMatcher())
	cs.markStrippedMetrics()
	_, stripped = strippedTags(cs, key)
	require.False(t, stripped)
	assert.True(t, cs.releasesAtCommit(key))
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100.1), filterlistimpl.NewNoopTagMatcher())
	cs.commit(100.3, &flushMatcher)
	_, sketches := cs.flushMerged()
	require.Len(t, sketches, 1)
	assert.Equal(t, key, sketches[0].key)
	assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, tagsOf(sketches[0].sketch.Tags))
	assert.Equal(t, int64(100), sketches[0].sketch.Points[0].Ts)
	assert.True(t, cs.releasesAtCommit(key))
	assert.Empty(t, cs.contextResolver.resolver.stripped.byKey)
}

func TestCheckSamplerStripsFilteredCounts(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	matcher := stripPodMatcher(t, "my.count", "my.histogram")
	flushMatcher := metricname.NewMatcher(nil, false)

	for _, pod := range []string{"a", "b"} {
		cs.addSample(podSample("my.count", metrics.CountType, pod, 2, 100.1), matcher)
		cs.addSample(podSample("my.histogram", metrics.HistogramType, pod, 2, 100.1), matcher)
	}
	cs.commit(100.4, &flushMatcher)
	out := flushSampler(cs)

	require.Len(t, out.mergedSeries, 2)
	for _, s := range out.mergedSeries {
		assert.Equal(t, "my.count", s.Name)
		assert.Equal(t, metrics.APICountType, s.MType)
		assert.ElementsMatch(t, []string{"env:prod"}, tagsOf(s.Tags))
	}
	require.NotEmpty(t, out.series)
	for _, s := range out.series {
		assert.True(t, strings.HasPrefix(s.Name, "my.histogram."), "histogram statistics can't be merged, so they keep their tags")
		assert.True(t, slices.ContainsFunc(tagsOf(s.Tags), func(tag string) bool { return strings.HasPrefix(tag, "pod:") }))
	}
}

func TestCountAndCounterWithTheSameNameStayApart(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newStrippedIdentityTable()
	matcher := stripPodMatcher(t, "my.mixed")
	flushMatcher := metricname.NewMatcher(nil, false)
	merger := newTestCheckOutputMerger()
	var series metrics.Series

	for i, mtype := range []metrics.MetricType{metrics.CountType, metrics.CounterType} {
		cs := newStripTestSampler(string(rune('a'+i)), table)
		cs.addSample(podSample("my.mixed", mtype, string(rune('a'+i)), 1, 100), matcher)
		cs.commit(100.4, &flushMatcher)
		out := flushSampler(cs)
		require.Empty(t, out.series)
		merger.mergeSeries(out.mergedSeries)
	}
	merger.drain(&series, nil)

	require.Len(t, series, 2, "values of different types are never added up: both are sent, and the intake keeps one")
	assert.ElementsMatch(t, []metrics.APIMetricType{metrics.APICountType, metrics.APIRateType}, []metrics.APIMetricType{series[0].MType, series[1].MType})
}

func TestCheckSamplerKeepsMonotonicCountsPerSource(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	matcher := stripPodMatcher(t, "my.mono")
	flushMatcher := metricname.NewMatcher(nil, false)
	merger := newTestCheckOutputMerger()

	// Three sources strip to one series: two with a pod tag, and one whose full tags already are the stripped
	// series'. Their raw counters are far apart: sharing state between them would produce differences between
	// sources instead of each source's increase.
	sourceTags := map[string][]string{"a": {"env:prod", "pod:a"}, "b": {"env:prod", "pod:b"}, "c": {"env:prod"}}
	type raw struct {
		value      float64
		flushFirst bool
	}
	run := func(second int, samples map[string]raw) (series metrics.Series) {
		for source, r := range samples {
			cs.addSample(&metrics.MetricSample{Name: "my.mono", Mtype: metrics.MonotonicCountType, Value: r.value, Tags: sourceTags[source],
				SampleRate: 1, Timestamp: float64(second), FlushFirstValue: r.flushFirst}, matcher)
		}
		cs.commit(float64(second)+0.5, &flushMatcher)
		out := flushSampler(cs)
		require.Empty(t, out.series)
		merger.mergeSeries(out.mergedSeries)
		merger.drain(&series, nil)
		return series
	}
	values := func(series metrics.Series) map[string]float64 {
		out := map[string]float64{}
		for _, s := range series {
			for _, p := range s.Points {
				out[strings.Join(sortedTagsOf(s.Tags), ",")] += p.Value
			}
		}
		return out
	}

	assert.Empty(t, run(100, map[string]raw{"a": {10, false}, "b": {1000, false}, "c": {100000, false}}), "no previous values yet")
	assert.Equal(t, map[string]float64{"env:prod": 5 + 2 + 4}, values(run(101, map[string]raw{"a": {15, false}, "b": {1002, false}, "c": {100004, false}})))
	// b resets without flushFirstValue: its sample is ignored.
	assert.Equal(t, map[string]float64{"env:prod": 1 + 1}, values(run(102, map[string]raw{"a": {16, false}, "b": {1, false}, "c": {100005, false}})))
	// a stops: its context expires after two commits without samples, but its state is kept.
	assert.Equal(t, map[string]float64{"env:prod": 2 + 1}, values(run(103, map[string]raw{"b": {3, false}, "c": {100006, false}})))
	assert.Equal(t, map[string]float64{"env:prod": 1 + 1}, values(run(104, map[string]raw{"b": {4, false}, "c": {100007, false}})))
	assert.Equal(t, map[string]float64{"env:prod": 1}, values(run(105, map[string]raw{"c": {100008, false}})))
	// a comes back with flushFirstValue: only its increase since its previous value counts. b resets with
	// flushFirstValue: its new value counts.
	assert.Equal(t, map[string]float64{"env:prod": 5 + 2 + 1}, values(run(106, map[string]raw{"a": {21, true}, "b": {2, true}, "c": {100009, false}})))

	// The rule is removed: every source goes on from its own state, in its own series.
	cs.restrip(filterlistimpl.NewNoopTagMatcher())
	cs.markStrippedMetrics()
	matcher = filterlistimpl.NewNoopTagMatcher()
	assert.Equal(t, map[string]float64{"env:prod,pod:a": 1, "env:prod,pod:b": 1, "env:prod": 1},
		values(run(107, map[string]raw{"a": {22, false}, "b": {3, false}, "c": {100010, false}})))
}

func sortedTagsOf(tags tagsJoiner) []string {
	out := tagsOf(tags)
	sort.Strings(out)
	return out
}

func TestCheckSamplerObservesMonotonicCountsPerSource(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	handle := &recordingHandle{}
	cs.SetObserverHandle(handle)
	matcher := stripPodMatcher(t, "my.mono", "my.count")

	cs.addSample(podSample("my.mono", metrics.MonotonicCountType, "a", 10, 100), matcher)
	cs.addSample(podSample("my.count", metrics.CountType, "a", 1, 100), matcher)

	require.Len(t, handle.calls, 2)
	assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, handle.calls[0].tags, "running totals are only meaningful per source")
	assert.Equal(t, []string{"env:prod"}, handle.calls[1].tags)
	assertObservedContextKey(t, handle.calls[0])
	assertObservedContextKey(t, handle.calls[1])
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

// flushThrough commits a sampler, and sends its output through a merger.
func flushThrough(cs *CheckSampler, merger *checkOutputMerger, commit float64, cutoff int64) (metrics.Series, metrics.SketchSeriesList) {
	flushMatcher := metricname.NewMatcher(nil, false)
	cs.commit(commit, &flushMatcher)
	out := flushSampler(cs)
	sentSeries, sentSketches := out.series, out.sketches
	merger.mergeSeries(out.mergedSeries)
	merger.mergeSketches(out.mergedSketches)
	merger.flush(cutoff, &sentSeries, &sentSketches)
	return sentSeries, sentSketches
}

func TestCheckSamplerMergesStampedSketchesAfterARuleIsRemoved(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newStripTestSampler("check:0", newStrippedIdentityTable())
	merger := newTestCheckOutputMerger()
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100), stripPodMatcher(t, "my.dist"))
	flushThrough(cs, merger, 100.5, 101)

	// The rule is removed: the metric's sketches stay stamped at commit, so two commits in one second are two
	// points for the same second, which must be merged.
	noRule := filterlistimpl.NewNoopTagMatcher()
	cs.restrip(noRule)
	cs.markStrippedMetrics()
	var sketches metrics.SketchSeriesList
	for _, commit := range []float64{103.2, 103.6} {
		cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, commit-0.1), noRule)
		_, sent := flushThrough(cs, merger, commit, int64(commit)) // flushes in the same second hold it
		sketches = append(sketches, sent...)
	}
	_, sent := flushThrough(cs, merger, 104.5, 110)
	sketches = append(sketches, sent...)

	var points int
	var count int64
	for _, s := range sketches {
		assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, tagsOf(s.Tags))
		for _, p := range s.Points {
			points++
			count += p.Sketch.Basic.Cnt
			assert.Equal(t, int64(103), p.Ts)
		}
	}
	assert.Equal(t, 1, points, "one point for the second")
	assert.Equal(t, int64(2), count)
}

func TestCheckSamplerKeepsExplicitTimestampsOutOfTheMerge(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newStrippedIdentityTable()
	matcher := stripPodMatcher(t, "my.count")
	merger := newTestCheckOutputMerger()

	// A filtered count: second 100 is sent, and second 101 is held.
	filtered := newStripTestSampler("check:0", table)
	filtered.addSample(podSample("my.count", metrics.CountType, "a", 1, 100), matcher)
	flushThrough(filtered, merger, 100.5, 101)
	filtered.addSample(podSample("my.count", metrics.CountType, "a", 1, 101), matcher)
	flushThrough(filtered, merger, 101.5, 101)

	// A count with an explicit timestamp, whose full tags are the filtered series': explicit-timestamp metrics are
	// never filtered, and keep their timestamps.
	explicit := newStripTestSampler("check:1", table)
	explicit.addSample(&metrics.MetricSample{Name: "my.count", Mtype: metrics.CountWithTimestampType, Value: 5,
		Tags: []string{"env:prod"}, SampleRate: 1, Timestamp: 90}, matcher)
	series, _ := flushThrough(explicit, merger, 101.6, 101)

	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 90, Value: 5}}, series[0].Points)
}
