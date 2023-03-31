// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trivy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/utils"
	"github.com/hashicorp/golang-lru/simplelru"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
)

const sizeOfKey = 32

var telemetryTick = 1 * time.Minute

type CacheProvider func() (cache.Cache, error)

func NewBoltCache(cacheDir string) (cache.Cache, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}

	return cache.NewFSCache(cacheDir)
}

func NewCustomBoltCache(cacheDir string) (cache.Cache, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}
	db, err := NewBoltDB(cacheDir)
	if err != nil {
		return nil, err
	}
	cache, err := NewPersistentCache(
		config.Datadog.GetInt("custom_cache_max_cache_entries"),
		config.Datadog.GetInt("custom_cache_max_disk_size"),
		db,
		NewMaintainer(context.TODO(), config.Datadog.GetDuration("custom_cache_gc_interval")),
	)
	if err != nil {
		return nil, err
	}
	return &TrivyCache{
		Cache: cache,
	}, nil
}

type Cache interface {
	Clear() error
	Close() error
	Contains(string) bool
	Remove([]string) error
	Set(string, []byte) error
	Get(string) ([]byte, error)
}

func (cache *PersistentCache) collectTelemetry() {
	diskSize, err := cache.db.Size()
	if err != nil {
		log.Errorf("could not collect telemetry: %v", err)
	}
	telemetry.SBOMCacheDiskSize.Set(float64(diskSize))
}

type TrivyCache struct {
	Cache Cache
}

func NewTrivyCache(cache Cache) *TrivyCache {
	return &TrivyCache{
		Cache: cache,
	}
}

func trivyCachePut[T any](cache *TrivyCache, id string, info T) error {
	objectBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("error converting object with ID %q to JSON: %w", id, err)
	}
	return cache.Cache.Set(id, objectBytes)
}

func trivyCacheGet[T any](cache *TrivyCache, id string) (T, error) {
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

func (c *TrivyCache) MissingBlobs(artifactID string, blobIDs []string) (bool, []string, error) {
	var missingBlobIDs []string
	for _, blobID := range blobIDs {
		if ok := c.Cache.Contains(blobID); !ok {
			missingBlobIDs = append(missingBlobIDs, blobID)
		}
	}
	return !c.Cache.Contains(artifactID), missingBlobIDs, nil
}

func (c *TrivyCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) error {
	return trivyCachePut(c, artifactID, artifactInfo)
}

func (c *TrivyCache) PutBlob(blobID string, blobInfo types.BlobInfo) error {
	return trivyCachePut(c, blobID, blobInfo)
}

func (c *TrivyCache) DeleteBlobs(blobIDs []string) error {
	return c.Cache.Remove(blobIDs)
}

func (c *TrivyCache) Clear() error {
	return c.Cache.Clear()
}

func (c *TrivyCache) Close() error {
	return c.Cache.Close()
}

func (c *TrivyCache) GetArtifact(id string) (types.ArtifactInfo, error) {
	return trivyCacheGet[types.ArtifactInfo](c, id)
}

func (c *TrivyCache) GetBlob(id string) (types.BlobInfo, error) {
	return trivyCacheGet[types.BlobInfo](c, id)
}

var (
	ttlTicker = 5 * time.Minute
)

type Maintainer struct {
	gcTicker        *time.Ticker
	telemetryTicker *time.Ticker
}

func (c *Maintainer) Clean(cache *PersistentCache) {
	listedImages := make(map[string]struct{})
	for _, imageMetadata := range workloadmeta.GetGlobalStore().ListImages() {
		sbom := imageMetadata.SBOM
		listedImages[sbom.ArtifactID] = struct{}{}
		for _, blobID := range sbom.BlobIDs {
			listedImages[blobID] = struct{}{}
		}
	}
	var toRemove []string
	for _, key := range cache.Keys() {
		toRemove = append(toRemove, key)
	}
	err := cache.Remove(toRemove)
	if err != nil {
		// will always be triggered if the database is closed
		log.Errorf("error cleaning the database: %v", err)
	}
}

func (m *Maintainer) Maintain(cache *PersistentCache) {
	for {
		select {
		case <-m.telemetryTicker.C:
			cache.collectTelemetry()
		case <-m.gcTicker.C:
			m.Clean(cache)
		}
	}
}

func NewMaintainer(ctx context.Context, interval time.Duration) *Maintainer {
	return &Maintainer{
		gcTicker:        time.NewTicker(interval),
		telemetryTicker: time.NewTicker(telemetryTick),
	}
}

type PersistentCache struct {
	ctx                     context.Context
	lruCache                *simplelru.LRU
	db                      PersistentDB
	mutex                   sync.RWMutex
	currentCachedObjectSize int
	maximumCachedObjectSize int
	lastEvicted             string
}

func NewPersistentCache(
	maxCacheSize int,
	maxCachedObjectSize int,
	localDB PersistentDB,
	maintainer *Maintainer,
) (*PersistentCache, error) {

	persistentCache := &PersistentCache{
		db:                      localDB,
		currentCachedObjectSize: 0,
		maximumCachedObjectSize: maxCachedObjectSize,
	}

	lruCache, err := simplelru.NewLRU(maxCacheSize, func(key interface{}, _ interface{}) {
		persistentCache.lastEvicted = key.(string)
		telemetry.SBOMCacheEvicts.Inc()
	})
	if err != nil {
		return nil, err
	}

	var evicted []string
	if err = localDB.ForEach(func(key string, value []byte) error {
		if ok := lruCache.Add(key, struct{}{}); ok {
			evicted = append(evicted, persistentCache.lastEvicted)
		}
		persistentCache.IncCurrentCachedObjectSize(len(value))
		return nil
	}); err != nil {
		return nil, err
	}

	err = persistentCache.Remove(evicted)
	if err != nil {
		return nil, err
	}

	go maintainer.Maintain(persistentCache)

	return persistentCache, nil
}

func (c *PersistentCache) Contains(key string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lruCache.Contains(key)
}

func (c *PersistentCache) Keys() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	keys := make([]string, c.lruCache.Len())
	for i, key := range c.lruCache.Keys() {
		keys[i] = key.(string)
	}
	return keys
}

func (c *PersistentCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.lruCache.Len()
}

func (c *PersistentCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err := c.db.Clear(); err != nil {
		return err
	}
	c.lruCache.Purge()
	c.currentCachedObjectSize = 0
	telemetry.SBOMCachedObjectSize.Set(0)
	return nil
}

func (c *PersistentCache) Resize(size int) int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.lruCache.Resize(size)
}

func (c *PersistentCache) RemoveOldest() (string, []byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	key, _, ok := c.lruCache.RemoveOldest()
	if !ok {
		return "", nil, fmt.Errorf("in-memory cache is empty")
	}

	val, err := c.db.Get(key.(string))
	c.DecCurrentCachedObjectSize(len(val))
	return key.(string), val, err
}

func (c *PersistentCache) ReduceSize(target int) error {
	if target > c.maximumCachedObjectSize {
		return fmt.Errorf("cache can not exceed %d", c.maximumCachedObjectSize)
	}

	for c.currentCachedObjectSize > target {
		_, _, err := c.RemoveOldest()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *PersistentCache) Close() error {
	return c.db.Close()
}

func (c *PersistentCache) Set(key string, value []byte) error {
	if len(value) > c.maximumCachedObjectSize {
		return fmt.Errorf("value of [%s] is too big for the cache : %d", key, c.maximumCachedObjectSize)
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if evict := c.lruCache.Add(key, struct{}{}); evict {
		if evictedValue, err := c.db.Delete([]string{c.lastEvicted}); err != nil {
			c.DecCurrentCachedObjectSize(len(evictedValue))
			return err
		}
	}

	err := c.db.Store(key, value)
	if err != nil {
		return err
	}

	c.IncCurrentCachedObjectSize(len(value))
	return nil
}

func (c *PersistentCache) Get(key string) ([]byte, error) {
	ok := c.Contains(key)
	if !ok {
		telemetry.SBOMCacheMisses.Inc()
		return nil, fmt.Errorf("Key not found")
	}

	res, err := c.db.Get(key)
	if err != nil {
		_ = c.Remove([]string{key})
		return nil, err
	}
	telemetry.SBOMCacheHits.Inc()
	return res, nil
}

func (c *PersistentCache) Remove(keys []string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		c.lruCache.Remove(key)
	}
	values, err := c.db.Delete(keys)
	if err != nil {
		return err
	}
	for _, val := range values {
		c.DecCurrentCachedObjectSize(len(val))
	}
	return nil
}

func (c *PersistentCache) GetCurrentCachedObjectSize() int {
	return c.currentCachedObjectSize
}

func (c *PersistentCache) IncCurrentCachedObjectSize(val int) {
	c.currentCachedObjectSize += val
	telemetry.SBOMCachedObjectSize.Add(float64(val))
}

func (c *PersistentCache) DecCurrentCachedObjectSize(val int) {
	c.currentCachedObjectSize -= val
	telemetry.SBOMCachedObjectSize.Sub(float64(val))
}
