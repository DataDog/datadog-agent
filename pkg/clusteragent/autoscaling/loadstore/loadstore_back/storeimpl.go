// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EntityValueHeap is a min-heap of EntityValues based on timestamp.
type EntityValueHeap []*EntityValue

func (h EntityValueHeap) Len() int           { return len(h) }
func (h EntityValueHeap) Less(i, j int) bool { return h[i].timestamp < h[j].timestamp }
func (h EntityValueHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *EntityValueHeap) Push(x interface{}) {
	*h = append(*h, x.(*EntityValue))
}

func (h *EntityValueHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

var _ Store = (*EntityStore)(nil)

type dataItem struct {
	entity       *Entity
	valuesWithTs *EntityValueHeap // values with timestamp
	lastActiveTs Timestamp
}

func (d *dataItem) memoryUsage() uint32 {
	return uint32(unsafe.Sizeof(&Entity{})) + uint32(unsafe.Sizeof(&EntityValue{}))*uint32(len(*d.valuesWithTs)) + uint32(unsafe.Sizeof(uint32(0)))
}

// EntityStore stores entities to values, hash keys to entities mapping.
type EntityStore struct {
	key2ValuesMap     map[uint64]*dataItem           // Maps hash key to a entity and its values
	metric2KeysMap    map[string]map[uint64]struct{} // Maps metric name to a set of keys
	namespace2KeysMap map[string]map[uint64]struct{} // Maps namespace to a set of keys
	lock              sync.RWMutex                   // Protects access to store and entityMap
}

// NewEntityStore creates a new EntityStore.
func NewEntityStore(ctx context.Context) *EntityStore {
	store := EntityStore{
		key2ValuesMap:     make(map[uint64]*dataItem),
		metric2KeysMap:    make(map[string]map[uint64]struct{}),
		namespace2KeysMap: make(map[string]map[uint64]struct{}),
	}
	store.startCleanupInBackground(ctx)
	return &store
}

// SetEntitiesValues inserts entities into the store.
func (es *EntityStore) SetEntitiesValues(entities map[*Entity]*EntityValue) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	for entity, value := range entities {
		hash := hashEntityToUInt64(entity)
		if _, exists := es.key2ValuesMap[hash]; !exists {
			valueHeap := &EntityValueHeap{}
			heap.Init(valueHeap)
			es.key2ValuesMap[hash] = &dataItem{
				entity:       entity,
				valuesWithTs: valueHeap,
				lastActiveTs: value.timestamp,
			}
			heap.Push(es.key2ValuesMap[hash].valuesWithTs, value)
		} else {
			if value.timestamp > es.key2ValuesMap[hash].lastActiveTs {
				es.key2ValuesMap[hash].lastActiveTs = value.timestamp
			}
			heap.Push(es.key2ValuesMap[hash].valuesWithTs, value)
		}
		if es.key2ValuesMap[hash].valuesWithTs.Len() > maxDataPoints {
			heap.Pop(es.key2ValuesMap[hash].valuesWithTs) // Pop the earliest value if more than maxDataPoints
		}

		if _, exists := es.metric2KeysMap[entity.MetricName]; !exists {
			es.metric2KeysMap[entity.MetricName] = make(map[uint64]struct{})
		}
		es.metric2KeysMap[entity.MetricName][hash] = struct{}{}
		if _, exists := es.namespace2KeysMap[entity.Namespace]; !exists {
			es.namespace2KeysMap[entity.Namespace] = make(map[uint64]struct{})
		}
		es.namespace2KeysMap[entity.Namespace][hash] = struct{}{}
	}
}

func (es *EntityStore) getEntityByHashKeyInternal(hash uint64) (*Entity, *EntityValue) {
	entityValues, exists := es.key2ValuesMap[hash]
	if !exists || entityValues.entity == nil || entityValues.valuesWithTs.Len() == 0 {
		return nil, nil
	}
	for _, v := range *entityValues.valuesWithTs {
		if v.timestamp == entityValues.lastActiveTs {
			return entityValues.entity, v
		}
	}
	return nil, nil
}

// GetEntityByHashKey to get entity and latest value by hash key
func (es *EntityStore) GetEntityByHashKey(hash uint64) (*Entity, *EntityValue) {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	return es.getEntityByHashKeyInternal(hash)
}

// GetEntitiesByMetricName to get all entities by metric name
func (es *EntityStore) GetEntitiesByMetricName(metricName string) map[*Entity]*EntityValue {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	result := make(map[*Entity]*EntityValue)
	keys, exists := es.metric2KeysMap[metricName]
	if !exists {
		return result
	}
	for key := range keys {
		entity, value := es.getEntityByHashKeyInternal(key)
		result[entity] = value
	}
	return result
}

// GetEntitiesByNamespace to get all entities by namespace
func (es *EntityStore) GetEntitiesByNamespace(namespace string) map[*Entity]*EntityValue {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	result := make(map[*Entity]*EntityValue)
	keys, exists := es.namespace2KeysMap[namespace]
	if !exists {
		return result
	}
	for key := range keys {
		entity, value := es.getEntityByHashKeyInternal(key)
		result[entity] = value
	}
	return result
}

func (es *EntityStore) deleteInternal(hash uint64) {
	entityValueBlob, exists := es.key2ValuesMap[hash]
	if !exists || entityValueBlob == nil || entityValueBlob.entity == nil {
		return
	}
	delete(es.key2ValuesMap, hash)
	delete(es.metric2KeysMap[entityValueBlob.entity.MetricName], hash)
	delete(es.namespace2KeysMap[entityValueBlob.entity.Namespace], hash)
	if len(es.metric2KeysMap[entityValueBlob.entity.MetricName]) == 0 {
		delete(es.metric2KeysMap, entityValueBlob.entity.MetricName)
	}
	if len(es.namespace2KeysMap[entityValueBlob.entity.Namespace]) == 0 {
		delete(es.namespace2KeysMap, entityValueBlob.entity.Namespace)
	}
}

// DeleteEntityByHashKey deltes an entity from the store.
func (es *EntityStore) DeleteEntityByHashKey(hash uint64) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	es.deleteInternal(hash)
}

// purgeInactiveEntities purges inactive entities.
func (es *EntityStore) purgeInactiveEntities(purgeInterval time.Duration) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	for hash, entityValueBlob := range es.key2ValuesMap {
		lastActive := entityValueBlob.lastActiveTs
		if time.Since(time.Unix(int64(lastActive), 0)) > purgeInterval {
			es.deleteInternal(hash)
		}
	}
}

// startCleanupInBackground purges expired entities periodically.
func (es *EntityStore) startCleanupInBackground(ctx context.Context) {
	log.Infof("Starting entity store cleanup")
	// Launch periodic cleanup mechanism
	go func() {
		ticker := time.NewTicker(defaultPurgeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				es.purgeInactiveEntities(defaultExpireInterval)
			case <-ctx.Done():
				break
			}
		}
	}()
}

// GetAllMetricNamesWithCount returns all metric names.
func (es *EntityStore) GetAllMetricNamesWithCount() map[string]int64 {
	es.lock.RLock()
	defer es.lock.RUnlock()
	metricNames := make(map[string]int64)
	for metric, keys := range es.metric2KeysMap {
		metricNames[metric] = int64(len(keys))
	}
	return metricNames
}

// GetAllNamespaceNamesWithCount returns all namespace names.
func (es *EntityStore) GetAllNamespaceNamesWithCount() map[string]int64 {
	es.lock.RLock()
	defer es.lock.RUnlock()
	namespaceNames := make(map[string]int64)
	for namespace, keys := range es.namespace2KeysMap {
		namespaceNames[namespace] = int64(len(keys))
	}
	return namespaceNames
}

// GetStoreInfo returns the store information.
func (es *EntityStore) GetStoreInfo() string {
	es.lock.RLock()
	defer es.lock.RUnlock()
	// Constructing the information string
	totalMemoryUsage := uint32(0)
	for _, entityValues := range es.key2ValuesMap {
		totalMemoryUsage += entityValues.memoryUsage()
	}
	info := fmt.Sprintf("\n===============================Entity Store Information===============================\n")
	for e, keys := range es.metric2KeysMap {
		info += fmt.Sprintf("Metric Name: %s, Entity Count:: %d\n", e, len(keys))
		totalMemoryUsage += uint32(len(keys)) * uint32(unsafe.Sizeof(uint64(0)))
	}
	for e, keys := range es.namespace2KeysMap {
		info += fmt.Sprintf("Namespace Name: %s, Entity Count:: %d\n", e, len(keys))
		totalMemoryUsage += uint32(len(keys)) * uint32(unsafe.Sizeof(uint64(0)))
	}
	totalMemoryUsage = totalMemoryUsage / 1024 // Convert to KB
	info += fmt.Sprintf("Time: %d, Store total entity count: %d\n, memory usage :%d kB", getCurrentTime(), len(es.key2ValuesMap), totalMemoryUsage)
	return info
}
