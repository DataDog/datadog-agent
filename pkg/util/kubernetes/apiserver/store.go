// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"sync"

	agentcache "github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/patrickmn/go-cache"
)

// globalMetaBundleStore uses the global cache instance for the Agent.
var globalMetaBundleStore = &metaBundleStore{
	cache: agentcache.Cache,
}

// metaBundleStore is a cache for metadataMapperBundles for each node in the cluster
// and allows multiple goroutines to safely get or create meta bundles for the same nodes
// without overwriting each other.
type metaBundleStore struct {
	mu sync.RWMutex

	// we don't expire items in the cache and instead rely on the `MetadataController`
	// to delete items for nodes that were deleted in the apiserver to prevent data
	// from going missing until the next resync period.
	cache *cache.Cache
}

func (m *metaBundleStore) get(nodeName string) (*metadataMapperBundle, bool) {
	cacheKey := agentcache.BuildAgentKey(metadataMapperCachePrefix, nodeName)

	var metaBundle *metadataMapperBundle

	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}

	metaBundle, ok = v.(*metadataMapperBundle)
	if !ok {
		log.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
		return nil, false
	}

	return metaBundle, true
}

func (m *metaBundleStore) getCopyOrNew(nodeName string) *metadataMapperBundle {
	cacheKey := agentcache.BuildAgentKey(metadataMapperCachePrefix, nodeName)

	metaBundle := newMetadataMapperBundle()

	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.cache.Get(cacheKey)
	if ok {
		oldMetaBundle, ok := v.(*metadataMapperBundle)
		if !ok {
			log.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
		} else {
			metaBundle.DeepCopy(oldMetaBundle)
		}
	}

	return metaBundle
}

func (m *metaBundleStore) set(nodeName string, metaBundle *metadataMapperBundle) {
	cacheKey := agentcache.BuildAgentKey(metadataMapperCachePrefix, nodeName)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Set(cacheKey, metaBundle, cache.NoExpiration)
}

func (m *metaBundleStore) delete(nodeName string) {
	cacheKey := agentcache.BuildAgentKey(metadataMapperCachePrefix, nodeName)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Delete(cacheKey)
}
