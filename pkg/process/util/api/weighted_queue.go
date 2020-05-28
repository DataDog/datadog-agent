// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"container/list"
	"sync"
)

// WeightedItem is an item that has a type and weight that can be added to a WeightedQueue
type WeightedItem interface {
	// Weight returns the weight of this item
	Weight() int64
	// Type returns the type of this item
	Type() string
}

// WeightedQueue is a queue of WeightedItems.
//
// The queue is configured with a maximum size (the maximum number of elements allowed in the queue) as well as
// a maximum weight.  If adding an item to the queue would violate either the max weight or max size, earlier items
// are purged from the queue until there is room for the newest item.
//
// Items added to the queue have a weight and type.  When purging existing items to make room for new, items of the same
// type being added will be removed first before moving on to other types.
type WeightedQueue struct {
	// dataAvailable is a channel used for communication between callers invoking the Add method and callers
	// blocked on Poll.  If a caller calls Poll and the queue is empty, the call will block waiting for a send
	// on dataAvailable.  When Add is invoked, it will perform a non-blocking send on dataAvailable to notify the caller
	// blocked on Poll
	dataAvailable chan struct{}

	// Guards the mutable internal state for the queue
	mu sync.Mutex
	// guarded by: mu
	queue *list.List
	// guarded by: mu
	currentWeight int64

	maxSize   int
	maxWeight int64
}

// NewWeightedQueue returns a new WeightedQueue with the given maximum size & weight
func NewWeightedQueue(maxSize int, maxWeight int64) *WeightedQueue {
	return &WeightedQueue{
		dataAvailable: make(chan struct{}),
		queue:         list.New(),
		maxSize:       maxSize,
		maxWeight:     maxWeight,
		currentWeight: 0,
	}
}

// Len returns the number of items in the queue
func (q *WeightedQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queue.Len()
}

// Weight returns the current weight of the queue
func (q *WeightedQueue) Weight() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.currentWeight
}

// Poll retrieves the head of the queue or blocks until an item is available.  The provided exit channel can be closed
// to interrupt the blocking operation.  Returns the head of the queue and true or nil, false if the poll was
// interrupted by the closing of the exit channel
func (q *WeightedQueue) Poll(exit chan struct{}) (WeightedItem, bool) {
	for {
		select {
		case <-exit:
			return nil, false
		default:

		}

		q.mu.Lock()

		// If the queue is empty, wait for a signal on the dataAvailable channel
		if q.queue.Len() == 0 {
			q.mu.Unlock()

			select {
			case <-q.dataAvailable:
				continue
			case <-exit:
				return nil, false
			}
		}

		e := q.queue.Front()
		item := e.Value.(WeightedItem)
		q.queue.Remove(e)
		q.currentWeight -= item.Weight()

		q.mu.Unlock()

		return item, true
	}
}

// Add adds the item to the queue.
func (q *WeightedQueue) Add(item WeightedItem) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// If the item won't fit, don't even bother trying
	if item.Weight() > q.maxWeight {
		return
	}

	q.currentWeight += item.Weight()

	if q.currentWeight > q.maxWeight {
		// Try to find an item of the same type that we can expire
		for iter := q.iterator(); iter.hasNext(); iter.next() {
			if v := iter.value(); v.Type() == item.Type() {
				iter.remove()
				q.currentWeight -= v.Weight()
				if q.currentWeight <= q.maxWeight {
					break
				}
			}
		}

		// If we didn't find enough free weight removing similar items, start purging the earliest items
		// until there is room
		if q.currentWeight > q.maxWeight {
			for iter := q.iterator(); iter.hasNext(); iter.next() {
				v := iter.value()
				iter.remove()
				q.currentWeight -= v.Weight()
				if q.currentWeight <= q.maxWeight {
					break
				}
			}
		}
	}

	// If the queue is full, expire a single item to make room
	if q.queue.Len() == q.maxSize {
		// Try to find an item of the same type that we can expire
		removed := false
		for iter := q.iterator(); iter.hasNext(); iter.next() {
			if v := iter.value(); v.Type() == item.Type() {
				iter.remove()
				q.currentWeight -= v.Weight()
				removed = true
				break
			}
		}

		// No similar items, remove the oldest element from the queue
		if !removed {
			e := q.queue.Front()
			v := e.Value.(WeightedItem)
			q.currentWeight -= v.Weight()
			q.queue.Remove(e)
		}
	}

	q.queue.PushBack(item)

	// Send a signal on the dataAvailable channel if needed
	select {
	case q.dataAvailable <- struct{}{}:
	default:

	}
}

func (q *WeightedQueue) iterator() *iterator {
	return &iterator{
		queue:   q.queue,
		current: q.queue.Front(),
	}
}

type iterator struct {
	queue    *list.List
	current  *list.Element
	advanced bool
}

func (iter *iterator) next() {
	if !iter.advanced {
		iter.current = iter.current.Next()
	}
	iter.advanced = false
}

func (iter *iterator) hasNext() bool {
	return iter.current != nil
}

func (iter *iterator) remove() {
	toRemove := iter.current
	iter.current = iter.current.Next()
	iter.queue.Remove(toRemove)
	iter.advanced = true // Set the advanced flag so that next doesn't move it forward again
}

func (iter *iterator) value() WeightedItem {
	return iter.current.Value.(WeightedItem)
}
