// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import (
	"sync"

	"github.com/patrickmn/go-cache"

	agentcache "github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// globalMetaBundleStore uses the global cache instance for the Agent.
var globalMetaBundleStore = &MetaBundleStore{
	cache:       agentcache.Cache,
	subscribers: make(map[string]chan struct{}),
}

// GetGlobalMetaBundleStore returns the global MetaBundleStore instance.
func GetGlobalMetaBundleStore() *MetaBundleStore {
	return globalMetaBundleStore
}

// MetaBundleStore is a cache for metadataMapperBundles for each node in the cluster
// and allows multiple goroutines to safely get or create meta bundles for the same nodes
// without overwriting each other.
type MetaBundleStore struct {
	mu sync.RWMutex

	// we don't expire items in the cache and instead rely on the `metadataController`
	// to delete items for nodes that were deleted in the apiserver to prevent data
	// from going missing until the next resync period.
	cache *cache.Cache

	// subscribers holds a notification channel per node name.
	// When a bundle is updated or deleted, the subscriber for that node
	// receives a signal.
	subscribers map[string]chan struct{}
}

// Get returns the bundle for a given node, or false if not found.
func (m *MetaBundleStore) Get(nodeName string) (*apiserver.MetadataMapperBundle, bool) {
	cacheKey := agentcache.BuildAgentKey(apiserver.MetadataMapperCachePrefix, nodeName)

	var metaBundle *apiserver.MetadataMapperBundle

	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}

	metaBundle, ok = v.(*apiserver.MetadataMapperBundle)
	if !ok {
		log.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
		return nil, false
	}

	return metaBundle, true
}

func (m *MetaBundleStore) getCopyOrNew(nodeName string) *apiserver.MetadataMapperBundle {
	cacheKey := agentcache.BuildAgentKey(apiserver.MetadataMapperCachePrefix, nodeName)

	metaBundle := apiserver.NewMetadataMapperBundle()

	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.cache.Get(cacheKey)
	if ok {
		oldMetaBundle, ok := v.(*apiserver.MetadataMapperBundle)
		if !ok {
			log.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
		} else {
			metaBundle.DeepCopy(oldMetaBundle)
		}
	}

	return metaBundle
}

func (m *MetaBundleStore) set(nodeName string, metaBundle *apiserver.MetadataMapperBundle) {
	cacheKey := agentcache.BuildAgentKey(apiserver.MetadataMapperCachePrefix, nodeName)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Set(cacheKey, metaBundle, cache.NoExpiration)
	m.notifyLocked(nodeName)
}

func (m *MetaBundleStore) delete(nodeName string) {
	cacheKey := agentcache.BuildAgentKey(apiserver.MetadataMapperCachePrefix, nodeName)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Delete(cacheKey)
	m.notifyLocked(nodeName)
}

// Subscribe registers interest in changes for a given node and returns a
// notification channel. The channel receives a signal when the bundle for this
// node is updated or deleted. The caller should call Unsubscribe when done.
func (m *MetaBundleStore) Subscribe(nodeName string) <-chan struct{} {
	ch := make(chan struct{}, 1)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscribers[nodeName]; exists {
		log.Warnf("Overwriting existing subscriber for node %s", nodeName)
	}

	m.subscribers[nodeName] = ch
	return ch
}

// Unsubscribe removes a subscriber.
func (m *MetaBundleStore) Unsubscribe(nodeName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.subscribers, nodeName)
}

// notifyLocked signals the subscriber for the given node.
// Must be called with mutex held.
func (m *MetaBundleStore) notifyLocked(nodeName string) {
	if ch, ok := m.subscribers[nodeName]; ok {
		// Non-blocking send: if a signal is already pending, we drop it. This
		// is safe because the consumer re-reads the full state from the store
		// on each signal.
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
