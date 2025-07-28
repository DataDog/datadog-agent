// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRolloutCache_SetAndGet(t *testing.T) {
	cache := newRolloutCache(1 * time.Second)

	// Test cache miss
	duration, found := cache.get("test-key")
	assert.False(t, found)
	assert.Equal(t, float64(0), duration)

	// Test cache hit
	cache.set("test-key", 120.5)
	duration, found = cache.get("test-key")
	assert.True(t, found)
	assert.Equal(t, 120.5, duration)
}

func TestRolloutCache_Expiration(t *testing.T) {
	cache := newRolloutCache(50 * time.Millisecond)

	// Set a value
	cache.set("test-key", 100.0)
	
	// Should be found immediately
	duration, found := cache.get("test-key")
	assert.True(t, found)
	assert.Equal(t, 100.0, duration)

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	duration, found = cache.get("test-key")
	assert.False(t, found)
	assert.Equal(t, float64(0), duration)
}

func TestRolloutCache_Size(t *testing.T) {
	cache := newRolloutCache(1 * time.Second)

	assert.Equal(t, 0, cache.size())

	cache.set("key1", 100.0)
	assert.Equal(t, 1, cache.size())

	cache.set("key2", 200.0)
	assert.Equal(t, 2, cache.size())

	cache.set("key1", 150.0) // Update existing key
	assert.Equal(t, 2, cache.size())
}

func TestRolloutCache_Clear(t *testing.T) {
	cache := newRolloutCache(1 * time.Second)

	cache.set("key1", 100.0)
	cache.set("key2", 200.0)
	assert.Equal(t, 2, cache.size())

	cache.clear()
	assert.Equal(t, 0, cache.size())

	// Verify entries are really gone
	_, found := cache.get("key1")
	assert.False(t, found)
}

func TestRolloutCache_Cleanup(t *testing.T) {
	cache := newRolloutCache(50 * time.Millisecond)

	// Add some entries
	cache.set("key1", 100.0)
	cache.set("key2", 200.0)
	
	// Wait for some to expire
	time.Sleep(60 * time.Millisecond)
	
	// Add a fresh entry
	cache.set("key3", 300.0)
	
	assert.Equal(t, 3, cache.size()) // Expired entries still in memory
	
	cache.cleanup()
	
	assert.Equal(t, 1, cache.size()) // Only fresh entry remains
	
	// Verify the fresh entry is still there
	duration, found := cache.get("key3")
	assert.True(t, found)
	assert.Equal(t, 300.0, duration)
}

func TestRolloutCache_ConcurrentAccess(t *testing.T) {
	cache := newRolloutCache(1 * time.Second)

	// Test concurrent writes and reads
	done := make(chan bool, 2)

	// Concurrent writer
	go func() {
		for i := 0; i < 100; i++ {
			cache.set("key", float64(i))
		}
		done <- true
	}()

	// Concurrent reader
	go func() {
		for i := 0; i < 100; i++ {
			cache.get("key")
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Should not panic and cache should be in a valid state
	assert.Equal(t, 1, cache.size())
}