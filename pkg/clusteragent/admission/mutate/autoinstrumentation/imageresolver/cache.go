// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"net/http"
	"sync"
	"time"
)

type registryCache map[string]repositoryCache
type repositoryCache map[string]tagCache
type tagCache map[string]cacheEntry
type cacheEntry struct {
	digest     string
	whenCached time.Time
}

type httpDigestCache struct {
	cache   registryCache
	ttl     time.Duration
	mu      sync.RWMutex
	fetcher *httpDigestFetcher
}

func (c *httpDigestCache) get(registry string, repository string, tag string) (string, error) {
	if digest := c.checkCache(registry, repository, tag); digest != "" {
		return digest, nil
	}

	digest, err := c.fetcher.digest(registry + "/" + repository + ":" + tag)
	if err != nil {
		return "", err
	}

	return c.store(registry, repository, tag, digest), nil
}

func (c *httpDigestCache) checkCache(registry, repository, tag string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if repos, exists := c.cache[registry]; exists {
		if tags, exists := repos[repository]; exists {
			if entry, exists := tags[tag]; exists {
				if time.Since(entry.whenCached) < c.ttl {
					return entry.digest
				}
			}
		}
	}
	return ""
}

func (c *httpDigestCache) store(registry, repository, tag, digest string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	registryCache, exists := c.cache[registry]
	if !exists {
		return ""
	}
	_, exists = registryCache[repository]
	if !exists {
		registryCache[repository] = make(tagCache)
	}

	c.cache[registry][repository][tag] = cacheEntry{
		digest:     digest,
		whenCached: time.Now(),
	}
	return digest
}

func newHTTPDigestCache(ttl time.Duration, ddRegistries map[string]struct{}, rt http.RoundTripper) *httpDigestCache {
	cache := make(registryCache)
	for registry := range ddRegistries {
		cache[registry] = make(repositoryCache)
	}

	return &httpDigestCache{
		cache:   cache,
		ttl:     ttl,
		fetcher: newHTTPDigestFetcher(rt),
	}
}
