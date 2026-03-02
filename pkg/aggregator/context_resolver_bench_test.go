// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"strconv"
	"testing"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func benchmarkContextResolver(numContexts int, b *testing.B) {
	var samples []metrics.MetricSample
	matcher := filterlistimpl.NewNoopTagMatcher()

	for i := 0; i < numContexts; i++ {
		samples = append(samples, metrics.MetricSample{
			Name:       "my.metric.name",
			Value:      1,
			Mtype:      metrics.GaugeType,
			Tags:       []string{"foo", "bar", strconv.Itoa(i)},
			SampleRate: 1,
		})
	}
	cache := tags.NewStore(true, "test")
	cr := newContextResolver(nooptagger.NewComponent(), cache, "0")

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		cr.trackContext(&samples[n%numContexts], 0, matcher)
	}
	b.ReportAllocs()
}

// Benchmark context tracking with different number of contexts.

func BenchmarkContextResolver1(b *testing.B) {
	benchmarkContextResolver(1, b)
}

func BenchmarkContextResolver1000(b *testing.B) {
	benchmarkContextResolver(1000, b)
}

func BenchmarkContextResolver1000000(b *testing.B) {
	benchmarkContextResolver(1000000, b)
}

// Benchmark trackContext with tag filtering enabled
func benchmarkContextResolverWithFiltering(numContexts, numTags, numFilterTags int, b *testing.B) {
	var samples []metrics.MetricSample

	// Create a tag matcher with the specified number of filter tags
	metricTagList := map[string]filterlistimpl.MetricTagList{}
	if numFilterTags > 0 {
		tags := make([]string, numFilterTags)
		for i := 0; i < numFilterTags; i++ {
			tags[i] = "filter_tag_filter_tag_filter_tag_" + strconv.Itoa(i)
		}
		metricTagList["my.distribution.metric"] = filterlistimpl.MetricTagList{
			Tags:   tags,
			Action: "exclude",
		}
	}
	matcher := filterlistimpl.NewTagMatcher(metricTagList)

	// Create samples with distributions (only distributions trigger tag stripping)
	for i := 0; i < numContexts; i++ {
		tags := make([]string, numTags)
		for j := 0; j < numTags; j += 2 {
			tags[j] = "filter_tag_filter_tag_filter_tag_" + strconv.Itoa(j) + ":value_" + strconv.Itoa(i)
			tags[j+1] = "foltor_tog_foltor_tog_foltor_tog_" + strconv.Itoa(j) + ":value_" + strconv.Itoa(i)
		}
		samples = append(samples, metrics.MetricSample{
			Name:       "my.distribution.metric",
			Value:      1,
			Mtype:      metrics.DistributionType, // Only distributions trigger tag stripping
			Tags:       tags,
			SampleRate: 1,
		})
	}
	cache := tags.NewStore(true, "test")
	cr := newContextResolver(nooptagger.NewComponent(), cache, "bench")

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		cr.trackContext(&samples[n%numContexts], 0, matcher)
	}
	b.ReportAllocs()
}

// Baseline: No filter rules (empty matcher)
func BenchmarkTrackContext_NoFilters_10Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 10, 0, b)
}

func BenchmarkTrackContext_NoFilters_50Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 50, 0, b)
}

// Small filter list (5 tags to filter)
func BenchmarkTrackContext_SmallFilter_10Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 10, 5, b)
}

// Large filter list (50 tags to filter)
func BenchmarkTrackContext_LargeFilter_10Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 10, 50, b)
}

// Very large filter list (100 tags to filter)
func BenchmarkTrackContext_VeryLargeFilter_10Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 10, 100, b)
}

// Test with more tags in the samples
func BenchmarkTrackContext_SmallFilter_50Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 50, 5, b)
}

func BenchmarkTrackContext_LargeFilter_50Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 50, 50, b)
}

func BenchmarkTrackContext_VeryLargeFilter_50Tags(b *testing.B) {
	benchmarkContextResolverWithFiltering(1000, 50, 100, b)
}
