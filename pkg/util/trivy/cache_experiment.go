package trivy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
)

var (
	ttlTicker = 5 * time.Minute
)

type Cache interface {
	Contains(string) bool
	Remove(string) error
	Set(string, []byte) error
	Get(string) ([]byte, error)
}

type Maintainer struct {
	ctx    context.Context
	ticker *time.Ticker
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
	for _, key := range cache.Keys() {
		if _, ok := listedImages[key]; !ok {
			cache.Remove(key)
		}
	}
}

func (c *Maintainer) Maintain(cache *PersistentCache) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.ticker.C:
			c.Clean(cache)
		}
	}
}

func (c *Maintainer) NewMaintainer(ctx context.Context, interval time.Duration) *Maintainer {
	return &Maintainer{
		ctx:    ctx,
		ticker: time.NewTicker(interval),
	}
}

type PersistentDB interface {
	Clear() error
	Close() error
	Delete(string) ([]byte, error)
	Get(string) ([]byte, error)
	Store(string, []byte) error
	GetAllKeys() []string
	Size() uint
}

type PersistentCache struct {
	lruCache        *simplelru.LRU[string, struct{}]
	db              PersistentDB
	mutex           sync.RWMutex
	currentDiskSize uint
	maximumDiskSize uint
}

func NewPersistentCache(
	cacheSize int,
	localDB PersistentDB,
	maintainer *Maintainer,
) (*PersistentCache, error) {

	lruCache, err := simplelru.NewLRU[string, struct{}](cacheSize, func(string, struct{}) {})
	if err != nil {
		return nil, err
	}

	for _, key := range localDB.GetAllKeys() {
		lruCache.Add(key, struct{}{})
	}

	persistentCache := &PersistentCache{
		lruCache: lruCache,
		db:       localDB,
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
	return c.lruCache.Keys()
}

func (c *PersistentCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.lruCache.Len()
}

func (c *PersistentCache) Purge() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range c.lruCache.Keys() {
		_, _ = c.db.Delete(key)
	}
	c.lruCache.Purge()
	c.currentDiskSize = 0
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
		return "", nil, fmt.Errorf("cache is empty")
	}

	val, err := c.db.Get(key)
	c.currentDiskSize -= uint(len(val))
	return key, val, err
}

func (c *PersistentCache) ReduceSize(target uint) error {
	if target > c.maximumDiskSize {
		return fmt.Errorf("cache can not exceed %d", c.maximumDiskSize)
	}

	for c.currentDiskSize > target {
		_, _, err := c.RemoveOldest()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *PersistentCache) Set(key string, value []byte) error {

	if len(value) > int(c.maximumDiskSize) {
		return fmt.Errorf("value of [%s] is too big for the cache : %d", key, c.maximumDiskSize)
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lruCache.Add(key, struct{}{})
	err := c.db.Store(key, value)
	if err != nil {
		c.lruCache.Remove(key)
		return err
	}
	c.currentDiskSize += uint(len(value))
	return nil
}

func (c *PersistentCache) Get(key string) ([]byte, error) {
	ok := c.Contains(key)
	if !ok {
		return nil, fmt.Errorf("Key not found")
	}

	res, err := c.db.Get(key)
	if err != nil {
		_ = c.Remove(key)
		return nil, err
	}

	return res, nil
}

func (c *PersistentCache) Remove(key string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ok := c.lruCache.Remove(key)
	if !ok {
		return fmt.Errorf("entry [%s] does not exist in the cache", key)
	}

	val, err := c.db.Delete(key)
	c.currentDiskSize -= uint(len(val))
	return err
}

type TrivyCache struct {
	Cache Cache
}

func NewTrivyCache(cache Cache) *TrivyCache {
	return &TrivyCache{
		Cache: cache,
	}
}

// MissingBlobs returns missing blob IDs such as layer IDs in cache
func (c *TrivyCache) MissingBlobs(artifactID string, blobIDs []string) (bool, []string, error) {
	var missingBlobIDs []string
	for _, blobID := range blobIDs {
		if ok := c.Cache.Contains(blobID); !ok {
			missingBlobIDs = append(missingBlobIDs, blobID)
		}
	}
	return c.Cache.Contains(artifactID), missingBlobIDs, nil
}

func trivyCachePut[T any](cache *TrivyCache, id string, info T) error {
	objectBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("error converting object with ID %q to JSON: %w", id, err)
	}
	return cache.Cache.Set(id, objectBytes)
}

// PutArtifact stores artifact information such as image metadata in cache
func (c *TrivyCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) error {
	return trivyCachePut(c, artifactID, artifactInfo)
}

// PutBlob stores blob information such as layer information in local cache
func (c *TrivyCache) PutBlob(blobID string, blobInfo types.BlobInfo) error {
	return trivyCachePut(c, blobID, blobInfo)
}

// DeleteBlobs removes blobs by IDs
func (c *TrivyCache) DeleteBlobs(blobIDs []string) error {
	err := make([]error, len(blobIDs))
	for i, blobID := range blobIDs {
		err[i] = c.Cache.Remove(blobID)
	}
	return kerrors.NewAggregate(err)
}
