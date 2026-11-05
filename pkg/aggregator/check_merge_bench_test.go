// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// The check output benchmarks compare the aggregator's check path without filtering, with a rule that strips the
// pod tag (sources collapse into a few series), and with a rule that matches no metric.

const benchMetric = "bench.metric"

type benchMode int

const (
	benchOff     benchMode = iota // no rule: the baseline
	benchOn                       // the rule strips the pod tag of benchMetric
	benchNoMatch                  // a rule for another metric
)

func (m benchMode) String() string { return [...]string{"off", "on", "no-match"}[m] }

// benchScenario is check instances submitting one metric type. Each source context has a pod tag, which the rule
// strips, and a ctx tag, which it keeps: sources collapse into `contexts` series.
type benchScenario struct {
	name            string
	mtype           metrics.MetricType
	buckets         int // monotonic histogram buckets per source, instead of samples of mtype
	instances       int
	podsPerInstance int
	contexts        int
	samples         int  // per source context per run
	churn           bool // every run, each instance reports new pods
}

var benchScenarios = []benchScenario{
	{name: "dist-per-pod-instances", mtype: metrics.DistributionType, instances: 200, podsPerInstance: 1, contexts: 50, samples: 5},
	{name: "dist-one-instance", mtype: metrics.DistributionType, instances: 1, podsPerInstance: 200, contexts: 50, samples: 5},
	{name: "dist-churn", mtype: metrics.DistributionType, instances: 200, podsPerInstance: 1, contexts: 50, samples: 5, churn: true},
	{name: "count", mtype: metrics.CountType, instances: 200, podsPerInstance: 1, contexts: 50, samples: 5},
	{name: "monotonic-count", mtype: metrics.MonotonicCountType, instances: 200, podsPerInstance: 1, contexts: 50, samples: 1},
	{name: "histogram-buckets", buckets: 10, instances: 200, podsPerInstance: 1, contexts: 50},
}

// benchSink counts what the aggregator sends, without keeping it.
type benchSink struct{ sent *int }

func (s benchSink) Append(*metrics.Serie) { *s.sent++ }

type benchSketchSink struct{ sent *int }

func (s benchSketchSink) Append(*metrics.SketchSeries) { *s.sent++ }

type benchHarness struct {
	sc   benchScenario
	agg  *BufferedAggregator
	ids  []checkid.ID
	sent int // series and sketches
}

func newBenchHarness(tb testing.TB, sc benchScenario, mode benchMode) *benchHarness {
	h := &benchHarness{sc: sc, agg: newModelAggregator()}
	rules := map[string]filterlistimpl.MetricTagList{}
	switch mode {
	case benchOn:
		rules[benchMetric] = filterlistimpl.MetricTagList{Tags: []string{"pod"}, Action: "exclude"}
	case benchNoMatch:
		rules["other.metric"] = filterlistimpl.MetricTagList{Tags: []string{"pod"}, Action: "exclude"}
	}
	h.agg.setFilterListAt(filterlistimpl.NewTagMatcher(rules, logmock.New(tb)), 0)
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
	if h.sc.churn {
		generation = run
	}
	for i, id := range h.ids {
		for p := 0; p < h.sc.podsPerInstance; p++ {
			pod := fmt.Sprintf("pod:%d-%d-%d", i, p, generation)
			for c := 0; c < h.sc.contexts; c++ {
				tags := []string{"env:prod", pod, fmt.Sprintf("ctx:%d", c)}
				for k := 0; k < h.sc.buckets; k++ {
					h.agg.handleSenderBucket(senderHistogramBucket{id: id, bucket: &metrics.HistogramBucket{
						Name: benchMetric, Value: int64(run+1) * 10, LowerBound: float64(k), UpperBound: float64(k + 1),
						Monotonic: true, Tags: tags, Timestamp: ts, MultipleBuckets: true}})
				}
				for k := 0; k < h.sc.samples; k++ {
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
		seriesSink:   benchSink{&h.sent},
		sketchesSink: benchSketchSink{&h.sent},
	})
}

func (h *benchHarness) run(run int) {
	h.submit(run)
	h.commit(run)
	h.flush(run)
}

func (h *benchHarness) contexts() int {
	n := 0
	for _, cs := range h.agg.checkSamplers {
		n += cs.contextResolver.length()
	}
	return n
}

// liveHeap returns the bytes of live heap objects, after a full collection.
func liveHeap() int64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int64(ms.HeapAlloc)
}

var benchModes = []benchMode{benchOff, benchOn, benchNoMatch}

// TestCheckOutputMemory reports live heap at each phase of a run, as the most seen over several runs, relative to
// the heap before the aggregator is created. Set CHECK_OUTPUT_MEMORY=1 to run it.
func TestCheckOutputMemory(t *testing.T) {
	if os.Getenv("CHECK_OUTPUT_MEMORY") == "" {
		t.Skip("set CHECK_OUTPUT_MEMORY=1 to measure")
	}
	configmock.New(t).SetInTest("metric_tag_filterlist_adp_only", false)

	const warmup, runs = 3, 5
	t.Logf("%-24s %-9s %12s %12s %12s %9s %12s", "scenario", "mode", "submitted", "committed", "flushed", "contexts", "series/run")
	for _, sc := range benchScenarios {
		for _, mode := range benchModes {
			before := liveHeap()
			h := newBenchHarness(t, sc, mode)
			for r := 0; r < warmup; r++ {
				h.run(r)
			}
			var submitted, committed, flushed int64
			sentBefore := h.sent
			for r := warmup; r < warmup+runs; r++ {
				h.submit(r)
				submitted = max(submitted, liveHeap()-before)
				h.commit(r)
				committed = max(committed, liveHeap()-before)
				h.flush(r)
				flushed = max(flushed, liveHeap()-before)
			}
			t.Logf("%-24s %-9s %12d %12d %12d %9d %12d", sc.name, mode, submitted, committed, flushed,
				h.contexts(), (h.sent-sentBefore)/runs)
			runtime.KeepAlive(h)
		}
	}
}

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

// BenchmarkCheckOutputRuleChange measures applying new rules with many contexts, which restrips every context of
// every check sampler under the aggregator's lock.
func BenchmarkCheckOutputRuleChange(b *testing.B) {
	configmock.New(b).SetInTest("metric_tag_filterlist_adp_only", false)
	for _, instances := range []int{200, 2000} {
		sc := benchScenario{mtype: metrics.CountType, instances: instances, podsPerInstance: 1, contexts: 50, samples: 1}
		b.Run(fmt.Sprintf("contexts=%d", instances*sc.contexts), func(b *testing.B) {
			h := newBenchHarness(b, sc, benchOn)
			h.run(0)
			rules := []map[string]filterlistimpl.MetricTagList{
				{benchMetric: {Tags: []string{"pod"}, Action: "exclude"}},
				{benchMetric: {Tags: []string{"ctx"}, Action: "exclude"}},
			}
			matchers := []filterlist.TagMatcher{filterlistimpl.NewTagMatcher(rules[0], logmock.New(b)), filterlistimpl.NewTagMatcher(rules[1], logmock.New(b))}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.agg.setFilterListAt(matchers[i%2], int64(i))
			}
		})
	}
}
