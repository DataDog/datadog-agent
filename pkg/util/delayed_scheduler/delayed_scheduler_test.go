// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package delayedscheduler

import (
	"container/heap"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPriorityQueue_Len(t *testing.T) {
	var pq PriorityQueue
	pq = append(pq, &item{
		processAt: time.Now(),
	})
	assert.Len(t, pq, 1)
}

func TestPriorityQueue_Less(t *testing.T) {
	now := time.Now()
	item1 := &item{processAt: now}
	item2 := &item{processAt: now.Add(time.Minute)}

	pq := PriorityQueue{item1, item2}
	assert.True(t, pq.Less(0, 1))
	assert.False(t, pq.Less(1, 0))
}

func TestPriorityQueue_Swap(t *testing.T) {
	var v string
	item1 := &item{f: func() { v = "first" }}
	item2 := &item{f: func() { v = "second" }}

	pq := PriorityQueue{item1, item2}
	pq.Swap(0, 1)

	pq[0].f()
	assert.Equal(t, "second", v)
	pq[1].f()
	assert.Equal(t, "first", v)
}

func TestPriorityQueue_PushAndPop(t *testing.T) {
	var v string
	var pq PriorityQueue
	i := &item{f: func() { v = "test" }, processAt: time.Now()}
	heap.Push(&pq, i)
	assert.Len(t, pq, 1)
	popped := heap.Pop(&pq).(*item)
	popped.f()
	assert.Equal(t, "test", v)
}

func TestScheduler_Schedule(t *testing.T) {
	var wg sync.WaitGroup
	var v string
	wg.Add(1)
	scheduler := NewScheduler(1)
	scheduler.Schedule(func() { v = "ok"; wg.Done() }, time.Now().Add(-time.Second)) // schedule in the past to trigger immediately
	wg.Wait()
	assert.Equal(t, "ok", v)
	scheduler.Stop()
}

func TestScheduler_Schedule_Multiple(t *testing.T) {
	ch := make(chan string)

	scheduler := NewScheduler(1)
	scheduler.Schedule(func() { ch <- "task-2s" }, time.Now().Add(2*time.Second))
	scheduler.Schedule(func() { ch <- "task-1s" }, time.Now().Add(1*time.Second))
	// task-1s should be processed first
	assert.Equal(t, "task-1s", <-ch)
	assert.Equal(t, "task-2s", <-ch)

	// no other task should be processed
	select {
	case <-ch:
		t.Error("unexpected task processed")
	case <-time.After(1 * time.Second):
	}
	scheduler.Stop()
}
