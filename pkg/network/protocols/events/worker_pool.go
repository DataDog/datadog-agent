// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"fmt"
	"sync"
)

type workerPool struct {
	size      int
	jobs      chan func()
	waitGroup sync.WaitGroup
	once      sync.Once
}

func newWorkerPool(size int) (*workerPool, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid worker pool size %d", size)
	}

	pool := &workerPool{
		size: size,
		jobs: make(chan func()),
	}

	for i := 0; i < size; i++ {
		pool.waitGroup.Add(1)
		go func() {
			defer pool.waitGroup.Done()
			for f := range pool.jobs {
				f()
			}
		}()
	}

	return pool, nil
}

func (wp *workerPool) Do(f func()) {
	wp.jobs <- f
}

func (wp *workerPool) Stop() {
	wp.once.Do(func() {
		close(wp.jobs)
		wp.waitGroup.Wait()
	})
}
