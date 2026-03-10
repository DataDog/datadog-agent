// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"sort"
	"testing"
	"time"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// reportContextResolverPercentiles emits p50/p95/p99 percentile metrics to
// expose tail-latency behaviour in the context-tracking hot path.
func reportContextResolverPercentiles(b *testing.B, durations []int64) {
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
// Tag-count benchmarks
// trackContext() hashes all tags via HashingTagsAccumulator on every call.
// Tag count is the primary driver of hashing cost.
// ---------------------------------------------------------------------------

func BenchmarkContextResolverTagCount(b *testing.B) {
	for _, tagCount := range []int{0, 2, 5, 10, 20, 50} {
		tagCount := tagCount
		b.Run(fmt.Sprintf("%d-tags", tagCount), func(b *testing.B) {
			for _, useStore := range []bool{true, false} {
				useStore := useStore
				b.Run(fmt.Sprintf("useStore=%v", useStore), func(b *testing.B) {
					cache := tags.NewStore(useStore, "test")
					cr := newContextResolver(nooptagger.NewComponent(), cache, "0")
					matcher := filterlistimpl.NewNoopTagMatcher()

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
						cr.trackContext(&sample, 0, matcher)
						durations[i] = time.Since(start).Nanoseconds()
					}
					b.StopTimer()
					b.ReportAllocs()
					reportContextResolverPercentiles(b, durations)
				})
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Context hit vs. miss benchmarks
// A "hit" is a context already in contextsByKey — the dominant case in steady state.
// A "miss" allocates a new Context and inserts into the map.
// ---------------------------------------------------------------------------

// BenchmarkContextResolverHotPath — always the same context (100% hits after warmup).
// Represents stable, low-cardinality traffic.
func BenchmarkContextResolverHotPath(b *testing.B) {
	cache := tags.NewStore(true, "test")
	cr := newContextResolver(nooptagger.NewComponent(), cache, "0")
	matcher := filterlistimpl.NewNoopTagMatcher()

	sample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"env:prod", "service:api"},
		SampleRate: 1,
	}
	// Warm the resolver so the context already exists
	cr.trackContext(&sample, 0, matcher)

	durations := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		cr.trackContext(&sample, 0, matcher)
		durations[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	b.ReportAllocs()
	reportContextResolverPercentiles(b, durations)
}

// BenchmarkContextResolverColdPath — always a new unique context (100% misses).
// Represents a cardinality-explosion scenario or initial startup.
func BenchmarkContextResolverColdPath(b *testing.B) {
	cache := tags.NewStore(true, "test")
	cr := newContextResolver(nooptagger.NewComponent(), cache, "0")
	matcher := filterlistimpl.NewNoopTagMatcher()

	// Pre-generate samples so Name allocation is not included in timing
	samples := make([]metrics.MetricSample, b.N)
	for i := range samples {
		samples[i] = metrics.MetricSample{
			Name:       fmt.Sprintf("unique.metric.%d", i),
			Value:      float64(i),
			Mtype:      metrics.GaugeType,
			Tags:       []string{"env:prod"},
			SampleRate: 1,
		}
	}

	durations := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		cr.trackContext(&samples[i], 0, matcher)
		durations[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	b.ReportAllocs()
	reportContextResolverPercentiles(b, durations)
}

// ---------------------------------------------------------------------------
// Cardinality benchmarks
// High cardinality grows the contextsByKey map, increasing hash-table lookup
// time and GC pressure. These tests reveal the scalability ceiling.
// ---------------------------------------------------------------------------

// BenchmarkContextResolverCardinality pre-populates the resolver and measures
// steady-state trackContext() cost at various cardinality levels.
func BenchmarkContextResolverCardinality(b *testing.B) {
	for _, numContexts := range []int{10, 100, 1_000, 10_000, 100_000} {
		numContexts := numContexts
		b.Run(fmt.Sprintf("%d-contexts", numContexts), func(b *testing.B) {
			cache := tags.NewStore(true, "test")
			cr := newContextResolver(nooptagger.NewComponent(), cache, "0")
			matcher := filterlistimpl.NewNoopTagMatcher()

			// Pre-populate with numContexts distinct contexts
			warmSamples := make([]metrics.MetricSample, numContexts)
			for i := 0; i < numContexts; i++ {
				warmSamples[i] = metrics.MetricSample{
					Name:       fmt.Sprintf("metric.%d", i),
					Value:      1,
					Mtype:      metrics.GaugeType,
					Tags:       []string{fmt.Sprintf("shard:%d", i%100)},
					SampleRate: 1,
				}
				cr.trackContext(&warmSamples[i], 0, matcher)
			}

			// Benchmark lookup of a known context in the populated map
			sample := &warmSamples[0]
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cr.trackContext(sample, 0, matcher)
			}
			b.ReportAllocs()
		})
	}
}

// BenchmarkContextResolverMixedCardinality simulates a realistic traffic mix:
// 80% of samples hit existing contexts (hot path), 20% create new ones.
func BenchmarkContextResolverMixedCardinality(b *testing.B) {
	cases := []struct {
		name     string
		poolSize int // number of unique contexts in the hot pool
	}{
		{"low-cardinality-10", 10},
		{"medium-cardinality-500", 500},
		{"high-cardinality-5000", 5000},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			cache := tags.NewStore(true, "test")
			cr := newContextResolver(nooptagger.NewComponent(), cache, "0")
			matcher := filterlistimpl.NewNoopTagMatcher()

			// Build a pool of pre-existing contexts (80% traffic share)
			hotPool := make([]metrics.MetricSample, tc.poolSize)
			for i := 0; i < tc.poolSize; i++ {
				hotPool[i] = metrics.MetricSample{
					Name:       fmt.Sprintf("hot.metric.%d", i),
					Value:      1,
					Mtype:      metrics.GaugeType,
					Tags:       []string{"env:prod", fmt.Sprintf("svc:%d", i%50)},
					SampleRate: 1,
				}
				cr.trackContext(&hotPool[i], 0, matcher)
			}

			// Build a pool of novel contexts (20% traffic share — cold misses)
			coldPool := make([]metrics.MetricSample, b.N/5+1)
			for i := range coldPool {
				coldPool[i] = metrics.MetricSample{
					Name:       fmt.Sprintf("cold.metric.%d.%d", tc.poolSize, i),
					Value:      1,
					Mtype:      metrics.GaugeType,
					Tags:       []string{"env:staging"},
					SampleRate: 1,
				}
			}

			coldIdx := 0
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// 80% hot, 20% cold
				if i%5 == 4 {
					cr.trackContext(&coldPool[coldIdx%len(coldPool)], 0, matcher)
					coldIdx++
				} else {
					cr.trackContext(&hotPool[i%tc.poolSize], 0, matcher)
				}
			}
			b.ReportAllocs()
		})
	}
}

// ---------------------------------------------------------------------------
// Throughput benchmarks
// Measures how many trackContext() calls/sec the resolver supports.
// ---------------------------------------------------------------------------

// BenchmarkContextResolverLowThroughput simulates low-rate DogStatsD input
// (100 unique metrics, all pre-existing contexts).
func BenchmarkContextResolverLowThroughput(b *testing.B) {
	const numContexts = 100
	cache := tags.NewStore(true, "test")
	cr := newContextResolver(nooptagger.NewComponent(), cache, "0")
	matcher := filterlistimpl.NewNoopTagMatcher()

	samples := make([]metrics.MetricSample, numContexts)
	for i := range samples {
		samples[i] = metrics.MetricSample{
			Name:       fmt.Sprintf("metric.%d", i),
			Value:      1,
			Mtype:      metrics.GaugeType,
			Tags:       []string{"env:prod"},
			SampleRate: 1,
		}
		cr.trackContext(&samples[i], 0, matcher)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.trackContext(&samples[i%numContexts], 0, matcher)
	}
	b.ReportAllocs()
}

// BenchmarkContextResolverHighThroughput simulates high-rate DogStatsD input
// cycling through a large context pool — stresses map access patterns.
func BenchmarkContextResolverHighThroughput(b *testing.B) {
	const numContexts = 50_000
	cache := tags.NewStore(true, "test")
	cr := newContextResolver(nooptagger.NewComponent(), cache, "0")
	matcher := filterlistimpl.NewNoopTagMatcher()

	samples := make([]metrics.MetricSample, numContexts)
	for i := range samples {
		samples[i] = metrics.MetricSample{
			Name:       fmt.Sprintf("metric.%d", i),
			Value:      1,
			Mtype:      metrics.GaugeType,
			Tags:       []string{fmt.Sprintf("shard:%d", i%1000)},
			SampleRate: 1,
		}
		cr.trackContext(&samples[i], 0, matcher)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.trackContext(&samples[i%numContexts], 0, matcher)
	}
	b.ReportAllocs()
}
