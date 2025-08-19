// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package queue

import (
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
)

func newMockFlush[T any]() (callback func([]T), waitCallbackCalled func(int), getAccumulator func() [][]T) {
	accumulator := [][]T{}
	nbCalled := 0
	var mutex sync.Mutex
	cond := sync.NewCond(&mutex)

	callback = func(elems []T) {
		mutex.Lock()
		defer mutex.Unlock()
		accumulator = append(accumulator, elems)
		nbCalled++
		cond.Signal()
	}

	waitCallbackCalled = func(nb int) {
		mutex.Lock()
		defer mutex.Unlock()
		for nbCalled < nb {
			cond.Wait()
		}
	}

	getAccumulator = func() [][]T {
		mutex.Lock()
		defer mutex.Unlock()
		return accumulator
	}

	return
}

func TestQueue(t *testing.T) {
	callback, wait, accumulator := newMockFlush[int]()
	cl := clock.NewMock()
	queue := newQueue(3, 1*time.Minute, callback, cl)

	for i := 0; i <= 10; i++ {
		queue <- i
	}

	wait(3) // wait callback to have been called 3 times

	assert.Equal(
		t,
		[][]int{
			{0, 1, 2},
			{3, 4, 5},
			{6, 7, 8},
		},
		accumulator(),
	)

	cl.Add(2 * time.Minute)

	wait(4) // wait callback to have been called one more time

	assert.Equal(
		t,
		[][]int{
			{0, 1, 2},
			{3, 4, 5},
			{6, 7, 8},
			{9, 10},
		},
		accumulator(),
	)

	close(queue)
}
