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
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func benchmarkContextResolver(numContexts int, b *testing.B) {
	benchmarkContextResolverTags(numContexts, false, b)
}

// benchmarkContextResolverTags tracks contexts for samples carrying either plain
// string tags or interned handles, so the two representations can be compared
// through the exact same aggregator path.
func benchmarkContextResolverTags(numContexts int, interned bool, b *testing.B) {
	var samples []metrics.MetricSample
	matcher := filterlistimpl.NewNoopTagMatcher()

	for i := 0; i < numContexts; i++ {
		sample := metrics.MetricSample{
			Name:       "my.metric.name",
			Value:      1,
			Mtype:      metrics.GaugeType,
			SampleRate: 1,
		}
		tags := []string{"foo", "bar", strconv.Itoa(i)}
		if interned {
			sample.ITags = tagset.InternAll(tags)
		} else {
			sample.Tags = tags
		}
		samples = append(samples, sample)
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

// Same benchmarks, with samples that carry interned tags, as the dogstatsd
// pipeline produces them.

func BenchmarkContextResolverInterned1(b *testing.B) {
	benchmarkContextResolverTags(1, true, b)
}

func BenchmarkContextResolverInterned1000(b *testing.B) {
	benchmarkContextResolverTags(1000, true, b)
}

func BenchmarkContextResolverInterned1000000(b *testing.B) {
	benchmarkContextResolverTags(1000000, true, b)
}
