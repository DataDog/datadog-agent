// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
)

// batcher accumulates metrics, logs, and trace stats in per-type ring buffers
// and flushes them via dedicated goroutines through the shared Transport.
//
// All five signal types (metric contexts, metric points, log contexts,
// log entries, trace stats) use the generic flushChunked() function and get
// their own independent flush goroutine.
type batcher struct {
	transport     Transport
	flushInterval time.Duration
	counters      *counters

	// Metric context definitions (first-occurrence only, with name+tags).
	defs ringBuf[contextDef]
	// Metric data points (compact, context_key reference only).
	pts ringBuf[metricPoint]
	// Log context definitions (first-occurrence only).
	logDefs ringBuf[contextDef]
	// Log entries (compact, context_key reference only).
	logs ringBuf[logEntry]
	// Trace stats entries.
	tss ringBuf[capturedTraceStat]

	// FlatBuffers builder pool.
	builderPool *builderPool

	// seenContexts tracks context keys already sent with full name+tags.
	// Shared across metrics and logs — a context key is signal-agnostic.
	seenContexts *contextSet

	// Per-signal flush channels.
	defsFlushCh    chan struct{}
	ptsFlushCh     chan struct{}
	logDefsFlushCh chan struct{}
	logsFlushCh    chan struct{}
	tssFlushCh     chan struct{}
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// initialCap returns a small starting capacity (1/8th of max, min 1000).
func initialCap(maxCap int) int {
	c := maxCap / 8
	if c < 1000 {
		c = 1000
	}
	return c
}

func newBatcher(transport Transport, flushInterval time.Duration, ptCapacity, defCapacity, logCapacity, traceStatsCapacity int, seenContexts *contextSet, c *counters) *batcher {
	b := &batcher{
		transport:     transport,
		flushInterval: flushInterval,
		counters:      c,

		defs:    newRingBuf[contextDef](initialCap(defCapacity), defCapacity),
		pts:     newRingBuf[metricPoint](initialCap(ptCapacity), ptCapacity),
		logDefs: newRingBuf[contextDef](initialCap(defCapacity), defCapacity),
		logs:    newRingBuf[logEntry](initialCap(logCapacity), logCapacity),
		tss:     newRingBuf[capturedTraceStat](initialCap(traceStatsCapacity), traceStatsCapacity),

		builderPool:    newBuilderPool(),
		seenContexts:   seenContexts,
		defsFlushCh:    make(chan struct{}, 1),
		ptsFlushCh:     make(chan struct{}, 1),
		logDefsFlushCh: make(chan struct{}, 1),
		logsFlushCh:    make(chan struct{}, 1),
		tssFlushCh:     make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
	}
	b.wg.Add(5)
	go b.flushLoopFn(b.defsFlushCh, b.flushContexts)
	go b.flushLoopFn(b.ptsFlushCh, b.flushPoints)
	go b.flushLoopFn(b.logDefsFlushCh, b.flushLogContexts)
	go b.flushLoopFn(b.logsFlushCh, b.flushLogs)
	go b.flushLoopFn(b.tssFlushCh, b.flushTraceStats)
	return b
}

// AddPoint enqueues a compact metric data point (known context, no strings).
func (b *batcher) AddPoint(p metricPoint) {
	if b.pts.add(p, func() { b.counters.incMetricsDroppedOverflow(1) }) {
		signalCh(b.ptsFlushCh)
	}
}

// AddContextDef enqueues a context definition (first occurrence, with strings).
func (b *batcher) AddContextDef(d contextDef) {
	if b.defs.add(d, func() { b.counters.incMetricsDroppedOverflow(1) }) {
		signalCh(b.defsFlushCh)
	}
}

// AddLogEntry enqueues a compact log entry (context key already computed).
func (b *batcher) AddLogEntry(e logEntry) {
	if b.logs.add(e, func() { b.counters.incLogsDroppedOverflow(1) }) {
		signalCh(b.logsFlushCh)
	}
}

// AddLogContextDef enqueues a log context definition (first occurrence only).
func (b *batcher) AddLogContextDef(d contextDef) {
	if b.logDefs.add(d, func() { b.counters.incLogsDroppedOverflow(1) }) {
		signalCh(b.logDefsFlushCh)
	}
}

// AddTraceStat enqueues a trace stats entry.
func (b *batcher) AddTraceStat(t capturedTraceStat) {
	if b.tss.add(t, func() { b.counters.incTraceStatsDroppedOverflow(1) }) {
		signalCh(b.tssFlushCh)
	}
}

// flushLoopFn runs a flush loop for a single signal type.
func (b *batcher) flushLoopFn(flushCh <-chan struct{}, flushFn func()) {
	defer b.wg.Done()
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			flushFn()
			return
		case <-ticker.C:
			flushFn()
		case <-flushCh:
			flushFn()
			select {
			case <-ticker.C:
			default:
			}
		}
	}
}

func signalCh(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (b *batcher) flushContexts() {
	flushChunked(
		&b.defs,
		func(pool *builderPool, buf []contextDef, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeContextBatch(pool, buf, tail, count, cap)
		},
		b.builderPool,
		b.transport,
		b.counters,
		"contexts",
		b.counters.incMetricsSent,
		b.counters.incMetricsDroppedTransport,
	)
}

func (b *batcher) flushPoints() {
	flushChunked(
		&b.pts,
		func(pool *builderPool, buf []metricPoint, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodePointBatch(pool, buf, tail, count, cap)
		},
		b.builderPool,
		b.transport,
		b.counters,
		"points",
		b.counters.incMetricsSent,
		b.counters.incMetricsDroppedTransport,
	)
}

func (b *batcher) flushLogContexts() {
	flushChunked(
		&b.logDefs,
		func(pool *builderPool, buf []contextDef, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeLogContextBatch(pool, buf, tail, count, cap)
		},
		b.builderPool,
		b.transport,
		b.counters,
		"log_contexts",
		b.counters.incLogsSent,
		b.counters.incLogsDroppedTransport,
	)
}

func (b *batcher) flushLogs() {
	flushChunked(
		&b.logs,
		func(pool *builderPool, buf []logEntry, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeLogEntryBatch(pool, buf, tail, count, cap)
		},
		b.builderPool,
		b.transport,
		b.counters,
		"logs",
		b.counters.incLogsSent,
		b.counters.incLogsDroppedTransport,
	)
}

func (b *batcher) flushTraceStats() {
	flushChunked(
		&b.tss,
		func(pool *builderPool, buf []capturedTraceStat, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeTraceStatsBatchRing(pool, buf, tail, count, cap)
		},
		b.builderPool,
		b.transport,
		b.counters,
		"trace_stats",
		b.counters.incTraceStatsSent,
		b.counters.incTraceStatsDroppedTransport,
	)
}

// IsContextKnown returns true if the context key has already been sent to the
// sidecar with full name+tags. If unknown, it atomically marks it as known.
func (b *batcher) IsContextKnown(key uint64) bool {
	return b.seenContexts.IsKnown(key)
}

// Stop drains the buffers and stops all flush goroutines.
func (b *batcher) Stop() {
	close(b.stopCh)
	b.wg.Wait()
}

// returnDefSlicesRing returns pooled tag slices for context definitions.
func returnDefSlicesRing(buf []contextDef, tail, count, capacity int) {
	for i := 0; i < count; i++ {
		idx := (tail + i) % capacity
		if buf[idx].TagPoolSlice != nil {
			*buf[idx].TagPoolSlice = buf[idx].Tags[:0]
			tagPool.Put(buf[idx].TagPoolSlice)
			buf[idx].TagPoolSlice = nil
		}
	}
}
