// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

func makeQueue[Item any, ID comparable](
	id func(Item) ID,
) queue[Item, ID] {
	return queue[Item, ID]{
		id:   id,
		list: list[Item]{},
		m:    make(map[ID]*listItem[Item]),
	}
}

// queue is a FIFO queue that offers O(1) pushBack and popFront operations,
// and remove (by ID) operations.
type queue[Item any, ID comparable] struct {
	id   func(Item) ID
	list list[Item]
	m    map[ID]*listItem[Item]
}

func (q *queue[Item, ID]) popFront() (Item, bool) {
	program, ok := q.list.popFront()
	if !ok {
		return *new(Item), false
	}
	delete(q.m, q.id(program))
	return program, true
}

func (q *queue[Item, ID]) pushBack(program Item) (prev Item, havePrev bool) {
	id := q.id(program)
	if prevListItem, ok := q.m[id]; ok {
		prev, havePrev = q.list.remove(prevListItem), true
	}
	item := q.list.pushBack(program)
	q.m[q.id(program)] = item
	return prev, havePrev
}

func (q *queue[Item, ID]) remove(k ID) (Item, bool) {
	item, ok := q.m[k]
	if !ok {
		return *new(Item), false
	}
	delete(q.m, k)
	return q.list.remove(item), true
}

func (q *queue[Item, ID]) len() int {
	return len(q.m)
}

// list is a circular doubly-linked list.
//
// Sure, with more allocations and it's painful API, this could just be replaced
// with container/list.
type list[T any] struct {
	head *listItem[T]
}

type listItem[T any] struct {
	value      T
	next, prev *listItem[T]
}

func (l *list[T]) pushBack(value T) *listItem[T] {
	item := &listItem[T]{value: value}
	if l.head == nil {
		l.head = item
		item.next = item
		item.prev = item
	} else {
		tail := l.head.prev
		tail.next = item
		item.prev = tail
		item.next = l.head
		l.head.prev = item
	}
	return item
}

func (l *list[T]) popFront() (T, bool) {
	if l.head == nil {
		return *new(T), false
	}
	return l.remove(l.head), true
}

func (l *list[T]) remove(item *listItem[T]) T {
	if item.next == item { // only item
		l.head = nil
		item.next = nil
		item.prev = nil
	} else {
		if l.head == item {
			l.head = item.next
		}
		next := item.next
		prev := item.prev
		next.prev = prev
		prev.next = next
	}
	return item.value
}
