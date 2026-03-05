// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package queue

import (
	"testing"
)

func TestQueue_EnqueueDequeue(t *testing.T) {
	q := NewQueue[float64]()

	if l := q.Len(); l != 0 {
		t.Errorf("expected initial length to be 0, got %d", l)
	}

	// Dequeue from empty queue
	if val, ok := q.Dequeue(); ok || val != 0 {
		t.Errorf("expected Dequeue from empty queue to return (0, false), got (%v, %v)", val, ok)
	}

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

	if l := q.Len(); l != 0 {
		t.Errorf("expected length to be 0 after all dequeues, got %d", l)
	}
	if val, ok := q.Peek(); ok || val != 0 {
		t.Errorf("expected Peek on empty queue to return (0, false), got (%v, %v)", val, ok)
	}
	if val, ok := q.Dequeue(); ok || val != 0 {
		t.Errorf("expected Dequeue from empty queue to return (0, false), got (%v, %v)", val, ok)
	}
}

func TestQueue_CompactAfterManyDequeues(t *testing.T) {
	q := NewQueue[float64]()
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

func TestQueue_String(t *testing.T) {
	q := NewQueue[string]()

	if val, ok := q.Dequeue(); ok || val != "" {
		t.Errorf("expected Dequeue from empty queue to return (\"\", false), got (%v, %v)", val, ok)
	}

	q.Enqueue("foo")
	q.Enqueue("bar")
	q.Enqueue("baz")

	if l := q.Len(); l != 3 {
		t.Errorf("expected length 3, got %d", l)
	}
	if val, ok := q.Peek(); !ok || val != "foo" {
		t.Errorf("Peek: want (foo, true), got (%v, %v)", val, ok)
	}

	expected := []string{"foo", "bar", "baz"}
	for i, want := range expected {
		got, ok := q.Dequeue()
		if !ok || got != want {
			t.Errorf("dequeue %d: want %v, got %v (ok=%v)", i, want, got, ok)
		}
	}
	if q.Len() != 0 {
		t.Errorf("expected empty queue, got length %d", q.Len())
	}
}

func TestFloat64Queue_Alias(t *testing.T) {
	var q Float64Queue
	q.Enqueue(3.14)
	got, ok := q.Dequeue()
	if !ok || got != 3.14 {
		t.Errorf("Float64Queue: want (3.14, true), got (%v, %v)", got, ok)
	}
}
