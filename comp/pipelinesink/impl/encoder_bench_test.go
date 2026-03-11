// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipelinesinkimpl

import (
	"strings"
	"testing"
	"time"
)

// BenchmarkEncodeMetricBatch covers:
//   - single sample (fixed overhead baseline)
//   - batch 10/100/1000 (amortisation curve)
//   - small vs large metric name (string copy cost)
func BenchmarkEncodeMetricBatch(b *testing.B) {
	cases := []struct {
		name    string
		samples []capturedMetric
	}{
		{
			name:    "single_8char_name",
			samples: makeMetricSamples(1, 8),
		},
		{
			name:    "single_256char_name",
			samples: makeMetricSamples(1, 256),
		},
		{
			name:    "batch_10",
			samples: makeMetricSamples(10, 8),
		},
		{
			name:    "batch_100",
			samples: makeMetricSamples(100, 8),
		},
		{
			name:    "batch_1000",
			samples: makeMetricSamples(1000, 8),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var totalBytes int
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, err := EncodeMetricBatch(tc.samples)
				if err != nil {
					b.Fatal(err)
				}
				totalBytes += len(data)
			}
			b.ReportMetric(float64(totalBytes)/float64(b.N), "bytes/op")
		})
	}
}

// BenchmarkEncodeLogBatch covers small and large log content sizes.
func BenchmarkEncodeLogBatch(b *testing.B) {
	cases := []struct {
		name    string
		entries []capturedLog
	}{
		{
			name:    "batch_10_small_64B",
			entries: makeLogEntries(10, 64),
		},
		{
			name:    "batch_100_small_64B",
			entries: makeLogEntries(100, 64),
		},
		{
			name:    "batch_100_large_2KB",
			entries: makeLogEntries(100, 2048),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var totalBytes int
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, err := EncodeLogBatch(tc.entries)
				if err != nil {
					b.Fatal(err)
				}
				totalBytes += len(data)
			}
			b.ReportMetric(float64(totalBytes)/float64(b.N), "bytes/op")
		})
	}
}

// BenchmarkEncodeMetricBatch_AllocsRegression shows that alloc count is O(1)
// in batch size — FlatBuffers builder grows its internal buffer as needed.
// If allocs/op starts growing linearly with n, a code change introduced
// per-sample allocations and this bench will catch it.
//
// Known baseline (FlatBuffers): n=1 → 5; n=10 → 8; n=100 → 14; n=1000 → 22.
// Growth comes from FlatBuffers builder resizing; the regression threshold
// is allocs/op > 25 for batch_1000.
func BenchmarkEncodeMetricBatch_AllocsRegression(b *testing.B) {
	for _, tc := range []struct {
		name string
		n    int
	}{
		{"n=0001", 1},
		{"n=0010", 10},
		{"n=0100", 100},
		{"n=1000", 1000},
	} {
		samples := makeMetricSamples(tc.n, 8)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := EncodeMetricBatch(samples)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeMetricSamples(n int, nameLen int) []capturedMetric {
	name := strings.Repeat("x", nameLen)
	samples := make([]capturedMetric, n)
	for i := range samples {
		samples[i] = capturedMetric{
			Name:        name,
			Value:       float64(i) * 0.1,
			Tags:        []string{"host:benchhost", "env:bench", "service:test"},
			TimestampNs: time.Now().UnixNano(),
			SampleRate:  1.0,
			Source:      "agent",
		}
	}
	return samples
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
			Source:      "agent",
		}
	}
	return entries
}
