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
				Tags: []string{containerID, displayContainerName, namespace},
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
		}
	}
}

func TestStoreAndPurgeEntities(t *testing.T) {
	numPayloads := 100
	store := EntityStore{
		key2ValuesMap: make(map[uint64]*dataItem),
		lock:          sync.RWMutex{},
		namespaces:    make(map[string]struct{}),
		loadNames:     make(map[string]struct{}),
	}
	for i := 0; i < numPayloads; i++ {
		payload := createSeriesPayload(i)
		entities := createEntitiesFromPayload(payload)
		store.SetEntitiesValues(entities)
	}
	storeInfo := store.GetStoreInfo()
	assert.EqualValues(t, numPayloads, storeInfo.TotalEntityCount)
	loadNameStats := storeInfo.EntityStatsByLoadName
	namespaceStats := storeInfo.EntityStatsByNamespace
	assert.EqualValues(t, numPayloads, loadNameStats["datadog.test.run"].Count)
	assert.EqualValues(t, numPayloads, namespaceStats["test"].Count)
	assert.EqualValues(t, 99, namespaceStats["test"].Max)
	assert.EqualValues(t, 0, namespaceStats["test"].Min)
	assert.EqualValues(t, 49.5, namespaceStats["test"].Avg)
	assert.EqualValues(t, 49, namespaceStats["test"].Medium)
	assert.EqualValues(t, 98, namespaceStats["test"].P99)
	assert.EqualValues(t, 94, namespaceStats["test"].P95)
	store.purgeInactiveEntities(30 * time.Second)
	storeInfo = store.GetStoreInfo()
	loadNameStats = storeInfo.EntityStatsByLoadName
	namespaceStats = storeInfo.EntityStatsByNamespace
	assert.Equal(t, 0, int(storeInfo.TotalEntityCount))
	assert.Equal(t, 0, int(namespaceStats["test"].Count))
	assert.Equal(t, 0, int(loadNameStats["datadog.test.run"].Count))
}
