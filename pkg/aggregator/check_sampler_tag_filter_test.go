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

func filterPodMatcher(t *testing.T, names ...string) filterlistdef.TagMatcher {
	rules := map[string]filterlistimpl.MetricTagList{}
	for _, name := range names {
		rules[name] = filterlistimpl.MetricTagList{Tags: []string{"pod"}, Action: "exclude"}
	}
	return filterlistimpl.NewTagMatcher(rules, logmock.New(t))
}

func newTagFilterTestSampler(id string, table *filteredIdentityTable) *CheckSampler {
	return newCheckSampler(2, true, false, time.Hour, true, true, tags.NewStore(true, "test"), checkid.ID(id), nooptagger.NewComponent(), table)
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
	identity, filtered := cs.contextResolver.resolver.filteredIdentityOf(ctx)
	if !filtered {
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

func TestContextFitsIn64Bytes(t *testing.T) {
	// The filtered identity reference and the output flags live in Context's padding.
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
	require.Len(t, out.mergedSeries, 2, "sources are aggregated apart, then merged after the sampler")
	assert.Equal(t, out.mergedSeries[0].ContextKey, out.mergedSeries[1].ContextKey)
	for _, s := range out.mergedSeries {
		assert.ElementsMatch(t, []string{"env:prod"}, tagsOf(s.Tags))
	}

	require.Len(t, out.mergedSketches, 2)
	assert.Equal(t, out.mergedSketches[0].key, out.mergedSketches[1].key)
	for _, s := range out.mergedSketches {
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
	_, merged := cs.flushMerged()
	assert.Empty(t, merged)
	_, sketches := cs.flush()
	require.Len(t, sketches, 1)
	assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, tagsOf(sketches[0].Tags))
	assert.Equal(t, int64(100), sketches[0].Points[0].Ts)
	assert.Empty(t, cs.contextResolver.resolver.filteredIdentities.byKey)
}

func TestCheckSamplerFiltersTagsOfCounts(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newTagFilterTestSampler("check:0", newFilteredIdentityTable())
	matcher := filterPodMatcher(t, "my.count", "my.histogram")
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
	table := newFilteredIdentityTable()
	matcher := filterPodMatcher(t, "my.mixed")
	flushMatcher := metricname.NewMatcher(nil, false)
	merger := newCheckOutputMerger()
	var series metrics.Series

	for i, mtype := range []metrics.MetricType{metrics.CountType, metrics.CounterType} {
		cs := newTagFilterTestSampler(string(rune('a'+i)), table)
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
	cs := newTagFilterTestSampler("check:0", newFilteredIdentityTable())
	matcher := filterPodMatcher(t, "my.mono")
	flushMatcher := metricname.NewMatcher(nil, false)
	merger := newCheckOutputMerger()

	// Three sources filter to one series: two with a pod tag, and one whose full tags already are the filtered
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
		series = append(series, out.series...)
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
	cs.refilter(filterlistimpl.NewNoopTagMatcher())
	matcher = filterlistimpl.NewNoopTagMatcher()
	assert.Equal(t, map[string]float64{"env:prod,pod:a": 1, "env:prod,pod:b": 1, "env:prod": 1},
		values(run(107, map[string]raw{"a": {22, false}, "b": {3, false}, "c": {100010, false}})))
}

func sortedTagsOf(tags tagsJoiner) []string {
	out := tagsOf(tags)
	sort.Strings(out)
	return out
}

// Context keys don't include the metric type: samples of different types with the same name and tags share a
// context. Once a context gets samples of a type that can't be filtered, none of its output is filtered.
func TestCheckSamplerDoesNotFilterContextsSharedWithOtherTypes(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	flushMatcher := metricname.NewMatcher(nil, false)
	histogram := podSample("my.metric", metrics.HistogramType, "a", 1, 100)
	distribution := podSample("my.metric", metrics.DistributionType, "a", 1, 100)
	explicitCount := &metrics.MetricSample{Name: "my.metric", Mtype: metrics.CountWithTimestampType, Value: 5,
		Tags: []string{"env:prod", "pod:a"}, SampleRate: 1, Timestamp: 90}

	for name, samples := range map[string][]*metrics.MetricSample{
		"histogram then distribution":                {histogram, distribution},
		"distribution then histogram":                {distribution, histogram},
		"distribution then explicit-timestamp count": {distribution, explicitCount},
	} {
		t.Run(name, func(t *testing.T) {
			table := newFilteredIdentityTable()
			cs := newTagFilterTestSampler("check:0", table)
			matcher := filterPodMatcher(t, "my.metric")
			for _, sample := range samples {
				cs.addSample(sample, matcher)
			}
			cs.commit(101, &flushMatcher)
			out := flushSampler(cs)

			assert.Empty(t, out.mergedSeries)
			assert.Empty(t, out.mergedSketches)
			require.Len(t, out.sketches, 1)
			assert.Contains(t, tagsOf(out.sketches[0].Tags), "pod:a")
			for _, s := range out.series {
				assert.Contains(t, tagsOf(s.Tags), "pod:a", s.Name)
			}
			assert.Empty(t, table.byKey, "the identity the context filtered to is released")
		})
	}
}

func TestCheckSamplerStopsMergingAContextSharedWithAnotherType(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newFilteredIdentityTable()
	cs := newTagFilterTestSampler("check:0", table)
	matcher := filterPodMatcher(t, "my.metric")
	flushMatcher := metricname.NewMatcher(nil, false)

	cs.addSample(podSample("my.metric", metrics.CounterType, "a", 1, 100), matcher)
	cs.commit(100.5, &flushMatcher)
	out := flushSampler(cs)
	require.Len(t, out.mergedSeries, 1)
	require.Len(t, table.byKey, 1)

	// A gauge with the same name and tags arrives: the context's output keeps its tags from then on.
	cs.addSample(podSample("my.metric", metrics.CounterType, "a", 1, 101), matcher)
	cs.addSample(podSample("my.metric", metrics.GaugeType, "a", 1, 101), matcher)
	cs.commit(101.5, &flushMatcher)
	out = flushSampler(cs)
	assert.Empty(t, out.mergedSeries)
	assert.Empty(t, table.byKey, "the identity is released")
	require.NotEmpty(t, out.series)
	for _, s := range out.series {
		assert.Contains(t, tagsOf(s.Tags), "pod:a")
	}
}

// TestCheckSamplerOnlyMergesSeriesThatAddUp checks that the state of a metric that outlived its context, such as a
// histogram's, doesn't reach the merger through a new context with the same key.
func TestCheckSamplerOnlyMergesSeriesThatAddUp(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newTagFilterTestSampler("check:0", newFilteredIdentityTable())
	matcher := filterPodMatcher(t, "my.metric")
	flushMatcher := metricname.NewMatcher(nil, false)

	cs.addSample(podSample("my.metric", metrics.HistorateType, "a", 1, 100), matcher)
	cs.addSample(podSample("my.metric", metrics.HistorateType, "a", 3, 100.2), matcher)
	cs.commit(100.5, &flushMatcher)
	// The context expires after two commits without samples, but the historate's state is kept.
	for commit := 0; commit < 3; commit++ {
		cs.commit(float64(101+commit), &flushMatcher)
	}
	flushSampler(cs)

	// A counter with the same key creates a new context, which merges at output, and goes into the historate's state.
	cs.addSample(podSample("my.metric", metrics.CounterType, "a", 1, 105), matcher)
	cs.addSample(podSample("my.metric", metrics.CounterType, "a", 5, 105.2), matcher)
	cs.commit(105.5, &flushMatcher)
	out := flushSampler(cs)
	for _, s := range out.mergedSeries {
		assert.Empty(t, s.NameSuffix, "%s: a histogram's statistics don't add up", s.Name)
		assert.NotEqual(t, metrics.APIGaugeType, s.MType, s.Name)
	}
	assert.NotEmpty(t, out.series, "the series that don't add up are sent with their tags")
}

func TestCheckSamplerDoesNotFilterSketchesStampedPerSecond(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newFilteredIdentityTable()
	cs := newCheckSampler(2, true, false, time.Hour, true, false, tags.NewStore(true, "test"), checkid.ID("check:0"), nooptagger.NewComponent(), table)
	matcher := filterPodMatcher(t, "my.counter", "my.dist")
	flushMatcher := metricname.NewMatcher(nil, false)

	cs.addSample(podSample("my.counter", metrics.CounterType, "a", 1, 100.1), matcher)
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100.1), matcher)
	cs.commit(101.4, &flushMatcher)
	out := flushSampler(cs)

	require.Len(t, out.mergedSeries, 1, "counters are still filtered and merged")
	assert.Empty(t, out.mergedSketches)
	require.Len(t, out.sketches, 1)
	assert.Contains(t, tagsOf(out.sketches[0].Tags), "pod:a", "sketches stamped per second aren't filtered")
	assert.Equal(t, int64(100), out.sketches[0].Points[0].Ts)
	assert.Len(t, table.byKey, 1, "only the counter has a filtered identity")
}

func TestBecomesSketch(t *testing.T) {
	assert.True(t, becomesSketch(&metrics.MetricSample{Mtype: metrics.DistributionType}))
	assert.True(t, becomesSketch(&metrics.HistogramBucket{}), "histogram buckets are interpolated into sketches")
	assert.False(t, becomesSketch(&metrics.MetricSample{Mtype: metrics.CounterType}))
	assert.False(t, becomesSketch(&metrics.MetricSample{Mtype: metrics.HistogramType}), "ordinary histograms become series")
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

func TestCheckSamplerKeepsExplicitTimestampsOutOfTheMerge(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newFilteredIdentityTable()
	matcher := filterPodMatcher(t, "my.count")
	merger := newCheckOutputMerger()

	// A filtered count: second 100 is sent, and second 101 is held.
	filtered := newTagFilterTestSampler("check:0", table)
	filtered.addSample(podSample("my.count", metrics.CountType, "a", 1, 100), matcher)
	flushThrough(filtered, merger, 100.5, 101)
	filtered.addSample(podSample("my.count", metrics.CountType, "a", 1, 101), matcher)
	flushThrough(filtered, merger, 101.5, 101)

	// A count with an explicit timestamp, whose full tags are the filtered series': explicit-timestamp metrics are
	// never filtered, and keep their timestamps.
	explicit := newTagFilterTestSampler("check:1", table)
	explicit.addSample(&metrics.MetricSample{Name: "my.count", Mtype: metrics.CountWithTimestampType, Value: 5,
		Tags: []string{"env:prod"}, SampleRate: 1, Timestamp: 90}, matcher)
	series, _ := flushThrough(explicit, merger, 101.6, 101)

	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 90, Value: 5}}, series[0].Points)
}
