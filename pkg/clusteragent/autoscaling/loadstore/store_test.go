// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package loadstore

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/stretchr/testify/assert"
)

func createSeriesPayload(i int) *gogen.MetricPayload {
	container_id := fmt.Sprintf("container_id:%d", i)
	display_container_name := fmt.Sprintf("display_container_name:%d", i)
	namespace := fmt.Sprintf("kube_namespace:test")
	payload := gogen.MetricPayload{
		Series: []*gogen.MetricPayload_MetricSeries{
			{
				Metric: "datadog.test.run",
				Type:   3, // Gauge
				Points: []*gogen.MetricPayload_MetricPoint{
					{
						Timestamp: time.Now().Unix() - 100, // 100 seconds ago
						Value:     1.0,
					},
				},
				Tags: []string{container_id, display_container_name, namespace},
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
			assert.Equal(t, "datadog.test.run", k.MetricName)
			assert.Equal(t, "localHost", k.Host)
			assert.Equal(t, strconv.Itoa(i), k.SourceID)
			assert.Equal(t, ValueType(1.0), v.value)
		}
	}
}

func TestStoreAndPurgeEntities(t *testing.T) {
	numPayloads := 100
	store := EntityStore{
		key2ValuesMap:     make(map[uint64]*dataItem),
		metric2KeysMap:    make(map[string]map[uint64]struct{}),
		namespace2KeysMap: make(map[string]map[uint64]struct{}),
	}
	for i := 0; i < numPayloads; i++ {
		payload := createSeriesPayload(i)
		entities := createEntitiesFromPayload(payload)
		store.SetEntitiesValues(entities)
	}
	store.purgeInactiveEntities(30 * time.Second)
	assert.Equal(t, 0, len(store.key2ValuesMap))
	assert.Equal(t, 0, len(store.metric2KeysMap))
	assert.Equal(t, 0, len(store.namespace2KeysMap))
}
