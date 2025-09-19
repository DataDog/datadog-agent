//go:build windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package publishermetadatacacheimpl implements the publishermetadatacache component interface.
package publishermetadatacacheimpl

import (
	"time"

	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
)

// Requires defines the dependencies for the publishermetadatacache component
type Requires struct {
}

// Provides defines the output of the publishermetadatacache component
type Provides struct {
	Comp publishermetadatacache.Component
}

type cacheItem struct {
	handle    evtapi.EventPublisherMetadataHandle
	timestamp time.Time
}

// publisherMetadataCache implements the Component interface
type publisherMetadataCache struct {
	cache        map[string]cacheItem
	evtapi       evtapi.API
	maxCacheSize int
}

// NewComponent creates a new publishermetadatacache component
func NewComponent(reqs Requires) (Provides, error) {
	cache := &publisherMetadataCache{
		cache:        make(map[string]cacheItem),
		evtapi:       winevtapi.New(),
		maxCacheSize: 50,
	}

	return Provides{
		Comp: cache,
	}, nil
}

func (c *publisherMetadataCache) addCacheEntry(publisherName string, handle evtapi.EventPublisherMetadataHandle) {
	if c.isCacheFull() {
		c.flushOldestEntry()
	}
	c.cache[publisherName] = cacheItem{
		handle:    handle,
		timestamp: time.Now(),
	}
	log.Debugf("Cached publisher metadata handle for provider: %s", publisherName)
}

func (c *publisherMetadataCache) isCacheFull() bool {
	return len(c.cache) >= c.maxCacheSize
}

func (c *publisherMetadataCache) flushOldestEntry() {
	currentTime := time.Now()
	keyToDelete := ""
	maxTimeDiff := time.Duration(0)
	for publisherName, cacheItem := range c.cache {
		timeDiff := currentTime.Sub(cacheItem.timestamp)
		if timeDiff > maxTimeDiff {
			maxTimeDiff = timeDiff
			keyToDelete = publisherName
		}
	}
	if keyToDelete != "" {
		oldestItem := c.cache[keyToDelete]
		evtapi.EvtClosePublisherMetadata(c.evtapi, oldestItem.handle)
		delete(c.cache, keyToDelete)
	}
}

// Get retrieves a cached EventPublisherMetadataHandle for the given publisher name.
// If not found in cache, it calls EvtOpenPublisherMetadata and caches the result.
func (c *publisherMetadataCache) Get(publisherName string, event evtapi.EventRecordHandle) (evtapi.EventPublisherMetadataHandle, error) {
	cacheItem, exists := c.cache[publisherName]

	if !exists {
		handle, err := c.evtapi.EvtOpenPublisherMetadata(publisherName, "")
		if err != nil {
			return evtapi.EventPublisherMetadataHandle(0), err
		}
		c.addCacheEntry(publisherName, handle)
		return handle, nil
	}

	// Check if the handle is valid, provider metadata could be uninstalled
	_, err := c.evtapi.EvtFormatMessage(cacheItem.handle, event, 0, nil, evtapi.EvtFormatMessageEvent)
	if err != nil {
		evtapi.EvtClosePublisherMetadata(c.evtapi, cacheItem.handle)
		delete(c.cache, publisherName)
		return c.Get(publisherName, event)
	}

	cacheItem.timestamp = time.Now()
	return cacheItem.handle, nil
}
