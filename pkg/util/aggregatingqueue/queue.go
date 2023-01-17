// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package queue

import (
	"time"
)

type queue[T any] struct {
	maxNbItem        int
	maxRetentionTime time.Duration
	flushCB          func([]T)
	enqueueCh        chan T
	data             []T
	timer            *time.Timer
}

// NewQueue returns a chan to enqueue elements
// The flushCB function will be called with a slice of elements as soon as
// * either maxNbItem elements have been enqueued since the last flush
// * or maxRetentionTime has elapsed since the first element has been enqueued after the last flush.
func NewQueue[T any](maxNbItem int, maxRetentionTime time.Duration, flushCB func([]T)) chan T {
	q := queue[T]{
		maxNbItem:        maxNbItem,
		maxRetentionTime: maxRetentionTime,
		flushCB:          flushCB,
		enqueueCh:        make(chan T),
		data:             make([]T, 0, maxNbItem),
		timer:            time.NewTimer(maxRetentionTime),
	}

	if !q.timer.Stop() {
		<-q.timer.C
	}

	go func() {
		for {
			select {
			case <-q.timer.C:
				q.flush()
			case elem, more := <-q.enqueueCh:
				if !more {
					return
				}
				q.enqueue(elem)
			}
		}
	}()

	return q.enqueueCh
}

func (q *queue[T]) enqueue(elem T) {
	if len(q.data) == 0 {
		q.timer.Reset(q.maxRetentionTime)
	}

	q.data = append(q.data, elem)

	if len(q.data) == q.maxNbItem {
		q.flush()
	}
}

func (q *queue[T]) flush() {
	q.timer.Stop()
	q.flushCB(q.data)
	q.data = make([]T, 0, q.maxNbItem)
}
