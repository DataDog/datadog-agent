// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricSamplePool(t *testing.T) {
	batchSize := 10

	t.Run("without telemetry", func(t *testing.T) {
		pool := NewMetricSamplePool(batchSize, false)
		require.NotNil(t, pool)
		assert.False(t, pool.tlmEnabled)
	})

	t.Run("with telemetry", func(t *testing.T) {
		pool := NewMetricSamplePool(batchSize, true)
		require.NotNil(t, pool)
		assert.True(t, pool.tlmEnabled)
	})
}

func TestMetricSamplePoolGetBatch(t *testing.T) {
	batchSize := 10

	t.Run("returns batch of correct size", func(t *testing.T) {
		pool := NewMetricSamplePool(batchSize, false)
		batch := pool.GetBatch()
		require.NotNil(t, batch)
		assert.Len(t, batch, batchSize)
	})

	t.Run("nil pool returns nil", func(t *testing.T) {
		var pool *MetricSamplePool
		batch := pool.GetBatch()
		assert.Nil(t, batch)
	})

	t.Run("with telemetry enabled", func(t *testing.T) {
		pool := NewMetricSamplePool(batchSize, true)
		batch := pool.GetBatch()
		require.NotNil(t, batch)
		assert.Len(t, batch, batchSize)
	})
}

func TestMetricSamplePoolPutBatch(t *testing.T) {
	batchSize := 10

	t.Run("put and get batch", func(t *testing.T) {
		pool := NewMetricSamplePool(batchSize, false)

		// Get a batch
		batch := pool.GetBatch()
		require.NotNil(t, batch)

		// Put it back
		pool.PutBatch(batch)

		// Get another batch - should reuse the pool
		batch2 := pool.GetBatch()
		require.NotNil(t, batch2)
		assert.Len(t, batch2, batchSize)
	})

	t.Run("nil pool does not panic", func(t *testing.T) {
		var pool *MetricSamplePool
		batch := make(MetricSampleBatch, batchSize)
		// Should not panic
		pool.PutBatch(batch)
	})

	t.Run("with telemetry enabled", func(t *testing.T) {
		pool := NewMetricSamplePool(batchSize, true)
		batch := pool.GetBatch()
		require.NotNil(t, batch)
		pool.PutBatch(batch)
	})
}

func TestMetricSamplePoolConcurrent(t *testing.T) {
	batchSize := 10
	pool := NewMetricSamplePool(batchSize, false)

	done := make(chan bool, 10)

	// Concurrent get and put
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				batch := pool.GetBatch()
				pool.PutBatch(batch)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
