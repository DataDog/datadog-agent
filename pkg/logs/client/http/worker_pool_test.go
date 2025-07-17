// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
)

var defaultMinWorkers = 4
var defaultMaxWorkers = defaultMinWorkers * 10

func defaultPool() *workerPool {
	return newWorkerPool(0, ewmaAlpha, defaultMinWorkers, defaultMaxWorkers, targetLatency, client.NewNoopDestinationMetadata())
}

func TestRetryableError(t *testing.T) {
	pool := defaultPool()
	for i := 0; i < 100; i++ {
		pool.run(func() destinationResult {
			return destinationResult{latency: targetLatency * 2}
		})
	}

	require.Equal(t, defaultMinWorkers*2, pool.inUseWorkers)

	pool.run(func() destinationResult {
		return destinationResult{latency: targetLatency * 2, err: client.NewRetryableError(errors.New(""))}
	})

	start := time.Now()

	pool.Lock()
	backoff := pool.shouldBackoff
	pool.Unlock()
	for !backoff && time.Since(start) < 500*time.Millisecond {
		time.Sleep(time.Millisecond)
		pool.Lock()
		backoff = pool.shouldBackoff
		pool.Unlock()
	}

	assert.Equal(t, true, pool.shouldBackoff)

	// The next pool run will detect the backoff flag to
	// 1. Reduce the worker load to the minimum
	// 2. Reset the tracked latency to force a gradual increase.
	pool.run(func() destinationResult {
		return destinationResult{latency: targetLatency * 2}
	})

	assert.Equal(t, defaultMinWorkers, pool.inUseWorkers)
	assert.Equal(t, time.Duration(0), pool.virtualLatency)

	// Confirm that we do in fact recover over time
	for i := 0; i < 100; i++ {
		pool.run(func() destinationResult {
			return destinationResult{latency: targetLatency * 2}
		})
	}

	require.Equal(t, defaultMinWorkers*2, pool.inUseWorkers)
}

func TestNonRetryableError(t *testing.T) {
	pool := defaultPool()
	for i := 0; i < 100; i++ {
		pool.run(func() destinationResult {
			return destinationResult{latency: targetLatency * 2}
		})
	}

	require.Equal(t, defaultMinWorkers*2, pool.inUseWorkers)
	pool.run(func() destinationResult {
		return destinationResult{latency: targetLatency * 2, err: errors.New("")}
	})
	pool.resizeUnsafe()
	pool.resizeUnsafe()
	assert.Equal(t, defaultMinWorkers*2, pool.inUseWorkers)
}

func TestWorkerCounts(t *testing.T) {
	scenarios := []struct {
		name                string
		latency             time.Duration
		expectedWorkerCount int
	}{
		{
			name:                "Mininum Workers chosen if latency below target",
			latency:             0,
			expectedWorkerCount: defaultMinWorkers,
		},
		{
			name:                "Reasonable number of workers added at higher than target latency",
			latency:             targetLatency * 2,
			expectedWorkerCount: defaultMinWorkers * 2,
		},
		{
			name:                "Maximum number of workers not exceeded",
			latency:             targetLatency * 20,
			expectedWorkerCount: defaultMaxWorkers,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			pool := defaultPool()

			for i := 0; i < 500; i++ {
				pool.run(func() destinationResult {
					return destinationResult{latency: s.latency}
				})
			}
			time.Sleep(10 * time.Millisecond)

			assert.Equal(t, s.expectedWorkerCount, pool.inUseWorkers)
			assert.Equal(t, s.expectedWorkerCount, len(pool.pool))
			assert.InDelta(t, s.latency, pool.virtualLatency, float64(time.Millisecond))
		})
	}
}
