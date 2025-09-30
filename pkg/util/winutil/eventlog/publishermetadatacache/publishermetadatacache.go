// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package publishermetadatacache provides a cache for Windows Event Log publisher metadata handles
package publishermetadatacache

import (
	"time"

	"github.com/patrickmn/go-cache"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"golang.org/x/sys/windows"
)

// InvalidHandle represents an invalid EventPublisherMetadataHandle
const InvalidHandle = evtapi.EventPublisherMetadataHandle(0)

// PublisherMetadataCache implements the Component interface
type PublisherMetadataCache struct {
	cache  *cache.Cache
	evtapi evtapi.API
}

// New creates a new publishermetadatacache
func New(api evtapi.API) *PublisherMetadataCache {
	// Create cache with 5 minute expiration and handle our own cleanup.
	// Only using expiration for invalid handles to retry creating the handle once it expires.
	// Ignore expiration for valid handles.
	c := cache.New(5*time.Minute, 0)

	c.OnEvicted(func(_ string, value interface{}) {
		if handle, ok := value.(evtapi.EventPublisherMetadataHandle); ok {
			evtapi.EvtClosePublisherMetadata(api, handle)
		}
	})

	return &PublisherMetadataCache{
		cache:  c,
		evtapi: api,
	}
}

// Get retrieves a cached EventPublisherMetadataHandle for the given publisher name.
// If not found in cache, it calls EvtOpenPublisherMetadata and caches the result.
func (c *PublisherMetadataCache) Get(publisherName string) evtapi.EventPublisherMetadataHandle {
	if cachedValue, expiration, found := c.cache.GetWithExpiration(publisherName); found {
		if handle, ok := cachedValue.(evtapi.EventPublisherMetadataHandle); ok {
			// If the handle is invalid and expired, delete the cache entry
			// so the next Get call will try creating a new handle.
			// No need to delete an expired valid handle.
			if handle == InvalidHandle && expiration.Before(time.Now()) {
				c.cache.Delete(publisherName)
				return InvalidHandle
			}
			return handle
		}
	}

	handle, err := c.evtapi.EvtOpenPublisherMetadata(publisherName, "")
	if err != nil {
		// Cache the invalid handle and retry creating the handle once it expires from the cache.
		handle = InvalidHandle
	}

	c.cache.SetDefault(publisherName, handle)
	return handle
}

func (c *PublisherMetadataCache) FormatMessage(publisherName string, event evtapi.EventRecordHandle, flags uint) string {
	handle := c.Get(publisherName)
	if handle == InvalidHandle {
		// Continue without formatting the message.
		return ""
	}
	message, err := c.evtapi.EvtFormatMessage(handle, event, 0, nil, flags)
	if err != nil {
		// Ignore these errors
		if err == windows.ERROR_EVT_MESSAGE_NOT_FOUND ||
			err == windows.ERROR_EVT_MESSAGE_ID_NOT_FOUND ||
			err == windows.ERROR_EVT_MESSAGE_LOCALE_NOT_FOUND {
			return ""
		}
		// FormatMessage failed with an old valid handle, so delete the cache entry
		// and retry creating the handle on the next Get call.
		c.cache.Delete(publisherName)
		return ""
	}
	return message
}

// Flush cleans up all cached handles when the component shuts down
func (c *PublisherMetadataCache) Flush() {
	c.cache.Flush()
}

// GetCache returns the internal cache for testing purposes
func (c *PublisherMetadataCache) GetCache() *cache.Cache {
	return c.cache
}
