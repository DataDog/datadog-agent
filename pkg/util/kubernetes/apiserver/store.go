package apiserver

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/patrickmn/go-cache"
)

type metaBundleStore struct {
	mu      sync.RWMutex
	cache   *cache.Cache
	keyFunc func(...string) string
}

func (m *metaBundleStore) Get(nodeName string) (*MetadataMapperBundle, bool) {
	cacheKey := m.keyFunc(metadataMapperCachePrefix, nodeName)

	var metaBundle *MetadataMapperBundle

	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}

	metaBundle, ok = v.(*MetadataMapperBundle)
	if !ok {
		log.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
		return nil, false
	}

	return metaBundle, true
}

func (m *metaBundleStore) GetOrCreate(nodeName string) *MetadataMapperBundle {
	cacheKey := m.keyFunc(metadataMapperCachePrefix, nodeName)

	var metaBundle *MetadataMapperBundle

	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.cache.Get(cacheKey)
	if ok {
		metaBundle, ok := v.(*MetadataMapperBundle)
		if ok {
			return metaBundle
		}
		log.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
	}

	metaBundle = newMetadataMapperBundle()

	m.cache.Set(cacheKey, metaBundle, cache.NoExpiration)

	return metaBundle
}

func (m *metaBundleStore) Set(nodeName string, metaBundle *MetadataMapperBundle) {
	cacheKey := m.keyFunc(metadataMapperCachePrefix, nodeName)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Set(cacheKey, metaBundle, cache.NoExpiration)
}

func (m *metaBundleStore) Delete(nodeName string) {
	cacheKey := m.keyFunc(metadataMapperCachePrefix, nodeName)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Delete(cacheKey)
}
