// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipelinesinkimpl

import (
	"sync"
	"time"
)

// batcher accumulates metrics and logs in per-type ring buffers and flushes
// them at a fixed interval via the provided Transport.
type batcher struct {
	transport     Transport
	flushInterval time.Duration
	capacity      int
	counters      *counters

	mu          sync.Mutex
	metricsBuf  []capturedMetric
	logsBuf     []capturedLog
	metricsHead int // write position in metricsBuf (ring)
	logsHead    int // write position in logsBuf (ring)
	metricsLen  int // number of valid items in metricsBuf
	logsLen     int // number of valid items in logsBuf

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func newBatcher(transport Transport, flushInterval time.Duration, capacity int, c *counters) *batcher {
	b := &batcher{
		transport:     transport,
		flushInterval: flushInterval,
		capacity:      capacity,
		counters:      c,
		metricsBuf:    make([]capturedMetric, capacity),
		logsBuf:       make([]capturedLog, capacity),
		stopCh:        make(chan struct{}),
	}
	b.wg.Add(1)
	go b.flushLoop()
	return b
}

// AddMetric enqueues a metric sample. When the ring buffer is full the oldest
// item is overwritten and the drop counter increments.
func (b *batcher) AddMetric(m capturedMetric) {
	b.mu.Lock()
	if b.metricsLen == b.capacity {
		b.counters.incMetricsDropped(1)
	} else {
		b.metricsLen++
	}
	b.metricsBuf[b.metricsHead] = m
	b.metricsHead = (b.metricsHead + 1) % b.capacity
	b.mu.Unlock()
}

// AddLog enqueues a log entry. When the ring buffer is full the oldest item is
// overwritten and the drop counter increments.
func (b *batcher) AddLog(l capturedLog) {
	b.mu.Lock()
	if b.logsLen == b.capacity {
		b.counters.incLogsDropped(1)
	} else {
		b.logsLen++
	}
	b.logsBuf[b.logsHead] = l
	b.logsHead = (b.logsHead + 1) % b.capacity
	b.mu.Unlock()
}

func (b *batcher) flushLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			b.flush()
			return
		case <-ticker.C:
			b.flush()
		}
	}
}

func (b *batcher) flush() {
	b.flushMetrics()
	b.flushLogs()
}

func (b *batcher) flushMetrics() {
	b.mu.Lock()
	if b.metricsLen == 0 {
		b.mu.Unlock()
		return
	}
	// Drain the ring into a flat slice (oldest-first).
	count := b.metricsLen
	samples := make([]capturedMetric, count)
	// tail (oldest write position) = (head - len + capacity) % capacity
	tail := (b.metricsHead - count + b.capacity) % b.capacity
	for i := 0; i < count; i++ {
		samples[i] = b.metricsBuf[(tail+i)%b.capacity]
	}
	b.metricsLen = 0
	b.metricsHead = 0
	b.mu.Unlock()

	data, err := EncodeMetricBatch(samples)
	if err != nil {
		b.counters.incMetricsDropped(uint64(len(samples)))
		returnMetricSlices(samples)
		return
	}
	if err := b.transport.Send(data); err != nil {
		b.counters.incMetricsDropped(uint64(len(samples)))
		returnMetricSlices(samples)
		return
	}
	b.counters.incMetricsSent(uint64(len(samples)))
	b.counters.incBytesSent(uint64(len(data)))
	returnMetricSlices(samples)
}

func (b *batcher) flushLogs() {
	b.mu.Lock()
	if b.logsLen == 0 {
		b.mu.Unlock()
		return
	}
	count := b.logsLen
	entries := make([]capturedLog, count)
	tail := (b.logsHead - count + b.capacity) % b.capacity
	for i := 0; i < count; i++ {
		entries[i] = b.logsBuf[(tail+i)%b.capacity]
	}
	b.logsLen = 0
	b.logsHead = 0
	b.mu.Unlock()

	data, err := EncodeLogBatch(entries)
	if err != nil {
		b.counters.incLogsDropped(uint64(len(entries)))
		returnLogSlices(entries)
		return
	}
	if err := b.transport.Send(data); err != nil {
		b.counters.incLogsDropped(uint64(len(entries)))
		returnLogSlices(entries)
		return
	}
	b.counters.incLogsSent(uint64(len(entries)))
	b.counters.incBytesSent(uint64(len(data)))
	returnLogSlices(entries)
}

// Stop drains the buffers and stops the flush goroutine.
func (b *batcher) Stop() {
	close(b.stopCh)
	b.wg.Wait()
}

// returnMetricSlices returns pooled tag slices back to the tag pool.
func returnMetricSlices(samples []capturedMetric) {
	for i := range samples {
		if samples[i].TagPoolSlice != nil {
			*samples[i].TagPoolSlice = samples[i].Tags[:0]
			tagPool.Put(samples[i].TagPoolSlice)
			samples[i].TagPoolSlice = nil
		}
	}
}

// returnLogSlices returns pooled content and tag slices back to their pools.
func returnLogSlices(entries []capturedLog) {
	for i := range entries {
		if entries[i].ContentPoolSlice != nil {
			*entries[i].ContentPoolSlice = entries[i].Content[:0]
			contentPool.Put(entries[i].ContentPoolSlice)
			entries[i].ContentPoolSlice = nil
		}
		if entries[i].TagPoolSlice != nil {
			*entries[i].TagPoolSlice = entries[i].Tags[:0]
			tagPool.Put(entries[i].TagPoolSlice)
			entries[i].TagPoolSlice = nil
		}
	}
}
