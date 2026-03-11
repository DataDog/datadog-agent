// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipelinesinkimpl

import (
	"testing"
	"time"
)

// noopTransport implements Transport and discards all data.
type noopTransport struct{}

func (noopTransport) Send(_ []byte) error { return nil }
func (noopTransport) Close() error        { return nil }

// BenchmarkBatcher_AddMetric_Hot measures the raw AddMetric latency when the
// ring buffer is pre-saturated (always-full path). This is the worst case from
// a lock-contention perspective: every call hits the drop counter increment.
// Production rates are far lower, so this is an upper bound on overhead.
//
// The drop% will always be ~100% here — that is intentional and expected.
func BenchmarkBatcher_AddMetric_Hot(b *testing.B) {
	c := &counters{}
	// Small capacity so the ring is saturated immediately; no I/O wait.
	bat := newBatcher(noopTransport{}, time.Hour, 1000, c)
	defer bat.Stop()

	m := capturedMetric{
		Name:        "bench.gauge",
		Value:       42.0,
		Tags:        []string{"env:bench", "host:benchhost"},
		TimestampNs: time.Now().UnixNano(),
		SampleRate:  1.0,
		Source:      "agent",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bat.AddMetric(m)
	}
}

// BenchmarkBatcher_AddMetric_WithFlush measures AddMetric while the flush
// goroutine runs at a realistic 100 ms interval. Compared with AddMetric_Hot
// this shows the extra cost of mutex contention when encoding + noopTransport
// sends compete with enqueue operations.
func BenchmarkBatcher_AddMetric_WithFlush(b *testing.B) {
	c := &counters{}
	bat := newBatcher(noopTransport{}, 100*time.Millisecond, 10_000, c)
	defer bat.Stop()

	m := capturedMetric{
		Name:        "bench.gauge",
		Value:       42.0,
		Tags:        []string{"env:bench", "host:benchhost"},
		TimestampNs: time.Now().UnixNano(),
		SampleRate:  1.0,
		Source:      "agent",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bat.AddMetric(m)
	}
	b.StopTimer()

	if b.N > 0 {
		b.ReportMetric(float64(c.metricsDropped.Load())/float64(b.N)*100, "drop%")
	}
}

// BenchmarkBatcher_Flush_1000 measures the cost of one flush of 1000 queued
// metrics: encoding to Cap'n Proto + sending to noopTransport. This is the
// "per-flush-interval" work done by the background goroutine.
func BenchmarkBatcher_Flush_1000(b *testing.B) {
	m := capturedMetric{
		Name:        "bench.gauge",
		Value:       42.0,
		Tags:        []string{"env:bench", "host:benchhost"},
		TimestampNs: time.Now().UnixNano(),
		SampleRate:  1.0,
		Source:      "agent",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Rebuild the batcher to reset state cleanly between iterations.
		c := &counters{}
		bat := newBatcher(noopTransport{}, time.Hour, 10_000, c)
		for j := 0; j < 1000; j++ {
			bat.AddMetric(m)
		}
		b.StartTimer()

		bat.flush()

		b.StopTimer()
		bat.Stop()
		b.StartTimer()
	}
}

// BenchmarkSubscriberCallback measures the full subscriber callback path:
// pool.Get → tag copy → AddMetric. This captures the allocation improvement
// from using sync.Pool for tag slices instead of make([]string, n) per call.
func BenchmarkSubscriberCallback(b *testing.B) {
	c := &counters{}
	bat := newBatcher(noopTransport{}, 100*time.Millisecond, 10_000, c)
	defer bat.Stop()

	// Simulate a realistic tag set from the hook payload.
	rawTags := []string{"env:prod", "host:web-01", "service:api", "version:1.2.3"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp := tagPool.Get().(*[]string)
		tags := append((*sp)[:0], rawTags...)
		bat.AddMetric(capturedMetric{
			Name:         "bench.callback",
			Value:        42.0,
			Tags:         tags,
			TagPoolSlice: sp,
			TimestampNs:  time.Now().UnixNano(),
			SampleRate:   1.0,
		})
	}
}

// BenchmarkBatcher_AddLog_Hot measures raw AddLog latency on the drop path,
// analogous to BenchmarkBatcher_AddMetric_Hot.
func BenchmarkBatcher_AddLog_Hot(b *testing.B) {
	c := &counters{}
	bat := newBatcher(noopTransport{}, time.Hour, 1000, c)
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
