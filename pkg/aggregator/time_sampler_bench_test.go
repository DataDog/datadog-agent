// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"sort"
	"testing"
	"time"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// reportTimeSamplerPercentiles sorts durations and emits p50/p95/p99 metrics.
// Used across multiple benchmarks to expose tail-latency characteristics that
// the default ns/op mean conceals.
func reportTimeSamplerPercentiles(b *testing.B, durations []int64) {
	b.Helper()
	n := len(durations)
	if n == 0 {
		return
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[n/2]
	p95idx := int(float64(n)*0.95) - 1
	if p95idx < 0 {
		p95idx = 0
	}
	p99idx := int(float64(n)*0.99) - 1
	if p99idx < 0 {
		p99idx = 0
	}
	b.ReportMetric(float64(p50), "p50-ns")
	b.ReportMetric(float64(durations[p95idx]), "p95-ns")
	b.ReportMetric(float64(durations[p99idx]), "p99-ns")
	if p50 > 0 {
		b.ReportMetric(float64(durations[p99idx])/float64(p50), "p99/p50-ratio")
	}
}

// ---------------------------------------------------------------------------
// Sample() hot-path benchmarks
// Every DogStatsD metric passes through TimeSampler.sample(). These benchmarks
// isolate the cost of the tag-hashing → context-key → bucket-lookup path.
// ---------------------------------------------------------------------------

// BenchmarkTimeSamplerSampleTagCount measures how tag volume affects sample() cost.
// Higher tag counts increase the KeyGenerator hashing work per sample.
func BenchmarkTimeSamplerSampleTagCount(b *testing.B) {
	for _, tagCount := range []int{0, 2, 5, 10, 20, 50} {
		tagCount := tagCount
		b.Run(fmt.Sprintf("%d-tags", tagCount), func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
				matcher := filterlist.NewNoopTagMatcher()

				tagSlice := make([]string, tagCount)
				for i := range tagSlice {
					tagSlice[i] = fmt.Sprintf("tag%d:val%d", i, i)
				}
				sample := metrics.MetricSample{
					Name:       "my.metric.name",
					Value:      1,
					Mtype:      metrics.GaugeType,
					Tags:       tagSlice,
					SampleRate: 1,
				}

				durations := make([]int64, b.N)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					start := time.Now()
					sampler.sample(&sample, 12345.0, matcher)
					durations[i] = time.Since(start).Nanoseconds()
				}
				b.StopTimer()
				b.ReportAllocs()
				reportTimeSamplerPercentiles(b, durations)
			})
		})
	}
}

// BenchmarkTimeSamplerSampleMetricTypes isolates per-type aggregation cost.
// Distribution (sketch insert) is significantly more expensive than scalar types.
func BenchmarkTimeSamplerSampleMetricTypes(b *testing.B) {
	cases := []struct {
		name  string
		mtype metrics.MetricType
	}{
		{"gauge", metrics.GaugeType},
		{"counter", metrics.CounterType},
		{"histogram", metrics.HistogramType},
		{"distribution", metrics.DistributionType},
		{"set", metrics.SetType},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
				matcher := filterlist.NewNoopTagMatcher()

				sample := metrics.MetricSample{
					Name:       "bench.metric",
					Value:      1,
					RawValue:   "val",
					Mtype:      tc.mtype,
					Tags:       []string{"env:prod", "service:api"},
					SampleRate: 1,
				}

				durations := make([]int64, b.N)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					start := time.Now()
					sampler.sample(&sample, 12345.0, matcher)
					durations[i] = time.Since(start).Nanoseconds()
				}
				b.StopTimer()
				b.ReportAllocs()
				reportTimeSamplerPercentiles(b, durations)
			})
		})
	}
}

// BenchmarkTimeSamplerSampleHighCardinality tests sample() under high context count.
// A large contextsByKey map increases hash-table lookup cost. This benchmark
// pre-populates N distinct contexts then measures steady-state sample() cost.
func BenchmarkTimeSamplerSampleHighCardinality(b *testing.B) {
	for _, numContexts := range []int{100, 1_000, 10_000, 100_000} {
		numContexts := numContexts
		b.Run(fmt.Sprintf("%d-contexts", numContexts), func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
				matcher := filterlist.NewNoopTagMatcher()

				// Pre-populate sampler with numContexts distinct metrics
				for i := 0; i < numContexts; i++ {
					pre := metrics.MetricSample{
						Name:       fmt.Sprintf("metric.%d", i),
						Value:      float64(i),
						Mtype:      metrics.GaugeType,
						Tags:       []string{fmt.Sprintf("shard:%d", i%100)},
						SampleRate: 1,
					}
					sampler.sample(&pre, 10000.0, matcher)
				}

				// Benchmark steady-state sampling on a pre-existing context
				sample := metrics.MetricSample{
					Name:       "metric.0",
					Value:      1,
					Mtype:      metrics.GaugeType,
					Tags:       []string{"shard:0"},
					SampleRate: 1,
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sampler.sample(&sample, 10001.0, matcher)
				}
				b.ReportAllocs()
			})
		})
	}
}

// ---------------------------------------------------------------------------
// Flush benchmarks
// flush() is called every 15 seconds. Its cost grows with cardinality.
// These benchmarks expose the per-context serialization path.
// ---------------------------------------------------------------------------

// BenchmarkTimeSamplerFlushCardinality measures flush time vs. context count.
// This is the dominant cost at flush time for high-cardinality deployments.
func BenchmarkTimeSamplerFlushCardinality(b *testing.B) {
	for _, numContexts := range []int{10, 100, 1_000, 10_000} {
		numContexts := numContexts
		b.Run(fmt.Sprintf("%d-contexts", numContexts), func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				matcher := filterlist.NewNoopTagMatcher()

				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					b.StopTimer()
					// Fresh sampler each iteration so flush always has data
					sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
					for i := 0; i < numContexts; i++ {
						s := metrics.MetricSample{
							Name:       fmt.Sprintf("metric.%d", i),
							Value:      float64(i),
							Mtype:      metrics.GaugeType,
							Tags:       []string{fmt.Sprintf("tag:%d", i)},
							SampleRate: 1,
						}
						sampler.sample(&s, 10000.0, matcher)
					}
					var series metrics.Series
					var sketches metrics.SketchSeriesList
					b.StartTimer()

					sampler.flush(10020.0, &series, &sketches, nil, true)
				}
				b.ReportAllocs()
				b.ReportMetric(float64(numContexts), "contexts")
			})
		})
	}
}

// BenchmarkTimeSamplerFlushMetricTypes measures flush cost per metric type.
// Distributions (sketches) have a higher serialization cost than scalars.
func BenchmarkTimeSamplerFlushMetricTypes(b *testing.B) {
	cases := []struct {
		name  string
		mtype metrics.MetricType
	}{
		{"gauge", metrics.GaugeType},
		{"counter", metrics.CounterType},
		{"histogram", metrics.HistogramType},
		{"distribution", metrics.DistributionType},
	}

	const numMetrics = 1000

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				matcher := filterlist.NewNoopTagMatcher()

				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					b.StopTimer()
					sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
					for i := 0; i < numMetrics; i++ {
						s := metrics.MetricSample{
							Name:       fmt.Sprintf("bench.metric.%d", i),
							Value:      float64(i),
							Mtype:      tc.mtype,
							Tags:       []string{"env:prod", fmt.Sprintf("shard:%d", i%10)},
							SampleRate: 1,
						}
						sampler.sample(&s, 10000.0, matcher)
					}
					var series metrics.Series
					var sketches metrics.SketchSeriesList
					b.StartTimer()

					sampler.flush(10020.0, &series, &sketches, nil, true)
				}
				b.ReportAllocs()
				b.ReportMetric(float64(numMetrics), "metrics")
			})
		})
	}
}

// BenchmarkTimeSamplerFlushWithTagCount measures the impact of tag cardinality on flush.
// More tags per context → more work when building the Series/Sketch output.
func BenchmarkTimeSamplerFlushWithTagCount(b *testing.B) {
	const numMetrics = 500

	for _, tagCount := range []int{2, 10, 30} {
		tagCount := tagCount
		b.Run(fmt.Sprintf("%d-tags", tagCount), func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				matcher := filterlist.NewNoopTagMatcher()

				tagSlice := make([]string, tagCount)
				for i := range tagSlice {
					tagSlice[i] = fmt.Sprintf("tag%d:val%d", i, i)
				}

				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					b.StopTimer()
					sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
					for i := 0; i < numMetrics; i++ {
						s := metrics.MetricSample{
							Name:       fmt.Sprintf("bench.metric.%d", i),
							Value:      float64(i),
							Mtype:      metrics.GaugeType,
							Tags:       tagSlice,
							SampleRate: 1,
						}
						sampler.sample(&s, 10000.0, matcher)
					}
					var series metrics.Series
					var sketches metrics.SketchSeriesList
					b.StartTimer()

					sampler.flush(10020.0, &series, &sketches, nil, true)
				}
				b.ReportAllocs()
			})
		})
	}
}

// BenchmarkTimeSamplerFlushMultiBucket tests flush when metrics span many time buckets.
// More buckets → more map iteration at flush time.
func BenchmarkTimeSamplerFlushMultiBucket(b *testing.B) {
	for _, bucketCount := range []int{1, 5, 15} {
		bucketCount := bucketCount
		b.Run(fmt.Sprintf("%d-buckets", bucketCount), func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				matcher := filterlist.NewNoopTagMatcher()
				const metricsPerBucket = 100

				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					b.StopTimer()
					sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
					for bucket := 0; bucket < bucketCount; bucket++ {
						ts := float64(10000 + bucket*10) // each bucket is 10s apart
						for i := 0; i < metricsPerBucket; i++ {
							s := metrics.MetricSample{
								Name:       fmt.Sprintf("bench.metric.%d", i),
								Value:      float64(i),
								Mtype:      metrics.GaugeType,
								Tags:       []string{"env:prod"},
								SampleRate: 1,
							}
							sampler.sample(&s, ts, matcher)
						}
					}
					var series metrics.Series
					var sketches metrics.SketchSeriesList
					// Flush timestamp beyond all buckets
					flushTs := float64(10000 + bucketCount*10 + 20)
					b.StartTimer()

					sampler.flush(flushTs, &series, &sketches, nil, true)
				}
				b.ReportAllocs()
			})
		})
	}
}

// BenchmarkTimeSamplerSampleThenFlushCycle exercises the full sample→flush cycle
// at realistic ratios (15s of samples at 1000 metrics/s = 15,000 samples before flush).
func BenchmarkTimeSamplerSampleThenFlushCycle(b *testing.B) {
	for _, samplesPerFlush := range []int{1_000, 15_000, 100_000} {
		samplesPerFlush := samplesPerFlush
		b.Run(fmt.Sprintf("%d-samples-per-flush", samplesPerFlush), func(b *testing.B) {
			benchWithTagsStore(b, func(b *testing.B, store *tags.Store) {
				matcher := filterlist.NewNoopTagMatcher()
				const numContexts = 500

				samples := make([]metrics.MetricSample, samplesPerFlush)
				for i := range samples {
					samples[i] = metrics.MetricSample{
						Name:       fmt.Sprintf("bench.metric.%d", i%numContexts),
						Value:      float64(i),
						Mtype:      metrics.GaugeType,
						Tags:       []string{"env:prod", fmt.Sprintf("shard:%d", i%10)},
						SampleRate: 1,
					}
				}

				durations := make([]int64, b.N)
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					b.StopTimer()
					sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
					for i, s := range samples {
						sampler.sample(&s, float64(10000+i/100), matcher)
					}
					var series metrics.Series
					var sketches metrics.SketchSeriesList
					b.StartTimer()

					start := time.Now()
					sampler.flush(20000.0, &series, &sketches, nil, true)
					durations[n] = time.Since(start).Nanoseconds()
				}
				b.StopTimer()
				b.ReportAllocs()
				b.ReportMetric(float64(samplesPerFlush), "samples-per-cycle")
				reportTimeSamplerPercentiles(b, durations)
			})
		})
	}
}
