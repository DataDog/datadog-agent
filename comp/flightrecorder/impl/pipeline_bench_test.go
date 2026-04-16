// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"testing"
	"time"
)

// noopTransport implements Transport and discards all data.
type noopTransport struct{}

func (noopTransport) Send(_ []byte) error { return nil }
func (noopTransport) Close() error        { return nil }

func BenchmarkPipeline_AddPoint_Hot(b *testing.B) {
	c := &counters{}
	pipe := newTestMetricsPipeline(noopTransport{}, time.Hour, 1000, 100, c)
	defer pipe.Stop()

	p := metricPoint{
		ContextKey: 12345, Value: 42.0,
		TimestampNs: time.Now().UnixNano(), SampleRate: 1.0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipe.AddEntry(p)
	}
}

func BenchmarkPipeline_AddPoint_WithFlush(b *testing.B) {
	c := &counters{}
	pipe := newTestMetricsPipeline(noopTransport{}, 100*time.Millisecond, 10_000, 1000, c)
	defer pipe.Stop()

	p := metricPoint{
		ContextKey: 12345, Value: 42.0,
		TimestampNs: time.Now().UnixNano(), SampleRate: 1.0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipe.AddEntry(p)
	}
	b.StopTimer()

	if b.N > 0 {
		b.ReportMetric(float64(c.metricsDropped.Load())/float64(b.N)*100, "drop%")
	}
}

func BenchmarkPipeline_Flush_1000(b *testing.B) {
	p := metricPoint{
		ContextKey: 12345, Value: 42.0,
		TimestampNs: time.Now().UnixNano(), SampleRate: 1.0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c := &counters{}
		pipe := newTestMetricsPipeline(noopTransport{}, time.Hour, 10_000, 1000, c)
		for j := 0; j < 1000; j++ {
			pipe.AddEntry(p)
		}
		b.StartTimer()

		pipe.flushEntries()

		b.StopTimer()
		pipe.Stop()
		b.StartTimer()
	}
}

func BenchmarkPipeline_SubscriberCallback(b *testing.B) {
	c := &counters{}
	pipe := newTestMetricsPipeline(noopTransport{}, 100*time.Millisecond, 10_000, 1000, c)
	defer pipe.Stop()

	rawTags := []string{"env:prod", "host:web-01", "service:api", "version:1.2.3"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp := tagPool.Get().(*[]string)
		tags := append((*sp)[:0], rawTags...)
		pipe.AddContextDef(contextDef{
			ContextKey:   uint64(i),
			Name:         "bench.callback",
			Tags:         tags,
			TagPoolSlice: sp,
			Source:       "dogstatsd",
		})
	}
}

func BenchmarkPipeline_AddLogEntry_Hot(b *testing.B) {
	c := &counters{}
	pipe := newTestLogsPipeline(noopTransport{}, time.Hour, 1000, 100, c)
	defer pipe.Stop()

	entry := logEntry{
		ContextKey:  42,
		Content:     []byte("benchmark log entry with realistic content length for a typical log line"),
		TimestampNs: time.Now().UnixNano(),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipe.AddEntry(entry)
	}
}
