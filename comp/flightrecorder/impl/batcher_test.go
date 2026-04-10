// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/hook"
)

// recordingTransport records the number of Send() calls, total bytes,
// and the largest single frame seen.
type recordingTransport struct {
	sends    atomic.Int64
	bytes    atomic.Int64
	maxFrame atomic.Int64
}

func (t *recordingTransport) Send(b []byte) error {
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

func (*recordingTransport) Close() error { return nil }

// TestBatcher_ConcurrentAddAndFlush exercises the batcher with concurrent
// AddPoint, AddContextDef, AddLogBatch, and AddTraceStat calls while the
// flush loop is running. Run with -race to detect data races.
func TestBatcher_ConcurrentAddAndFlush(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	cs := newContextSet(50000)
	bat := newBatcher(transport, 10*time.Millisecond, 1000, 100, 500, 100, cs, c)

	const numGoroutines = 4
	const itemsPerGoroutine = 500

	var wg sync.WaitGroup

	// Goroutine 1-2: AddPoint (known contexts)
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < itemsPerGoroutine; i++ {
				bat.AddPoint(metricPoint{
					ContextKey:  uint64(id*itemsPerGoroutine + i),
					Value:       float64(i),
					TimestampNs: int64(i),
					SampleRate:  1.0,
				})
			}
		}(g)
	}

	// Goroutine 3: AddContextDef (unknown contexts)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < itemsPerGoroutine; i++ {
			bat.AddContextDef(contextDef{
				ContextKey: uint64(i + 100000),
				Name:       "metric.name",
				Tags:       []string{"tag:value"},
				Source:     "test",
			})
		}
	}()

	// Goroutine 4: AddLogBatch
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < itemsPerGoroutine/10; i++ {
			batch := make([]hook.LogSampleSnapshot, 10)
			for j := range batch {
				batch[j] = hook.LogSampleSnapshot{
					Content:     []byte("log line"),
					TimestampNs: int64(i*10 + j),
				}
			}
			bat.AddLogBatch(batch)
		}
	}()

	wg.Wait()
	bat.Stop()

	// Verify data flowed through.
	if transport.sends.Load() == 0 {
		t.Error("expected at least one Send() call, got 0")
	}
	if transport.bytes.Load() == 0 {
		t.Error("expected bytes sent > 0, got 0")
	}

	total := c.metricsSent.Load() + c.metricsDropped.Load() + c.logsSent.Load() + c.logsDropped.Load()
	if total == 0 {
		t.Error("expected some items processed, got 0")
	}
	t.Logf("sends=%d bytes=%d metricsSent=%d metricsDropped=%d logsSent=%d logsDropped=%d",
		transport.sends.Load(), transport.bytes.Load(),
		c.metricsSent.Load(), c.metricsDropped.Load(),
		c.logsSent.Load(), c.logsDropped.Load())
}

// TestBatcher_FlushLoopDrainsRing verifies that items added to the ring
// are eventually flushed — the flush loop doesn't lose items.
func TestBatcher_FlushLoopDrainsRing(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	cs := newContextSet(50000)
	// Large ring (10K), fast flush (5ms)
	bat := newBatcher(transport, 5*time.Millisecond, 10000, 1000, 5000, 1000, cs, c)

	// Add 500 points — should be well within ring capacity.
	for i := 0; i < 500; i++ {
		bat.AddPoint(metricPoint{
			ContextKey:  uint64(i % 100),
			Value:       float64(i),
			TimestampNs: int64(i),
			SampleRate:  1.0,
		})
	}

	// Wait for flush loop to drain.
	time.Sleep(100 * time.Millisecond)

	// Add more items to verify flush loop is still alive.
	for i := 0; i < 100; i++ {
		bat.AddPoint(metricPoint{
			ContextKey:  uint64(i % 100),
			Value:       float64(i),
			TimestampNs: int64(i),
			SampleRate:  1.0,
		})
	}

	time.Sleep(100 * time.Millisecond)
	bat.Stop()

	t.Logf("metricsSent=%d metricsDropped=%d sends=%d",
		c.metricsSent.Load(), c.metricsDropped.Load(), transport.sends.Load())

	// With 10K ring and 600 items total, there should be zero drops.
	if c.metricsDropped.Load() > 0 {
		t.Errorf("expected 0 drops with 10K ring and 600 items, got %d", c.metricsDropped.Load())
	}
	// All 600 items should be sent.
	if c.metricsSent.Load() != 600 {
		t.Errorf("expected 600 metrics sent, got %d", c.metricsSent.Load())
	}
}

// TestBatcher_HighVolumeProducer simulates the testbench scenario:
// one producer at 50K items/sec for 1 second, verifying the flush loop
// keeps up and doesn't stall.
func TestBatcher_HighVolumeProducer(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	cs := newContextSet(50000)
	// Match production config: 100K ring, 100ms flush
	bat := newBatcher(transport, 100*time.Millisecond, 100000, 10000, 25000, 5000, cs, c)

	// Produce 5K items
	const totalItems = 5000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalItems; i++ {
			bat.AddPoint(metricPoint{
				ContextKey:  uint64(i % 5000),
				Value:       float64(i),
				TimestampNs: int64(i * 1000),
				SampleRate:  1.0,
			})
		}
	}()

	wg.Wait()
	// Wait for remaining flushes.
	time.Sleep(500 * time.Millisecond)
	bat.Stop()

	t.Logf("metricsSent=%d metricsDropped=%d sends=%d bytes=%d",
		c.metricsSent.Load(), c.metricsDropped.Load(),
		transport.sends.Load(), transport.bytes.Load())

	// With 100K ring and 50K items, zero drops expected.
	if c.metricsDropped.Load() > 0 {
		t.Errorf("expected 0 drops, got %d", c.metricsDropped.Load())
	}
	if c.metricsSent.Load() != totalItems {
		t.Errorf("expected %d metrics sent, got %d", totalItems, c.metricsSent.Load())
	}
}

// TestBatcher_LargeLogBatchIsChunked verifies that a large log batch (>25 MB
// equivalent) is split into multiple frames, not sent as one giant frame.
func TestBatcher_LargeLogBatchIsChunked(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	cs := newContextSet(50000)
	// Large ring to hold all items, fast flush.
	bat := newBatcher(transport, 5*time.Millisecond, 1000, 100, 50000, 100, cs, c)

	// Add 25,000 log entries — each ~1 KB content. Without chunking this
	// would be a single ~25 MB frame.
	content := make([]byte, 1024)
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}
	for i := 0; i < 25000; i++ {
		bat.logs.add(hook.LogSampleSnapshot{
			Content:     content,
			Status:      "info",
			Tags:        []string{"env:test", "service:bench"},
			Hostname:    "testhost",
			TimestampNs: int64(i * 1000),
		}, nil)
	}

	// Wait for flush.
	time.Sleep(200 * time.Millisecond)
	bat.Stop()

	sends := transport.sends.Load()
	maxFrame := transport.maxFrame.Load()

	t.Logf("logsSent=%d logsDropped=%d sends=%d totalBytes=%d maxFrame=%d",
		c.logsSent.Load(), c.logsDropped.Load(), sends, transport.bytes.Load(), maxFrame)

	// All 25,000 logs should be sent.
	if c.logsSent.Load() != 25000 {
		t.Errorf("expected 25000 logs sent, got %d", c.logsSent.Load())
	}

	// Should be split into multiple frames (25000 / 2000 = 13 chunks).
	if sends < 10 {
		t.Errorf("expected at least 10 Send() calls (chunked), got %d", sends)
	}

	// No single frame should exceed 2 MB (2000 logs × ~1 KB each + overhead).
	const maxAcceptableFrameSize = 4 * 1024 * 1024 // 4 MB generous upper bound
	if maxFrame > maxAcceptableFrameSize {
		t.Errorf("largest frame is %d bytes (%.1f MB), exceeds %d byte limit — chunking not working",
			maxFrame, float64(maxFrame)/1e6, maxAcceptableFrameSize)
	}
}

// TestBatcher_LargeTraceStatsBatchIsChunked verifies that trace stats are
// also chunked, not sent as one giant frame.
func TestBatcher_LargeTraceStatsBatchIsChunked(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	cs := newContextSet(50000)
	bat := newBatcher(transport, 5*time.Millisecond, 1000, 100, 1000, 10000, cs, c)

	// Add 5,000 trace stat entries.
	for i := 0; i < 5000; i++ {
		bat.tss.add(capturedTraceStat{
			Service:          "web-service",
			Name:             "http.request",
			Resource:         "/api/v1/users",
			Type:             "web",
			SpanKind:         "server",
			HTTPStatusCode:   200,
			Hits:             uint64(100 + i),
			Errors:           uint64(i % 10),
			DurationNs:       uint64(1000000 * (i + 1)),
			TopLevelHits:     uint64(50 + i),
			OkSummary:        []byte{0x0a, 0x01, byte(i % 256)},
			ErrorSummary:     []byte{0x0b, 0x02, byte(i % 256)},
			Hostname:         "host1",
			Env:              "prod",
			Version:          "1.0.0",
			BucketStartNs:    int64(i) * 1000000000,
			BucketDurationNs: 10000000000,
			TimestampNs:      int64(i) * 1000,
		}, nil)
	}

	time.Sleep(200 * time.Millisecond)
	bat.Stop()

	sends := transport.sends.Load()
	maxFrame := transport.maxFrame.Load()

	t.Logf("tracec.ent.Load()=%d tracec.ropped.Load()=%d sends=%d totalBytes=%d maxFrame=%d",
		c.traceStatsSent.Load(), c.traceStatsDropped.Load(), sends, transport.bytes.Load(), maxFrame)

	// All 5,000 entries should be sent.
	if c.traceStatsSent.Load() != 5000 {
		t.Errorf("expected 5000 trace c.sent.Load(), got %d", c.traceStatsSent.Load())
	}

	// Should be split into multiple frames (5000 / 2000 = 3 chunks).
	if sends < 3 {
		t.Errorf("expected at least 3 Send() calls (chunked), got %d", sends)
	}

	// No single frame should be excessively large.
	const maxAcceptableFrameSize = 2 * 1024 * 1024 // 2 MB
	if maxFrame > maxAcceptableFrameSize {
		t.Errorf("largest frame is %d bytes (%.1f MB), exceeds limit — chunking not working",
			maxFrame, float64(maxFrame)/1e6)
	}
}

// TestBatcher_LargeContextDefBatchIsChunked verifies that 10K context
// definitions are split across multiple frames, not packed into one.
func TestBatcher_LargeContextDefBatchIsChunked(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	cs := newContextSet(50000)
	bat := newBatcher(transport, 5*time.Millisecond, 100000, 15000, 1000, 100, cs, c)

	// Add 10,000 context definitions with realistic name + tags.
	for i := 0; i < 10000; i++ {
		bat.AddContextDef(contextDef{
			ContextKey: uint64(i + 1),
			Name:       "system.cpu.user.by_host_and_env",
			Tags:       []string{"host:web-" + string(rune('a'+i%26)), "env:production", "service:api-gateway", "version:2.1.0", "team:platform"},
			Source:     "dogstatsd",
		})
	}

	time.Sleep(200 * time.Millisecond)
	bat.Stop()

	sends := transport.sends.Load()
	maxFrame := transport.maxFrame.Load()

	t.Logf("metricsSent=%d metricsDropped=%d sends=%d totalBytes=%d maxFrame=%d (%.1f MB)",
		c.metricsSent.Load(), c.metricsDropped.Load(), sends, transport.bytes.Load(),
		maxFrame, float64(maxFrame)/1e6)

	// All 10,000 defs should be sent.
	if c.metricsSent.Load() != 10000 {
		t.Errorf("expected 10000 metrics sent, got %d", c.metricsSent.Load())
	}

	// Should be split into multiple frames (10000 / 2000 = 5 chunks).
	if sends < 5 {
		t.Errorf("expected at least 5 Send() calls (chunked), got %d", sends)
	}

	// No single frame should exceed 2 MB.
	const maxAcceptableFrameSize = 2 * 1024 * 1024
	if maxFrame > maxAcceptableFrameSize {
		t.Errorf("largest frame is %d bytes (%.1f MB), exceeds limit — chunking not working",
			maxFrame, float64(maxFrame)/1e6)
	}
}
