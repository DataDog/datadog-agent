// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package series

import (
	"container/heap"
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
)

// EntityType defines the type of entity.
type EntityType int

// Enumeration of entity types.
const (
	ContainerType EntityType = iota
	UnknownType
)

const (
	// maxDataPoints is the maximum number of data points to store per entity.
	maxDataPoints = 3
	// defaultPurgeInterval is the default interval to purge inactive entities.
	defaultPurgeInterval = 5 * time.Minute
	// defaultExpireInterval is the default interval to expire entities.
	defaultExpireInterval = 5 * time.Minute
)

// Entity represents an entity with a type and its attributes.
type Entity struct {
	EntityType EntityType
	SourceID   string
	Host       string // serie.Host
	EntityName string // display_container_name
	Namespace  string
	MetricName string
}

// hashEntityToUInt64 generates an uint64 hash for an Entity.
func hashEntityToUInt64(entity *Entity) uint64 {
	// Initialize a new FNV-1a hasher
	hasher := fnv.New64a()
	// Convert and write the entity's SourceID (string) to the hasher
	hasher.Write([]byte(entity.SourceID))
	// Convert and write the entity's host (string) to the hasher
	hasher.Write([]byte(entity.Host))
	// Convert and write the entity's namespace (string) to the hasher
	hasher.Write([]byte(entity.Namespace))
	// Convert and write the entity's metricname (string) to the hasher
	hasher.Write([]byte(entity.MetricName))
	return hasher.Sum64()
}

// String returns a string representation of the Entity.
func (e *Entity) String() string {
	return fmt.Sprintf(
		"  Key: %d,"+
			"  SourceID: %s,"+
			"  MetricName: %s"+
			"  EntityName: %s,"+
			"  EntityType: %d,"+
			"  Host: %s,"+
			"  Namespace: %s",
		hashEntityToUInt64(e), e.SourceID, e.MetricName, e.EntityName, e.EntityType, e.Host, e.Namespace)
}

type EntityValue struct {
	value     float64
	timestamp uint32
}

// String returns a string representation of the EntityValue.
func (ev *EntityValue) String() string {
	// Convert the timestamp to a time.Time object assuming the timestamp is in seconds.
	// If the timestamp is in milliseconds, use time.UnixMilli(ev.timestamp) instead.
	readableTime := time.Unix(int64(ev.timestamp), 0).Local().Format(time.RFC3339)
	return fmt.Sprintf("Value: %f, Timestamp: %s", ev.value, readableTime)
}

// getCurrentTime returns the current time in uint32
func getCurrentTime() uint32 {
	return uint32(time.Now().Unix())
}

func createEntitiesFromPayload(payload *gogen.MetricPayload) map[Entity][]*EntityValue {
	entities := make(map[Entity][]*EntityValue)
	splitTag := func(tag string) (key string, value string) {
		split := strings.SplitN(tag, ":", 2)
		if len(split) < 2 || split[0] == "" || split[1] == "" {
			return "", ""
		}
		return split[0], split[1]
	}
	for _, series := range payload.Series {
		metricName := series.GetMetric()
		points := series.GetPoints()
		tags := series.GetTags()
		resources := series.GetResources()
		entity := Entity{
			EntityType: UnknownType,
			SourceID:   "",
			Host:       "",
			EntityName: "",
			Namespace:  "",
			MetricName: metricName,
		}
		for _, resource := range resources {
			if resource.Type == "host" {
				entity.Host = resource.Name
			}
		}
		for _, tag := range tags {
			k, v := splitTag(tag)
			switch k {
			case "display_container_name":
				entity.EntityName = v
			case "kube_namespace":
				entity.Namespace = v
			case "container_id":
				entity.SourceID = v
				entity.EntityType = ContainerType
			}
		}
		entityValues := make([]*EntityValue, 0, len(points))
		if entity.MetricName == "" || entity.Host == "" || entity.EntityType == UnknownType || entity.Namespace == "" || entity.SourceID == "" {
			continue
		}
		for _, point := range points {
			if point != nil && !math.IsNaN(point.Value) {
				entityValues = append(entityValues, &EntityValue{
					value:     point.Value,
					timestamp: getCurrentTime(),
				})
			}
		}
		if len(entityValues) > 0 {
			entities[entity] = entityValues
		}
	}
	return entities
}

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

// EntityStore stores entities to values, hash keys to entities mapping.
type EntityStore struct {
	store             map[uint64]*EntityValueHeap    // Maps hash key to a priority queue of EntityValues
	key2EntityMap     map[uint64]*Entity             // Maps hash key to Entity for reverse lookup
	key2LastActiveMap map[uint64]uint32              // Maps hash key to last active time
	metric2KeysMap    map[string]map[uint64]struct{} // Maps metric name to a set of keys
	namespace2KeysMap map[string]map[uint64]struct{} // Maps namespace to a set of keys
	lock              sync.RWMutex                   // Protects access to store and entityMap
}

// NewEntityStore creates a new EntityStore.
func NewEntityStore() *EntityStore {
	return &EntityStore{
		store:             make(map[uint64]*EntityValueHeap),
		key2EntityMap:     make(map[uint64]*Entity),
		metric2KeysMap:    make(map[string]map[uint64]struct{}),
		namespace2KeysMap: make(map[string]map[uint64]struct{}),
		key2LastActiveMap: make(map[uint64]uint32),
	}
}

// Insert an entity with a value into the store.
func (es *EntityStore) Insert(entity *Entity, value *EntityValue) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	hash := hashEntityToUInt64(entity)
	if _, exists := es.store[hash]; !exists {
		es.store[hash] = &EntityValueHeap{}
		heap.Init(es.store[hash])
	}
	heap.Push(es.store[hash], value)
	if es.store[hash].Len() > maxDataPoints {
		heap.Pop(es.store[hash]) // Pop the earliest value if more than 3 values are present
	}
	if _, exists := es.key2EntityMap[hash]; !exists {
		es.key2EntityMap[hash] = entity
	}
	if _, exists := es.metric2KeysMap[entity.MetricName]; !exists {
		es.metric2KeysMap[entity.MetricName] = make(map[uint64]struct{})
	}
	es.metric2KeysMap[entity.MetricName][hash] = struct{}{}
	if _, exists := es.namespace2KeysMap[entity.Namespace]; !exists {
		es.namespace2KeysMap[entity.Namespace] = make(map[uint64]struct{})
	}
	es.namespace2KeysMap[entity.Namespace][hash] = struct{}{}
	es.key2LastActiveMap[hash] = getCurrentTime()
}

// GetValues returns sorted EntityValues by timestamp
func (es *EntityStore) GetValues(hash uint64) []*EntityValue {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	heap := *es.store[hash] // Dereference to get the slice
	values := make([]*EntityValue, len(heap))
	for i, v := range heap {
		values[i] = v // Copy each value to the new slice
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].timestamp < values[j].timestamp
	})
	return values
}

// New function to get an entity by its hash key
func (es *EntityStore) GetEntityByHashKey(hash uint64) (*Entity, bool) {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	entity, exists := es.key2EntityMap[hash]
	return entity, exists
}

// GetEntitiesByMetricName to get all entities by metric name
func (es *EntityStore) GetEntitiesByMetricName(metricName string) []*Entity {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	keys, exists := es.metric2KeysMap[metricName]
	if !exists {
		return nil
	}
	entities := make([]*Entity, 0, len(keys))
	for key := range keys {
		entity, exists := es.key2EntityMap[key]
		if exists {
			entities = append(entities, entity)
		}
	}
	return entities
}

// GetEntitiesByNamespace to get all entities by namespace
func (es *EntityStore) GetEntitiesByNamespace(namespace string) []*Entity {
	es.lock.RLock() // Lock for writing
	defer es.lock.RUnlock()
	keys, exists := es.namespace2KeysMap[namespace]
	if !exists {
		return nil
	}
	entities := make([]*Entity, 0, len(keys))
	for key := range keys {
		entity, exists := es.key2EntityMap[key]
		if exists {
			entities = append(entities, entity)
		}
	}
	return entities
}

// Delete an entity from the store.
func (es *EntityStore) Delete(hash uint64) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	entity, exists := es.key2EntityMap[hash]
	if !exists {
		return
	}
	delete(es.key2EntityMap, hash)
	delete(es.metric2KeysMap[entity.MetricName], hash)
	delete(es.namespace2KeysMap[entity.Namespace], hash)
	delete(es.store, hash)
}

// purgeInactiveEntities purges inactive entities.
func (es *EntityStore) purgeInactiveEntities(purgeInterval time.Duration) {
	es.lock.Lock() // Lock for writing
	defer es.lock.Unlock()
	for hash, lastActive := range es.key2LastActiveMap {
		if time.Since(time.Unix(int64(lastActive), 0)) > purgeInterval {
			es.Delete(hash)
		}
	}
}

// StartCleanupInBackground purges expired entities periodically.
func (es *EntityStore) StartCleanupInBackground(ctx context.Context) {
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
func (es *EntityStore) GetStoreInfo() string {
	es.lock.RLock()
	defer es.lock.RUnlock()

	// Count of entities
	entityCount := len(es.key2EntityMap)
	// Count of metrics
	metricCount := len(es.metric2KeysMap)
	// Count of namespaces
	namespaceCount := len(es.namespace2KeysMap)
	// Assuming an additional method or field exists to get data points per entity
	// This part is pseudo-code and needs to be adjusted based on actual implementation
	dataPointsPerEntity := make(map[string]int)
	for key, heapValue := range es.store {
		entity, exists := es.key2EntityMap[key]
		if exists {
			dataPointsPerEntity[entity.EntityName+":"+entity.MetricName] = heapValue.Len()
		}
	}

	// Constructing the information string
	info := fmt.Sprintf("\n===============================Entity Store Information===============================\n")
	info += fmt.Sprintf("Entity Count: %d\nMetric Count: %d\nNamespace Count: %d\n", entityCount, metricCount, namespaceCount)
	for e, count := range dataPointsPerEntity {
		info += fmt.Sprintf("Entity Name: %s, Data Points: %d\n", e, count)
	}
	info += fmt.Sprintf("TS: %d, Store Total Size: %d\n", getCurrentTime(), len(es.store))

	return info
}
