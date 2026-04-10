// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"strings"
	"testing"
	"time"
)

// BenchmarkEncodeContextBatch benchmarks context definition encoding through
// the production code path (EncodeContextBatch + builder pool).
func BenchmarkEncodeContextBatch(b *testing.B) {
	pool := newBuilderPool()
	for _, n := range []int{10, 100, 1000} {
		defs := makeContextDefs(n)
		b.Run(strings.Replace("n="+strings.Repeat("0", 4-len(string(rune('0'+n/1000))))+string(rune('0'+n)), "\x00", "", -1), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				builder, err := EncodeContextBatch(pool, defs, 0, len(defs), len(defs))
				if err != nil {
					b.Fatal(err)
				}
				pool.put(builder)
			}
		})
	}
}

// BenchmarkEncodePointBatch benchmarks metric point encoding through
// the production code path (EncodePointBatch + builder pool).
func BenchmarkEncodePointBatch(b *testing.B) {
	pool := newBuilderPool()
	for _, n := range []int{100, 1000, 2000} {
		pts := makeMetricPoints(n)
		b.Run(strings.Replace("n="+string(rune('0'+n/1000))+"k", "\x00", "", -1), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				builder, err := EncodePointBatch(pool, pts, 0, len(pts), len(pts))
				if err != nil {
					b.Fatal(err)
				}
				pool.put(builder)
			}
		})
	}
}

// BenchmarkEncodeLogBatchRing benchmarks log encoding through the production
// code path (EncodeLogBatchRing + builder pool).
func BenchmarkEncodeLogBatchRing(b *testing.B) {
	pool := newBuilderPool()
	for _, tc := range []struct {
		name       string
		n          int
		contentLen int
	}{
		{"10x64B", 10, 64},
		{"100x64B", 100, 64},
		{"100x2KB", 100, 2048},
	} {
		entries := makeLogEntries(tc.n, tc.contentLen)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				builder, err := EncodeLogBatchRing(pool, entries, 0, len(entries), len(entries))
				if err != nil {
					b.Fatal(err)
				}
				pool.put(builder)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeContextDefs(n int) []contextDef {
	defs := make([]contextDef, n)
	for i := range defs {
		defs[i] = contextDef{
			ContextKey: uint64(i + 1),
			Name:       "system.cpu.user",
			Tags:       []string{"host:benchhost", "env:bench", "service:test"},
			Source:     "dogstatsd",
		}
	}
	return defs
}

func makeMetricPoints(n int) []metricPoint {
	pts := make([]metricPoint, n)
	for i := range pts {
		pts[i] = metricPoint{
			ContextKey:  uint64(i%100 + 1),
			Value:       float64(i) * 0.1,
			TimestampNs: time.Now().UnixNano(),
			SampleRate:  1.0,
		}
	}
	return pts
}

func makeLogEntries(n int, contentLen int) []capturedLog {
	content := []byte(strings.Repeat("a", contentLen))
	entries := make([]capturedLog, n)
	for i := range entries {
		entries[i] = capturedLog{
			Content:     content,
			Status:      "info",
			Tags:        []string{"host:benchhost", "env:bench"},
			Hostname:    "benchhost",
			TimestampNs: time.Now().UnixNano(),
		}
	}
	return entries
}
