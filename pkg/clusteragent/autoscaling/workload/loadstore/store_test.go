// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package loadstore

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/stretchr/testify/assert"
)

func createSeriesPayload(i int) *gogen.MetricPayload {
	containerID := fmt.Sprintf("container_id:%d", i)
	displayContainerName := fmt.Sprintf("display_container_name:%d", i)
	namespace := "kube_namespace:test"
	deployment := "kube_deployment:redis_test"
	payload := gogen.MetricPayload{
		Series: []*gogen.MetricPayload_MetricSeries{
			{
				Metric: "datadog.test.run",
				Type:   3, // Gauge
				Points: []*gogen.MetricPayload_MetricPoint{
					{
						Timestamp: time.Now().Unix() - 100, // 100 seconds ago
						Value:     float64(i),
					},
				},
				Tags: []string{containerID, displayContainerName, namespace, deployment},
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

func createSeriesPayload2(i int) *gogen.MetricPayload {
	containerID := fmt.Sprintf("container_id:%d", i)
	displayContainerName := fmt.Sprintf("display_container_name:%d", i)
	namespace := "kube_namespace:test"
	deployment := "kube_deployment:nginx_test"
	payload := gogen.MetricPayload{
		Series: []*gogen.MetricPayload_MetricSeries{
			{
				Metric: "container.cpu.usage",
				Type:   3, // Gauge
				Points: []*gogen.MetricPayload_MetricPoint{
					{
						Timestamp: time.Now().Unix() - 100, // 100 seconds ago
						Value:     float64(i),
					},
				},
				Tags: []string{containerID, displayContainerName, namespace, deployment},
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
		payload := createSeriesPayload(i)
		entities := createEntitiesFromPayload(payload)
		assert.Equal(t, len(entities), 1)
		for k, v := range entities {
			assert.Equal(t, "datadog.test.run", k.LoadName)
			assert.Equal(t, "localHost", k.Host)
			assert.Equal(t, strconv.Itoa(i), k.SourceID)
			assert.Equal(t, ValueType(i), v.value)
			assert.Equal(t, "redis_test", k.Deployment)
		}
	}
}

func TestStoreAndPurgeEntities(t *testing.T) {
	numPayloads := 100
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		lock:          sync.RWMutex{},
		keyAttrTable:  make(map[string]map[string]map[string]map[uint64]struct{}),
	}
	for i := 0; i < numPayloads; i++ {
		payload := createSeriesPayload(i)
		entities := createEntitiesFromPayload(payload)
		store.SetEntitiesValues(entities)
		payload2 := createSeriesPayload2(i)
		entities2 := createEntitiesFromPayload(payload2)
		store.SetEntitiesValues(entities2)
	}
	storeInfo := store.GetStoreInfo()
	assert.Equal(t, 2, len(storeInfo.StatsResults))
	for _, statsResult := range storeInfo.StatsResults {
		assert.Equal(t, numPayloads, statsResult.Count)
		assert.EqualValues(t, 99, statsResult.Max)
		assert.EqualValues(t, 0, statsResult.Min)
		assert.EqualValues(t, 49.5, statsResult.Avg)
		assert.EqualValues(t, 49, statsResult.Medium)
		assert.EqualValues(t, 98, statsResult.P99)
		assert.EqualValues(t, 94, statsResult.P95)
	}

	store.purgeInactiveEntities(30 * time.Second)
	storeInfo = store.GetStoreInfo()
	for _, statsResult := range storeInfo.StatsResults {
		assert.Equal(t, 0, statsResult.Count)
	}

}
