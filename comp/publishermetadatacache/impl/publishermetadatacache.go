// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package publishermetadatacacheimpl implements the publishermetadatacache component interface.
package publishermetadatacacheimpl

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
	"golang.org/x/sys/windows"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
)

// InvalidHandle represents an invalid EventPublisherMetadataHandle
const InvalidHandle = evtapi.EventPublisherMetadataHandle(0)

// Requires defines the dependencies for the publishermetadatacache component
type Requires struct {
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the publishermetadatacache component
type Provides struct {
	Comp publishermetadatacache.Component
}

// publisherMetadataCache implements the Component interface
type publisherMetadataCache struct {
	cache  *cache.Cache
	evtapi evtapi.API
}

// New creates a new publishermetadatacache
func New(api evtapi.API) publishermetadatacache.Component {
	// Create cache with 5 minute expiration and handle our own cleanup.
	// Only using expiration for invalid handles to retry creating the handle once it expires.
	// Ignore expiration for valid handles.
	c := cache.New(5*time.Minute, 0)

	c.OnEvicted(func(_ string, value interface{}) {
		if handle, ok := value.(evtapi.EventPublisherMetadataHandle); ok {
			evtapi.EvtClosePublisherMetadata(api, handle)
		}
	})

	return &publisherMetadataCache{
		cache:  c,
		evtapi: api,
	}
}

// NewComponent creates a new publishermetadatacache component
func NewComponent(reqs Requires) Provides {
	cache := New(winevtapi.New())

	// Register cleanup hook to close all handles when component shuts down
	reqs.Lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			cache.Flush()
			return nil
		},
	})

	return Provides{
		Comp: cache,
	}
}

// Get retrieves a cached EventPublisherMetadataHandle for the given publisher name.
// If not found in cache, it calls EvtOpenPublisherMetadata and caches the result.
func (c *publisherMetadataCache) Get(publisherName string) evtapi.EventPublisherMetadataHandle {
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

func (c *publisherMetadataCache) FormatMessage(publisherName string, event evtapi.EventRecordHandle, flags uint) string {
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
func (c *publisherMetadataCache) Flush() {
	c.cache.Flush()
}
