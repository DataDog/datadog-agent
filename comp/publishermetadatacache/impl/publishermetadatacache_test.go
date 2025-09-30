// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package publishermetadatacacheimpl

import (
	"testing"

	publishermetadatacache "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/publishermetadatacache"
	"github.com/stretchr/testify/assert"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	fakeevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/fake"
)

func TestPublisherMetadataCache_Get(t *testing.T) {
	cache := publishermetadatacache.New(fakeevtapi.New())

	publisherName1 := "Publisher1"
	publisherName2 := "Publisher2"

	handle1 := cache.Get(publisherName1)
	handle2 := cache.Get(publisherName2)

	assert.NotEqual(t, publishermetadatacache.InvalidHandle, handle1)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, handle2)

	// Verify item is in cache
	cachedValue, found := cache.GetCache().Get(publisherName1)
	assert.True(t, found)
	assert.Equal(t, handle1, cachedValue.(evtapi.EventPublisherMetadataHandle))

	// Verify item is in cache
	cachedValue, found = cache.GetCache().Get(publisherName2)
	assert.True(t, found)
	assert.Equal(t, handle2, cachedValue.(evtapi.EventPublisherMetadataHandle))
}

func TestPublisherMetadataCache_FormatMessage_InvalidHandle_RecreatesCache(t *testing.T) {
	fakeAPI := fakeevtapi.New()
	cache := publishermetadatacache.New(fakeAPI)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)

	originalHandle := cache.Get(publisherName)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, originalHandle)

	// Invalidate the handle to simulate provider being uninstalled
	err := fakeAPI.InvalidatePublisherHandle(originalHandle)
	assert.NoError(t, err)

	// FormatMessage should detect invalid handle and remove from cache
	message := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.Empty(t, message) // Should return empty string when handle is invalid

	// Verify cache entry was removed
	_, found := cache.GetCache().Get(publisherName)
	assert.False(t, found)

	// Next Get call should create a new handle
	newHandle := cache.Get(publisherName)
	assert.NotEqual(t, originalHandle, newHandle)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, newHandle)
}

func TestPublisherMetadataCache_Close_CleansUpAllHandles(t *testing.T) {
	cache := publishermetadatacache.New(fakeevtapi.New())

	cache.Get("Publisher1")
	cache.Get("Publisher2")

	// Verify items are in cache before closing
	_, found1 := cache.GetCache().Get("Publisher1")
	assert.True(t, found1)
	_, found2 := cache.GetCache().Get("Publisher2")
	assert.True(t, found2)

	cache.Flush()

	// Verify cache is empty after close
	_, found1 = cache.GetCache().Get("Publisher1")
	assert.False(t, found1)
	_, found2 = cache.GetCache().Get("Publisher2")
	assert.False(t, found2)
	assert.Equal(t, 0, cache.GetCache().ItemCount())
}

func TestPublisherMetadataCache_FormatMessage_FakeImplementation(t *testing.T) {
	cache := publishermetadatacache.New(fakeevtapi.New())

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)

	// First Get to ensure handle is cached
	handle := cache.Get(publisherName)
	assert.NotEqual(t, publishermetadatacache.InvalidHandle, handle)

	// Verify handle was cached
	cachedValue, found := cache.GetCache().Get(publisherName)
	assert.True(t, found)
	assert.Equal(t, handle, cachedValue.(evtapi.EventPublisherMetadataHandle))

	// FormatMessage will fail with fake API (not implemented) and remove cache entry
	message := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.Empty(t, message) // Fake API returns empty string on error

	// Verify cache entry was removed due to FormatMessage error
	_, found = cache.GetCache().Get(publisherName)
	assert.False(t, found)
}
