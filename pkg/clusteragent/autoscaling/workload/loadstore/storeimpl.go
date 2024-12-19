// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var _ Store = (*EntityStore)(nil)

type dataItem struct {
	entity       *Entity
	valueQue     EntityValueQueue // value queue
	lastActiveTs Timestamp
}

// EntityStore stores entities to values, hash keys to entities mapping.
type EntityStore struct {
	key2ValuesMap map[uint64]*dataItem                                 // Maps hash key to a entity and its values
	keyAttrTable  map[string]map[string]map[string]map[uint64]struct{} // map namespace -> deployment -> loadName -> hashKeys
	lock          sync.RWMutex                                         // Protects access to store and entityMap
}

// NewEntityStore creates a new EntityStore.
func NewEntityStore(ctx context.Context) *EntityStore {
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[string]map[string]map[string]map[uint64]struct{}),
	}
	store.startCleanupInBackground(ctx)
	return &store
}

// SetEntitiesValues inserts entities into the store.
func (es *EntityStore) SetEntitiesValues(entities map[*Entity]*EntityValue) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	for entity, value := range entities {
		if entity.Deployment == "" || entity.LoadName == "" || entity.Namespace == "" {
			log.Tracef("Skipping entity with empty namespace, deployment or loadName: %v", entity)
			continue
		}
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
		// Update the key attribute table
		if _, ok := es.keyAttrTable[entity.Namespace]; !ok {
			es.keyAttrTable[entity.Namespace] = make(map[string]map[string]map[uint64]struct{})
		}
		if _, ok := es.keyAttrTable[entity.Namespace][entity.Deployment]; !ok {
			es.keyAttrTable[entity.Namespace][entity.Deployment] = make(map[string]map[uint64]struct{})
		}
		if _, ok := es.keyAttrTable[entity.Namespace][entity.Deployment][entity.LoadName]; !ok {
			es.keyAttrTable[entity.Namespace][entity.Deployment][entity.LoadName] = make(map[uint64]struct{})
		}
		es.keyAttrTable[entity.Namespace][entity.Deployment][entity.LoadName][hash] = struct{}{}
	}
}

// GetEntitiesStats to get all entities by given search filters
func (es *EntityStore) GetEntitiesStats(namespace string, deployment string, loadName string) StatsResult {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	return es.calculateStatsByConditions(namespace, deployment, loadName)
}

// calculateStatsByFilter is an internal function to scan entire table (slow). Caller should acquire lock.
func (es *EntityStore) calculateStatsByFilter(namespace string, deployment string, loadName string) StatsResult {
	filter1 := namespaceFilter{namespace: namespace}
	filter2 := deploymentFilter{deployment: deployment}
	filter3 := loadNameFilter{loadName: loadName}
	filter := newANDEntityFilter(&filter1, &filter2, &filter3)
	rawData := make([]ValueType, 0, len(es.key2ValuesMap))
	for _, dataItem := range es.key2ValuesMap {
		entity := dataItem.entity
		if !filter.IsIncluded(entity) {
			continue
		}
		entityValue := dataItem.valueQue.value()
		rawData = append(rawData, entityValue)
	}
	return es.statsCalc(rawData, namespace, deployment, loadName)
}

func (es *EntityStore) calculateStatsByConditions(namespace string, deployment string, loadName string) StatsResult {
	rawData := make([]ValueType, 0, len(es.key2ValuesMap))
	if namespace == "" || deployment == "" || loadName == "" {
		// scan entire table (slow)
		return es.calculateStatsByFilter(namespace, deployment, loadName)
	}

	// Traverse the keyAttrTable based on provided conditions
	if nsMap, nsExists := es.keyAttrTable[namespace]; nsExists {
		// Check if deployment is provided and exists in the namespace map
		if depMap, depExists := nsMap[deployment]; depExists {
			// Check if loadName is provided and exists in the deployment map
			if loadMap, loadExists := depMap[loadName]; loadExists {
				// Populate rawData with hash keys
				for hash := range loadMap {
					rawData = append(rawData, es.key2ValuesMap[hash].valueQue.value())
				}
			}
		}
	}
	return es.statsCalc(rawData, namespace, deployment, loadName)
}

func (es *EntityStore) statsCalc(rawData []ValueType, namespace string, deployment string, loadName string) StatsResult {
	count := len(rawData)
	if count == 0 {
		return StatsResult{
			Namespace:  namespace,
			Deployment: deployment,
			LoadName:   loadName,
			Count:      0,
			Min:        0,
			Max:        0,
			Avg:        0,
			Medium:     0,
			P10:        0,
			P95:        0,
			P99:        0,
		}
	}
	sum := ValueType(0)
	for _, value := range rawData {
		sum += value
	}
	sort.Slice(rawData, func(i, j int) bool { return rawData[i] < rawData[j] })
	percentileIndexFunc := func(percentile float64) int {
		return int(math.Floor(float64(len(rawData)-1) * percentile))
	}
	return StatsResult{
		Namespace:  namespace,
		Deployment: deployment,
		LoadName:   loadName,
		Count:      count,
		Min:        rawData[0],
		Max:        rawData[len(rawData)-1],
		Avg:        sum / ValueType(count),
		Medium:     rawData[percentileIndexFunc(0.5)],
		P10:        rawData[percentileIndexFunc(0.1)],
		P95:        rawData[percentileIndexFunc(0.95)],
		P99:        rawData[percentileIndexFunc(0.99)],
	}
}

func (es *EntityStore) deleteInternal(hash uint64) {
	if dataItem, exists := es.key2ValuesMap[hash]; exists {
		// Remove the entity from the keyAttrTable
		entity := dataItem.entity
		delete(es.keyAttrTable[entity.Namespace][entity.Deployment][entity.LoadName], hash)
		// Remove the entity from the key2ValuesMap
	}
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
	var results []*StatsResult
	// Iterate through namespaces
	for namespace, nsMap := range es.keyAttrTable {
		// Iterate through deployments
		for deployment, depMap := range nsMap {
			// Iterate through loadNames
			for loadName, entities := range depMap {
				if len(entities) > 0 {
					var rawData []ValueType
					// Populate rawData with hash keys
					for hash := range entities {
						rawData = append(rawData, es.key2ValuesMap[hash].valueQue.value())
					}
					// Generate StatsResult and add to results
					statsResult := es.statsCalc(rawData, namespace, deployment, loadName)
					results = append(results, &statsResult)
				} else {
					results = append(results, &StatsResult{
						Namespace:  namespace,
						Deployment: deployment,
						LoadName:   loadName,
						Count:      0,
						Min:        0,
						Max:        0,
						Avg:        0,
						Medium:     0,
						P10:        0,
						P95:        0,
						P99:        0,
					})
				}
			}
		}
	}

	return StoreInfo{
		currentTime:  getCurrentTime(),
		StatsResults: results,
	}
}
