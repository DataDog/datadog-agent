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
	}
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
	}
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
