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
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// cacheSize is the number of entries that can be stored in the LRU cache
const cacheSize = 1600

// telemetryTick is the frequency at which the cache usage metrics are collected.
var telemetryTick = 1 * time.Minute

// defaultCacheDir returns/creates the default cache-dir to be used for trivy operations
func defaultCacheDir() string {
	tmpDir, err := os.UserCacheDir()
	if err != nil {
		tmpDir = os.TempDir()
	}
	return filepath.Join(tmpDir, "trivy")
}

// NewCustomBoltCache returns a BoltDB cache using an LRU algorithm with a
// maximum disk size and garbage collection of unused images with its custom cleaner.
func NewCustomBoltCache(wmeta optional.Option[workloadmeta.Component], cacheDir string, maxDiskSize int) (CacheWithCleaner, error) {
	if cacheDir == "" {
		cacheDir = defaultCacheDir()
	}
	db, err := NewBoltDB(cacheDir)
	if err != nil {
		return nil, err
	}
	c, err := newPersistentCache(
		maxDiskSize,
		db,
	)
	if err != nil {
		return nil, err
	}
	trivyCache := &ScannerCache{cache: c, wmeta: wmeta, cachedKeysForEntity: make(map[string][]string)}
	return trivyCache, nil
}

// cachedObject describes an object that can be stored with ScannerCache
type cachedObject interface {
	types.ArtifactInfo | types.BlobInfo
}

// trivyCachePut stores the provided cachedObject in the ScannerCache with the provided key.
func trivyCachePut[T cachedObject](cache *ScannerCache, id string, info T) error {
	objectBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("error converting object with ID %q to JSON: %w", id, err)
	}
	return cache.cache.Set(id, objectBytes)
}

// trivyCacheGet retrieves the object stored with the provided key.
func trivyCacheGet[T cachedObject](cache *ScannerCache, id string) (T, error) {
	rawValue, err := cache.cache.Get(id)
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

// CacheWithCleaner implements trivy's cache interface and adds a Clean method.
type CacheWithCleaner interface {
	cache.Cache
	// clean removes unused cached entries from the cache.
	clean() error
	setKeysForEntity(entity string, cachedKeys []string)
}

// Make sure that ScannerCache implements CacheWithCleaner
var _ CacheWithCleaner = &ScannerCache{}

// ScannerCache wraps a generic Cache and implements trivy.Cache.
type ScannerCache struct {
	// cache is the underlying cache
	cache *persistentCache

	cachedKeysForEntity map[string][]string
	wmeta               optional.Option[workloadmeta.Component]
}

// clean removes entries of deleted images from the cache.
func (c *ScannerCache) clean() error {
	instance, ok := c.wmeta.Get()
	if !ok {
		return nil
	}
	images := instance.ListImages()

	toKeep := make(map[string]struct{}, len(images))
	for _, imageMetadata := range images {
		for _, key := range c.cachedKeysForEntity[imageMetadata.EntityID.ID] {
			toKeep[key] = struct{}{}
		}
	}

	toRemove := make([]string, 0, c.cache.Len())
	for _, key := range c.cache.Keys() {
		if _, ok := toKeep[key]; !ok {
			toRemove = append(toRemove, key)
		}
	}

	if err := c.cache.Remove(toRemove); err != nil {
		return err
	}
	return nil
}

// setKeysForEntity sets keys of items stored in the cache for the given entity.
func (c *ScannerCache) setKeysForEntity(entity string, cachedKeys []string) {
	c.cachedKeysForEntity[entity] = cachedKeys
}

// MissingBlobs implements cache.Cache#MissingBlobs
func (c *ScannerCache) MissingBlobs(artifactID string, blobIDs []string) (bool, []string, error) {
	var missingBlobIDs []string
	for _, blobID := range blobIDs {
		if ok := c.cache.Contains(blobID); !ok {
			missingBlobIDs = append(missingBlobIDs, blobID)
			telemetry.SBOMCacheMisses.Inc()
		} else {
			telemetry.SBOMCacheHits.Inc()
		}
	}
	return !c.cache.Contains(artifactID), missingBlobIDs, nil
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
func (c *ScannerCache) DeleteBlobs([]string) error {
	return nil
}

// Clear implements cache.Cache#Clear
func (c *ScannerCache) Clear() error {
	return c.cache.Clear()
}

// Close implements cache.Cache#Close
func (c *ScannerCache) Close() error {
	return c.cache.Close()
}

// GetArtifact implements cache.Cache#GetArtifact
func (c *ScannerCache) GetArtifact(id string) (types.ArtifactInfo, error) {
	return trivyCacheGet[types.ArtifactInfo](c, id)
}

// GetBlob implements cache.Cache#GetBlob
func (c *ScannerCache) GetBlob(id string) (types.BlobInfo, error) {
	return trivyCacheGet[types.BlobInfo](c, id)
}

// persistentCache is a cache that uses a persistent database for storage.
type persistentCache struct {
	lruCache                     *simplelru.LRU[string, struct{}]
	db                           BoltDB
	mutex                        sync.RWMutex
	currentCachedObjectTotalSize int
	maximumCachedObjectSize      int
	lastEvicted                  string
}

// newPersistentCache creates a new instance of persistentCache and returns a pointer to it.
func newPersistentCache(
	maxCachedObjectSize int,
	localDB BoltDB,
) (*persistentCache, error) {

	persistentCache := &persistentCache{
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
			// TODO: add database compaction. BoltDB deletes the old pages but does not shrink the file.
		}
	}()

	return persistentCache, nil
}

// Contains returns true if the key is found in the cache. It only performs an in-memory check.
func (c *persistentCache) Contains(key string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// using lruCache.Get moves the key to the head of the evictList
	// it is necessary to avoid evicting a key after calling MissingBlobs
	_, ok := c.lruCache.Get(key)
	return ok
}

// Keys returns all the keys stored in the lru cache.
func (c *persistentCache) Keys() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	keys := make([]string, c.lruCache.Len())
	for i, key := range c.lruCache.Keys() {
		keys[i] = key
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *persistentCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lruCache.Len()
}

// Clear deletes all the entries in the cache and closes the db.
func (c *persistentCache) Clear() error {
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
func (c *persistentCache) removeOldest() error {
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
func (c *persistentCache) RemoveOldest() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.removeOldest()
}

// reduceSize reduces the size of the cache to the target size by evicting the oldest items.
func (c *persistentCache) reduceSize(target int) error {
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

// Close closes the database.
func (c *persistentCache) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.db.Close()
}

// set stores the key-value pair in the cache.
func (c *persistentCache) set(key string, value []byte) error {
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
func (c *persistentCache) Set(key string, value []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.set(key, value)
}

// Get implements Cache#Get.
func (c *persistentCache) Get(key string) ([]byte, error) {
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

// remove removes an entry from the cache, returning an error if an I/O error occurs.
func (c *persistentCache) remove(keys []string) error {
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

// Remove removes an entry from the cache.
func (c *persistentCache) Remove(keys []string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.remove(keys)
}

// addKeyInMemory adds the provided key in the lrucache, returning if an entry was evicted.
func (c *persistentCache) addKeyInMemory(key string) bool {
	return c.lruCache.Add(key, struct{}{})
}

// removeKeyFromMemory removes the provided key from the lrucache, returning if the
// key was contained.
func (c *persistentCache) removeKeyFromMemory(key string) bool {
	return c.lruCache.Remove(key)
}

// removeOldestKeyFromMemory removes the oldest key from the lrucache returning the key and
// if a key was removed.
func (c *persistentCache) removeOldestKeyFromMemory() (string, bool) {
	key, _, ok := c.lruCache.RemoveOldest()
	return key, ok
}

// GetCurrentCachedObjectTotalSize returns the current cached object total size.
func (c *persistentCache) GetCurrentCachedObjectTotalSize() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.currentCachedObjectTotalSize
}

// addCurrentCachedObjectTotalSize adds val to the current cached object total size.
func (c *persistentCache) addCurrentCachedObjectTotalSize(val int) {
	c.currentCachedObjectTotalSize += val
}

// subCurrentCachedObjectTotalSize subtract val to the current cached object total size.
func (c *persistentCache) subCurrentCachedObjectTotalSize(val int) {
	c.currentCachedObjectTotalSize -= val
}

// collectTelemetry collects the database's size
func (c *persistentCache) collectTelemetry() {
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
func (c *memoryCache) clean() error                      { return nil }
func (c *memoryCache) setKeysForEntity(string, []string) {}
