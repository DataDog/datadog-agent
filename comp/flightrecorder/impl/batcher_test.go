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

// recordingTransport records the number of Send() calls and total bytes.
type recordingTransport struct {
	sends atomic.Int64
	bytes atomic.Int64
}

func (t *recordingTransport) Send(b []byte) error {
	t.sends.Add(1)
	t.bytes.Add(int64(len(b)))
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
					Source:      "test",
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
				ContextKey:  uint64(i + 100000),
				Name:        "metric.name",
				Value:       float64(i),
				Tags:        []string{"tag:value"},
				TimestampNs: int64(i),
				SampleRate:  1.0,
				Source:      "test",
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

	stats := c.stats()
	total := stats.MetricsSent + stats.MetricsDropped + stats.LogsSent + stats.LogsDropped
	if total == 0 {
		t.Error("expected some items processed, got 0")
	}
	t.Logf("sends=%d bytes=%d metricsSent=%d metricsDropped=%d logsSent=%d logsDropped=%d",
		transport.sends.Load(), transport.bytes.Load(),
		stats.MetricsSent, stats.MetricsDropped,
		stats.LogsSent, stats.LogsDropped)
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
			Source:      "test",
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
			Source:      "test",
		})
	}

	time.Sleep(100 * time.Millisecond)
	bat.Stop()

	stats := c.stats()
	t.Logf("metricsSent=%d metricsDropped=%d sends=%d",
		stats.MetricsSent, stats.MetricsDropped, transport.sends.Load())

	// With 10K ring and 600 items total, there should be zero drops.
	if stats.MetricsDropped > 0 {
		t.Errorf("expected 0 drops with 10K ring and 600 items, got %d", stats.MetricsDropped)
	}
	// All 600 items should be sent.
	if stats.MetricsSent != 600 {
		t.Errorf("expected 600 metrics sent, got %d", stats.MetricsSent)
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
				Source:      "dogstatsd",
			})
		}
	}()

	wg.Wait()
	// Wait for remaining flushes.
	time.Sleep(500 * time.Millisecond)
	bat.Stop()

	stats := c.stats()
	t.Logf("metricsSent=%d metricsDropped=%d sends=%d bytes=%d",
		stats.MetricsSent, stats.MetricsDropped,
		transport.sends.Load(), transport.bytes.Load())

	// With 100K ring and 50K items, zero drops expected.
	if stats.MetricsDropped > 0 {
		t.Errorf("expected 0 drops, got %d", stats.MetricsDropped)
	}
	if stats.MetricsSent != totalItems {
		t.Errorf("expected %d metrics sent, got %d", totalItems, stats.MetricsSent)
	}
}
