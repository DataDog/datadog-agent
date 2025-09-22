//go:build windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package publishermetadatacacheimpl implements the publishermetadatacache component interface.
package publishermetadatacacheimpl

import (
	"context"
	"time"

	"go.uber.org/fx"

	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
)

// Requires defines the dependencies for the publishermetadatacache component
type Requires struct {
	fx.In

	Lc fx.Lifecycle
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

	// Register cleanup hook to close all handles when component shuts down
	reqs.Lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return cache.stop()
		},
	})

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
	c.cache[publisherName] = cacheItem
	return cacheItem.handle, nil
}

// stop cleans up all cached handles when the component shuts down
func (c *publisherMetadataCache) stop() error {
	for publisherName, cacheItem := range c.cache {
		evtapi.EvtClosePublisherMetadata(c.evtapi, cacheItem.handle)
		delete(c.cache, publisherName)
	}
	return nil
}
