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

	flatbuffers "github.com/google/flatbuffers/go"
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

// Test helpers to reduce pipeline construction boilerplate.

func newTestMetricsPipeline(transport Transport, flushInterval time.Duration, ptCap, defCap int, c *counters) *pipeline[metricPoint] {
	return newPipeline[metricPoint](
		transport, flushInterval, ptCap, defCap,
		func(p *builderPool, buf []metricPoint, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodePointBatch(p, buf, tail, count, cap)
		},
		func(p *builderPool, buf []contextDef, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeContextBatch(p, buf, tail, count, cap)
		},
		"metrics", c.incMetricsSent, c.incMetricsDroppedOverflow, c.incMetricsDroppedTransport,
		newBuilderPool(), c,
	)
}

func newTestLogsPipeline(transport Transport, flushInterval time.Duration, logCap, defCap int, c *counters) *pipeline[logEntry] {
	return newPipeline[logEntry](
		transport, flushInterval, logCap, defCap,
		func(p *builderPool, buf []logEntry, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeLogEntryBatch(p, buf, tail, count, cap)
		},
		func(p *builderPool, buf []contextDef, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeLogContextBatch(p, buf, tail, count, cap)
		},
		"logs", c.incLogsSent, c.incLogsDroppedOverflow, c.incLogsDroppedTransport,
		newBuilderPool(), c,
	)
}

func newTestTracePipeline(transport Transport, flushInterval time.Duration, tssCap int, c *counters) *pipeline[capturedTraceStat] {
	return newPipeline[capturedTraceStat](
		transport, flushInterval, tssCap, 0,
		func(p *builderPool, buf []capturedTraceStat, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeTraceStatsBatchRing(p, buf, tail, count, cap)
		},
		nil,
		"trace_stats", c.incTraceStatsSent, c.incTraceStatsDroppedOverflow, c.incTraceStatsDroppedTransport,
		newBuilderPool(), c,
	)
}

// TestPipeline_ConcurrentAddAndFlush exercises pipelines with concurrent
// AddEntry and AddContextDef calls while flush loops are running.
func TestPipeline_ConcurrentAddAndFlush(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}

	metricsPipe := newTestMetricsPipeline(transport, 10*time.Millisecond, 1000, 100, c)
	logsPipe := newTestLogsPipeline(transport, 10*time.Millisecond, 500, 100, c)

	const itemsPerGoroutine = 500
	var wg sync.WaitGroup

	// Goroutine 1-2: metric points
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < itemsPerGoroutine; i++ {
				metricsPipe.AddEntry(metricPoint{
					ContextKey:  uint64(id*itemsPerGoroutine + i),
					Value:       float64(i),
					TimestampNs: int64(i),
					SampleRate:  1.0,
				})
			}
		}(g)
	}

	// Goroutine 3: metric context defs
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < itemsPerGoroutine; i++ {
			metricsPipe.AddContextDef(contextDef{
				ContextKey: uint64(i + 100000),
				Name:       "metric.name",
				Tags:       []string{"tag:value"},
				Source:     "test",
			})
		}
	}()

	// Goroutine 4: log entries
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < itemsPerGoroutine; i++ {
			logsPipe.AddEntry(logEntry{
				ContextKey:  uint64(i % 100),
				Content:     []byte("log line"),
				TimestampNs: int64(i),
			})
		}
	}()

	wg.Wait()
	metricsPipe.Stop()
	logsPipe.Stop()

	if transport.sends.Load() == 0 {
		t.Error("expected at least one Send() call, got 0")
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

// TestPipeline_FlushLoopDrainsRing verifies that items are eventually flushed.
func TestPipeline_FlushLoopDrainsRing(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	pipe := newTestMetricsPipeline(transport, 5*time.Millisecond, 10000, 1000, c)

	for i := 0; i < 500; i++ {
		pipe.AddEntry(metricPoint{
			ContextKey: uint64(i % 100), Value: float64(i),
			TimestampNs: int64(i), SampleRate: 1.0,
		})
	}

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 100; i++ {
		pipe.AddEntry(metricPoint{
			ContextKey: uint64(i % 100), Value: float64(i),
			TimestampNs: int64(i), SampleRate: 1.0,
		})
	}

	time.Sleep(100 * time.Millisecond)
	pipe.Stop()

	if c.metricsDropped.Load() > 0 {
		t.Errorf("expected 0 drops, got %d", c.metricsDropped.Load())
	}
	if c.metricsSent.Load() != 600 {
		t.Errorf("expected 600 metrics sent, got %d", c.metricsSent.Load())
	}
}

// TestPipeline_HighVolumeProducer verifies the flush loop keeps up at high throughput.
func TestPipeline_HighVolumeProducer(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	pipe := newTestMetricsPipeline(transport, 100*time.Millisecond, 100000, 10000, c)

	const totalItems = 5000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalItems; i++ {
			pipe.AddEntry(metricPoint{
				ContextKey: uint64(i % 5000), Value: float64(i),
				TimestampNs: int64(i * 1000), SampleRate: 1.0,
			})
		}
	}()

	wg.Wait()
	time.Sleep(500 * time.Millisecond)
	pipe.Stop()

	if c.metricsDropped.Load() > 0 {
		t.Errorf("expected 0 drops, got %d", c.metricsDropped.Load())
	}
	if c.metricsSent.Load() != totalItems {
		t.Errorf("expected %d metrics sent, got %d", totalItems, c.metricsSent.Load())
	}
}

// TestPipeline_LargeLogBatchIsChunked verifies log entries are chunked.
func TestPipeline_LargeLogBatchIsChunked(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	pipe := newTestLogsPipeline(transport, 5*time.Millisecond, 50000, 100, c)

	content := make([]byte, 1024)
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}
	for i := 0; i < 25000; i++ {
		pipe.entries.add(logEntry{
			ContextKey: uint64(i % 100), Content: content,
			TimestampNs: int64(i * 1000),
		}, nil)
	}

	time.Sleep(200 * time.Millisecond)
	pipe.Stop()

	if c.logsSent.Load() != 25000 {
		t.Errorf("expected 25000 logs sent, got %d", c.logsSent.Load())
	}
	if transport.sends.Load() < 10 {
		t.Errorf("expected at least 10 Send() calls (chunked), got %d", transport.sends.Load())
	}
	const maxAcceptableFrameSize = 4 * 1024 * 1024
	if transport.maxFrame.Load() > maxAcceptableFrameSize {
		t.Errorf("largest frame is %d bytes, exceeds limit", transport.maxFrame.Load())
	}
}

// TestPipeline_LargeTraceStatsBatchIsChunked verifies trace stats are chunked.
func TestPipeline_LargeTraceStatsBatchIsChunked(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	pipe := newTestTracePipeline(transport, 5*time.Millisecond, 10000, c)

	for i := 0; i < 5000; i++ {
		pipe.entries.add(capturedTraceStat{
			Service: "web-service", Name: "http.request", Resource: "/api/v1/users",
			Type: "web", SpanKind: "server", HTTPStatusCode: 200,
			Hits: uint64(100 + i), Errors: uint64(i % 10),
			DurationNs: uint64(1000000 * (i + 1)), TopLevelHits: uint64(50 + i),
			OkSummary: []byte{0x0a, 0x01, byte(i % 256)},
			ErrorSummary: []byte{0x0b, 0x02, byte(i % 256)},
			Hostname: "host1", Env: "prod", Version: "1.0.0",
			BucketStartNs: int64(i) * 1000000000, BucketDurationNs: 10000000000,
			TimestampNs: int64(i) * 1000,
		}, nil)
	}

	time.Sleep(200 * time.Millisecond)
	pipe.Stop()

	if c.traceStatsSent.Load() != 5000 {
		t.Errorf("expected 5000 trace stats sent, got %d", c.traceStatsSent.Load())
	}
	if transport.sends.Load() < 3 {
		t.Errorf("expected at least 3 Send() calls (chunked), got %d", transport.sends.Load())
	}
}

// TestPipeline_LargeContextDefBatchIsChunked verifies context defs are chunked.
func TestPipeline_LargeContextDefBatchIsChunked(t *testing.T) {
	transport := &recordingTransport{}
	c := &counters{}
	pipe := newTestMetricsPipeline(transport, 5*time.Millisecond, 100000, 15000, c)

	for i := 0; i < 10000; i++ {
		pipe.AddContextDef(contextDef{
			ContextKey: uint64(i + 1),
			Name:       "system.cpu.user.by_host_and_env",
			Tags:       []string{"host:web-" + string(rune('a'+i%26)), "env:production", "service:api-gateway", "version:2.1.0", "team:platform"},
			Source:     "dogstatsd",
		})
	}

	time.Sleep(200 * time.Millisecond)
	pipe.Stop()

	if c.metricsSent.Load() != 10000 {
		t.Errorf("expected 10000 metrics sent, got %d", c.metricsSent.Load())
	}
	if transport.sends.Load() < 5 {
		t.Errorf("expected at least 5 Send() calls (chunked), got %d", transport.sends.Load())
	}
	const maxAcceptableFrameSize = 2 * 1024 * 1024
	if transport.maxFrame.Load() > maxAcceptableFrameSize {
		t.Errorf("largest frame is %d bytes, exceeds limit", transport.maxFrame.Load())
	}
}
