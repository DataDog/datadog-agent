// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/utils"
)

// telemetryTick is the frequency at which the cache usage metrics are collected.
var telemetryTick = 1 * time.Minute

// CacheProvider describe a function that provides a type implementing the trivy cache interface
// and a cache cleaner
type CacheProvider func() (cache.Cache, CacheCleaner, error)

// NewBoltCache is a CacheProvider. It returns a BoltDB cache provided by Trivy and an empty cleaner.
func NewBoltCache(cacheDir string) (cache.Cache, CacheCleaner, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}
	cache, err := cache.NewFSCache(cacheDir)
	return cache, &StubCacheCleaner{}, err
}

// StubCacheCleaner is a stub
type StubCacheCleaner struct{}

// Clean does nothing
func (c *StubCacheCleaner) Clean() error { return nil }

// Cache describes an interface for a key-value cache.
type Cache interface {
	// Clear removes all entries from the cache and closes it.
	Clear() error
	// Close closes the cache.
	Close() error
	// Contains returns true if the given key exists in the cache.
	Contains(key string) bool
	// Remove deletes the entries associated with the given keys from the cache.
	Remove(keys []string) error
	// Set inserts or updates an entry in the cache with the given key-value pair.
	Set(key string, value []byte) error
	// Get returns the value associated with the given key. It returns an error if the key was not found.
	Get(key string) ([]byte, error)
}

// TrivyCache holds a generic Cache and implements cache.Cache from Trivy.
type TrivyCache struct {
	Cache Cache
}

// cachedObject describe an object that can be stored with TrivyCache
type cachedObject interface {
	types.ArtifactInfo | types.BlobInfo
}

// NewTrivyCache creates a new TrivyCache instance with the provided Cache.
func NewTrivyCache(cache Cache) *TrivyCache {
	return &TrivyCache{
		Cache: cache,
	}
}

// trivyCachePut stores the provided cachedObject in the TrivyCache with the provided key.
func trivyCachePut[T cachedObject](cache *TrivyCache, id string, info T) error {
	objectBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("error converting object with ID %q to JSON: %w", id, err)
	}
	return cache.Cache.Set(id, objectBytes)
}

// trivyCacheGet retrieves the object stored with the provided key.
func trivyCacheGet[T cachedObject](cache *TrivyCache, id string) (T, error) {
	rawValue, err := cache.Cache.Get(id)
	var empty T

	if err != nil {
		return empty, fmt.Errorf("error getting object with ID %q from Badger cache: %w", id, err)
	}

	var res T
	if err := json.Unmarshal(rawValue, &res); err != nil {
		return empty, fmt.Errorf("JSON unmarshal error: %w", err)
	}

	return res, nil
}

// Implements cache.Cache#MissingBlobs
func (c *TrivyCache) MissingBlobs(artifactID string, blobIDs []string) (bool, []string, error) {
	var missingBlobIDs []string
	for _, blobID := range blobIDs {
		if ok := c.Cache.Contains(blobID); !ok {
			missingBlobIDs = append(missingBlobIDs, blobID)
		}
	}
	return !c.Cache.Contains(artifactID), missingBlobIDs, nil
}

// Implements cache.Cache#PutArtifact
func (c *TrivyCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) error {
	return trivyCachePut(c, artifactID, artifactInfo)
}

// Implements cache.Cache#PutBlob
func (c *TrivyCache) PutBlob(blobID string, blobInfo types.BlobInfo) error {
	return trivyCachePut(c, blobID, blobInfo)
}

// Implements cache.Cache#DeleteBlobs
func (c *TrivyCache) DeleteBlobs(blobIDs []string) error {
	return c.Cache.Remove(blobIDs)
}

// Implements cache.Cache#Clear
func (c *TrivyCache) Clear() error {
	return c.Cache.Clear()
}

// Implements cache.Cache#Close
func (c *TrivyCache) Close() error {
	return c.Cache.Close()
}

// Implements cache.Cache#GetArtifact
func (c *TrivyCache) GetArtifact(id string) (types.ArtifactInfo, error) {
	return trivyCacheGet[types.ArtifactInfo](c, id)
}

// Implements cache.Cache#GetBlob
func (c *TrivyCache) GetBlob(id string) (types.BlobInfo, error) {
	return trivyCacheGet[types.BlobInfo](c, id)
}
