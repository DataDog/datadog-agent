// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"strconv"
	"testing"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// benchmarkFlushSketchesBase populates numContexts distribution contexts using the
// provided sample generator, then measures the flushSketches path.
func benchmarkFlushSketchesBase(numContexts int, tagFilter filterlist.TagMatcher, makeSample func(i int) metrics.MetricSample, b *testing.B) {
	store := tags.NewStore(true, "bench")
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")

	// Pre-populate contexts so they are tracked by the contextResolver.
	for i := 0; i < numContexts; i++ {
		s := makeSample(i)
		sampler.sample(&s, 1000.0)
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// Re-populate the sketchMap (contexts already tracked in contextResolver).
		for i := 0; i < numContexts; i++ {
			s := makeSample(i)
			sampler.sample(&s, 1000.0)
		}
		var sketches metrics.SketchSeriesList
		sampler.flushSketches(10000, &sketches, true, tagFilter)
	}
	b.ReportAllocs()
}

// BenchmarkFlushSketches_NoFilter benchmarks flushSketches with no tag filtering (noop matcher).
// Baseline performance - same as old flushSketches.
func BenchmarkFlushSketches_NoFilter_100(b *testing.B) {
	benchmarkFlushSketchesBase(100, filterlistimpl.NewNoopTagMatcher(), func(i int) metrics.MetricSample {
		return metrics.MetricSample{
			Name: "my.distribution." + strconv.Itoa(i), Value: 1.0,
			Mtype: metrics.DistributionType, Tags: []string{"foo", "bar"}, SampleRate: 1, Timestamp: 1000.0,
		}
	}, b)
}

func BenchmarkFlushSketches_NoFilter_1000(b *testing.B) {
	benchmarkFlushSketchesBase(1000, filterlistimpl.NewNoopTagMatcher(), func(i int) metrics.MetricSample {
		return metrics.MetricSample{
			Name: "my.distribution." + strconv.Itoa(i), Value: 1.0,
			Mtype: metrics.DistributionType, Tags: []string{"foo", "bar"}, SampleRate: 1, Timestamp: 1000.0,
		}
	}, b)
}

// BenchmarkFlushSketches_Filter_NoCollision benchmarks flushSketches with tag filtering
// where no contexts merge (each stripped context is still unique).
// Tags: ["keep:i", "strip:yes"] -> after stripping "strip", each context has unique "keep:i".
func BenchmarkFlushSketches_Filter_NoCollision_100(b *testing.B) {
	tagFilter := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
		"my.distribution": {Tags: []string{"strip"}, Action: "exclude"},
	})
	benchmarkFlushSketchesBase(100, tagFilter, func(i int) metrics.MetricSample {
		return metrics.MetricSample{
			Name: "my.distribution", Value: 1.0,
			Mtype:      metrics.DistributionType,
			Tags:       []string{"keep:" + strconv.Itoa(i), "strip:yes"},
			SampleRate: 1, Timestamp: 1000.0,
		}
	}, b)
}

// BenchmarkFlushSketches_Filter_HighCollision benchmarks flushSketches with tag filtering
// where all contexts merge into a single stripped context.
// Tags: ["keep:yes", "strip:i"] -> after stripping "strip", all share the same "keep:yes".
func BenchmarkFlushSketches_Filter_HighCollision_100(b *testing.B) {
	tagFilter := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
		"my.distribution": {Tags: []string{"strip"}, Action: "exclude"},
	})
	benchmarkFlushSketchesBase(100, tagFilter, func(i int) metrics.MetricSample {
		return metrics.MetricSample{
			Name: "my.distribution", Value: 1.0,
			Mtype:      metrics.DistributionType,
			Tags:       []string{"keep:yes", "strip:" + strconv.Itoa(i)},
			SampleRate: 1, Timestamp: 1000.0,
		}
	}, b)
}
