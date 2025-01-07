// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var _ Store = (*EntityStore)(nil)

type dataItem struct {
	entity       *Entity
	valueQue     EntityValueQueue // value queue, default 3 data points
	lastActiveTs Timestamp        // last active timestamp
}

func convertsToEntityValueSlice(data []*EntityValue) []EntityValue {
	result := make([]EntityValue, len(data))
	for i, v := range data {
		if v != nil {
			result[i] = *v
		}
	}
	return result
}

// compositeKey is a hash id of composite key for the keyAttrTable, which is used for quick filtering
type compositeKey uint64

func generateCompositeKey(namespace, podOwnerName, metricName string) compositeKey {
	return compositeKey(generateHash(namespace, podOwnerName, metricName))
}

// dataPerPod stores the mapping between contaienr name and entity hash id and pod level entity hash id if available
// {containerName: entityHashId, containerName2: entityHashId2...}
type dataPerPod struct {
	containers  map[string]uint64 // map container name -> entity hash id
	podEntityID uint64            // pod level entity hash id, if not available, it will be 0
}

// podList has a map of pod name (i.e. pod name: expod-hash1-hash2 ) to dataPerPod
type podList struct {
	pods         map[string]*dataPerPod
	namespace    string
	podOwnerName string
	metricName   string
}

// EntityStore manages mappings between entities and their hashed keys.
type EntityStore struct {
	key2ValuesMap map[uint64]*dataItem     // Maps hash(entity) to a dataitem (entity and its values)
	keyAttrTable  map[compositeKey]podList // map Hash<namespace, deployment, metricName> -> pod name ->  dataPerPod
	lock          sync.RWMutex             // Protects access to store and entityMap
}

// NewEntityStore creates a new EntityStore.
func NewEntityStore(ctx context.Context) *EntityStore {
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
	}
	store.startCleanupInBackground(ctx)
	return &store
}

// SetEntitiesValues inserts entities into the store.
func (es *EntityStore) SetEntitiesValues(entities map[*Entity]*EntityValue) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	for entity, value := range entities {
		if entity.EntityName == "" || entity.MetricName == "" || entity.Namespace == "" || entity.PodOwnerName == "" {
			log.Tracef("Skipping entity with empty entityName, podOwnerName, namespace or metricName: %v", entity)
			continue
		}
		entityHash := hashEntityToUInt64(entity)
		data, exists := es.key2ValuesMap[entityHash]
		if !exists {
			data = &dataItem{
				entity: entity,
				valueQue: EntityValueQueue{
					data:     make([]*EntityValue, maxDataPoints),
					head:     0,
					tail:     0,
					size:     0,
					capacity: maxDataPoints,
				},
				lastActiveTs: value.timestamp,
			}
			data.valueQue.pushBack(value)
			es.key2ValuesMap[entityHash] = data
		} else {
			if data.lastActiveTs < value.timestamp {
				// Update the last active timestamp
				data.lastActiveTs = value.timestamp
				data.valueQue.pushBack(value)
			} //else if lastActiveTs is greater than value.timestamp, skip the value because it is outdated
		}

		// Update the key attribute table
		compositeKeyHash := generateCompositeKey(entity.Namespace, entity.PodOwnerName, entity.MetricName)
		if _, ok := es.keyAttrTable[compositeKeyHash]; !ok {
			es.keyAttrTable[compositeKeyHash] = podList{
				pods:         make(map[string]*dataPerPod),
				namespace:    entity.Namespace,
				podOwnerName: entity.PodOwnerName,
				metricName:   entity.MetricName,
			}
		}
		if _, ok := (es.keyAttrTable[compositeKeyHash].pods)[entity.PodName]; !ok {
			(es.keyAttrTable[compositeKeyHash].pods)[entity.PodName] = &dataPerPod{
				containers:  make(map[string]uint64),
				podEntityID: 0,
			}
		}
		// Update the pod level entity hash id
		if entity.EntityType == PodType {
			(es.keyAttrTable[compositeKeyHash].pods)[entity.PodName].podEntityID = entityHash
		}
		if entity.EntityType == ContainerType {
			(es.keyAttrTable[compositeKeyHash].pods)[entity.PodName].containers[entity.ContainerName] = entityHash
		}
	}
}

/*
GetMetricsRaw to get all entities by given search filters

	metricName: required
	namespace: required
	podOwnerName: required
	containerName: optional
*/
func (es *EntityStore) GetMetricsRaw(metricName string,
	namespace string,
	podOwnerName string,
	containerName string) QueryResult {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	compositeKeyHash := generateCompositeKey(namespace, podOwnerName, metricName)
	podList, ok := es.keyAttrTable[compositeKeyHash]
	if !ok {
		return QueryResult{}
	}
	var result QueryResult
	for podName, dataPerPod := range podList.pods {
		if dataPerPod.podEntityID != 0 { // if it is a pod level entity
			entity := es.key2ValuesMap[dataPerPod.podEntityID]
			podResult := PodResult{
				PodName:       podName,
				PodLevelValue: convertsToEntityValueSlice(entity.valueQue.data),
			}
			result.results = append(result.results, podResult)
		} else {
			podList := PodResult{
				PodName:         podName,
				ContainerValues: make(map[string][]EntityValue),
			}
			for containerNameKey, entityHash := range dataPerPod.containers {
				if containerName != "" && containerName != containerNameKey {
					continue
				}
				entity := es.key2ValuesMap[entityHash]
				podList.ContainerValues[containerNameKey] = convertsToEntityValueSlice(entity.valueQue.data)
			}
			if len(podList.ContainerValues) > 0 {
				result.results = append(result.results, podList)
			}
		}
	}
	return result
}

func (es *EntityStore) deleteInternal(hash uint64) {
	if toBeDelItem, exists := es.key2ValuesMap[hash]; exists { // find the entity to delete
		compositeKeyHash := generateCompositeKey(toBeDelItem.entity.Namespace, toBeDelItem.entity.PodOwnerName, toBeDelItem.entity.MetricName) // calculate the composite key
		if _, ok := es.keyAttrTable[compositeKeyHash]; ok {                                                                                    // search the composite key in the lookup table
			if dataPerPod, ok := (es.keyAttrTable[compositeKeyHash].pods)[toBeDelItem.entity.PodName]; ok { // search the pod name in the lookup table
				// Delete the container from the pod
				if toBeDelItem.entity.EntityType == ContainerType {
					delete(dataPerPod.containers, toBeDelItem.entity.ContainerName) // delete the container from the pod
				}
				// Delete the pod from the keyAttrTable if there is no container
				if toBeDelItem.entity.EntityType == PodType ||
					(len(dataPerPod.containers) == 0 && dataPerPod.podEntityID == 0) {
					delete((es.keyAttrTable[compositeKeyHash].pods), toBeDelItem.entity.PodName)
				}
			}
		}
		// Delete the entity from the key2ValuesMap
		delete(es.key2ValuesMap, hash)
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

// GetStoreInfo returns the store information, aggregated by namespace, podOwner, and metric name
func (es *EntityStore) GetStoreInfo() StoreInfo {
	es.lock.RLock()
	defer es.lock.RUnlock()
	var storeInfo StoreInfo
	for _, podList := range es.keyAttrTable {
		namespace := podList.namespace
		podOwnerName := podList.podOwnerName
		metricName := podList.metricName
		count := 0
		for _, dataPerPod := range podList.pods {
			count += len(dataPerPod.containers)
			if dataPerPod.podEntityID != 0 {
				count++
			}
		}
		storeInfo.StatsResults = append(storeInfo.StatsResults, &StatsResult{
			Namespace:  namespace,
			PodOwner:   podOwnerName,
			MetricName: metricName,
			Count:      count,
		})
	}
	storeInfo.currentTime = getCurrentTime()
	return storeInfo
}
