// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ringbuffer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"
)

func TestDispatcherRecoversFromHandlerPanic(t *testing.T) {
	done := make(chan []byte, 1)
	var n int
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rb := New(ctx, func(_ int, data []byte) {
		n++
		if n == 1 {
			panic("boom")
		}
		done <- append([]byte(nil), data...)
	}, nil)
	rb.queue = newByteQueue(4096)

	var wg sync.WaitGroup
	wg.Add(1)
	go rb.dispatch(&wg)

	rb.handleQueuedEvent(&ringbuf.Record{RawSample: []byte{1}}, nil, nil)
	rb.handleQueuedEvent(&ringbuf.Record{RawSample: []byte{2}}, nil, nil)

	select {
	case got := <-done:
		require.Equal(t, []byte{2}, got)
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not recover from handler panic")
	}

	require.Zero(t, rb.occupancy.Load())

	cancel()
	wg.Wait()
}

func waitForEnqueueBlocked(t *testing.T, q *byteQueue) {
	t.Helper()
	require.Eventually(t, func() bool {
		return q.waiters.Load() == 1
	}, time.Second, time.Millisecond)
}

func TestByteQueueBlocksUntilBytesFreed(t *testing.T) {
	q := newByteQueue(1000)
	t.Cleanup(q.close)
	require.True(t, q.enqueue(&ringbuf.Record{RawSample: make([]byte, 400)}))
	require.True(t, q.enqueue(&ringbuf.Record{RawSample: make([]byte, 400)}))

	unblocked := make(chan struct{})
	go func() {
		require.True(t, q.enqueue(&ringbuf.Record{RawSample: make([]byte, 400)}))
		close(unblocked)
	}()

	waitForEnqueueBlocked(t, q)
	select {
	case <-unblocked:
		t.Fatal("enqueue returned while the queue was over budget")
	default:
	}

	got, ok := q.dequeue()
	require.True(t, ok)
	require.Len(t, got.RawSample, 400)

	select {
	case <-unblocked:
	case <-time.After(time.Second):
		t.Fatal("enqueue did not proceed after bytes were freed")
	}
}

func TestByteQueueOversizedWaitsIfNotEmpty(t *testing.T) {
	q := newByteQueue(100)
	t.Cleanup(q.close)
	require.True(t, q.enqueue(&ringbuf.Record{RawSample: make([]byte, 40)}))

	unblocked := make(chan struct{})
	oversized := &ringbuf.Record{RawSample: make([]byte, 200)}
	go func() {
		require.True(t, q.enqueue(oversized))
		close(unblocked)
	}()

	waitForEnqueueBlocked(t, q)
	select {
	case <-unblocked:
		t.Fatal("oversized event was admitted while the queue was not empty")
	default:
	}

	got, ok := q.dequeue()
	require.True(t, ok)
	require.Len(t, got.RawSample, 40)

	select {
	case <-unblocked:
	case <-time.After(time.Second):
		t.Fatal("oversized event was not admitted after the queue became empty")
	}

	got, ok = q.dequeue()
	require.True(t, ok)
	require.Equal(t, oversized, got)
}

func TestByteQueueCloseUnblocksEnqueue(t *testing.T) {
	q := newByteQueue(100)
	t.Cleanup(q.close)
	require.True(t, q.enqueue(&ringbuf.Record{RawSample: make([]byte, 80)}))

	done := make(chan bool, 1)
	go func() {
		done <- q.enqueue(&ringbuf.Record{RawSample: make([]byte, 80)})
	}()

	waitForEnqueueBlocked(t, q)
	select {
	case admitted := <-done:
		t.Fatalf("enqueue returned %v while the queue was full", admitted)
	default:
	}

	q.close()

	select {
	case admitted := <-done:
		require.False(t, admitted)
	case <-time.After(time.Second):
		t.Fatal("enqueue was not unblocked by close")
	}
}
