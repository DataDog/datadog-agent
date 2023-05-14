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

func newMockFlush[T any]() (callback func([]T), getAccumulator func() [][]T) {
	accumulator := [][]T{}
	var mutex sync.RWMutex

	callback = func(elems []T) {
		mutex.Lock()
		defer mutex.Unlock()
		accumulator = append(accumulator, elems)
	}

	getAccumulator = func() [][]T {
		mutex.RLock()
		defer mutex.RUnlock()
		return accumulator
	}

	return
}

func TestQueue(t *testing.T) {
	callback, accumulator := newMockFlush[int]()
	cl := clock.NewMock()
	queue := newQueue(3, 1*time.Minute, callback, cl)

	for i := 0; i <= 10; i++ {
		queue <- i
	}

	assert.Equal(
		t,
		accumulator(),
		[][]int{
			{0, 1, 2},
			{3, 4, 5},
			{6, 7, 8},
		},
	)

	cl.Add(2 * time.Minute)

	assert.Equal(
		t,
		accumulator(),
		[][]int{
			{0, 1, 2},
			{3, 4, 5},
			{6, 7, 8},
			{9, 10},
		},
	)

	close(queue)
}
