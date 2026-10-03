// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ringbuffer

import (
	"sync"
	"sync/atomic"

	"github.com/cilium/ebpf/ringbuf"
)

type byteQueue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	buf      []*ringbuf.Record
	head     int
	count    int
	bytes    int64
	maxBytes int64
	closed   bool
	waiters  atomic.Int32
}

func newByteQueue(maxBytes int64) *byteQueue {
	if maxBytes < 0 {
		maxBytes = 0
	}
	q := &byteQueue{
		buf:      make([]*ringbuf.Record, 64),
		maxBytes: maxBytes,
	}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

func (q *byteQueue) canAdmitLocked(size int64) bool {
	// A single event larger than the budget would otherwise wait forever.
	if q.count == 0 && size > q.maxBytes {
		return true
	}
	return q.bytes+size <= q.maxBytes
}

func (q *byteQueue) enqueue(rec *ringbuf.Record) bool {
	return q.enqueueAndPublish(rec, nil)
}

func (q *byteQueue) enqueueAndPublish(rec *ringbuf.Record, afterPublish func(int64)) bool {
	size := int64(len(rec.RawSample))
	q.mu.Lock()
	defer q.mu.Unlock()
	for !q.closed && !q.canAdmitLocked(size) {
		q.waiters.Add(1)
		q.notFull.Wait()
		q.waiters.Add(-1)
	}
	if q.closed {
		return false
	}
	q.pushLocked(rec)
	q.bytes += size
	if afterPublish != nil {
		afterPublish(size)
	}
	q.notEmpty.Signal()
	return true
}

func (q *byteQueue) dequeue() (*ringbuf.Record, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.count == 0 && !q.closed {
		q.notEmpty.Wait()
	}
	if q.count == 0 {
		return nil, false
	}
	rec := q.popLocked()
	q.bytes -= int64(len(rec.RawSample))
	q.notFull.Signal()
	return rec, true
}

func (q *byteQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}

func (q *byteQueue) capacity() int64 {
	return q.maxBytes
}

func (q *byteQueue) pushLocked(rec *ringbuf.Record) {
	if q.count == len(q.buf) {
		q.growLocked()
	}
	q.buf[(q.head+q.count)%len(q.buf)] = rec
	q.count++
}

func (q *byteQueue) popLocked() *ringbuf.Record {
	rec := q.buf[q.head]
	q.buf[q.head] = nil
	q.head = (q.head + 1) % len(q.buf)
	q.count--
	return rec
}

func (q *byteQueue) growLocked() {
	n := len(q.buf) * 2
	if n == 0 {
		n = 64
	}
	next := make([]*ringbuf.Record, n)
	for i := 0; i < q.count; i++ {
		next[i] = q.buf[(q.head+i)%len(q.buf)]
	}
	q.buf = next
	q.head = 0
}
