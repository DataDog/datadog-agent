// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// The check output benchmarks compare the aggregator's check path without filtering, with a rule that filters the
// pod tag (sources collapse into a few series), and with a rule that matches no metric.

const benchMetric = "bench.metric"

type benchMode int

const (
	benchNoRule benchMode = iota
	benchFiltersPod
	benchRuleForAnotherMetric
)

func (m benchMode) String() string { return [...]string{"off", "on", "no-match"}[m] }

// benchScenario is check instances submitting one metric type, or monotonic histogram buckets. Each source context
// has a pod tag, which the rule filters, and a ctx tag, which it keeps: sources collapse into `contexts` series.
type benchScenario struct {
	name             string
	mtype            metrics.MetricType
	bucketsPerSource int
	instances        int
	podsPerInstance  int
	contexts         int
	samplesPerRun    int
	newPodsEachRun   bool
}

var benchScenarios = []benchScenario{
	{name: "dist-per-pod-instances", mtype: metrics.DistributionType, instances: 200, podsPerInstance: 1, contexts: 50, samplesPerRun: 5},
	{name: "dist-one-instance", mtype: metrics.DistributionType, instances: 1, podsPerInstance: 200, contexts: 50, samplesPerRun: 5},
	{name: "dist-churn", mtype: metrics.DistributionType, instances: 200, podsPerInstance: 1, contexts: 50, samplesPerRun: 5, newPodsEachRun: true},
	{name: "counter", mtype: metrics.CounterType, instances: 200, podsPerInstance: 1, contexts: 50, samplesPerRun: 5},
	{name: "count", mtype: metrics.CountType, instances: 200, podsPerInstance: 1, contexts: 50, samplesPerRun: 5},
	{name: "monotonic-count", mtype: metrics.MonotonicCountType, instances: 200, podsPerInstance: 1, contexts: 50, samplesPerRun: 1},
	{name: "histogram-buckets", bucketsPerSource: 10, instances: 200, podsPerInstance: 1, contexts: 50},
}

// benchSink drops what the aggregator sends.
type benchSink struct{}

func (benchSink) Append(*metrics.Serie) {}

type benchSketchSink struct{}

func (benchSketchSink) Append(*metrics.SketchSeries) {}

type benchHarness struct {
	sc  benchScenario
	agg *BufferedAggregator
	ids []checkid.ID
}

func newBenchHarness(tb testing.TB, sc benchScenario, mode benchMode) *benchHarness {
	h := &benchHarness{sc: sc, agg: newModelAggregator()}
	rules := map[string]filterlistimpl.MetricTagList{}
	switch mode {
	case benchFiltersPod:
		rules[benchMetric] = filterlistimpl.MetricTagList{Tags: []string{"pod"}, Action: "exclude"}
	case benchRuleForAnotherMetric:
		rules["other.metric"] = filterlistimpl.MetricTagList{Tags: []string{"pod"}, Action: "exclude"}
	}
	h.agg.setFilterList(filterlistimpl.NewTagMatcher(rules, logmock.New(tb)))
	for i := 0; i < sc.instances; i++ {
		id := checkid.ID(fmt.Sprintf("openmetrics:%d", i))
		h.agg.handleRegisterSampler(id)
		h.ids = append(h.ids, id)
	}
	return h
}

func benchBase(run int) float64 { return 1_000_000 + 15*float64(run) }

func (h *benchHarness) submit(run int) {
	ts := benchBase(run) + 0.1
	generation := 0
	if h.sc.newPodsEachRun {
		generation = run
	}
	for i, id := range h.ids {
		for p := 0; p < h.sc.podsPerInstance; p++ {
			pod := fmt.Sprintf("pod:%d-%d-%d", i, p, generation)
			for c := 0; c < h.sc.contexts; c++ {
				tags := []string{"env:prod", pod, fmt.Sprintf("ctx:%d", c)}
				for k := 0; k < h.sc.bucketsPerSource; k++ {
					h.agg.handleSenderBucket(senderHistogramBucket{id: id, bucket: &metrics.HistogramBucket{
						Name: benchMetric, Value: int64(run+1) * 10, LowerBound: float64(k), UpperBound: float64(k + 1),
						Monotonic: true, Tags: tags, Timestamp: ts, MultipleBuckets: true}})
				}
				for k := 0; k < h.sc.samplesPerRun; k++ {
					value := float64(k)
					if h.sc.mtype == metrics.MonotonicCountType {
						value = float64(run * 10)
					}
					h.agg.handleSenderSample(senderMetricSample{id: id, metricSample: &metrics.MetricSample{
						Name: benchMetric, Value: value, Mtype: h.sc.mtype, Tags: tags, SampleRate: 1, Timestamp: ts}})
				}
			}
		}
	}
}

func (h *benchHarness) commit(run int) {
	h.agg.mu.Lock()
	defer h.agg.mu.Unlock()
	for _, id := range h.ids {
		h.agg.checkSamplers[id].commit(benchBase(run)+0.5, &h.agg.flushFilterList)
	}
}

func (h *benchHarness) flush(run int) {
	h.agg.flushSeriesAndSketches(flushTrigger{
		trigger:      trigger{time: time.Unix(int64(benchBase(run))+7, 0)},
		seriesSink:   benchSink{},
		sketchesSink: benchSketchSink{},
	})
}

func (h *benchHarness) run(run int) {
	h.submit(run)
	h.commit(run)
	h.flush(run)
}

var benchModes = []benchMode{benchNoRule, benchFiltersPod, benchRuleForAnotherMetric}

func BenchmarkCheckOutputRun(b *testing.B) {
	configmock.New(b).SetInTest("metric_tag_filterlist_adp_only", false)
	for _, sc := range benchScenarios {
		for _, mode := range benchModes {
			b.Run(sc.name+"/"+mode.String(), func(b *testing.B) {
				h := newBenchHarness(b, sc, mode)
				for r := 0; r < 3; r++ {
					h.run(r)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					h.run(3 + i)
				}
			})
		}
	}
}

// BenchmarkCheckOutputRuleChange measures applying new rules with many contexts, which refilters every context of
// every check sampler under the aggregator's lock.
func BenchmarkCheckOutputRuleChange(b *testing.B) {
	configmock.New(b).SetInTest("metric_tag_filterlist_adp_only", false)
	for _, instances := range []int{200, 2000} {
		sc := benchScenario{mtype: metrics.CounterType, instances: instances, podsPerInstance: 1, contexts: 50, samplesPerRun: 1}
		b.Run(fmt.Sprintf("contexts=%d", instances*sc.contexts), func(b *testing.B) {
			h := newBenchHarness(b, sc, benchFiltersPod)
			h.run(0)
			rules := []map[string]filterlistimpl.MetricTagList{
				{benchMetric: {Tags: []string{"pod"}, Action: "exclude"}},
				{benchMetric: {Tags: []string{"ctx"}, Action: "exclude"}},
			}
			matchers := []filterlist.TagMatcher{filterlistimpl.NewTagMatcher(rules[0], logmock.New(b)), filterlistimpl.NewTagMatcher(rules[1], logmock.New(b))}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.agg.setFilterList(matchers[i%2])
			}
		})
	}
}
