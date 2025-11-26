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

var defaultDoWork = func() destinationResult {
	return destinationResult{latency: targetLatency * 2}
}

func runXWorkers(pool *workerPool, x int, doWork func() destinationResult) {
	for i := 0; i < x; i++ {
		<-pool.pool
		pool.performWork(doWork)
		pool.resizeUnsafe()
	}
}

func TestRetryableError(t *testing.T) {
	pool := defaultPool()
	runXWorkers(pool, 100, defaultDoWork)
	require.Equal(t, defaultMinWorkers*2, pool.inUseWorkers)

	retryWork := func() destinationResult {
		return destinationResult{latency: targetLatency * 2, err: client.NewRetryableError(errors.New(""))}
	}

	// The next pool run will detect the backoff flag to
	// 1. Reduce the worker load to the minimum
	// 2. Reset the tracked latency to force a gradual increase.
	runXWorkers(pool, 1, retryWork)

	assert.Equal(t, defaultMinWorkers, pool.inUseWorkers)
	assert.Equal(t, time.Duration(0), pool.virtualLatency)

	// Confirm that we do in fact recover over time
	runXWorkers(pool, 100, defaultDoWork)
	assert.Equal(t, defaultMinWorkers*2, pool.inUseWorkers)
}

func TestNonRetryableError(t *testing.T) {
	pool := defaultPool()
	runXWorkers(pool, 100, defaultDoWork)
	assert.Equal(t, defaultMinWorkers*2, pool.inUseWorkers)

	nonRetryableWork := func() destinationResult {
		return destinationResult{latency: targetLatency * 2, err: errors.New("")}
	}
	runXWorkers(pool, 1, nonRetryableWork)
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
			runXWorkers(pool, 500, func() destinationResult {
				return destinationResult{latency: s.latency}
			})
			assert.Equal(t, s.expectedWorkerCount, pool.inUseWorkers)
			assert.Equal(t, s.expectedWorkerCount, len(pool.pool))
			assert.InDelta(t, s.latency, pool.virtualLatency, float64(s.latency)/100)
		})
	}
}
