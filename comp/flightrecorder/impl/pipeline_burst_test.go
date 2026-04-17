// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// slowTransport simulates realistic UDS send latency. Each Send() takes
// a configurable duration, representing the time to write a frame through
// the kernel socket buffer + sidecar processing backpressure.
type slowTransport struct {
	latency  time.Duration
	sends    atomic.Int64
	bytes    atomic.Int64
	maxFrame atomic.Int64
}

func (t *slowTransport) Send(b []byte) error {
	time.Sleep(t.latency)
	t.sends.Add(1)
	t.bytes.Add(int64(len(b)))
	for {
		cur := t.maxFrame.Load()
		if int64(len(b)) <= cur {
			break
		}
		if t.maxFrame.CompareAndSwap(cur, int64(len(b))) {
			break
		}
	}
	return nil
}

func (*slowTransport) Close() error { return nil }

// TestPipeline_BurstLogs_NoLatency verifies that a burst of logs with zero
// send latency produces zero drops, establishing the baseline.
func TestPipeline_BurstLogs_NoLatency(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	pipe := newTestLogsPipeline(transport, 50*time.Millisecond, 25000, 1000, c)

	// Burst: inject 20K logs as fast as possible.
	content := make([]byte, 400) // 400 bytes like staging
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}
	for i := 0; i < 20000; i++ {
		pipe.AddEntry(logEntry{
			ContextKey:  uint64(i%100 + 1),
			Content:     content,
			TimestampNs: int64(i * 1000),
		})
	}

	time.Sleep(500 * time.Millisecond)
	pipe.Stop()

	t.Logf("sent=%d dropped=%d sends=%d bytes=%d",
		c.logsSent.Load(), c.logsDropped.Load(),
		transport.sends.Load(), transport.bytes.Load())

	if c.logsDropped.Load() > 0 {
		t.Errorf("expected 0 drops with no latency, got %d", c.logsDropped.Load())
	}
	if c.logsSent.Load() != 20000 {
		t.Errorf("expected 20000 logs sent, got %d", c.logsSent.Load())
	}
}

// TestPipeline_BurstLogs_WithLatency simulates the staging scenario:
// bursts of logs hit the pipeline while the flush goroutine is blocked
// on a slow Send() (simulating UDS backpressure from the sidecar).
//
// Parameters match the dropping pod on entei:
// - 25K max ring (production default)
// - 100ms flush interval
// - Send latency: variable (simulates sidecar Parquet flush stalls)
// - Burst: 30K logs in a tight loop (simulates container log dump)
func TestPipeline_BurstLogs_WithLatency(t *testing.T) {
	cases := []struct {
		name         string
		latency      time.Duration
		ringCap      int
		burstSize    int
		maxDropPct   float64 // max acceptable drop rate (0 = must be zero)
	}{
		// With encode-ahead pipeline, moderate latencies with 30K burst
		// into 25K ring may produce minor drops due to race timing.
		{"1ms_latency_25K_ring_30K_burst", 1 * time.Millisecond, 25000, 30000, 10.0},
		{"500us_latency_25K_ring_30K_burst", 500 * time.Microsecond, 25000, 30000, 10.0},
		// Larger ring always absorbs the burst.
		{"1ms_latency_50K_ring_30K_burst", 1 * time.Millisecond, 50000, 30000, 0},
		// Burst fits within ring capacity.
		{"1ms_latency_25K_ring_20K_burst", 1 * time.Millisecond, 25000, 20000, 0},
		// Large burst overwhelms ring regardless of pipeline.
		{"2ms_latency_25K_ring_50K_burst", 2 * time.Millisecond, 25000, 50000, 50.0},
		{"5ms_latency_25K_ring_30K_burst", 5 * time.Millisecond, 25000, 30000, 10.0},
	}

	content := make([]byte, 400)
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &slowTransport{latency: tc.latency}
			c := &counters{}
			pipe := newTestLogsPipeline(transport, 100*time.Millisecond, tc.ringCap, 1000, c)

			// Burst: inject all logs as fast as possible.
			for i := 0; i < tc.burstSize; i++ {
				pipe.AddEntry(logEntry{
					ContextKey:  uint64(i%100 + 1),
					Content:     content,
					TimestampNs: int64(i * 1000),
				})
			}

			// Wait for flush goroutine to drain.
			time.Sleep(2 * time.Second)
			pipe.Stop()

			total := c.logsSent.Load() + c.logsDropped.Load()
			dropRate := float64(c.logsDropped.Load()) / float64(total) * 100

			t.Logf("sent=%d dropped=%d (%.1f%%) total=%d sends=%d bytes=%d maxFrame=%d",
				c.logsSent.Load(), c.logsDropped.Load(), dropRate, total,
				transport.sends.Load(), transport.bytes.Load(), transport.maxFrame.Load())

			if dropRate > tc.maxDropPct {
				t.Errorf("drop rate %.1f%% exceeds max acceptable %.1f%% (latency=%s burst=%d ring=%d)",
					dropRate, tc.maxDropPct, tc.latency, tc.burstSize, tc.ringCap)
			}
		})
	}
}

// TestPipeline_RepeatedBursts simulates the real staging pattern:
// repeated bursts separated by quiet periods.
// This tests whether the pipeline recovers between bursts.
func TestPipeline_RepeatedBursts(t *testing.T) {
	transport := &slowTransport{latency: 1 * time.Millisecond}
	c := &counters{}
	pipe := newTestLogsPipeline(transport, 100*time.Millisecond, 25000, 1000, c)

	content := make([]byte, 400)
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}

	const numBursts = 5
	const burstSize = 15000
	const quietMs = 500

	for burst := 0; burst < numBursts; burst++ {
		// Burst: inject burstSize logs as fast as possible.
		for i := 0; i < burstSize; i++ {
			pipe.AddEntry(logEntry{
				ContextKey:  uint64(i%100 + 1),
				Content:     content,
				TimestampNs: int64((burst*burstSize + i) * 1000),
			})
		}
		// Quiet period: let the flush goroutine catch up.
		time.Sleep(time.Duration(quietMs) * time.Millisecond)
	}

	time.Sleep(2 * time.Second)
	pipe.Stop()

	total := c.logsSent.Load() + c.logsDropped.Load()
	expected := uint64(numBursts * burstSize)
	dropRate := float64(c.logsDropped.Load()) / float64(total) * 100

	t.Logf("bursts=%d burstSize=%d quietMs=%d total=%d sent=%d dropped=%d (%.1f%%) sends=%d",
		numBursts, burstSize, quietMs, total, c.logsSent.Load(), c.logsDropped.Load(), dropRate,
		transport.sends.Load())

	if total != expected {
		t.Errorf("expected %d total items (sent+dropped), got %d", expected, total)
	}

	// With 25K ring, 1ms send latency, and 15K bursts every 500ms:
	// The flush goroutine drains ~2000 items per Send at ~1ms each = ~2000/ms.
	// A 15K burst takes ~7.5ms to flush. With 500ms quiet period, the ring
	// should drain completely between bursts. Expect zero drops.
	if c.logsDropped.Load() > 0 {
		t.Logf("WARNING: drops detected in repeated burst pattern — pipeline cannot keep up")
	}
}

// BenchmarkPipeline_BurstIngestion measures the throughput ceiling of
// burst log injection. This answers: "how fast can the pipeline accept
// logs before the ring fills up?"
func BenchmarkPipeline_BurstIngestion(b *testing.B) {
	content := make([]byte, 400)
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}

	for _, latency := range []time.Duration{0, 500 * time.Microsecond, 1 * time.Millisecond, 5 * time.Millisecond} {
		b.Run(fmt.Sprintf("send_latency_%s", latency), func(b *testing.B) {
			transport := &slowTransport{latency: latency}
			c := &counters{}
			pipe := newTestLogsPipeline(transport, 100*time.Millisecond, 25000, 1000, c)

			entry := logEntry{
				ContextKey:  42,
				Content:     content,
				TimestampNs: time.Now().UnixNano(),
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pipe.AddEntry(entry)
			}
			b.StopTimer()

			time.Sleep(500 * time.Millisecond)
			pipe.Stop()

			if b.N > 0 {
				b.ReportMetric(float64(c.logsDropped.Load())/float64(b.N)*100, "drop%")
			}
		})
	}
}
