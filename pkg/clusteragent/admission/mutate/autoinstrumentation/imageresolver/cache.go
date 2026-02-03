// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"sync"
	"time"
)

type repositoryCache map[string]tagCache // repository -> tagCache
type tagCache map[string]cacheEntry      // tag -> cacheEntry
type cacheEntry struct {
	resolvedImage *ResolvedImage
	whenCached    time.Time
}

type httpDigestCache struct {
	cache   repositoryCache
	ttl     time.Duration
	mu      sync.RWMutex
	fetcher *httpDigestFetcher
}

func (c *httpDigestCache) get(registry string, repository string, tag string) (*ResolvedImage, bool) {
	if resolved := c.checkCache(repository, tag); resolved != nil {
		return resolved, true
	}

	digest, err := c.fetcher.digest(registry + "/" + repository + ":" + tag)
	if err != nil {
		return nil, false
	}

	return c.store(registry, repository, tag, digest), true
}

func (c *httpDigestCache) checkCache(repository, tag string) *ResolvedImage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if tags, exists := c.cache[repository]; exists {
		if entry, exists := tags[tag]; exists {
			if time.Since(entry.whenCached) < c.ttl {
				return entry.resolvedImage
			}
		}
	}
	return nil
}

func (c *httpDigestCache) store(registry, repository, tag, digest string) *ResolvedImage {
	c.mu.Lock()
	defer c.mu.Unlock()

	// DEV: Check if another goroutine has already cached this
	if tags, exists := c.cache[repository]; exists {
		if entry, exists := tags[tag]; exists {
			if time.Since(entry.whenCached) < c.ttl {
				return entry.resolvedImage
			}
		}
	}

	if c.cache[repository] == nil {
		c.cache[repository] = make(tagCache)
	}

	resolved := &ResolvedImage{
		FullImageRef:     registry + "/" + repository + "@" + digest,
		CanonicalVersion: tag,
	}
	c.cache[repository][tag] = cacheEntry{
		resolvedImage: resolved,
		whenCached:    time.Now(),
	}
	return resolved
}

func newHTTPDigestCache(ttl time.Duration) *httpDigestCache {
	return &httpDigestCache{
		cache:   make(repositoryCache),
		ttl:     ttl,
		fetcher: newHTTPDigestFetcher(),
	}
}
