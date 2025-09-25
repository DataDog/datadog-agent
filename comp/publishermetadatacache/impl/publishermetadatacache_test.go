//go:build windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package publishermetadatacacheimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	fakeevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/fake"
)

// newTestCache creates a new publishermetadatacache for testing purposes
func newTestCache(api evtapi.API, maxCacheSize int) *publisherMetadataCache {
	return &publisherMetadataCache{
		cache:        make(map[string]cacheItem),
		evtapi:       api,
		maxCacheSize: maxCacheSize,
	}
}

func TestPublisherMetadataCache_Get_CacheMiss(t *testing.T) {
	cache := newTestCache(fakeevtapi.New(), 2)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)

	handle, err := cache.Get(publisherName, eventHandle)

	assert.NoError(t, err)
	assert.NotEqual(t, evtapi.EventPublisherMetadataHandle(0), handle)

	assert.Contains(t, cache.cache, publisherName)
	assert.Equal(t, handle, cache.cache[publisherName].handle)
}

func TestPublisherMetadataCache_Get_CacheHit(t *testing.T) {
	cache := newTestCache(fakeevtapi.New(), 2)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)

	cachedHandle, err := cache.Get(publisherName, eventHandle)
	assert.NoError(t, err)

	oldTimestamp := cache.cache[publisherName].timestamp

	// Small delay to ensure timestamp difference is measurable
	time.Sleep(1 * time.Millisecond)

	handle, err := cache.Get(publisherName, eventHandle)

	assert.NoError(t, err)
	assert.Equal(t, cachedHandle, handle)
	assert.True(t, cache.cache[publisherName].timestamp.After(oldTimestamp))
}

func TestPublisherMetadataCache_isMetadataHandleValid(t *testing.T) {
	fakeAPI := fakeevtapi.New()
	cache := newTestCache(fakeAPI, 2)

	handle, err := cache.Get("TestPublisher", evtapi.EventRecordHandle(100))
	assert.NoError(t, err)

	assert.True(t, cache.isMetadataHandleValid(handle))

	err = fakeAPI.InvalidatePublisherHandle(handle)
	assert.NoError(t, err)

	assert.False(t, cache.isMetadataHandleValid(handle))
}

func TestPublisherMetadataCache_Get_InvalidHandle_RecreatesCache(t *testing.T) {
	fakeAPI := fakeevtapi.New()
	cache := newTestCache(fakeAPI, 2)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)

	originalHandle, err := cache.Get(publisherName, eventHandle)
	assert.NoError(t, err)

	err = fakeAPI.InvalidatePublisherHandle(originalHandle)
	assert.NoError(t, err)

	newHandle, err := cache.Get(publisherName, eventHandle)

	assert.NoError(t, err)
	assert.NotEqual(t, originalHandle, newHandle)
	assert.NotEqual(t, evtapi.EventPublisherMetadataHandle(0), newHandle)

	assert.Contains(t, cache.cache, publisherName)
	assert.Equal(t, newHandle, cache.cache[publisherName].handle)
}

func TestPublisherMetadataCache_CacheEviction(t *testing.T) {
	cache := newTestCache(fakeevtapi.New(), 2)

	// Fill cache to max capacity (2)
	Publisher1 := "Publisher1"
	Publisher2 := "Publisher2"
	_, err := cache.Get(Publisher1, evtapi.EventRecordHandle(100))
	assert.NoError(t, err)

	time.Sleep(1 * time.Millisecond)

	_, err = cache.Get(Publisher2, evtapi.EventRecordHandle(200))
	assert.NoError(t, err)

	assert.Len(t, cache.cache, 2)

	// Add third publisher, should evict Publisher1 (oldest)
	Publisher3 := "Publisher3"
	_, err = cache.Get(Publisher3, evtapi.EventRecordHandle(300))
	assert.NoError(t, err)

	assert.NotContains(t, cache.cache, Publisher1) // Oldest should be evicted
	assert.Contains(t, cache.cache, Publisher2)
	assert.Contains(t, cache.cache, Publisher3)
	assert.Len(t, cache.cache, 2)
}

func TestPublisherMetadataCache_Stop_CleansUpAllHandles(t *testing.T) {
	cache := newTestCache(fakeevtapi.New(), 2)

	_, err := cache.Get("Publisher1", evtapi.EventRecordHandle(100))
	assert.NoError(t, err)
	_, err = cache.Get("Publisher2", evtapi.EventRecordHandle(200))
	assert.NoError(t, err)

	assert.Len(t, cache.cache, 2)

	err = cache.stop()
	assert.NoError(t, err)
	assert.Empty(t, cache.cache)
}
