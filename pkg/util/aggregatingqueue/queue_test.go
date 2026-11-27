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
		// The callback runs on the queue goroutine. Handing the batches over a
		// channel, rather than a shared slice, gives the race detector a
		// happens-before edge in both directions.
		flushed := make(chan []int, 10)
		queue := NewQueue(3, 1*time.Minute, func(elems []int) {
			flushed <- elems
		})

		// receiveFlushed returns the batches flushed since the last call, once the
		// queue goroutine has processed everything it was given.
		receiveFlushed := func() [][]int {
			synctest.Wait()
			var batches [][]int
			for {
				select {
				case batch := <-flushed:
					batches = append(batches, batch)
				default:
					return batches
				}
			}
		}

		for i := 0; i <= 10; i++ {
			queue <- i
		}
		assert.Equal(t, [][]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}}, receiveFlushed())

		// Elements 9 and 10 are flushed once the retention time has elapsed.
		time.Sleep(2 * time.Minute)
		assert.Equal(t, [][]int{{9, 10}}, receiveFlushed())

		close(queue)
	})
}
