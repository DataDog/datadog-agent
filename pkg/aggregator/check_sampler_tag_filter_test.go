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
	cs.markFilteredMetrics()
	tags, filtered := filteredTags(cs, key)
	require.True(t, filtered)
	assert.ElementsMatch(t, []string{"env:prod"}, tags)

	// The rule is removed: the context stops being filtered, but the metric's output keeps going to the merger, with
	// its full tags, so that none of it can land in a second already sent for the series.
	cs.refilter(filterlistimpl.NewNoopTagMatcher())
	cs.markFilteredMetrics()
	_, filtered = filteredTags(cs, key)
	require.False(t, filtered)
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100.1), filterlistimpl.NewNoopTagMatcher())
	cs.commit(100.3, &flushMatcher)
	_, sketches := cs.flushMerged()
	require.Len(t, sketches, 1)
	assert.Equal(t, key, sketches[0].key)
	assert.ElementsMatch(t, []string{"env:prod", "pod:a"}, tagsOf(sketches[0].sketch.Tags))
	assert.Equal(t, int64(100), sketches[0].sketch.Points[0].Ts)
	assert.Empty(t, cs.contextResolver.resolver.filteredIdentities.byKey)
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

func TestCheckSamplerMergesStampedSketchesAfterARuleIsRemoved(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	cs := newTagFilterTestSampler("check:0", newFilteredIdentityTable())
	merger := newTestCheckOutputMerger()
	cs.addSample(podSample("my.dist", metrics.DistributionType, "a", 1, 100), filterPodMatcher(t, "my.dist"))
	flushThrough(cs, merger, 100.5, 101)

	// The rule is removed: the metric's sketches stay stamped at commit, so two commits in one second are two
	// points for the same second, which must be merged.
	noRule := filterlistimpl.NewNoopTagMatcher()
	cs.refilter(noRule)
	cs.markFilteredMetrics()
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

func TestFilteredIdentitiesMarkWhenMetricsStartBeingFiltered(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newFilteredIdentityTable()
	cs := newTagFilterTestSampler("check:0", table)
	flushMatcher := metricname.NewMatcher(nil, false)

	// Output committed before the metric is filtered may have used second 100.
	cs.addSample(podSample("my.counter", metrics.CounterType, "a", 1, 100), filterlistimpl.NewNoopTagMatcher())
	cs.commit(100.5, &flushMatcher)
	assert.Empty(t, table.takeNotBefore())

	// A new source starts filtering the metric, without a rule change.
	cs.addSample(podSample("my.counter", metrics.CounterType, "b", 1, 100.6), filterPodMatcher(t, "my.counter"))
	assert.Equal(t, map[string]int64{"my.counter": 101}, table.takeNotBefore())
	assert.Empty(t, table.takeNotBefore(), "taken once")

	cs.addSample(podSample("my.counter", metrics.CounterType, "c", 1, 100.7), filterPodMatcher(t, "my.counter"))
	assert.Empty(t, table.takeNotBefore(), "only the first time a metric is filtered")
}

func TestFilteredIdentitiesMarkPastPeriodicFlushes(t *testing.T) {
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
	table := newFilteredIdentityTable()
	periodic := newTagFilterTestSampler("check:0", table)
	other := newTagFilterTestSampler("check:1", table)
	flushMatcher := metricname.NewMatcher(nil, false)

	// A periodic flush may use second 109, and a commit of another check before it only second 108.
	other.commit(108.5, &flushMatcher)
	periodic.addSample(podSample("my.counter", metrics.CounterType, "a", 1, 109), filterlistimpl.NewNoopTagMatcher())
	periodic.commitDecomposable(110.5, &flushMatcher)

	periodic.addSample(podSample("my.counter", metrics.CounterType, "b", 1, 110.6), filterPodMatcher(t, "my.counter"))
	assert.Equal(t, map[string]int64{"my.counter": 110}, table.takeNotBefore())

	// A periodic flush stamped before the latest commit doesn't move the mark back.
	other.commit(112.5, &flushMatcher)
	periodic.commitDecomposable(112.6, &flushMatcher)
	periodic.addSample(podSample("other.counter", metrics.CounterType, "b", 1, 112.7), filterPodMatcher(t, "other.counter"))
	assert.Equal(t, map[string]int64{"other.counter": 113}, table.takeNotBefore())
}
