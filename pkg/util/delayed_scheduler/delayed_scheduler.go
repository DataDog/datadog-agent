// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package delayedscheduler implements a scheduler that can be used to schedule the execution of a function at a given time.
// Similar to time.AfterFunc but it doesn't use more than one timer at a time.
package delayedscheduler

import (
	"container/heap"
	"sync"
	"time"
)

// zeroValueItem is a dummy item with the highest possible priority and a nil f.
// It is used to signal that no item is available in the queue to make the main loop
// of the scheduler wait for a new item to be added.
var zeroValueItem = &item{
	processAt: time.Unix(1<<63-1, 1<<63-1),
}

// item represents a value to process by the scheduler and the timestamp at which it should be processed.
type item struct {
	f         ProcessFunc
	processAt time.Time // The priority of the item in the queue
}

// PriorityQueue implements heap.Interface and holds Items.
type PriorityQueue []*item

// Len returns the length of the queue.
func (pq PriorityQueue) Len() int { return len(pq) }

// Less returns true if the item at index i has a higher priority than the item at index j.
// The priority is determined by the timestamp.
func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].processAt.Before(pq[j].processAt)
}

// Swap swaps the items at index i and j.
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

// Push adds x as an element to the end of the queue.
func (pq *PriorityQueue) Push(x interface{}) {
	item := x.(*item)
	*pq = append(*pq, item)
}

// Pop removes and returns the element at the end of the queue.
// If no element is available, it returns a dummy item with the highest possible priority
// and a nil f.
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*pq = old[0 : n-1]
	return item
}

// ProcessFunc is a function that can be scheduled and executed in the scheduler.
type ProcessFunc func()

// Scheduler is a scheduler that schedules items based on their a timestamp such that they are never processed before
// the timestamp.
type Scheduler struct {
	lock sync.Mutex
	pq   PriorityQueue

	newValChan chan struct{}
}

// NewScheduler creates a new scheduler with the given buffer size.
func NewScheduler(bufferSize int) *Scheduler {
	s := &Scheduler{
		newValChan: make(chan struct{}, bufferSize),
	}
	go s.loop()
	return s
}

// Schedule adds a f to the queue with the given priority.
// It notifies the scheduler that a new f has been added to the queue.
func (s *Scheduler) Schedule(value ProcessFunc, priority time.Time) {
	s.lock.Lock()
	heap.Push(&s.pq, &item{
		f:         value,
		processAt: priority,
	})
	s.lock.Unlock()
	s.newValChan <- struct{}{}
}

// loop waits for the next item to be ready and calls the process function
func (s *Scheduler) loop() {
	for {
		// Find the next item to be processed
		next := s.next()
		if next == nil {
			return
		}
		// We create a new timer to avoid creating too many timers (as they spawn a new goroutine)
		timer := time.NewTimer(time.Until(next.processAt))
		select {
		// Sleep until `next.Priorty` is reached and process its f.
		case <-timer.C:
			timer.Stop()
			if next != nil && next.f != nil {
				next.f()
			}
		// If a new f is added detected, update `next`.
		case _, more := <-s.newValChan:
			timer.Stop()
			if !more {
				return
			}
			s.lock.Lock()
			heap.Push(&s.pq, next)
			s.lock.Unlock()
		}
	}
}

func (s *Scheduler) next() *item {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.pq.Len() == 0 {
		// return a dummy f to make the main loop wait for a new f to be added
		return zeroValueItem
	}
	return heap.Pop(&s.pq).(*item)
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	close(s.newValChan)
}
