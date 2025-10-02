// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package publishermetadatacacheimpl

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	publishermetadatacache "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/publishermetadatacache"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	fakeevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/fake"
)

func TestPublisherMetadataCache_Get(t *testing.T) {
	cache := publishermetadatacache.New(fakeevtapi.New())

	publisherName1 := "Publisher1"
	publisherName2 := "Publisher2"

	handle1, err := cache.Get(publisherName1)
	assert.NoError(t, err)
	handle2, err := cache.Get(publisherName2)
	assert.NoError(t, err)

	assert.NotEqual(t, publishermetadatacache.InvalidHandle, handle1)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, handle2)

	// Verify item is in cache
	cachedValue, found := cache.GetCache().Load(publisherName1)
	assert.True(t, found)
	assert.Equal(t, handle1, cachedValue.(publishermetadatacache.CacheEntry).Handle)

	// Verify item is in cache
	cachedValue, found = cache.GetCache().Load(publisherName2)
	assert.True(t, found)
	assert.Equal(t, handle2, cachedValue.(publishermetadatacache.CacheEntry).Handle)
}

func TestPublisherMetadataCache_FormatMessage_InvalidHandle_RecreatesCache(t *testing.T) {
	fakeAPI := fakeevtapi.New()
	cache := publishermetadatacache.New(fakeAPI)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)

	originalHandle, err := cache.Get(publisherName)
	assert.NoError(t, err)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, originalHandle)

	// Invalidate the handle to simulate provider being uninstalled
	err = fakeAPI.InvalidatePublisherHandle(originalHandle)
	assert.NoError(t, err)

	// FormatMessage should detect invalid handle and remove from cache
	message, err := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.Empty(t, message)
	assert.Contains(t, err.Error(), "not implemented") // Fake API returns error

	// Verify cache entry was removed
	_, found := cache.GetCache().Load(publisherName)
	assert.False(t, found)

	// Next Get call should create a new handle
	newHandle, err := cache.Get(publisherName)
	assert.NoError(t, err)
	assert.NotEqual(t, originalHandle, newHandle)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, newHandle)
}

func TestPublisherMetadataCache_Close_CleansUpAllHandles(t *testing.T) {
	cache := publishermetadatacache.New(fakeevtapi.New())

	cache.Get("Publisher1")
	cache.Get("Publisher2")

	// Verify items are in cache before closing
	_, found1 := cache.GetCache().Load("Publisher1")
	assert.True(t, found1)
	_, found2 := cache.GetCache().Load("Publisher2")
	assert.True(t, found2)

	cache.Flush()

	// Verify cache is empty after close
	_, found1 = cache.GetCache().Load("Publisher1")
	assert.False(t, found1)
	_, found2 = cache.GetCache().Load("Publisher2")
	assert.False(t, found2)
}

func TestPublisherMetadataCache_FormatMessage_FakeImplementation(t *testing.T) {
	cache := publishermetadatacache.New(fakeevtapi.New())

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)

	// First Get to ensure handle is cached
	handle, err := cache.Get(publisherName)
	assert.NoError(t, err)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, handle)

	// Verify handle was cached
	cachedValue, found := cache.GetCache().Load(publisherName)
	assert.True(t, found)
	assert.Equal(t, handle, cachedValue.(publishermetadatacache.CacheEntry).Handle)

	// FormatMessage will fail with fake API (not implemented) and remove cache entry
	message, err := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.Empty(t, message)
	assert.Contains(t, err.Error(), "not implemented") // Fake API returns error

	// Verify cache entry was removed due to FormatMessage error
	_, found = cache.GetCache().Load(publisherName)
	assert.False(t, found)
}

func TestPublisherMetadataCache_FormatMessage_Concurrency(t *testing.T) {
	cache := publishermetadatacache.New(fakeevtapi.New())

	publishers := []string{"Publisher1", "Publisher2", "Publisher3", "Publisher4", "Publisher5"}
	eventHandle := evtapi.EventRecordHandle(100)
	numGoroutinesPerPublisher := 5

	var wg sync.WaitGroup

	// Launch multiple goroutines for each publisher
	for _, publisher := range publishers {
		for range numGoroutinesPerPublisher {
			wg.Add(1)
			go func(pub string) {
				defer wg.Done()
				for range 100 {
					cache.FormatMessage(pub, eventHandle, 0)
				}
			}(publisher)
		}
	}

	wg.Wait()
}