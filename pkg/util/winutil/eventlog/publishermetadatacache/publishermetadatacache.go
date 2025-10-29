// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package publishermetadatacache provides a cache for Windows Event Log publisher metadata handles
package publishermetadatacache

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// invalidHandle represents an invalid EventPublisherMetadataHandle
const invalidHandle = evtapi.EventPublisherMetadataHandle(0)

// cacheEntry represents a cached EventPublisherMetadataHandle.
type cacheEntry struct {
	handle    evtapi.EventPublisherMetadataHandle
	err       error
	createdAt time.Time
}

// PublisherMetadataCache implements the Component interface
type PublisherMetadataCache struct {
	handleMu   sync.RWMutex
	cache      sync.Map
	evtapi     evtapi.API
	expiration time.Duration
}

// New creates a new publishermetadatacache
func New(api evtapi.API) *PublisherMetadataCache {
	// Create cache with 5 minute expiration and handle our own cleanup.
	// Only using expiration for invalid handles to retry creating the handle once it expires.
	// Ignore expiration for valid handles.
	return &PublisherMetadataCache{
		evtapi:     api,
		expiration: 5 * time.Minute,
	}
}

func (c *PublisherMetadataCache) deleteEntry(publisherName string) {
	c.handleMu.Lock()
	defer c.handleMu.Unlock()
	if value, found := c.cache.Load(publisherName); found {
		entry := value.(cacheEntry)
		if entry.handle != invalidHandle {
			evtapi.EvtClosePublisherMetadata(c.evtapi, entry.handle)
		}
		c.cache.Delete(publisherName)
	}
}

// Get retrieves a cached EventPublisherMetadataHandle for the given publisher name.
// If not found in cache, it calls EvtOpenPublisherMetadata and caches the result.
func (c *PublisherMetadataCache) Get(publisherName string) (evtapi.EventPublisherMetadataHandle, error) {
	if value, found := c.cache.Load(publisherName); found {
		entry := value.(cacheEntry)
		// If the handle is invalid and expired, delete the cache entry.
		// No need to delete an expired valid handle.
		if entry.handle == invalidHandle && entry.createdAt.Add(c.expiration).Before(time.Now()) {
			c.deleteEntry(publisherName)
		} else {
			return entry.handle, entry.err
		}

	}

	c.handleMu.Lock()
	defer c.handleMu.Unlock()
	// Double check another thread didn't already create the handle.
	if value, found := c.cache.Load(publisherName); found {
		entry := value.(cacheEntry)
		return entry.handle, entry.err
	}

	handle, err := c.evtapi.EvtOpenPublisherMetadata(publisherName, "")

	if err != nil {
		// Cache the invalid handle and retry creating the handle once it expires from the cache.
		handle = invalidHandle
	}

	c.cache.Store(publisherName, cacheEntry{
		handle:    handle,
		err:       err,
		createdAt: time.Now(),
	})
	return handle, err
}

// FormatMessage formats an event message using the cached EventPublisherMetadataHandle.
func (c *PublisherMetadataCache) FormatMessage(publisherName string, event evtapi.EventRecordHandle, flags uint) (string, error) {
	handle, err := c.Get(publisherName)

	c.handleMu.RLock()
	if handle == invalidHandle {
		// Continue without formatting the message.
		c.handleMu.RUnlock()
		return "", err
	}
	message, err := c.evtapi.EvtFormatMessage(handle, event, 0, nil, flags)
	c.handleMu.RUnlock()

	if err != nil {
		// Ignore these errors
		if errors.Is(err, windows.ERROR_EVT_MESSAGE_NOT_FOUND) ||
			errors.Is(err, windows.ERROR_EVT_MESSAGE_ID_NOT_FOUND) ||
			errors.Is(err, windows.ERROR_EVT_MESSAGE_LOCALE_NOT_FOUND) {
			return "", err
		}
		// FormatMessage failed with an old valid handle, so delete the cache entry
		// and retry creating the handle on the next Get call.
		c.deleteEntry(publisherName)
		return "", err
	}
	return message, nil
}

// Flush cleans up all cached handles when the component shuts down
func (c *PublisherMetadataCache) Flush() {
	c.handleMu.Lock()
	defer c.handleMu.Unlock()
	c.cache.Range(func(key, value interface{}) bool {
		entry := value.(cacheEntry)
		if entry.handle != invalidHandle {
			evtapi.EvtClosePublisherMetadata(c.evtapi, entry.handle)
		}
		c.cache.Delete(key)
		return true
	})
}
