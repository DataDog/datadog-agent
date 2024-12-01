// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Enumeration of filter types.
type filterType int

// EntityValueQueue represents a queue with a fixed capacity that removes the front element when full
type EntityValueQueue struct {
	data     []ValueType
	head     int
	tail     int
	size     int
	capacity int
}

// pushBack adds an element to the back of the queue.
// If the queue is full, it removes the front element first.
func (q *EntityValueQueue) pushBack(value ValueType) bool {
	if q.size == q.capacity {
		// Remove the front element
		q.head = (q.head + 1) % q.capacity
		q.size--
	}

	// Add the new element at the back
	q.data[q.tail] = value
	q.tail = (q.tail + 1) % q.capacity
	q.size++
	return true
}

// len returns the number of elements in the queue
func (q *EntityValueQueue) len() int {
	return q.size
}

// average calculates the average value of the queue.
func (q *EntityValueQueue) average() ValueType {
	if q.size == 0 {
		return 0
	}
	sum := ValueType(0)
	for i := 0; i < q.size; i++ {
		index := (q.head + i) % q.capacity
		sum += q.data[index]
	}
	return sum / ValueType(q.size)
}

// top returns the latest element of the queue
func (q *EntityValueQueue) top() (ValueType, error) {
	if q.size == 0 {
		return 0, fmt.Errorf("queue is empty")
	}
	return q.data[(q.tail-1+q.capacity)%q.capacity], nil
}

var _ Store = (*EntityStore)(nil)

type dataItem struct {
	entity       *Entity
	valueQue     EntityValueQueue // value queue
	lastActiveTs Timestamp
}

// EntityStore stores entities to values, hash keys to entities mapping.
type EntityStore struct {
	key2ValuesMap   map[uint64]*dataItem // Maps hash key to a entity and its values
	lock            sync.RWMutex         // Protects access to store and entityMap
	namespaces      map[string]struct{}  // Set of namespaces
	loadNames       map[string]struct{}  // Set of load names
	deploymentNames map[string]struct{}  // Set of deployment names
}

// NewEntityStore creates a new EntityStore.
func NewEntityStore(ctx context.Context) *EntityStore {
	store := EntityStore{
		key2ValuesMap:   make(map[uint64]*dataItem),
		namespaces:      make(map[string]struct{}),
		loadNames:       make(map[string]struct{}),
		deploymentNames: make(map[string]struct{}),
	}
	store.startCleanupInBackground(ctx)
	return &store
}

// SetEntitiesValues inserts entities into the store.
func (es *EntityStore) SetEntitiesValues(entities map[*Entity]*EntityValue) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	for entity, value := range entities {
		es.namespaces[entity.Namespace] = struct{}{}
		es.loadNames[entity.LoadName] = struct{}{}
		es.deploymentNames[entity.Deployment] = struct{}{}
		hash := hashEntityToUInt64(entity)
		data, exists := es.key2ValuesMap[hash]
		if !exists {
			data = &dataItem{
				entity: entity,
				valueQue: EntityValueQueue{
					data:     make([]ValueType, maxDataPoints),
					head:     0,
					tail:     0,
					size:     0,
					capacity: maxDataPoints,
				},
				lastActiveTs: value.timestamp,
			}
			data.valueQue.pushBack(value.value)
			es.key2ValuesMap[hash] = data
		} else {
			if data.lastActiveTs < value.timestamp {
				// Update the last active timestamp
				data.lastActiveTs = value.timestamp
				data.valueQue.pushBack(value.value)
			}
		}
	}
}

// GetEntitiesByLoadName to get all entities by load name
func (es *EntityStore) GetEntitiesStatsByLoadName(loadName string) StatsResult {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	filter := loadNameFilter{loadName: loadName}
	return es.calculateStatsByFilter(newANDEntityFilter(&filter))
}

// GetEntitiesByNamespace to get all entities by namespace
func (es *EntityStore) GetEntitiesStatsByNamespace(namespace string) StatsResult {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	filter := namespaceFilter{namespace: namespace}
	return es.calculateStatsByFilter(newANDEntityFilter(&filter))
}

// GetEntitiesByDeployment to get all entities by deployment
func (es *EntityStore) GetEntitiesStatsByDeployment(deployment string) StatsResult {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	filter := deploymentFilter{deployment: deployment}
	return es.calculateStatsByFilter(newANDEntityFilter(&filter))
}

// calculateStatsByFilter is an internal function. Caller should acquire lock.
func (es *EntityStore) calculateStatsByFilter(filter *ANDEntityFilter) StatsResult {
	rawData := make([]ValueType, 0, len(es.key2ValuesMap))
	sum := ValueType(0)
	for _, dataItem := range es.key2ValuesMap {
		entity := dataItem.entity
		if !filter.IsIncluded(entity) {
			continue
		}
		entityValue := dataItem.valueQue.average()
		rawData = append(rawData, entityValue)
		sum += entityValue
	}
	count := uint64(len(rawData))
	if count == 0 {
		return StatsResult{
			Count:  count,
			Min:    0,
			Max:    0,
			Avg:    0,
			Medium: 0,
			P10:    0,
			P95:    0,
			P99:    0,
		}
	}
	sort.Slice(rawData, func(i, j int) bool { return rawData[i] < rawData[j] })
	percentileIndexFunc := func(percentile float64) int {
		return int(math.Floor(float64(len(rawData)-1) * percentile))
	}
	statsResult := StatsResult{
		Count:  count,
		Min:    rawData[0],
		Max:    rawData[len(rawData)-1],
		Avg:    sum / ValueType(count),
		Medium: rawData[percentileIndexFunc(0.5)],
		P10:    rawData[percentileIndexFunc(0.1)],
		P95:    rawData[percentileIndexFunc(0.95)],
		P99:    rawData[percentileIndexFunc(0.99)],
	}
	return statsResult
}

func (es *EntityStore) deleteInternal(hash uint64) {
	delete(es.key2ValuesMap, hash)
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

// GetStoreInfo returns the store information.
func (es *EntityStore) GetStoreInfo() StoreInfo {
	es.lock.RLock()
	defer es.lock.RUnlock()
	storeInfo := StoreInfo{
		TotalEntityCount:        0,
		currentTime:             getCurrentTime(),
		EntityStatsByLoadName:   make(map[string]StatsResult),
		EntityStatsByNamespace:  make(map[string]StatsResult),
		EntityStatsByDeployment: make(map[string]StatsResult),
	}
	// Constructing the information string
	storeInfo.TotalEntityCount = uint64(len(es.key2ValuesMap))

	for namespace := range es.namespaces {
		namespaceFilter := namespaceFilter{namespace: namespace}
		storeInfo.EntityStatsByNamespace[namespace] = es.calculateStatsByFilter(newANDEntityFilter(&namespaceFilter))
	}
	for loadName := range es.loadNames {
		loadNameFilter := loadNameFilter{loadName: loadName}
		storeInfo.EntityStatsByLoadName[loadName] = es.calculateStatsByFilter(newANDEntityFilter(&loadNameFilter))
	}

	for deployment := range es.deploymentNames {
		deploymentFilter := deploymentFilter{deployment: deployment}
		storeInfo.EntityStatsByDeployment[deployment] = es.calculateStatsByFilter(newANDEntityFilter(&deploymentFilter))
	}
	return storeInfo
}
