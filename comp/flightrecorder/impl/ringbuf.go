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

// maxChunkSize is the maximum number of items encoded per FlatBuffers frame.
// Caps frame byte size to ~128 KB for data points, ~500 KB for context defs.
// Keeps the flush goroutine responsive and prevents partial writes on the socket.
const maxChunkSize = 2000

// ringBuf is a generic double-buffered ring that handles the swap/drain/chunk
// pattern uniformly across all signal types (metrics, logs, trace stats).
//
// The ring is adaptive: it starts at initialCap and doubles when full, up to
// maxCap. Only pods that actually need large buffers pay the memory cost.
// Growth happens under the mutex during add() — a one-time O(n) copy that
// occurs at most log2(maxCap/initialCap) times.
type ringBuf[T any] struct {
	mu        sync.Mutex
	cap       int
	maxCap    int
	active    []T
	drain     []T
	activeN   int
	activeH   int
	watermark int
}

func newRingBuf[T any](initialCap, maxCap int) ringBuf[T] {
	if maxCap < initialCap {
		maxCap = initialCap
	}
	return ringBuf[T]{
		cap:       initialCap,
		maxCap:    maxCap,
		active:    make([]T, initialCap),
		drain:     make([]T, initialCap),
		watermark: initialCap * 4 / 5,
	}
}

// grow doubles the ring capacity up to maxCap. Must be called with mu held.
// Linearizes the circular active buffer into the new larger buffer.
func (r *ringBuf[T]) grow() {
	newCap := r.cap * 2
	if newCap > r.maxCap {
		newCap = r.maxCap
	}
	if newCap == r.cap {
		return // already at max
	}

	newActive := make([]T, newCap)
	// Copy items from circular buffer [tail..head) into linear [0..activeN).
	tail := (r.activeH - r.activeN + r.cap) % r.cap
	for i := 0; i < r.activeN; i++ {
		newActive[i] = r.active[(tail+i)%r.cap]
	}

	r.active = newActive
	r.drain = make([]T, newCap)
	r.activeH = r.activeN // items are now at [0..activeN)
	r.cap = newCap
	r.watermark = newCap * 4 / 5
}

// add enqueues an item. Returns true if the watermark was reached (caller
// should signal an early flush). If the ring is full and at maxCap, the item
// overwrites the oldest entry and overflowFn is called. If below maxCap, the
// ring grows instead.
func (r *ringBuf[T]) add(item T, overflowFn func()) bool {
	r.mu.Lock()
	if r.activeN == r.cap {
		if r.cap < r.maxCap {
			r.grow()
		} else if overflowFn != nil {
			overflowFn()
		}
	}
	if r.activeN < r.cap {
		r.activeN++
	}
	r.active[r.activeH] = item
	r.activeH = (r.activeH + 1) % r.cap
	signal := r.activeN >= r.watermark
	r.mu.Unlock()
	return signal
}

// addBatch enqueues multiple items with a single lock acquisition.
func (r *ringBuf[T]) addBatch(items []T, overflowFn func()) bool {
	r.mu.Lock()
	for i := range items {
		if r.activeN == r.cap {
			if r.cap < r.maxCap {
				r.grow()
			} else if overflowFn != nil {
				overflowFn()
			}
		}
		if r.activeN < r.cap {
			r.activeN++
		}
		r.active[r.activeH] = items[i]
		r.activeH = (r.activeH + 1) % r.cap
	}
	signal := r.activeN >= r.watermark
	r.mu.Unlock()
	return signal
}

// swapResult holds the drain buffer state after a swap.
type swapResult[T any] struct {
	buf   []T
	tail  int
	count int
	cap   int
}

// swap atomically exchanges the active and drain buffers. Returns false
// if the ring was empty (nothing to drain).
func (r *ringBuf[T]) swap() (swapResult[T], bool) {
	r.mu.Lock()
	if r.activeN == 0 {
		r.mu.Unlock()
		return swapResult[T]{}, false
	}
	r.active, r.drain = r.drain, r.active
	count := r.activeN
	head := r.activeH
	cap := r.cap
	drain := r.drain
	r.activeN = 0
	r.activeH = 0
	r.mu.Unlock()

	tail := (head - count + cap) % cap
	return swapResult[T]{
		buf:   drain,
		tail:  tail,
		count: count,
		cap:   cap,
	}, true
}

// encodeFunc encodes a chunk of items from the ring into a FlatBuffers builder.
// It receives the drain buffer, the starting tail index, the chunk size, and
// the ring capacity. Returns the builder (which must be returned to the pool).
type encodeFunc[T any] func(pool *builderPool, buf []T, tail, count, cap int) (*flatbuffers.Builder, error)

// flushChunked swaps the ring, encodes items in chunks of maxChunkSize,
// and sends each chunk via the transport. Returns (sent, error).
//
// This is the single implementation of the swap → chunk → encode → send
// pattern used by all signal types.
func flushChunked[T any](
	ring *ringBuf[T],
	encode encodeFunc[T],
	pool *builderPool,
	transport Transport,
	counters *counters,
	signalType string,
	incSent func(uint64),
	incDropped func(uint64),
) int {
	sr, ok := ring.swap()
	if !ok {
		return 0
	}

	counters.setBatchSize(signalType, sr.count)
	counters.incFlushCycles()

	sent := 0
	for chunkStart := 0; chunkStart < sr.count; {
		chunkSize := sr.count - chunkStart
		if chunkSize > maxChunkSize {
			chunkSize = maxChunkSize
		}
		chunkTail := (sr.tail + chunkStart) % sr.cap

		builder, err := encode(pool, sr.buf, chunkTail, chunkSize, sr.cap)
		if err != nil {
			incDropped(uint64(chunkSize))
			break
		}
		data := builder.FinishedBytes()
		sendStart := time.Now()
		sendErr := transport.Send(data)
		counters.setSendDuration(time.Since(sendStart).Nanoseconds())
		if sendErr != nil {
			incDropped(uint64(chunkSize))
			pool.put(builder)
			break
		}
		sent += chunkSize
		counters.incBytesSent(uint64(len(data)), signalType)
		pool.put(builder)
		chunkStart += chunkSize
	}
	incSent(uint64(sent))
	return sent
}
