// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package loadstore

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/stretchr/testify/assert"
)

func createSeriesPayload(i int, timeDelta int64) *gogen.MetricPayload {
	containerID := fmt.Sprintf("container_id:%d", 10)
	containerName := "container_name:container_test"
	displayContainerName := fmt.Sprintf("display_container_name:pod_%d-container_test", i)
	namespace := "kube_namespace:test"
	deployment := "kube_deployment:redis_test"
	kubeOwnerrefName := "kube_ownerref_name:redis_test"
	kubeOwnerrefKind := "kube_ownerref_kind:deployment"
	podName := fmt.Sprintf("pod_name:redis_%d", i)
	payload := gogen.MetricPayload{
		Series: []*gogen.MetricPayload_MetricSeries{
			{
				Metric: "container.memory.usage",
				Type:   3, // Gauge
				Points: []*gogen.MetricPayload_MetricPoint{
					{
						Timestamp: time.Now().Unix() - timeDelta, // timeDelta seconds ago
						Value:     float64(i),
					},
				},
				Tags: []string{containerID, displayContainerName, namespace, deployment, kubeOwnerrefName, kubeOwnerrefKind, podName, containerName},
				Resources: []*gogen.MetricPayload_Resource{
					{
						Type: "host", Name: "localHost",
					},
				},
			},
		},
	}
	return &payload
}

func createSeriesPayload2(i int, timeDelta int64) *gogen.MetricPayload {
	containerID := fmt.Sprintf("container_id:%d", i)
	containerName := "container_name:container_test"
	displayContainerName := fmt.Sprintf("display_container_name:pod_%d-container_test", i)
	namespace := "kube_namespace:test"
	deployment := "kube_deployment:nginx_test"
	kubeOwnerrefName := "kube_ownerref_name:nginx_test-8957fc986"
	kubeOwnerrefKind := "kube_ownerref_kind:replicaset"
	podName := fmt.Sprintf("pod_name:nginx_%d", i)
	payload := gogen.MetricPayload{
		Series: []*gogen.MetricPayload_MetricSeries{
			{
				Metric: "container.cpu.usage",
				Type:   3, // Gauge
				Points: []*gogen.MetricPayload_MetricPoint{
					{
						Timestamp: time.Now().Unix() - timeDelta, // timeDelta seconds ago
						Value:     float64(i),
					},
				},
				Tags: []string{containerID, displayContainerName, namespace, deployment, kubeOwnerrefName, kubeOwnerrefKind, podName, containerName},
				Resources: []*gogen.MetricPayload_Resource{
					{
						Type: "host", Name: "localHost2",
					},
				},
			},
		},
	}
	return &payload
}

func TestCreateEntitiesFromPayload(t *testing.T) {
	numPayloads := 10
	for i := 0; i < numPayloads; i++ {
		payload := createSeriesPayload(i, 100)
		entities := createEntitiesFromPayload(payload)
		assert.Equal(t, len(entities), 1)
		for k, v := range entities {
			assert.Equal(t, "container.memory.usage", k.MetricName)
			assert.Equal(t, ValueType(i), v.Value)
			assert.Equal(t, fmt.Sprintf("redis_%d", i), k.PodName)
			assert.Equal(t, "test", k.Namespace)
			assert.Equal(t, "redis_test", k.PodOwnerName)
			assert.Equal(t, "container_test", k.ContainerName)
			assert.Equal(t, fmt.Sprintf("pod_%d-container_test", i), k.EntityName)
		}
	}
}

func TestStoreAndPurgeEntities(t *testing.T) {
	numPayloads := 100
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
		targets:       make(map[targetKey]struct{}),
	}
	// Set targets to test filtering behavior (without targets, all entities would be accepted anyway)
	store.SetTargets([]Target{
		{Namespace: "test", Kind: "Deployment", Name: "redis_test"},
		{Namespace: "test", Kind: "Deployment", Name: "nginx_test"},
	})
	for _, timeDelta := range []int64{100, 85, 70} {
		for i := 0; i < numPayloads; i++ {
			payload := createSeriesPayload(i, timeDelta)
			entities := createEntitiesFromPayload(payload)
			store.SetEntitiesValues(entities)
			payload2 := createSeriesPayload2(i, timeDelta)
			entities2 := createEntitiesFromPayload(payload2)
			store.SetEntitiesValues(entities2)

		}
	}
	storeInfo := store.GetStoreInfo()
	assert.Equal(t, 2, len(storeInfo.StatsResults))
	for _, statsResult := range storeInfo.StatsResults {
		assert.Equal(t, numPayloads, statsResult.Count)
		assert.Equal(t, "test", statsResult.Namespace)
		assert.Contains(t, []string{"redis_test", "nginx_test"}, statsResult.PodOwner)
		if statsResult.PodOwner == "redis_test" {
			assert.Equal(t, "container.memory.usage", statsResult.MetricName)
		} else { // nginx_test
			assert.Equal(t, "container.cpu.usage", statsResult.MetricName)
		}
	}
	store.purgeInactiveEntities(10 * time.Second)
	storeInfo = store.GetStoreInfo()
	for _, statsResult := range storeInfo.StatsResults {
		assert.Equal(t, 0, statsResult.Count)
	}
}

func TestGetMetrics(t *testing.T) {
	numPayloads := 100
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
		targets:       make(map[targetKey]struct{}),
	}
	// Set targets to test filtering behavior (without targets, all entities would be accepted anyway)
	store.SetTargets([]Target{
		{Namespace: "test", Kind: "Deployment", Name: "redis_test"},
		{Namespace: "test", Kind: "Deployment", Name: "nginx_test"},
	})
	for _, timeDelta := range []int64{100, 85, 80} {
		for i := 0; i < numPayloads; i++ {
			payload := createSeriesPayload(i, timeDelta)
			entities := createEntitiesFromPayload(payload)
			store.SetEntitiesValues(entities)
			payload2 := createSeriesPayload2(i, timeDelta)
			entities2 := createEntitiesFromPayload(payload2)
			store.SetEntitiesValues(entities2)
		}
	}
	queryResult := store.GetMetricsRaw("container.cpu.usage", "test", "nginx_test", "")
	assert.Equal(t, 100, len(queryResult.Results))
	for _, podResult := range queryResult.Results {
		assert.Equal(t, 1, len(podResult.ContainerValues))
		assert.Equal(t, 0, len(podResult.PodLevelValue))
		for containerName, entityValues := range podResult.ContainerValues {
			assert.Equal(t, "container_test", containerName)
			assert.Equal(t, 3, len(entityValues))
		}
	}

	emptyQueryResult := store.GetMetricsRaw("container.cpu.usage", "test", "nginx_test", "container_test2")
	assert.Equal(t, 0, len(emptyQueryResult.Results))

	filteredQueryResult := store.GetMetricsRaw("container.memory.usage", "test", "redis_test", "container_test")
	assert.Equal(t, 100, len(filteredQueryResult.Results))
}

func TestGetMetricsWithNonExistingEntityDoesNotPanic(t *testing.T) {
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
	}

	testDataPerPod := &dataPerPod{
		containers: map[string]uint64{
			"test-container": 1,
		},
		podEntityID: 0,
	}

	// add pod to keyAttrTable but not to key2ValuesMap
	compKey := generateCompositeKey("test-ns", "test-pod", "container.cpu.usage")
	store.keyAttrTable[compKey] = podList{
		namespace:    "test-ns",
		podOwnerName: "test-pod",
		metricName:   "container.cpu.usage",
		pods: map[string]*dataPerPod{
			"test-pod": testDataPerPod,
		},
	}

	assert.NotPanics(t, func() {
		queryResult := store.GetMetricsRaw("container.cpu.usage", "test-ns", "test-pod", "")
		assert.Equal(t, 0, len(queryResult.Results))
	})

	// add to key2ValuesMap but with nil entity
	store.key2ValuesMap[1] = nil
	assert.NotPanics(t, func() {
		queryResult := store.GetMetricsRaw("container.cpu.usage", "test-ns", "test-pod", "")
		assert.Equal(t, 0, len(queryResult.Results))
	})
}

// TestSetTargets verifies that SetTargets correctly registers targets
func TestSetTargets(t *testing.T) {
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
		targets:       make(map[targetKey]struct{}),
	}

	// Initially no targets
	assert.Equal(t, 0, len(store.targets))

	// Set multiple targets
	targets := []Target{
		{Namespace: "ns1", Kind: "Deployment", Name: "deploy1"},
		{Namespace: "ns2", Kind: "Deployment", Name: "deploy2"},
		{Namespace: "ns1", Kind: "Deployment", Name: "deploy3"},
	}
	store.SetTargets(targets)
	assert.Equal(t, 3, len(store.targets))

	// Verify specific targets are registered
	_, exists := store.targets[targetKey{namespace: "ns1", kind: Deployment, name: "deploy1"}]
	assert.True(t, exists)
	_, exists = store.targets[targetKey{namespace: "ns2", kind: Deployment, name: "deploy2"}]
	assert.True(t, exists)
	_, exists = store.targets[targetKey{namespace: "ns1", kind: Deployment, name: "deploy3"}]
	assert.True(t, exists)

	// Update targets (replaces existing)
	newTargets := []Target{
		{Namespace: "ns3", Kind: "Deployment", Name: "deploy4"},
	}
	store.SetTargets(newTargets)
	assert.Equal(t, 1, len(store.targets))
	_, exists = store.targets[targetKey{namespace: "ns3", kind: Deployment, name: "deploy4"}]
	assert.True(t, exists)
	// Old targets should be gone
	_, exists = store.targets[targetKey{namespace: "ns1", kind: Deployment, name: "deploy1"}]
	assert.False(t, exists)
}

// TestInvalidTargetsAreSkipped verifies that malformed targets (empty namespace, kind, or name) are skipped
func TestInvalidTargetsAreSkipped(t *testing.T) {
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
		targets:       make(map[targetKey]struct{}),
	}

	// Set targets with some invalid entries
	store.SetTargets([]Target{
		{Namespace: "", Kind: "Deployment", Name: "deploy1"},        // Invalid: empty namespace
		{Namespace: "ns1", Kind: "Deployment", Name: ""},            // Invalid: empty name
		{Namespace: "ns1", Kind: "", Name: "deploy1"},               // Invalid: empty kind
		{Namespace: "ns1", Kind: "DaemonSet", Name: "deploy1"},      // Invalid: unsupported kind
		{Namespace: "test", Kind: "Deployment", Name: "redis_test"}, // Valid
	})

	// Only the valid target should be registered
	assert.Equal(t, 1, len(store.targets), "Only valid targets should be registered")
	_, exists := store.targets[targetKey{namespace: "test", kind: Deployment, name: "redis_test"}]
	assert.True(t, exists, "Valid target should be registered")
}

// TestFilteringMixedEntities verifies filtering with mix of matching and non-matching entities
func TestFilteringMixedEntities(t *testing.T) {
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
		targets:       make(map[targetKey]struct{}),
	}

	// Set target for only redis_test
	store.SetTargets([]Target{
		{Namespace: "test", Kind: "Deployment", Name: "redis_test"},
	})

	// Create mixed entities - some matching, some not
	for i := 0; i < 50; i++ {
		// These match the target (namespace="test", kind=Deployment, name="redis_test")
		payload := createSeriesPayload(i, 100)
		entities := createEntitiesFromPayload(payload)
		store.SetEntitiesValues(entities)

		// These don't match (namespace="test", podOwnerName="nginx_test")
		payload2 := createSeriesPayload2(i, 100)
		entities2 := createEntitiesFromPayload(payload2)
		store.SetEntitiesValues(entities2)
	}

	// Verify only matching entities were stored
	storeInfo := store.GetStoreInfo()
	assert.Equal(t, 1, len(storeInfo.StatsResults), "Only one group should be stored")

	for _, statsResult := range storeInfo.StatsResults {
		assert.Equal(t, "test", statsResult.Namespace)
		assert.Equal(t, "redis_test", statsResult.PodOwner)
		assert.Equal(t, 50, statsResult.Count, "Only matching entities should be stored")
	}

	// Verify nginx_test is not accessible
	result := store.GetMetricsRaw("container.cpu.usage", "test", "nginx_test", "")
	assert.Equal(t, 0, len(result.Results), "nginx_test should not be stored")

	// Verify redis_test is accessible
	result = store.GetMetricsRaw("container.memory.usage", "test", "redis_test", "")
	assert.Equal(t, 50, len(result.Results), "redis_test should be stored")
}

// TestDynamicTargetUpdates verifies that target updates affect subsequent entity storage
func TestDynamicTargetUpdates(t *testing.T) {
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		keyAttrTable:  make(map[compositeKey]podList),
		lock:          sync.RWMutex{},
		targets:       make(map[targetKey]struct{}),
	}

	// Initially set target for redis_test
	store.SetTargets([]Target{
		{Namespace: "test", Kind: "Deployment", Name: "redis_test"},
	})

	// Store redis entities
	for i := 0; i < 5; i++ {
		payload := createSeriesPayload(i, 100)
		entities := createEntitiesFromPayload(payload)
		store.SetEntitiesValues(entities)
	}

	// Verify redis entities stored
	storeInfo := store.GetStoreInfo()
	assert.Equal(t, 1, len(storeInfo.StatsResults))
	assert.Equal(t, 5, storeInfo.StatsResults[0].Count)

	// Update targets to include nginx_test as well
	store.SetTargets([]Target{
		{Namespace: "test", Kind: "Deployment", Name: "redis_test"},
		{Namespace: "test", Kind: "Deployment", Name: "nginx_test"},
	})

	// Store nginx entities
	for i := 0; i < 5; i++ {
		payload := createSeriesPayload2(i, 100)
		entities := createEntitiesFromPayload(payload)
		store.SetEntitiesValues(entities)
	}

	// Verify both redis and nginx entities are stored
	storeInfo = store.GetStoreInfo()
	assert.Equal(t, 2, len(storeInfo.StatsResults))

	totalCount := 0
	for _, statsResult := range storeInfo.StatsResults {
		totalCount += statsResult.Count
	}
	assert.Equal(t, 10, totalCount, "Both redis and nginx entities should be stored")
}
