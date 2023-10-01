// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy holds trivy related files
package trivy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/utils"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// cacheSize is the number of entries that can be stored in the LRU cache
const cacheSize = 1600

// telemetryTick is the frequency at which the cache usage metrics are collected.
var telemetryTick = 1 * time.Minute

// CacheProvider describes a function that provides a type implementing the trivy cache interface
// and a cache cleaner
type CacheProvider func() (cache.Cache, CacheCleaner, error)

// NewCustomBoltCache is a CacheProvider. It returns a custom implementation of a BoltDB cache using an LRU algorithm with a
// maximum number of cache entries, maximum disk size and garbage collection of unused images with its custom cleaner.
func NewCustomBoltCache(cacheDir string, maxDiskSize int) (cache.Cache, CacheCleaner, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}
	db, err := NewBoltDB(cacheDir)
	if err != nil {
		return nil, &StubCacheCleaner{}, err
	}
	cache, err := NewPersistentCache(
		maxDiskSize,
		db,
	)
	if err != nil {
		return nil, &StubCacheCleaner{}, err
	}
	trivyCache := &ScannerCache{Cache: cache}
	return trivyCache, NewScannerCacheCleaner(trivyCache), nil
}

// NewBoltCache is a CacheProvider. It returns a BoltDB cache provided by Trivy and an empty cleaner.
func NewBoltCache(cacheDir string) (cache.Cache, CacheCleaner, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}
	cache, err := cache.NewFSCache(cacheDir)
	return cache, &StubCacheCleaner{}, err
}

// CacheCleaner interface
type CacheCleaner interface {
	Clean() error
	setKeysForEntity(entity string, cachedKeys []string)
}

// StubCacheCleaner is a stub
type StubCacheCleaner struct{}

// Clean does nothing
func (c *StubCacheCleaner) Clean() error { return nil }

// setKeysForEntity does nothing
func (c *StubCacheCleaner) setKeysForEntity(entity string, keys []string) {}

// GetKeysForEntity does nothing
func (c *StubCacheCleaner) GetKeysForEntity(entity string) []string { return nil }

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
	// Keys returns the cached keys. Required for the cache cleaning logic.
	Keys() []string
	// Len returns the length of the cache. Required for the cache cleaning logic.
	Len() int
}

// ScannerCache holds a generic Cache and implements cache.Cache from Trivy.
type ScannerCache struct {
	Cache Cache
}

// ScannerCacheCleaner is a cache cleaner for a ScannerCache instance. It holds a map
// that keeps track of all the entities using a given key.
type ScannerCacheCleaner struct {
	cachedKeysForEntity map[string][]string
	target              *ScannerCache
}

// NewScannerCacheCleaner creates a new instance of ScannerCacheCleaner and returns a pointer to it.
func NewScannerCacheCleaner(target *ScannerCache) *ScannerCacheCleaner {
	return &ScannerCacheCleaner{
		cachedKeysForEntity: make(map[string][]string),
		target:              target,
	}
}

// Clean implements CacheCleaner#Clean. It removes unused cached entries from the cache.
func (c *ScannerCacheCleaner) Clean() error {
	if workloadmeta.GetGlobalStore() == nil {
		return nil
	}

	images := workloadmeta.GetGlobalStore().ListImages()

	toKeep := make(map[string]struct{}, len(images))
	for _, imageMetadata := range images {
		for _, key := range c.cachedKeysForEntity[imageMetadata.EntityID.ID] {
			toKeep[key] = struct{}{}
		}
	}

	toRemove := make([]string, 0, c.target.Cache.Len())
	for _, key := range c.target.Cache.Keys() {
		if _, ok := toKeep[key]; !ok {
			toRemove = append(toRemove, key)
		}
	}

	if err := c.target.Cache.Remove(toRemove); err != nil {
		return err
	}
	return nil
}

// setKeysForEntity implements CacheCleaner#setKeysForEntity.
func (c *ScannerCacheCleaner) setKeysForEntity(entity string, cachedKeys []string) {
	c.cachedKeysForEntity[entity] = cachedKeys
}

// cachedObject describes an object that can be stored with ScannerCache
type cachedObject interface {
	types.ArtifactInfo | types.BlobInfo
}

// NewScannerCache creates a new ScannerCache instance with the provided Cache.
func NewScannerCache(cache Cache) *ScannerCache {
	return &ScannerCache{
		Cache: cache,
	}
}

// trivyCachePut stores the provided cachedObject in the ScannerCache with the provided key.
func trivyCachePut[T cachedObject](cache *ScannerCache, id string, info T) error {
	objectBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("error converting object with ID %q to JSON: %w", id, err)
	}
	return cache.Cache.Set(id, objectBytes)
}

// trivyCacheGet retrieves the object stored with the provided key.
func trivyCacheGet[T cachedObject](cache *ScannerCache, id string) (T, error) {
	rawValue, err := cache.Cache.Get(id)
	var empty T

	if err != nil {
		return empty, fmt.Errorf("error getting object with ID %q from cache: %w", id, err)
	}

	var res T
	if err := json.Unmarshal(rawValue, &res); err != nil {
		return empty, fmt.Errorf("JSON unmarshal error: %w", err)
	}

	return res, nil
}

// MissingBlobs implements cache.Cache#MissingBlobs
func (c *ScannerCache) MissingBlobs(artifactID string, blobIDs []string) (bool, []string, error) {
	var missingBlobIDs []string
	for _, blobID := range blobIDs {
		if ok := c.Cache.Contains(blobID); !ok {
			missingBlobIDs = append(missingBlobIDs, blobID)
			telemetry.SBOMCacheMisses.Inc()
		} else {
			telemetry.SBOMCacheHits.Inc()
		}
	}
	return !c.Cache.Contains(artifactID), missingBlobIDs, nil
}

// PutArtifact implements cache.Cache#PutArtifact
func (c *ScannerCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) error {
	return trivyCachePut(c, artifactID, artifactInfo)
}

// PutBlob implements cache.Cache#PutBlob
func (c *ScannerCache) PutBlob(blobID string, blobInfo types.BlobInfo) error {
	return trivyCachePut(c, blobID, blobInfo)
}

// DeleteBlobs implements cache.Cache#DeleteBlobs does nothing because the cache cleaning logic is
// managed by CacheCleaner
func (c *ScannerCache) DeleteBlobs(blobIDs []string) error {
	return nil
}

// Clear implements cache.Cache#Clear
func (c *ScannerCache) Clear() error {
	return c.Cache.Clear()
}

// Close implements cache.Cache#Close
func (c *ScannerCache) Close() error {
	return c.Cache.Close()
}

// GetArtifact implements cache.Cache#GetArtifact
func (c *ScannerCache) GetArtifact(id string) (types.ArtifactInfo, error) {
	return trivyCacheGet[types.ArtifactInfo](c, id)
}

// GetBlob implements cache.Cache#GetBlob
func (c *ScannerCache) GetBlob(id string) (types.BlobInfo, error) {
	return trivyCacheGet[types.BlobInfo](c, id)
}

// PersistentCache is a cache that uses a persistent database for storage.
type PersistentCache struct {
	lruCache                     *simplelru.LRU[string, struct{}]
	db                           PersistentDB
	mutex                        sync.RWMutex
	currentCachedObjectTotalSize int
	maximumCachedObjectSize      int
	lastEvicted                  string
}

// NewPersistentCache creates a new instance of PersistentCache and returns a pointer to it.
func NewPersistentCache(
	maxCachedObjectSize int,
	localDB PersistentDB,
) (*PersistentCache, error) {

	persistentCache := &PersistentCache{
		db:                           localDB,
		currentCachedObjectTotalSize: 0,
		maximumCachedObjectSize:      maxCachedObjectSize,
	}

	lruCache, err := simplelru.NewLRU(cacheSize, func(key string, _ struct{}) {
		persistentCache.lastEvicted = key
	})
	if err != nil {
		return nil, err
	}
	persistentCache.lruCache = lruCache

	var evicted []string
	if err = localDB.ForEach(func(key string, value []byte) error {
		if ok := persistentCache.addKeyInMemory(key); ok {
			evicted = append(evicted, persistentCache.lastEvicted)
		}
		persistentCache.addCurrentCachedObjectTotalSize(len(value))
		return nil
	}); err != nil {
		return nil, err
	}

	err = persistentCache.Remove(evicted)
	if err != nil {
		return nil, err
	}

	go func() {
		ticker := time.NewTicker(telemetryTick)
		for {
			for range ticker.C {
				persistentCache.collectTelemetry()
			}
		}
	}()

	return persistentCache, nil
}

// Contains implements Cache#Contains. It only performs an in-memory check.
func (c *PersistentCache) Contains(key string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// using lruCache.Get moves the key to the head of the evictList
	// it is necessary to avoid evicting a key after calling MissingBlobs
	_, ok := c.lruCache.Get(key)
	return ok
}

// Keys returns all the keys stored in the lru cache.
func (c *PersistentCache) Keys() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	keys := make([]string, c.lruCache.Len())
	for i, key := range c.lruCache.Keys() {
		keys[i] = key
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *PersistentCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lruCache.Len()
}

// Clear implements Cache#Clear.
func (c *PersistentCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err := c.db.Clear(); err != nil {
		return err
	}
	c.lruCache.Purge()
	c.currentCachedObjectTotalSize = 0
	return nil
}

// removeOldest removes the least recently used item from the cache.
func (c *PersistentCache) removeOldest() error {
	key, ok := c.removeOldestKeyFromMemory()
	if !ok {
		return fmt.Errorf("in-memory cache is empty")
	}

	evicted := 0
	if err := c.db.Delete([]string{key}, func(key string, value []byte) error {
		evicted += len(value)
		return nil
	}); err != nil {
		_ = c.addKeyInMemory(key)
		return err
	}

	c.subCurrentCachedObjectTotalSize(evicted)

	return nil
}

// RemoveOldest is a thread-safe version of removeOldest
func (c *PersistentCache) RemoveOldest() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.removeOldest()
}

// reduceSize reduces the size of the cache to the target size by evicting the oldest items.
func (c *PersistentCache) reduceSize(target int) error {
	for c.currentCachedObjectTotalSize > target {
		prev := c.currentCachedObjectTotalSize
		err := c.removeOldest()
		if err != nil {
			return err
		}
		if prev == c.currentCachedObjectTotalSize {
			// if c.currentCachedObjectTotalSize is not updated by removeOldest then an item is stored in the lrucache without being stored in the local storage
			return fmt.Errorf("cache and db are out of sync")
		}
	}
	return nil
}

// Close implements Cache#Close
func (c *PersistentCache) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.db.Close()
}

// set stores the key-value pair in the cache.
func (c *PersistentCache) set(key string, value []byte) error {
	if len(value) > c.maximumCachedObjectSize {
		return fmt.Errorf("value of [%s] is too big for the cache : %d", key, c.maximumCachedObjectSize)
	}

	if err := c.reduceSize(c.maximumCachedObjectSize - len(value)); err != nil {
		return fmt.Errorf("failed to reduce the size of the cache to store [%s]: %v", key, err)
	}

	if evict := c.addKeyInMemory(key); evict {
		evictedSize := 0
		if err := c.db.Delete([]string{c.lastEvicted}, func(_ string, value []byte) error {
			evictedSize += len(value)
			return nil
		}); err != nil {
			c.removeKeyFromMemory(key)
			c.addKeyInMemory(c.lastEvicted)
			return err
		}
		c.subCurrentCachedObjectTotalSize(evictedSize)
	}

	err := c.db.Store(key, value)
	if err != nil {
		c.removeKeyFromMemory(key)
		return err
	}

	c.addCurrentCachedObjectTotalSize(len(value))
	return nil
}

// Set implements Cache#Set. It is a thread-safe version of set.
func (c *PersistentCache) Set(key string, value []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.set(key, value)
}

// Get implements Cache#Get.
func (c *PersistentCache) Get(key string) ([]byte, error) {
	ok := c.Contains(key)
	if !ok {
		return nil, fmt.Errorf("key not found")
	}

	res, err := c.db.Get(key)
	if err != nil {
		_ = c.Remove([]string{key})
		return nil, err
	}
	return res, nil
}

// remove removes an entry from the cache.
func (c *PersistentCache) remove(keys []string) error {
	removedSize := 0
	if err := c.db.Delete(keys, func(_ string, value []byte) error {
		removedSize += len(value)
		return nil
	}); err != nil {
		return err
	}

	for _, key := range keys {
		_ = c.removeKeyFromMemory(key)
	}

	c.subCurrentCachedObjectTotalSize(removedSize)
	return nil
}

// Remove implements Cache#Remove. It is a thread safe version of remove.
func (c *PersistentCache) Remove(keys []string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.remove(keys)
}

// addKeyInMemory adds the provided key in the lrucache, returning if an entry was evicted.
func (c *PersistentCache) addKeyInMemory(key string) bool {
	return c.lruCache.Add(key, struct{}{})
}

// removeKeyFromMemory removes the provided key from the lrucache, returning if the
// key was contained.
func (c *PersistentCache) removeKeyFromMemory(key string) bool {
	return c.lruCache.Remove(key)
}

// removeOldestKeyFromMemory removes the oldest key from the lrucache returning the key and
// if a key was removed.
func (c *PersistentCache) removeOldestKeyFromMemory() (string, bool) {
	key, _, ok := c.lruCache.RemoveOldest()
	return key, ok
}

// GetCurrentCachedObjectTotalSize returns the current cached object total size.
func (c *PersistentCache) GetCurrentCachedObjectTotalSize() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.currentCachedObjectTotalSize
}

// addCurrentCachedObjectTotalSize adds val to the current cached object total size.
func (c *PersistentCache) addCurrentCachedObjectTotalSize(val int) {
	c.currentCachedObjectTotalSize += val
}

// subCurrentCachedObjectTotalSize subtract val to the current cached object total size.
func (c *PersistentCache) subCurrentCachedObjectTotalSize(val int) {
	c.currentCachedObjectTotalSize -= val
}

// collectTelemetry collects the database's size
func (c *PersistentCache) collectTelemetry() {
	diskSize, err := c.db.Size()
	if err != nil {
		log.Errorf("could not collect telemetry: %v", err)
	}
	telemetry.SBOMCacheDiskSize.Set(float64(diskSize))
}

func newMemoryCache() *memoryCache {
	return &memoryCache{}
}

type memoryCache struct {
	blobInfo     *types.BlobInfo
	blobID       string
	artifactInfo *types.ArtifactInfo
	artifactID   string
}

func (c *memoryCache) MissingBlobs(artifactID string, blobIDs []string) (missingArtifact bool, missingBlobIDs []string, err error) {
	for _, blobID := range blobIDs {
		if _, err := c.GetBlob(blobID); err != nil {
			missingBlobIDs = append(missingBlobIDs, blobID)
		}
	}

	if _, err := c.GetArtifact(artifactID); err != nil {
		missingArtifact = true
	}

	return
}

func (c *memoryCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) (err error) {
	c.artifactInfo = &artifactInfo
	c.artifactID = artifactID
	return nil
}

func (c *memoryCache) PutBlob(blobID string, blobInfo types.BlobInfo) (err error) {
	c.blobInfo = &blobInfo
	c.blobID = blobID
	return nil
}

func (c *memoryCache) DeleteBlobs(blobIDs []string) error {
	if c.blobInfo != nil {
		for _, blobID := range blobIDs {
			if blobID == c.blobID {
				c.blobInfo = nil
			}
		}
	}
	return nil
}

func (c *memoryCache) GetArtifact(artifactID string) (artifactInfo types.ArtifactInfo, err error) {
	if c.artifactInfo != nil && c.artifactID == artifactID {
		return *c.artifactInfo, nil
	}
	return types.ArtifactInfo{}, nil
}

func (c *memoryCache) GetBlob(blobID string) (blobInfo types.BlobInfo, err error) {
	if c.blobInfo != nil && c.blobID == blobID {
		return *c.blobInfo, nil
	}
	return types.BlobInfo{}, errors.New("not found")
}

func (c *memoryCache) Close() (err error) {
	c.artifactInfo = nil
	c.blobInfo = nil
	return nil
}

func (c *memoryCache) Clear() (err error) {
	return c.Close()
}
