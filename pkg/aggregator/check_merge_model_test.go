// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"math/rand"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// The model test drives the aggregator's check path through random sequences of samples, commits, flushes, rule
// changes, sampler removals and clock steps, on a clock the test controls. A model predicts every point: the series
// it's sent in, its second, and its value. For filtering, the model follows the documented behavior; for behavior
// that predates it (when contexts expire, sketches held in their commit second), it reproduces the existing code.
// The test checks:
//
//   - Every committed contribution is sent once, in the series it filters to under the rules when it's released, and
//     in its own second.
//   - Outside seconds a backward clock step reached again, a merged series never has a second sent twice, nor,
//     outside the second of a rule change, sent both merged and not. Like the rest of the Agent's output, the merge
//     stage doesn't guard against those.
//   - Without clock steps, a flush has sent everything committed before its cutoff, and, outside the second of a
//     rule change, a merged point isn't indexed if any of its second's sources asks not to be, and has the lowest
//     of their origins.
//   - Metrics no rule ever matches are sent exactly as by an aggregator without rules.
//   - Filtered identities are referenced by exactly the contexts that filter to them, and released with them.
//
// Monotonic counts keep state per source, which the model doesn't reproduce: the aggregator without rules computes
// each source as without filtering, and the model expects its output, filtered under the rules at the commit.

const (
	modelCounter                  = "m.counter"
	modelCount                    = "m.count"
	modelDist                     = "m.dist"
	modelMono                     = "m.mono"
	modelHistBuckets              = "m.hist"
	modelHistBucketBound          = 2
	modelUnfilteredCounter        = "m.other"
	modelUnfilteredDist           = "m.otherdist"
	modelSamplers                 = 3
	modelBucketCommitsCountExpiry = 2
)

// modelFiltered are the metrics rules match.
var modelFiltered = []string{modelCounter, modelCount, modelDist, modelMono, modelHistBuckets}

// modelRules are the rule sets a run switches between: which tag names are filtered from the filtered metrics.
var modelRules = [][]string{nil, {"pod"}, {"zone"}, {"pod", "zone"}}

var modelMetrics = []struct {
	name  string
	mtype metrics.MetricType
}{
	{modelCounter, metrics.CounterType},
	{modelCount, metrics.CountType},
	{modelDist, metrics.DistributionType},
	{modelMono, metrics.MonotonicCountType},
	{modelHistBuckets, metrics.HistogramType},
	{modelUnfilteredCounter, metrics.CounterType},
	{modelUnfilteredDist, metrics.DistributionType},
}

// modelSources returns the tags of a sampler's sources. The last sampler also has a source without a pod tag,
// whose full tags are what the others filter to when the pod tag is filtered.
func modelSources(sampler int) [][]string {
	return modelSourceTags[sampler]
}

var modelSourceTags = func() [][][]string {
	all := make([][][]string, modelSamplers)
	for i := range all {
		all[i] = makeModelSources(i)
	}
	return all
}()

// modelSourceMeta returns a source's metadata: some sources ask not to be indexed, and sources have different
// origins, to check how merged series combine them.
func modelSourceMeta(tags []string) (noIndex bool, source metrics.MetricSource) {
	var h uint
	for _, tag := range tags {
		for _, c := range tag {
			h = h*31 + uint(c)
		}
	}
	origins := []metrics.MetricSource{metrics.MetricSourceOpenmetrics, metrics.MetricSourceDogstatsd, metrics.MetricSourceJmxCustom}
	return h%3 == 0, origins[(h/3)%uint(len(origins))]
}

func makeModelSources(sampler int) [][]string {
	sources := [][]string{
		{"env:prod", fmt.Sprintf("pod:p%d-0", sampler), "zone:z0"},
		{"env:prod", fmt.Sprintf("pod:p%d-1", sampler), "zone:z1"},
	}
	if sampler == modelSamplers-1 {
		sources = append(sources, []string{"env:prod", "zone:z0"})
	}
	return sources
}

type modelConfig struct {
	ruleChanges bool
	clockSteps  bool
	// longClockSteps makes some backward steps longer than how far ahead the merger holds points.
	longClockSteps bool
	removals       bool
}

// modelContext is the model of a sampler's context: when it was last sampled, and its counter since the last commit.
type modelContext struct {
	name     string
	tags     []string
	lastSeen int
	counter  float64
	sampled  bool
}

type modelValue struct {
	Count int64
	Sum   float64
}

// modelMeta is what the sources of a series' second ask for: whether any asks not to be indexed, and the lowest
// origin.
type modelMeta struct {
	noIndex bool
	source  metrics.MetricSource
	set     bool
}

func (m modelMeta) add(noIndex bool, source metrics.MetricSource) modelMeta {
	if !m.set {
		return modelMeta{noIndex, source, true}
	}
	return modelMeta{m.noIndex || noIndex, min(m.source, source), true}
}

func (v modelValue) add(o modelValue) modelValue { return modelValue{v.Count + o.Count, v.Sum + o.Sum} }

type modelSampler struct {
	commits int // calls to expireContexts so far
	// cumulative is each source's raw monotonic counter.
	cumulative map[string]float64
	contexts   map[string]*modelContext
	// sketches are the sketch samples since the last commit, by context.
	sketches map[string]modelValue
}

// modelPoints are values by series identity, then second.
type modelPoints map[string]map[int64]modelValue

func (p modelPoints) add(series string, second int64, v modelValue) {
	if p[series] == nil {
		p[series] = map[int64]modelValue{}
	}
	p[series][second] = p[series][second].add(v)
}

type modelRun struct {
	t   *testing.T
	rng *rand.Rand
	cfg modelConfig
	agg *BufferedAggregator
	// baseline gets the same events without rules, for metrics no rule matches.
	baseline *BufferedAggregator

	clock     float64
	highWater float64 // highest clock reached before the latest backward step
	rule      int
	// Per sampler: whether it is registered, and whether it was deregistered since the last flush.
	registered []bool
	removing   []bool
	samplers   []*modelSampler
	// ruleChangeSeconds are the seconds rules changed in.
	ruleChangeSeconds map[int64]bool

	expected        modelPoints
	expectedMeta    map[string]modelMeta // by series and second
	sent            modelPoints
	mergedSeconds   map[string]bool
	unmergedSeconds map[string]bool
	events          []string // to report how a failing run got there
}

func newModelAggregator() *BufferedAggregator {
	return NewBufferedAggregator(nil, nil, haagentmock.NewMockHaAgent(), nooptagger.NewComponent(), "hostname", DefaultFlushInterval, filterlistmock.NewMockFilterList())
}

func newModelRun(t *testing.T, seed int64, cfg modelConfig) *modelRun {
	r := &modelRun{
		t: t, rng: rand.New(rand.NewSource(seed)), cfg: cfg,
		agg:               newModelAggregator(),
		baseline:          newModelAggregator(),
		clock:             1_000_000,
		registered:        make([]bool, modelSamplers),
		removing:          make([]bool, modelSamplers),
		samplers:          make([]*modelSampler, modelSamplers),
		ruleChangeSeconds: map[int64]bool{},
		expected:          modelPoints{},
		expectedMeta:      map[string]modelMeta{},
		sent:              modelPoints{},
		mergedSeconds:     map[string]bool{},
		unmergedSeconds:   map[string]bool{},
	}
	r.highWater = r.clock
	for i := range r.registered {
		r.register(i)
	}
	// The first rules are in place before any data, as when they're loaded at startup.
	r.applyRule(r.rng.Intn(len(modelRules)))
	return r
}

func (r *modelRun) event(format string, args ...any) {
	r.events = append(r.events, fmt.Sprintf("%.2f ", r.clock)+fmt.Sprintf(format, args...))
}

func (r *modelRun) history() string {
	return strings.Join(r.events[max(0, len(r.events)-200):], "\n")
}

func modelSamplerID(i int) checkid.ID { return checkid.ID(fmt.Sprintf("openmetrics:%d", i)) }

func (r *modelRun) register(i int) {
	r.event("register %d", i)
	r.agg.handleRegisterSampler(modelSamplerID(i))
	r.baseline.handleRegisterSampler(modelSamplerID(i))
	if !r.registered[i] {
		r.samplers[i] = &modelSampler{contexts: map[string]*modelContext{}, sketches: map[string]modelValue{}, cumulative: map[string]float64{}}
	}
	r.registered[i], r.removing[i] = true, false
}

// ruleMatches returns whether the current rules match a metric.
func (r *modelRun) ruleMatches(name string) bool {
	return len(modelRules[r.rule]) > 0 && slices.Contains(modelFiltered, name)
}

func (r *modelRun) applyRule(rule int) {
	r.event("rule %v", modelRules[rule])
	r.rule = rule
	r.ruleChangeSeconds[int64(r.clock)] = true
	rules := map[string]filterlistimpl.MetricTagList{}
	if filtered := modelRules[rule]; len(filtered) > 0 {
		for _, name := range modelFiltered {
			rules[name] = filterlistimpl.MetricTagList{Tags: filtered, Action: "exclude"}
		}
	}
	r.agg.setFilterList(filterlistimpl.NewTagMatcher(rules, logmock.New(r.t)))
}

// identity returns the series a source's metric is sent as under the current rules.
func (r *modelRun) identity(kind, name string, tags []string) string {
	kept := []string{}
	for _, tag := range tags {
		tagName, _, _ := strings.Cut(tag, ":")
		if !r.ruleMatches(name) || !slices.Contains(modelRules[r.rule], tagName) {
			kept = append(kept, tag)
		}
	}
	sort.Strings(kept)
	return kind + "|" + name + "|" + strings.Join(kept, ",")
}

// expect records a contribution the model predicts.
func (r *modelRun) expect(series string, second int64, v modelValue, tags []string) {
	r.expected.add(series, second, v)
	noIndex, source := modelSourceMeta(tags)
	key := fmt.Sprintf("%s@%d", series, second)
	r.expectedMeta[key] = r.expectedMeta[key].add(noIndex, source)
}

func (r *modelRun) sample(i int) {
	sources := modelSources(i)
	tags := sources[r.rng.Intn(len(sources))]
	if i == modelSamplers-1 && r.rng.Intn(2) == 0 {
		tags = sources[len(sources)-1] // the source whose full tags are what the others filter to, often
	}
	noIndex, origin := modelSourceMeta(tags)
	value := float64(1 + r.rng.Intn(5))
	// A check run's samples are stamped when submitted, which can be well before they're handled.
	ts := r.clock
	if r.rng.Intn(5) == 0 {
		ts -= 20 * r.rng.Float64()
	}
	r.event("sample %d %v ts=%.2f", i, tags, ts)
	s := r.samplers[i]
	for _, m := range modelMetrics {
		// A random subset of the metrics, so that series and sketches don't always move together.
		if r.rng.Intn(5) < 2 {
			continue
		}
		sampleValue, flushFirst := value, false
		if m.mtype == metrics.MonotonicCountType {
			// A raw counter, which sometimes resets; some checks flush the first value they see.
			source := strings.Join(tags, ",")
			if r.rng.Intn(10) == 0 {
				s.cumulative[source] = 0
			}
			s.cumulative[source] += value
			sampleValue, flushFirst = s.cumulative[source], r.rng.Intn(2) == 0
		}
		for _, agg := range []*BufferedAggregator{r.agg, r.baseline} {
			if m.name == modelHistBuckets {
				agg.handleSenderBucket(senderHistogramBucket{id: modelSamplerID(i), bucket: &metrics.HistogramBucket{
					Name: m.name, Value: int64(value), LowerBound: modelHistBucketBound, UpperBound: modelHistBucketBound,
					Tags: slices.Clone(tags), Timestamp: ts}})
				continue
			}
			sample := &metrics.MetricSample{Name: m.name, Mtype: m.mtype, Value: sampleValue, Tags: slices.Clone(tags), SampleRate: 1, Timestamp: ts,
				NoIndex: noIndex, Source: origin, FlushFirstValue: flushFirst}
			agg.handleSenderSample(senderMetricSample{id: modelSamplerID(i), metricSample: sample})
		}

		key := m.name + "|" + strings.Join(tags, ",")
		ctx, ok := s.contexts[key]
		if !ok {
			ctx = &modelContext{name: m.name, tags: tags}
			s.contexts[key] = ctx
		}
		ctx.lastSeen = s.commits
		if m.mtype == metrics.MonotonicCountType {
			continue // the aggregator without rules computes it
		}
		if m.mtype != metrics.DistributionType && m.name != modelHistBuckets {
			ctx.counter += value
			ctx.sampled = true
		} else {
			sample := modelValue{1, value}
			if m.name == modelHistBuckets {
				sample = modelValue{Count: int64(value)}
			}
			s.sketches[key] = s.sketches[key].add(sample)
		}
	}
}

func (r *modelRun) commit(i int) {
	r.event("commit %d", i)
	for _, agg := range []*BufferedAggregator{r.agg, r.baseline} {
		agg.mu.Lock()
		cs := agg.checkSamplers[modelSamplerID(i)]
		committed := len(cs.series)
		cs.commit(r.clock, &agg.flushFilterList)
		if agg == r.baseline {
			// This commit's monotonic counts, computed per source without filtering.
			for _, serie := range cs.series[committed:] {
				if serie.Name != modelMono {
					continue
				}
				tags := serie.Tags.UnsafeToReadOnlySliceString()
				for _, p := range serie.Points {
					r.expect(r.identity("serie", modelMono, tags), int64(p.Ts), modelValue{Sum: p.Value}, tags)
				}
			}
		}
		agg.mu.Unlock()
	}

	s, second := r.samplers[i], int64(r.clock)
	for _, ctx := range s.contexts {
		if ctx.sampled {
			r.expect(r.identity("serie", ctx.name, ctx.tags), second, modelValue{Sum: ctx.counter}, ctx.tags)
			ctx.counter, ctx.sampled = 0, false
		}
	}
	// Every sketch is released at commit, stamped at the commit.
	for key, v := range s.sketches {
		ctx := s.contexts[key]
		r.expect(r.identity("sketch", ctx.name, ctx.tags), second, v, ctx.tags)
		delete(s.sketches, key)
	}
	// Contexts expire after expiry commits without samples.
	for key, ctx := range s.contexts {
		if ctx.lastSeen <= s.commits-modelBucketCommitsCountExpiry {
			delete(s.contexts, key)
		}
	}
	s.commits++
}

func (r *modelRun) flush(final bool) {
	// The flush's cutoff can be older than commits it includes: its trigger is created before it's handled.
	cutoff := r.clock - 1.5*r.rng.Float64()
	r.event("flush cutoff=%.2f final=%v", cutoff, final)
	trigger := trigger{time: time.Unix(0, int64(cutoff*float64(time.Second))), final: final}
	got := r.flushAggregator(r.agg, trigger)
	want := r.flushAggregator(r.baseline, trigger)
	for i := range r.removing {
		if r.removing[i] {
			// The sampler is gone, with what its sources submitted since its last commit.
			r.registered[i], r.removing[i] = false, false
		}
	}

	exact := !r.cfg.clockSteps
	var gotOther, wantOther []string
	for _, p := range got {
		r.sent.add(p.series, p.second, p.value)
		// In the second a rule changes, a series can be sent both merged and not, each point with its own sources'.
		if meta, ok := r.expectedMeta[fmt.Sprintf("%s@%d", p.series, p.second)]; exact && ok && !r.ruleChangeSeconds[p.second] {
			// Check samplers don't carry NoIndex onto sketches, filtered or not (unlike DogStatsD's time samplers).
			if strings.HasPrefix(p.series, "serie|") {
				require.True(r.t, p.noIndex || !meta.noIndex, "%s@%d: a source asked not to index it, after:\n%s", p.series, p.second, r.history())
			}
			require.LessOrEqual(r.t, p.source, meta.source, "%s@%d: origin is the lowest of its sources', after:\n%s", p.series, p.second, r.history())
		}
		if p.unruled() {
			gotOther = append(gotOther, p.String())
		} else {
			r.recordSecond(p.series, p.second, p.merged)
		}
	}
	for _, p := range want {
		if p.unruled() {
			wantOther = append(wantOther, p.String())
		}
	}
	sort.Strings(gotOther)
	sort.Strings(wantOther)
	require.Equal(r.t, wantOther, gotOther, "metrics without a rule are sent as without rules, after:\n%s", r.history())

	// Nothing committed before the cutoff is held past the flush.
	if exact {
		for series, want := range r.expected {
			for second, v := range want {
				if float64(second) < float64(int64(cutoff)) || final {
					require.Equal(r.t, v, r.sent[series][second], "%s@%d: committed before the flush's cutoff, but not sent, after:\n%s", series, second, r.history())
				}
			}
		}
	}
}

type modelSentPoint struct {
	series  string
	second  int64
	value   modelValue
	merged  bool
	noIndex bool
	source  metrics.MetricSource
	detail  string
}

// unruled returns whether the point is of a metric no rule ever matches.
func (p modelSentPoint) unruled() bool {
	return strings.Contains(p.series, "|"+modelUnfilteredCounter+"|") || strings.Contains(p.series, "|"+modelUnfilteredDist+"|")
}

func (p modelSentPoint) String() string {
	return fmt.Sprintf("%s@%d=%+v merged=%v %s", p.series, p.second, p.value, p.merged, p.detail)
}

func (r *modelRun) flushAggregator(agg *BufferedAggregator, t trigger) []modelSentPoint {
	// As flushSeriesAndSketches, with the merger's output in sinks of its own, to tell which points it sent.
	var series, mergedSeries metrics.Series
	var sketches, mergedSketches metrics.SketchSeriesList
	agg.getSeriesAndSketches(t.time, &series, &sketches)
	if t.final {
		agg.drainCheckOutputs(&mergedSeries, &mergedSketches)
	} else {
		agg.flushCheckOutputs(t.time, &mergedSeries, &mergedSketches)
	}
	var points []modelSentPoint
	addSeries := func(series metrics.Series, merged bool) {
		for _, serie := range series {
			if !strings.HasPrefix(serie.Name, "m.") {
				continue
			}
			identity := "serie|" + serie.Name + "|" + strings.Join(sortedTags(serie.Tags.UnsafeToReadOnlySliceString()), ",")
			for _, p := range serie.Points {
				points = append(points, modelSentPoint{identity, int64(p.Ts), modelValue{Sum: p.Value}, merged, serie.NoIndex, serie.Source,
					fmt.Sprintf("ts=%v type=%v host=%s noindex=%v source=%v", p.Ts, serie.MType, serie.Host, serie.NoIndex, serie.Source)})
			}
		}
	}
	addSketches := func(sketches metrics.SketchSeriesList, merged bool) {
		for _, sketch := range sketches {
			identity := "sketch|" + sketch.Name + "|" + strings.Join(sortedTags(sketch.Tags.UnsafeToReadOnlySliceString()), ",")
			for _, p := range sketch.Points {
				value := modelValue{p.Sketch.Basic.Cnt, p.Sketch.Basic.Sum}
				if sketch.Name == modelHistBuckets {
					// Bucket counts are interpolated into sketches: only the counts are exact.
					value.Sum = 0
				}
				points = append(points, modelSentPoint{identity, p.Ts, value, merged, sketch.NoIndex, sketch.Source,
					fmt.Sprintf("min=%v max=%v host=%s noindex=%v source=%v", p.Sketch.Basic.Min, p.Sketch.Basic.Max, sketch.Host, sketch.NoIndex, sketch.Source)})
			}
		}
	}
	addSeries(series, false)
	addSeries(mergedSeries, true)
	addSketches(sketches, false)
	addSketches(mergedSketches, true)
	if agg == r.agg {
		for _, p := range points {
			r.event("  sent %s", p)
		}
	}
	return points
}

func sortedTags(tags []string) []string {
	tags = slices.Clone(tags)
	sort.Strings(tags)
	return tags
}

func (r *modelRun) recordSecond(series string, second int64, merged bool) {
	key := fmt.Sprintf("%s@%d", series, second)
	// A backward clock step makes data reuse seconds, merged or not: only seconds it didn't reach again count.
	fresh := !r.cfg.clockSteps || float64(second) > r.highWater
	if merged {
		require.False(r.t, fresh && r.mergedSeconds[key], "merged %s sent twice after:\n%s", key, r.history())
		r.mergedSeconds[key] = true
	} else {
		r.unmergedSeconds[key] = true
	}
	// In the second a rule changes, a source whose full tags are what the others filter to can send its series both
	// merged and not.
	if fresh && !r.ruleChangeSeconds[second] {
		require.False(r.t, r.mergedSeconds[key] && r.unmergedSeconds[key], "%s sent both merged and not after:\n%s", key, r.history())
	}
}

func (r *modelRun) step() {
	switch n := r.rng.Intn(100); {
	case n < 45:
		if i := r.rng.Intn(modelSamplers); r.registered[i] && !r.removing[i] {
			r.sample(i)
		}
	case n < 65:
		if i := r.rng.Intn(modelSamplers); r.registered[i] && !r.removing[i] {
			r.commit(i)
		}
	case n < 75:
		r.flush(false)
	case n < 79 && r.cfg.ruleChanges:
		r.applyRule(r.rng.Intn(len(modelRules)))
	case n < 82 && r.cfg.clockSteps:
		r.highWater = max(r.highWater, r.clock)
		// Within the flush interval of the highest clock reached, unless the steps are long.
		r.clock = max(r.clock-(0.5+2.5*r.rng.Float64()), r.highWater-10)
		if r.cfg.longClockSteps && r.rng.Intn(3) == 0 {
			r.clock -= []float64{20, 60, float64(maxHold) + 100}[r.rng.Intn(3)]
		}
		r.event("clock step")
	case n < 85 && r.cfg.removals:
		i := r.rng.Intn(modelSamplers)
		switch {
		case !r.registered[i] || r.removing[i]:
			r.register(i) // when being removed, it's rescheduled before the flush: the sampler is reused
		default:
			r.event("deregister %d", i)
			r.agg.handleDeregisterSampler(modelSamplerID(i))
			r.baseline.handleDeregisterSampler(modelSamplerID(i))
			r.removing[i] = true
		}
	default:
		// The clock often holds still for a while: bursts of events in one second.
		if r.rng.Intn(2) == 0 {
			r.clock += 0.7 * r.rng.Float64()
		}
	}
}

func (r *modelRun) finish() {
	r.clock += 2
	for i := range r.registered {
		if r.registered[i] && !r.removing[i] {
			r.commit(i)
		}
	}
	r.flush(true)

	for series, want := range r.expected {
		got := r.sent[series]
		var wantTotal, gotTotal modelValue
		for _, v := range want {
			wantTotal = wantTotal.add(v)
		}
		for _, v := range got {
			gotTotal = gotTotal.add(v)
		}
		require.Equal(r.t, wantTotal, gotTotal, "%s: every contribution is sent once, after:\n%s", series, r.history())
		require.Equal(r.t, want, got, "%s: every point is sent in its own second, after:\n%s", series, r.history())
	}
	for series := range r.sent {
		require.Contains(r.t, r.expected, series, "%s: sent, but nothing was committed for it, after:\n%s", series, r.history())
	}

	// Every filtered identity is referenced by the contexts that filter to it, and released with the last of them.
	refs := map[uint32]uint32{}
	for _, cs := range r.agg.checkSamplers {
		for _, entry := range cs.contextResolver.resolver.contextsByKey {
			if entry.context.filteredIdentityRef != 0 {
				refs[entry.context.filteredIdentityRef]++
			}
		}
	}
	table := r.agg.filteredIdentities
	require.Len(r.t, table.byKey, len(refs))
	for _, ref := range table.byKey {
		require.Equal(r.t, refs[ref], table.get(ref).refs, "references to a filtered identity")
	}
	for i := range r.registered {
		if r.registered[i] {
			r.agg.handleDeregisterSampler(modelSamplerID(i))
		}
	}
	r.flushAggregator(r.agg, trigger{time: time.Unix(0, int64(r.clock*float64(time.Second))), final: true})
	require.Empty(r.t, table.byKey, "filtered identities are released with their samplers")
}

// TestCheckOutputMatchesModel runs 200 random sequences of 300 steps per configuration, or 20 with -short. Set
// CHECK_MERGE_MODEL_SEEDS and CHECK_MERGE_MODEL_STEPS to run more.
func TestCheckOutputMatchesModel(t *testing.T) {
	defaultSeeds := 200
	if testing.Short() {
		defaultSeeds = 20
	}
	seeds, steps := modelEnvInt("CHECK_MERGE_MODEL_SEEDS", defaultSeeds), modelEnvInt("CHECK_MERGE_MODEL_STEPS", 300)
	configs := map[string]modelConfig{
		"steady":                {},
		"rule-changes":          {ruleChanges: true},
		"clock-steps":           {clockSteps: true},
		"removals":              {removals: true},
		"rule-changes-removals": {ruleChanges: true, removals: true},
		"everything":            {ruleChanges: true, clockSteps: true, removals: true},
		"long-clock-steps":      {clockSteps: true, longClockSteps: true, removals: true},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)
			for seed := int64(0); seed < int64(seeds); seed++ {
				t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
					r := newModelRun(t, seed, cfg)
					for i := 0; i < steps; i++ {
						r.step()
					}
					r.finish()
				})
			}
		})
	}
}

func modelEnvInt(name string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return fallback
}
