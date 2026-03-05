// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package queue

import (
	"testing"
)

// Test the basic operations of the Queue type.
func TestQueue_EnqueueDequeue(t *testing.T) {
	q := NewQueue()

	// Queue should be empty initially
	if l := q.Len(); l != 0 {
		t.Errorf("expected initial length to be 0, got %d", l)
	}

	// Dequeue from empty queue
	if val, ok := q.Dequeue(); ok || val != 0 {
		t.Errorf("expected Dequeue from empty queue to return (0, false), got (%v, %v)", val, ok)
	}

	// Enqueue elements
	q.Enqueue(1.5)
	q.Enqueue(2.5)
	q.Enqueue(3.5)

	if l := q.Len(); l != 3 {
		t.Errorf("expected length to be 3, got %d", l)
	}

	// Peek returns first element without removal
	if val, ok := q.Peek(); !ok || val != 1.5 {
		t.Errorf("Peek failed, got (%v, %v), want (1.5, true)", val, ok)
	}

	// Dequeue should return elements in FIFO order
	tests := []float64{1.5, 2.5, 3.5}
	for i, want := range tests {
		got, ok := q.Dequeue()
		if !ok {
			t.Errorf("expected ok=true on dequeue %d", i)
		}
		if got != want {
			t.Errorf("dequeue %d: want %v, got %v", i, want, got)
		}
	}
	// Now queue is empty
	if l := q.Len(); l != 0 {
		t.Errorf("expected length to be 0 after all dequeues, got %d", l)
	}
	// Peek should report empty
	if val, ok := q.Peek(); ok || val != 0 {
		t.Errorf("expected Peek on empty queue to return (0, false), got (%v, %v)", val, ok)
	}
	// Dequeue again should also be empty
	if val, ok := q.Dequeue(); ok || val != 0 {
		t.Errorf("expected Dequeue from empty queue to return (0, false), got (%v, %v)", val, ok)
	}
}

func TestQueue_CompactAfterManyDequeues(t *testing.T) {
	q := NewQueue()
	total := 200
	for i := 0; i < total; i++ {
		q.Enqueue(float64(i))
	}

	for i := 0; i < total/2; i++ {
		got, ok := q.Dequeue()
		if !ok || int(got) != i {
			t.Errorf("Dequeue %d: want %d, got %v (ok=%v)", i, i, got, ok)
		}
	}

	// The queue should have compacted at this point
	// Remaining elements are from total/2 to total-1
	for i := total / 2; i < total; i++ {
		got, ok := q.Dequeue()
		if !ok || int(got) != i {
			t.Errorf("Post-compact Dequeue %d: want %d, got %v (ok=%v)", i, i, got, ok)
		}
	}
	if q.Len() != 0 {
		t.Errorf("expected length to be 0 after all dequeues, got %d", q.Len())
	}
}
