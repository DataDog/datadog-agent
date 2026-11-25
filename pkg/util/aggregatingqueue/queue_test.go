// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package queue

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The callback is invoked from the queue goroutine. Reading the
		// accumulator after synctest.Wait() is race-free: Wait() guarantees
		// that the queue goroutine is durably blocked.
		var accumulator [][]int
		queue := NewQueue(3, 1*time.Minute, func(elems []int) {
			accumulator = append(accumulator, elems)
		})

		for i := 0; i <= 10; i++ {
			queue <- i
		}
		synctest.Wait()

		want := [][]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}}
		assert.Equal(t, want, accumulator)

		// Elements 9 and 10 are flushed once the retention time has elapsed.
		time.Sleep(2 * time.Minute)
		synctest.Wait()

		want = append(want, []int{9, 10})
		assert.Equal(t, want, accumulator)

		close(queue)
	})
}
