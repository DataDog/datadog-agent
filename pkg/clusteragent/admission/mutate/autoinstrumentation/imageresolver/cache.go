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

type registryCache map[string]repositoryCache // registry -> repositoryCache
type repositoryCache map[string]tagCache      // repository -> tagCache
type tagCache map[string]cacheEntry           // tag -> cacheEntry
type cacheEntry struct {
	resolvedImage *ResolvedImage
	whenCached    time.Time
}

type httpDigestCache struct {
	cache   registryCache
	ttl     time.Duration
	mu      sync.RWMutex
	fetcher *httpDigestFetcher
}

func (c *httpDigestCache) get(registry string, repository string, tag string) (*ResolvedImage, bool) {
	if resolved := c.checkCache(registry, repository, tag); resolved != nil {
		return resolved, true
	}

	digest, err := c.fetcher.digest(registry + "/" + repository + ":" + tag)
	if err != nil {
		return nil, false
	}

	return c.store(registry, repository, tag, digest), true
}

func (c *httpDigestCache) checkCache(registry, repository, tag string) *ResolvedImage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if repos, exists := c.cache[registry]; exists {
		if tags, exists := repos[repository]; exists {
			if entry, exists := tags[tag]; exists {
				if time.Since(entry.whenCached) < c.ttl {
					return entry.resolvedImage
				}
			}
		}
	}
	return nil
}

func (c *httpDigestCache) store(registry, repository, tag, digest string) *ResolvedImage {
	c.mu.Lock()
	defer c.mu.Unlock()

	registryCache, exists := c.cache[registry]
	if !exists {
		registryCache = make(repositoryCache)
		c.cache[registry] = registryCache
	}
	repositoryCache, exists := registryCache[repository]
	if !exists {
		repositoryCache = make(tagCache)
		registryCache[repository] = repositoryCache
	}

	resolved := &ResolvedImage{
		FullImageRef:     registry + "/" + repository + "@" + digest,
		CanonicalVersion: tag,
	}
	c.cache[registry][repository][tag] = cacheEntry{
		resolvedImage: resolved,
		whenCached:    time.Now(),
	}
	return resolved
}

func newHTTPDigestCache(ttl time.Duration, ddRegistries map[string]struct{}) *httpDigestCache {
	cache := make(registryCache)
	for registry := range ddRegistries {
		cache[registry] = make(repositoryCache)
	}

	return &httpDigestCache{
		cache:   cache,
		ttl:     ttl,
		fetcher: newHTTPDigestFetcher(),
	}
}
