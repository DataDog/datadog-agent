// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/DataDog/datadog-agent/pkg/hook"
)

// batcher accumulates metrics, logs, and trace stats in per-type ring buffers
// and flushes them at a fixed interval via the provided Transport.
//
// Metrics use a split-buffer design for minimal RSS:
//   - A large ring of compact metricPoint structs for data points.
//   - A small ring of contextDef structs for first-occurrence context
//     definitions that carry full name+tags strings.
//
// All rings use the generic ringBuf[T] with double-buffering and chunked
// flushing (maxChunkSize items per FlatBuffers frame) to keep the flush
// goroutine responsive and prevent oversized socket writes.
type batcher struct {
	transport     Transport
	flushInterval time.Duration
	counters      *counters

	// Metrics: compact data-point ring.
	pts ringBuf[metricPoint]
	// Metrics: context-definition ring (first-occurrence only).
	defs ringBuf[contextDef]
	// Logs ring.
	logs ringBuf[hook.LogSampleSnapshot]
	// Trace stats ring.
	tss ringBuf[capturedTraceStat]

	// FlatBuffers builder pool.
	builderPool *builderPool

	// seenContexts tracks context keys already sent with full name+tags.
	seenContexts *contextSet

	// Per-signal flush channels. Each signal type has its own flush
	// goroutine so encoding one type cannot block another.
	metricsFlushCh chan struct{}
	logsFlushCh    chan struct{}
	tssFlushCh     chan struct{}
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// initialCap returns a small starting capacity (1/8th of max, min 1000).
// The ring grows adaptively from here up to maxCap as needed.
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

		pts:  newRingBuf[metricPoint](initialCap(ptCapacity), ptCapacity),
		defs: newRingBuf[contextDef](initialCap(defCapacity), defCapacity),
		logs: newRingBuf[hook.LogSampleSnapshot](initialCap(logCapacity), logCapacity),
		tss:  newRingBuf[capturedTraceStat](initialCap(traceStatsCapacity), traceStatsCapacity),

		builderPool:    newBuilderPool(),
		seenContexts:   seenContexts,
		metricsFlushCh: make(chan struct{}, 1),
		logsFlushCh:    make(chan struct{}, 1),
		tssFlushCh:     make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
	}
	b.wg.Add(3)
	go b.flushLoopFn(b.metricsFlushCh, b.flushMetrics)
	go b.flushLoopFn(b.logsFlushCh, b.flushLogs)
	go b.flushLoopFn(b.tssFlushCh, b.flushTraceStats)
	return b
}

// AddPoint enqueues a compact metric data point (known context, no strings).
func (b *batcher) AddPoint(p metricPoint) {
	if b.pts.add(p, func() { b.counters.incMetricsDroppedOverflow(1) }) {
		signalCh(b.metricsFlushCh)
	}
}

// AddContextDef enqueues a context definition (first occurrence, with strings).
func (b *batcher) AddContextDef(d contextDef) {
	if b.defs.add(d, func() { b.counters.incMetricsDroppedOverflow(1) }) {
		signalCh(b.metricsFlushCh)
	}
}

// AddLogBatch enqueues a batch of log snapshots with a single lock acquisition.
func (b *batcher) AddLogBatch(batch []hook.LogSampleSnapshot) {
	if b.logs.addBatch(batch, func() { b.counters.incLogsDroppedOverflow(1) }) {
		signalCh(b.logsFlushCh)
	}
}

// AddTraceStat enqueues a trace stats entry.
func (b *batcher) AddTraceStat(t capturedTraceStat) {
	if b.tss.add(t, func() { b.counters.incTraceStatsDroppedOverflow(1) }) {
		signalCh(b.tssFlushCh)
	}
}

// flushLoopFn runs a flush loop for a single signal type. Each signal type
// gets its own goroutine so encoding one type cannot block another.
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

// signalCh sends a non-blocking signal to a flush channel.
func signalCh(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (b *batcher) flushMetrics() {
	// Metrics have two rings (defs + points) that must be swapped together.
	b.pts.mu.Lock()
	b.defs.mu.Lock()
	if b.pts.activeN == 0 && b.defs.activeN == 0 {
		b.defs.mu.Unlock()
		b.pts.mu.Unlock()
		return
	}

	b.pts.active, b.pts.drain = b.pts.drain, b.pts.active
	ptCount := b.pts.activeN
	ptHead := b.pts.activeH
	ptCap := b.pts.cap
	ptDrain := b.pts.drain
	b.pts.activeN = 0
	b.pts.activeH = 0

	b.defs.active, b.defs.drain = b.defs.drain, b.defs.active
	defCount := b.defs.activeN
	defHead := b.defs.activeH
	defCap := b.defs.cap
	defDrain := b.defs.drain
	b.defs.activeN = 0
	b.defs.activeH = 0

	b.defs.mu.Unlock()
	b.pts.mu.Unlock()

	ptTail := (ptHead - ptCount + ptCap) % ptCap
	defTail := (defHead - defCount + defCap) % defCap

	b.counters.setBatchSize("metrics", ptCount+defCount)
	b.counters.incFlushCycles()

	sent := 0
	defSent := 0
	ptSent := 0
	for defSent < defCount || ptSent < ptCount {
		chunkDefs := defCount - defSent
		if chunkDefs > maxChunkSize {
			chunkDefs = maxChunkSize
		}
		chunkDefTail := (defTail + defSent) % defCap

		chunkPts := ptCount - ptSent
		if chunkPts > maxChunkSize {
			chunkPts = maxChunkSize
		}
		chunkPtTail := (ptTail + ptSent) % ptCap

		builder, err := EncodeSplitMetricBatch(
			b.builderPool,
			defDrain, chunkDefTail, chunkDefs, defCap,
			ptDrain, chunkPtTail, chunkPts, ptCap,
		)
		if err != nil {
			b.counters.incMetricsDroppedTransport(uint64(chunkDefs + chunkPts))
			break
		}
		data := builder.FinishedBytes()
		sendStart := time.Now()
		sendErr := b.transport.Send(data)
		b.counters.setSendDuration(time.Since(sendStart).Nanoseconds())
		if sendErr != nil {
			b.counters.incMetricsDroppedTransport(uint64(chunkDefs + chunkPts))
			b.builderPool.put(builder)
			break
		}
		sent += chunkDefs + chunkPts
		b.counters.incBytesSent(uint64(len(data)), "metrics")
		b.builderPool.put(builder)
		defSent += chunkDefs
		ptSent += chunkPts
	}
	b.counters.incMetricsSent(uint64(sent))
	returnDefSlicesRing(defDrain, defTail, defCount, defCap)
}

func (b *batcher) flushLogs() {
	flushChunked(
		&b.logs,
		func(pool *builderPool, buf []hook.LogSampleSnapshot, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeLogBatchRing(pool, buf, tail, count, cap)
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

// Stop drains the buffers and stops the flush goroutine.
// The seenContexts set is NOT stopped here — it persists across reconnect
// cycles and is owned by sinkImpl.
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
