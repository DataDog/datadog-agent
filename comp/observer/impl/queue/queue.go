// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package queue

// Queue is a generic FIFO queue backed by a slice.
type Queue[T any] struct {
	data []T
	head int
}

// Float64Queue is a queue of float64 values.
type Float64Queue = Queue[float64]

// NewQueue creates a new Queue.
func NewQueue[T any]() *Queue[T] {
	return &Queue[T]{
		data: make([]T, 0),
	}
}

// Enqueue adds a value at the end of the queue.
func (q *Queue[T]) Enqueue(value T) {
	q.data = append(q.data, value)
}

// Dequeue removes and returns the value at the front of the queue.
// Returns false if the queue is empty.
func (q *Queue[T]) Dequeue() (T, bool) {
	var zero T
	if q.head >= len(q.data) {
		return zero, false
	}
	val := q.data[q.head]
	q.data[q.head] = zero // release reference
	q.head++

	// Compact once at least half the capacity sits behind the head pointer.
	if q.head > cap(q.data)/2 {
		n := copy(q.data, q.data[q.head:])
		q.data = q.data[:n]
		q.head = 0
	}
	return val, true
}

// Len returns the number of elements in the queue.
func (q *Queue[T]) Len() int {
	return len(q.data) - q.head
}

// Peek returns the value at the front of the queue without removing it.
// Returns false if the queue is empty.
func (q *Queue[T]) Peek() (T, bool) {
	var zero T
	if q.head >= len(q.data) {
		return zero, false
	}
	return q.data[q.head], true
}

func (q *Queue[T]) Flush() {
	q.head = 0
	q.data = q.data[:q.head]
}

// Slice returns the elements of the queue as a slice.
func (q *Queue[T]) Slice() []T {
	return q.data[q.head:]
}
