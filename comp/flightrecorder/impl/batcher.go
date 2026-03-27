// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"
	"time"
)

// maxEncodeBatchSize caps the number of metric points encoded per FlatBuffers
// frame. This keeps the builder under 256 KB (the pool retention limit) even
// at 100K samples/s, so the builder is reused from the pool instead of
// allocated fresh each flush. At high throughput, the batcher sends multiple
// smaller frames per flush cycle instead of one large frame.
const maxEncodeBatchSize = 2000

// batcher accumulates metrics and logs in per-type ring buffers and flushes
// them at a fixed interval via the provided Transport.
//
// Metrics use a split-buffer design for minimal RSS:
//   - A large ring of compact metricPoint structs (32 bytes each) for data
//     points (the 99.9% fast path after context warm-up).
//   - A small ring of contextDef structs for first-occurrence context
//     definitions that carry full name+tags strings.
//
// Both use the double-buffer pattern: the "active" buffer receives new items
// under a lock, while the "draining" buffer is encoded and sent without
// holding the lock. On flush the two are swapped.
//
// Adaptive flushing: in addition to the fixed-interval ticker, an early flush
// is triggered when any ring buffer exceeds 80% capacity. This eliminates
// drops at extreme throughput without increasing baseline RSS.
type batcher struct {
	transport     Transport
	flushInterval time.Duration
	counters      *counters

	mu sync.Mutex

	// Metrics: compact data-point ring (48 bytes/item, Source is a static string constant).
	ptCap      int
	ptsActive  []metricPoint
	ptsDrain   []metricPoint
	ptsActiveN int
	ptsActiveH int

	// Metrics: context-definition ring (first-occurrence only).
	defCap      int
	defsActive  []contextDef
	defsDrain   []contextDef
	defsActiveN int
	defsActiveH int

	// Logs double-buffer.
	logCap      int
	logsActive  []capturedLog
	logsDrain   []capturedLog
	logsActiveN int
	logsActiveH int

	// Trace stats double-buffer.
	tssCap      int
	tssActive   []capturedTraceStat
	tssDrain    []capturedTraceStat
	tssActiveN  int
	tssActiveH  int

	// FlatBuffers builder pool.
	builderPool *builderPool

	// seenContexts tracks context keys already sent with full name+tags.
	// Sharded bounded map: ~16 bytes/entry vs ~120 bytes for sync.Map.
	// Reset on transport reconnect (sidecar lost state) or when cap exceeded.
	seenContexts *contextSet

	// Watermark thresholds (80% of capacity). When any ring crosses its
	// threshold, a non-blocking signal is sent to flushCh.
	ptWatermark  int
	defWatermark int
	logWatermark int
	tssWatermark int
	flushCh      chan struct{} // capacity 1, non-blocking signal

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func newBatcher(transport Transport, flushInterval time.Duration, ptCapacity, defCapacity, logCapacity, traceStatsCapacity, contextCap int, c *counters) *batcher {
	b := &batcher{
		transport:     transport,
		flushInterval: flushInterval,
		counters:      c,

		ptCap:     ptCapacity,
		ptsActive: make([]metricPoint, ptCapacity),
		ptsDrain:  make([]metricPoint, ptCapacity),

		defCap:     defCapacity,
		defsActive: make([]contextDef, defCapacity),
		defsDrain:  make([]contextDef, defCapacity),

		logCap:     logCapacity,
		logsActive: make([]capturedLog, logCapacity),
		logsDrain:  make([]capturedLog, logCapacity),

		tssCap:    traceStatsCapacity,
		tssActive: make([]capturedTraceStat, traceStatsCapacity),
		tssDrain:  make([]capturedTraceStat, traceStatsCapacity),

		builderPool:  newBuilderPool(),
		seenContexts: newContextSet(contextCap),
		ptWatermark:  ptCapacity * 4 / 5,
		defWatermark: defCapacity * 4 / 5,
		logWatermark: logCapacity * 4 / 5,
		tssWatermark: traceStatsCapacity * 4 / 5,
		flushCh:      make(chan struct{}, 1),
		stopCh:       make(chan struct{}),
	}
	b.wg.Add(1)
	go b.flushLoop()
	return b
}

// AddPoint enqueues a compact metric data point (known context, no strings).
func (b *batcher) AddPoint(p metricPoint) {
	b.mu.Lock()
	if b.ptsActiveN == b.ptCap {
		b.counters.incMetricsDroppedOverflow(1)
	} else {
		b.ptsActiveN++
	}
	b.ptsActive[b.ptsActiveH] = p
	b.ptsActiveH = (b.ptsActiveH + 1) % b.ptCap
	signal := b.ptsActiveN >= b.ptWatermark
	b.mu.Unlock()
	if signal {
		b.signalFlush()
	}
}

// AddContextDef enqueues a context definition (first occurrence, with strings).
// When the ring is full the oldest entry is overwritten and the drop counter
// increments. The bloom filter does not support deletion, so evicted contexts
// are not re-sent — the sidecar handles unknown context_keys gracefully.
func (b *batcher) AddContextDef(d contextDef) {
	b.mu.Lock()
	if b.defsActiveN == b.defCap {
		b.counters.incMetricsDroppedOverflow(1)
	} else {
		b.defsActiveN++
	}
	b.defsActive[b.defsActiveH] = d
	b.defsActiveH = (b.defsActiveH + 1) % b.defCap
	signal := b.defsActiveN >= b.defWatermark
	b.mu.Unlock()
	if signal {
		b.signalFlush()
	}
}

// AddLog enqueues a log entry. When the ring buffer is full the oldest item is
// overwritten and the drop counter increments.
func (b *batcher) AddLog(l capturedLog) {
	b.mu.Lock()
	if b.logsActiveN == b.logCap {
		b.counters.incLogsDroppedOverflow(1)
	} else {
		b.logsActiveN++
	}
	b.logsActive[b.logsActiveH] = l
	b.logsActiveH = (b.logsActiveH + 1) % b.logCap
	signal := b.logsActiveN >= b.logWatermark
	b.mu.Unlock()
	if signal {
		b.signalFlush()
	}
}

// AddTraceStat enqueues a trace stats entry.
func (b *batcher) AddTraceStat(t capturedTraceStat) {
	b.mu.Lock()
	if b.tssActiveN == b.tssCap {
		b.counters.incTraceStatsDroppedOverflow(1)
	} else {
		b.tssActiveN++
	}
	b.tssActive[b.tssActiveH] = t
	b.tssActiveH = (b.tssActiveH + 1) % b.tssCap
	signal := b.tssActiveN >= b.tssWatermark
	b.mu.Unlock()
	if signal {
		b.signalFlush()
	}
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
		case <-b.flushCh:
			b.flush()
			// Drain the ticker if it fired during flush to avoid double-flushing.
			select {
			case <-ticker.C:
			default:
			}
		}
	}
}

// signalFlush sends a non-blocking signal to the flush loop.
func (b *batcher) signalFlush() {
	select {
	case b.flushCh <- struct{}{}:
	default: // already signaled, flush loop will pick it up
	}
}

func (b *batcher) flush() {
	b.flushMetrics()
	b.flushLogs()
	b.flushTraceStats()
}

func (b *batcher) flushMetrics() {
	// Enforce context set cap periodically — cheap atomic load.
	b.seenContexts.CheckCap()

	b.mu.Lock()
	if b.ptsActiveN == 0 && b.defsActiveN == 0 {
		b.mu.Unlock()
		return
	}

	// Swap both metric buffers under one lock.
	b.ptsActive, b.ptsDrain = b.ptsDrain, b.ptsActive
	ptCount := b.ptsActiveN
	ptHead := b.ptsActiveH
	b.ptsActiveN = 0
	b.ptsActiveH = 0

	b.defsActive, b.defsDrain = b.defsDrain, b.defsActive
	defCount := b.defsActiveN
	defHead := b.defsActiveH
	b.defsActiveN = 0
	b.defsActiveH = 0
	b.mu.Unlock()

	ptTail := (ptHead - ptCount + b.ptCap) % b.ptCap
	defTail := (defHead - defCount + b.defCap) % b.defCap

	b.counters.setBatchSize("metrics", ptCount+defCount)

	// Encode in chunks to keep the FlatBuffers builder under the pool cap.
	// Context defs are always sent first (small batch). Points are chunked.
	sent := 0
	for ptSent := 0; ptSent < ptCount || (ptSent == 0 && defCount > 0); {
		// First chunk includes all defs; subsequent chunks are points-only.
		chunkDefs := 0
		chunkDefTail := defTail
		if ptSent == 0 {
			chunkDefs = defCount
		}
		chunkPts := ptCount - ptSent
		if chunkPts > maxEncodeBatchSize {
			chunkPts = maxEncodeBatchSize
		}
		chunkPtTail := (ptTail + ptSent) % b.ptCap

		builder, err := EncodeSplitMetricBatch(
			b.builderPool,
			b.defsDrain, chunkDefTail, chunkDefs, b.defCap,
			b.ptsDrain, chunkPtTail, chunkPts, b.ptCap,
		)
		if err != nil {
			b.counters.incMetricsDroppedTransport(uint64(chunkDefs + chunkPts))
			break
		}
		data := builder.FinishedBytes()
		if err := b.transport.Send(data); err != nil {
			b.counters.incMetricsDroppedTransport(uint64(chunkDefs + chunkPts))
			b.builderPool.put(builder)
			break
		}
		sent += chunkDefs + chunkPts
		b.counters.incBytesSent(uint64(len(data)))
		b.builderPool.put(builder)
		ptSent += chunkPts
	}
	b.counters.incMetricsSent(uint64(sent))
	returnDefSlicesRing(b.defsDrain, defTail, defCount, b.defCap)
}

func (b *batcher) flushLogs() {
	b.mu.Lock()
	if b.logsActiveN == 0 {
		b.mu.Unlock()
		return
	}
	b.logsActive, b.logsDrain = b.logsDrain, b.logsActive
	count := b.logsActiveN
	head := b.logsActiveH
	b.logsActiveN = 0
	b.logsActiveH = 0
	b.mu.Unlock()

	drain := b.logsDrain
	tail := (head - count + b.logCap) % b.logCap

	b.counters.setBatchSize("logs", count)

	builder, err := EncodeLogBatchRing(b.builderPool, drain, tail, count, b.logCap)
	if err != nil {
		b.counters.incLogsDroppedTransport(uint64(count))
		returnLogSlicesRing(drain, tail, count, b.logCap)
		return
	}
	data := builder.FinishedBytes()
	if err := b.transport.Send(data); err != nil {
		b.counters.incLogsDroppedTransport(uint64(count))
		b.builderPool.put(builder)
		returnLogSlicesRing(drain, tail, count, b.logCap)
		return
	}
	b.counters.incLogsSent(uint64(count))
	b.counters.incBytesSent(uint64(len(data)))
	b.builderPool.put(builder)
	returnLogSlicesRing(drain, tail, count, b.logCap)
}

func (b *batcher) flushTraceStats() {
	b.mu.Lock()
	if b.tssActiveN == 0 {
		b.mu.Unlock()
		return
	}
	b.tssActive, b.tssDrain = b.tssDrain, b.tssActive
	count := b.tssActiveN
	head := b.tssActiveH
	b.tssActiveN = 0
	b.tssActiveH = 0
	b.mu.Unlock()

	drain := b.tssDrain
	tail := (head - count + b.tssCap) % b.tssCap

	b.counters.setBatchSize("trace_stats", count)

	builder, err := EncodeTraceStatsBatchRing(b.builderPool, drain, tail, count, b.tssCap)
	if err != nil {
		b.counters.incTraceStatsDroppedTransport(uint64(count))
		return
	}
	data := builder.FinishedBytes()
	if err := b.transport.Send(data); err != nil {
		b.counters.incTraceStatsDroppedTransport(uint64(count))
		b.builderPool.put(builder)
		return
	}
	b.counters.incTraceStatsSent(uint64(count))
	b.counters.incBytesSent(uint64(len(data)))
	b.builderPool.put(builder)
}

// IsContextKnown returns true if the context key has already been sent to the
// sidecar with full name+tags. If unknown, it atomically marks it as known.
func (b *batcher) IsContextKnown(key uint64) bool {
	return b.seenContexts.IsKnown(key)
}

// ResetContexts clears the seen-context set, forcing all context definitions
// to be re-sent. Called on transport reconnect because the sidecar lost state.
func (b *batcher) ResetContexts() {
	b.seenContexts.Reset()
}

// Stop drains the buffers and stops the flush goroutine.
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

// returnLogSlicesRing returns pooled content and tag slices for items in a ring buffer segment.
func returnLogSlicesRing(buf []capturedLog, tail, count, capacity int) {
	for i := 0; i < count; i++ {
		idx := (tail + i) % capacity
		if buf[idx].ContentPoolSlice != nil {
			*buf[idx].ContentPoolSlice = buf[idx].Content[:0]
			contentPool.Put(buf[idx].ContentPoolSlice)
			buf[idx].ContentPoolSlice = nil
		}
		if buf[idx].TagPoolSlice != nil {
			*buf[idx].TagPoolSlice = buf[idx].Tags[:0]
			tagPool.Put(buf[idx].TagPoolSlice)
			buf[idx].TagPoolSlice = nil
		}
	}
}
