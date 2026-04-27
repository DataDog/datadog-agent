// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"testing"

	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/hook"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const benchBatchSize = 32

// BenchmarkTimeSamplerHook measures the per-sample and per-batch cost of the
// metrics pipeline hook tap point, comparing four hook modes:
//
//   - noop_hook: NoopHook (zero overhead baseline — no channel, no atomic)
//   - 0sub: real Hook with zero subscribers (atomic fast-path, no channel send)
//   - 1sub: real Hook with one subscriber (one channel send per batch)
//   - 5sub: real Hook with five subscribers (five channel sends per batch)
//
// Two sub-benchmarks per mode:
//   - sample_only: cost of sample() alone (hookBatch append + context resolution)
//   - batch32_publish: 32 × sample() + publishHookBatch() (amortized publish cost)
func BenchmarkTimeSamplerHook(b *testing.B) {
	store := tags.NewStore(false, "bench")
	sample := metrics.MetricSample{
		Name:       "bench.metric",
		Value:      1.0,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"env:prod", "service:foo"},
		SampleRate: 1.0,
		Timestamp:  12345.0,
	}
	matcher := filterlistimpl.NewNoopTagMatcher()

	cases := []struct {
		name string
		h    hook.Hook[[]hook.MetricSampleSnapshot]
	}{
		{"noop_hook", hook.NewNoopHook[[]hook.MetricSampleSnapshot]()},
		{"0sub", makeMetricHook(b, 0)},
		{"1sub", makeMetricHook(b, 1)},
		{"5sub", makeMetricHook(b, 5)},
	}

	for _, tc := range cases {
		b.Run(fmt.Sprintf("sample_only/%s", tc.name), func(b *testing.B) {
			s := NewTimeSampler(0, 10, store, nooptagger.NewComponent(), "host", tc.h)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				s.sample(&sample, 12345.0, matcher)
				s.hookBatch = s.hookBatch[:0] // drain without publishing to isolate sample() cost
			}
		})

		b.Run(fmt.Sprintf("batch%d_publish/%s", benchBatchSize, tc.name), func(b *testing.B) {
			s := NewTimeSampler(0, 10, store, nooptagger.NewComponent(), "host", tc.h)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				for range benchBatchSize {
					s.sample(&sample, 12345.0, matcher)
				}
				s.publishHookBatch()
			}
		})
	}
}

// BenchmarkNoAggWorkerHook measures the hook tap point in the no-aggregation stream
// worker. Unlike TimeSampler, this path allocates a new snapshot slice per batch
// when subscribers are present. The noop and 0-subscriber cases remain zero-cost.
//
// Pattern (from no_aggregation_stream_worker.go):
//
//	if w.metricHook.HasSubscribers() {
//	    hookBatch := make([]MetricSampleSnapshot, len(samples))
//	    for i := range samples { hookBatch[i] = NewMetricSampleSnapshot(&samples[i]) }
//	    w.metricHook.Publish("dogstatsd-no-aggr", hookBatch)
//	}
func BenchmarkNoAggWorkerHook(b *testing.B) {
	samples := make([]metrics.MetricSample, benchBatchSize)
	for i := range samples {
		samples[i] = metrics.MetricSample{
			Name: "bench.metric", Value: float64(i),
			Mtype: metrics.GaugeType, Tags: []string{"env:prod", "service:foo"},
			SampleRate: 1.0, Timestamp: 12345.0,
		}
	}

	for _, tc := range hookBenchCases(b) {
		b.Run(fmt.Sprintf("batch%d/%s", benchBatchSize, tc.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				// Exact pattern from no_aggregation_stream_worker.go
				if tc.h.HasSubscribers() {
					hookBatch := make([]hook.MetricSampleSnapshot, len(samples))
					for i := range samples {
						hookBatch[i] = hook.NewMetricSampleSnapshot(&samples[i])
					}
					tc.h.Publish("dogstatsd-no-aggr", hookBatch)
				}
			}
		})
	}
}

// BenchmarkCheckSamplerHook measures the hook tap point in CheckSampler.addSample().
// Each check metric triggers a single-sample publish when subscribers are present,
// allocating a one-element slice. The noop and 0-subscriber cases remain zero-cost.
//
// Pattern (from check_sampler.go):
//
//	if cs.metricHook.HasSubscribers() {
//	    cs.metricHook.Publish("checks", []MetricSampleSnapshot{NewMetricSampleSnapshot(s)})
//	}
func BenchmarkCheckSamplerHook(b *testing.B) {
	sample := &metrics.MetricSample{
		Name: "bench.check", Value: 1.0,
		Mtype: metrics.GaugeType, Tags: []string{"env:prod", "service:foo"},
		SampleRate: 1.0, Timestamp: 12345.0,
	}

	for _, tc := range hookBenchCases(b) {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				// Exact pattern from check_sampler.go
				if tc.h.HasSubscribers() {
					tc.h.Publish("checks", []hook.MetricSampleSnapshot{hook.NewMetricSampleSnapshot(sample)})
				}
			}
		})
	}
}

// hookBenchCases returns the four hook modes used across all pipeline benchmarks.
func hookBenchCases(b *testing.B) []struct {
	name string
	h    hook.Hook[[]hook.MetricSampleSnapshot]
} {
	b.Helper()
	return []struct {
		name string
		h    hook.Hook[[]hook.MetricSampleSnapshot]
	}{
		{"noop_hook", hook.NewNoopHook[[]hook.MetricSampleSnapshot]()},
		{"0sub", makeMetricHook(b, 0)},
		{"1sub", makeMetricHook(b, 1)},
		{"5sub", makeMetricHook(b, 5)},
	}
}

// makeMetricHook returns a real hook with n no-op subscribers.
// The subscriber channel is sized to b.N * benchBatchSize so sends never block
// during the benchmark (no drop overhead measured).
func makeMetricHook(b *testing.B, n int) hook.Hook[[]hook.MetricSampleSnapshot] {
	b.Helper()
	h := hook.NewHook[[]hook.MetricSampleSnapshot]("bench-metrics")
	for i := range n {
		h.Subscribe(
			fmt.Sprintf("bench-%d", i),
			func(_ []hook.MetricSampleSnapshot) {},
			hook.WithBufferSize[[]hook.MetricSampleSnapshot](b.N*benchBatchSize+1),
		)
	}
	return h
}
