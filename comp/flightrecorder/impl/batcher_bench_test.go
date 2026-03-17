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

// BenchmarkBatcher_AddPoint_Hot measures the raw AddPoint latency when the
// ring buffer is pre-saturated (always-full path). This is the worst case from
// a lock-contention perspective: every call hits the drop counter increment.
func BenchmarkBatcher_AddPoint_Hot(b *testing.B) {
	c := &counters{}
	bat := newBatcher(noopTransport{}, time.Hour, 1000, 100, 1000, 0, c)
	defer bat.Stop()

	p := metricPoint{
		ContextKey:  12345,
		Value:       42.0,
		TimestampNs: time.Now().UnixNano(),
		SampleRate:  1.0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bat.AddPoint(p)
	}
}

// BenchmarkBatcher_AddPoint_WithFlush measures AddPoint while the flush
// goroutine runs at a realistic 100 ms interval.
func BenchmarkBatcher_AddPoint_WithFlush(b *testing.B) {
	c := &counters{}
	bat := newBatcher(noopTransport{}, 100*time.Millisecond, 10_000, 1000, 5000, 0, c)
	defer bat.Stop()

	p := metricPoint{
		ContextKey:  12345,
		Value:       42.0,
		TimestampNs: time.Now().UnixNano(),
		SampleRate:  1.0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bat.AddPoint(p)
	}
	b.StopTimer()

	if b.N > 0 {
		b.ReportMetric(float64(c.metricsDropped.Load())/float64(b.N)*100, "drop%")
	}
}

// BenchmarkBatcher_Flush_1000 measures the cost of one flush of 1000 queued
// metric points: FlatBuffers encoding + sending to noopTransport.
func BenchmarkBatcher_Flush_1000(b *testing.B) {
	p := metricPoint{
		ContextKey:  12345,
		Value:       42.0,
		TimestampNs: time.Now().UnixNano(),
		SampleRate:  1.0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c := &counters{}
		bat := newBatcher(noopTransport{}, time.Hour, 10_000, 1000, 5000, 0, c)
		for j := 0; j < 1000; j++ {
			bat.AddPoint(p)
		}
		b.StartTimer()

		bat.flush()

		b.StopTimer()
		bat.Stop()
		b.StartTimer()
	}
}

// BenchmarkSubscriberCallback measures the full subscriber callback path:
// pool.Get → tag copy → AddContextDef. This captures the allocation improvement
// from using sync.Pool for tag slices instead of make([]string, n) per call.
func BenchmarkSubscriberCallback(b *testing.B) {
	c := &counters{}
	bat := newBatcher(noopTransport{}, 100*time.Millisecond, 10_000, 1000, 5000, 0, c)
	defer bat.Stop()

	rawTags := []string{"env:prod", "host:web-01", "service:api", "version:1.2.3"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp := tagPool.Get().(*[]string)
		tags := append((*sp)[:0], rawTags...)
		bat.AddContextDef(contextDef{
			ContextKey:   uint64(i),
			Name:         "bench.callback",
			Value:        42.0,
			Tags:         tags,
			TagPoolSlice: sp,
			TimestampNs:  time.Now().UnixNano(),
			SampleRate:   1.0,
		})
	}
}

// BenchmarkBatcher_AddLog_Hot measures raw AddLog latency on the drop path.
func BenchmarkBatcher_AddLog_Hot(b *testing.B) {
	c := &counters{}
	bat := newBatcher(noopTransport{}, time.Hour, 1000, 100, 1000, 0, c)
	defer bat.Stop()

	l := capturedLog{
		Content:     []byte("benchmark log entry with realistic content length for a typical log line"),
		Status:      "info",
		Tags:        []string{"env:bench", "host:benchhost"},
		Hostname:    "benchhost",
		TimestampNs: time.Now().UnixNano(),
		Source:      "agent",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bat.AddLog(l)
	}
}
