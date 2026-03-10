// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

// buildMetricMessage constructs a single DogStatsD metric wire-format message.
//
//	typ: "g", "c", "h", "d", "s", "ms"
//	value: the value string (e.g. "1")
//	tagCount: number of "tagN:valN" tags to append
func buildMetricMessage(name, value, typ string, tagCount int) []byte {
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte(':')
	sb.WriteString(value)
	sb.WriteByte('|')
	sb.WriteString(typ)
	if tagCount > 0 {
		sb.WriteString("|#")
		for i := 0; i < tagCount; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, "tag%d:val%d", i, i)
		}
	}
	return []byte(sb.String())
}

// buildMultiMetricPacket builds a newline-delimited packet containing n copies of msg.
func buildMultiMetricPacket(msg []byte, n int) []byte {
	if n == 1 {
		return msg
	}
	total := len(msg)*n + (n - 1)
	buf := make([]byte, 0, total)
	for i := 0; i < n; i++ {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, msg...)
	}
	return buf
}

// reportPercentiles sorts durations and reports p50/p95/p99 via b.ReportMetric.
// This surfaces outlier latency that ns/op (the mean) can hide.
func reportPercentiles(b *testing.B, durations []int64) {
	b.Helper()
	n := len(durations)
	if n == 0 {
		return
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[n/2]
	p95idx := int(float64(n) * 0.95)
	if p95idx >= n {
		p95idx = n - 1
	}
	p99idx := int(float64(n) * 0.99)
	if p99idx >= n {
		p99idx = n - 1
	}
	b.ReportMetric(float64(p50), "p50-ns")
	b.ReportMetric(float64(durations[p95idx]), "p95-ns")
	b.ReportMetric(float64(durations[p99idx]), "p99-ns")
	if p50 > 0 {
		b.ReportMetric(float64(durations[p99idx])/float64(p50), "p99/p50-ratio")
	}
}

// ---------------------------------------------------------------------------
// Metric type benchmarks
// Tests how metric type affects parse cost (byte switch vs string compare).
// ---------------------------------------------------------------------------

func BenchmarkParseMetricTypes(b *testing.B) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"gauge", []byte("bench.metric:1|g|#env:prod")},
		{"counter", []byte("bench.metric:1|c|#env:prod")},
		{"histogram", []byte("bench.metric:1|h|#env:prod")},
		{"distribution", []byte("bench.metric:1|d|#env:prod")},
		{"set", []byte("bench.metric:value|s|#env:prod")},
		{"timing", []byte("bench.metric:1|ms|#env:prod")},
	}

	cfg := mock.New(b)
	deps := fulfillDeps(b)
	s := deps.Server.(*server)
	pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)
	demux := deps.Demultiplexer
	defer demux.Stop(false)

	histogram := deps.Telemetry.NewHistogram("dogstatsd", "channel_latency",
		[]string{"shard", "message_type"}, "ns", defaultChannelBuckets)

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
			parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
			batcher := newBatcher(demux, histogram)
			samplesBuf := make([]metrics.MetricSample, 0, 16)

			durations := make([]int64, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				samplesBuf, _ = s.parseMetricMessage(samplesBuf, parser, tc.input, "", 0, "", false, nil)
				durations[i] = time.Since(start).Nanoseconds()
				samplesBuf = samplesBuf[:0]
			}
			_ = batcher
			b.StopTimer()
			b.ReportAllocs()
			reportPercentiles(b, durations)
		})
	}
}

// ---------------------------------------------------------------------------
// Tag count benchmarks
// Measures parse cost as tag volume grows from 0 to 50 tags.
// High tag counts stress the tag splitting and string interning hot paths.
// ---------------------------------------------------------------------------

func BenchmarkParseMetricTagCount(b *testing.B) {
	for _, tagCount := range []int{0, 2, 5, 10, 20, 50} {
		tagCount := tagCount
		b.Run(fmt.Sprintf("%d-tags", tagCount), func(b *testing.B) {
			cfg := mock.New(b)
			deps := fulfillDeps(b)
			s := deps.Server.(*server)
			pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)
			demux := deps.Demultiplexer
			defer demux.Stop(false)

			msg := buildMetricMessage("bench.metric", "1", "g", tagCount)
			stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
			parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
			samplesBuf := make([]metrics.MetricSample, 0, 16)

			durations := make([]int64, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				samplesBuf, _ = s.parseMetricMessage(samplesBuf, parser, msg, "", 0, "", false, nil)
				durations[i] = time.Since(start).Nanoseconds()
				samplesBuf = samplesBuf[:0]
			}
			b.StopTimer()
			b.ReportAllocs()
			reportPercentiles(b, durations)
		})
	}
}

// ---------------------------------------------------------------------------
// Packet throughput benchmarks
// Measures how many metrics/sec can be extracted from a single packet.
// Tests both single-metric (low throughput) and dense-packed (high throughput).
// ---------------------------------------------------------------------------

func BenchmarkParsePacketMetricsPerPacket(b *testing.B) {
	for _, metricsPerPacket := range []int{1, 8, 32, 64, 128} {
		metricsPerPacket := metricsPerPacket
		b.Run(fmt.Sprintf("%d-metrics-per-packet", metricsPerPacket), func(b *testing.B) {
			cfg := mock.New(b)
			deps := fulfillDeps(b)
			s := deps.Server.(*server)
			pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)
			demux := deps.Demultiplexer
			defer demux.Stop(false)

			// 5 tags per metric — representative of real workloads
			singleMsg := buildMetricMessage("bench.metric", "1", "g", 5)
			rawPacket := buildMultiMetricPacket(singleMsg, metricsPerPacket)

			histogram := deps.Telemetry.NewHistogram("dogstatsd", "channel_latency",
				[]string{"shard", "message_type"}, "ns", defaultChannelBuckets)
			stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
			parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
			batcher := newBatcher(demux, histogram)
			samplesBuf := make([]metrics.MetricSample, 0, metricsPerPacket*2)

			pkt := &packets.Packet{Contents: rawPacket, Origin: packets.NoOrigin}
			pkts := packets.Packets{pkt}

			durations := make([]int64, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pkt.Contents = rawPacket
				start := time.Now()
				samplesBuf = s.parsePackets(batcher, parser, pkts, samplesBuf, nil)
				durations[i] = time.Since(start).Nanoseconds()
				samplesBuf = samplesBuf[:0]
			}
			b.StopTimer()
			b.ReportAllocs()
			b.ReportMetric(float64(metricsPerPacket), "metrics-per-packet")
			reportPercentiles(b, durations)
		})
	}
}

// BenchmarkParsePacketParallel tests throughput with multiple concurrent workers —
// the real production topology. Reveals lock contention if any.
func BenchmarkParsePacketParallel(b *testing.B) {
	cfg := mock.New(b)
	deps := fulfillDeps(b)
	s := deps.Server.(*server)
	pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)
	demux := deps.Demultiplexer
	defer demux.Stop(false)

	histogram := deps.Telemetry.NewHistogram("dogstatsd", "channel_latency",
		[]string{"shard", "message_type"}, "ns", defaultChannelBuckets)

	// 32 metrics per packet, 5 tags each — high-throughput representative workload
	rawPacket := buildPacketContent(32, 1)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
		parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
		batcher := newBatcher(demux, histogram)
		pkt := &packets.Packet{Origin: packets.NoOrigin}
		pkts := packets.Packets{pkt}
		samplesBuf := make([]metrics.MetricSample, 0, 64)

		for pb.Next() {
			pkt.Contents = rawPacket
			samplesBuf = s.parsePackets(batcher, parser, pkts, samplesBuf, nil)
			samplesBuf = samplesBuf[:0]
		}
	})
}

// ---------------------------------------------------------------------------
// String interner benchmarks
// The interner is called on every tag and metric name — it is one of the most
// frequently executed paths in the entire parsing pipeline.
// ---------------------------------------------------------------------------

// BenchmarkStringInternerAllHits — warm interner, all keys already interned.
// Represents steady-state traffic with a stable, repeating set of metric names.
func BenchmarkStringInternerAllHits(b *testing.B) {
	deps := fulfillDeps(b)
	siTelemetry := newSiTelemetry(false, deps.Telemetry)
	interner := newStringInterner(1024, 0, siTelemetry)

	keys := [][]byte{
		[]byte("service.requests.total"),
		[]byte("service.latency.p99"),
		[]byte("env:production"),
		[]byte("region:us-east-1"),
		[]byte("version:1.2.3"),
	}
	// Warm the interner so every key is already present
	for _, k := range keys {
		interner.LoadOrStore(k)
	}

	durations := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := keys[i%len(keys)]
		start := time.Now()
		_ = interner.LoadOrStore(key)
		durations[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	b.ReportAllocs()
	reportPercentiles(b, durations)
}

// BenchmarkStringInternerAllMisses — cold interner, every key is unique.
// Represents cardinality explosion scenarios or first startup.
func BenchmarkStringInternerAllMisses(b *testing.B) {
	deps := fulfillDeps(b)
	siTelemetry := newSiTelemetry(false, deps.Telemetry)
	// Large max size so generation rotation doesn't interfere
	interner := newStringInterner(100000, 0, siTelemetry)

	// Pre-generate unique keys so key generation cost is not included in timed loop
	keys := make([][]byte, b.N)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("unique.metric.name.%08d", i))
	}

	durations := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_ = interner.LoadOrStore(keys[i])
		durations[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	b.ReportAllocs()
	reportPercentiles(b, durations)
}

// BenchmarkStringInternerGenerationRotation — small max size forces frequent rotation.
// Measures the cost of the generation-swap event and promotion path.
func BenchmarkStringInternerGenerationRotation(b *testing.B) {
	deps := fulfillDeps(b)
	siTelemetry := newSiTelemetry(false, deps.Telemetry)
	// maxSize=8 causes a generation rotation every 8 unique keys
	interner := newStringInterner(8, 0, siTelemetry)

	// 16 unique keys ensures we cycle through both current and old generations
	keys := make([][]byte, 16)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("metric.%d", i))
	}

	durations := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_ = interner.LoadOrStore(keys[i%len(keys)])
		durations[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	b.ReportAllocs()
	reportPercentiles(b, durations)
}

// BenchmarkStringInternerMixedHitRate benchmarks realistic hit-rate scenarios.
func BenchmarkStringInternerMixedHitRate(b *testing.B) {
	cases := []struct {
		name       string
		uniqueKeys int // pool of unique keys; lower = higher hit rate
		maxSize    int
	}{
		{"high-hit-rate-10-unique", 10, 1024},
		{"medium-hit-rate-100-unique", 100, 1024},
		{"low-hit-rate-1000-unique", 1000, 1024},
	}

	deps := fulfillDeps(b)
	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			siTelemetry := newSiTelemetry(false, deps.Telemetry)
			interner := newStringInterner(tc.maxSize, 0, siTelemetry)

			keys := make([][]byte, tc.uniqueKeys)
			for i := range keys {
				keys[i] = []byte(fmt.Sprintf("metric.name.%d", i))
			}
			// Warm the interner
			for _, k := range keys {
				interner.LoadOrStore(k)
			}

			durations := make([]int64, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				_ = interner.LoadOrStore(keys[i%tc.uniqueKeys])
				durations[i] = time.Since(start).Nanoseconds()
			}
			b.StopTimer()
			b.ReportAllocs()
			reportPercentiles(b, durations)
		})
	}
}
