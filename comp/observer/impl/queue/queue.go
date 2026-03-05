// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package queue

// Float64 queue
type Queue struct {
	data []float64
	head int
}

// NewQueue creates a new Queue.
func NewQueue() *Queue {
	return &Queue{
		data: make([]float64, 0),
	}
}

// Enqueue adds a value at the end of the queue.
func (q *Queue) Enqueue(value float64) {
	q.data = append(q.data, value)
}

// Dequeue removes and returns the value at the front of the queue.
// Returns false if the queue is empty.
func (q *Queue) Dequeue() (float64, bool) {
	if q.head >= len(q.data) {
		return 0, false
	}
	val := q.data[q.head]
	q.data[q.head] = 0 // release reference
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
func (q *Queue) Len() int {
	return len(q.data) - q.head
}

// Peek returns the value at the front of the queue without removing it.
// Returns false if the queue is empty.
func (q *Queue) Peek() (float64, bool) {
	if q.head >= len(q.data) {
		return 0, false
	}
	return q.data[q.head], true
}

func (q *Queue) Slice() []float64 {
	return q.data[q.head:]
}
