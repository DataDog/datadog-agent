// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"strconv"
	"testing"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func benchmarkContextResolver(numContexts int, b *testing.B) {
	var samples []metrics.MetricSample

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
	cr := newContextResolver(nooptagger.NewTaggerClient(), cache, "0")

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		cr.trackContext(&samples[n%numContexts], 0)
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
