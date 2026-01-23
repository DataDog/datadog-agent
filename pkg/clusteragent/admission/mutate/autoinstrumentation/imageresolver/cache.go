// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
)

type Cache interface {
	Get(registry string, repository string, tag string) (*ResolvedImage, bool)
}

type CacheEntry struct {
	ResolvedImage *ResolvedImage
	WhenCached    time.Time
}

type craneCache struct {
	cache map[string]map[string]CacheEntry
	ttl   time.Duration
	mu    sync.RWMutex
}

func (c *craneCache) Get(registry string, repository string, tag string) (*ResolvedImage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.cache[repository]; !exists {
		c.cache[repository] = make(map[string]CacheEntry)
	}
	if _, exists := c.cache[repository][tag]; exists {
		if time.Since(c.cache[repository][tag].WhenCached) < c.ttl {
			return c.cache[repository][tag].ResolvedImage, true
		}
	}
	if digest, err := crane.Digest(registry + "/" + repository + ":" + tag); err == nil {
		c.cache[repository][tag] = CacheEntry{
			ResolvedImage: &ResolvedImage{
				FullImageRef:     registry + "/" + repository + "@" + digest,
				CanonicalVersion: tag, // DEV: This is the customer-provided tag, not the canonical version
			},
			WhenCached: time.Now(),
		}
		return c.cache[repository][tag].ResolvedImage, true
	}
	return nil, false
}

func NewCache(ttl time.Duration) Cache {
	return &craneCache{
		cache: make(map[string]map[string]CacheEntry),
		ttl:   ttl,
		mu:    sync.RWMutex{},
	}
}
