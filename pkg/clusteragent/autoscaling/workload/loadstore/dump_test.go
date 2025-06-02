// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"context"
	"strings"
	"testing"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/stretchr/testify/assert"
)

func newEntityValue(timestamp int64, value float64) *EntityValue {
	return &EntityValue{
		Timestamp: Timestamp(timestamp),
		Value:     ValueType(value),
	}
}

func newEntity(metricName, ns, deployment, podName, containerName string) *Entity {
	return &Entity{
		EntityType:    ContainerType,
		EntityName:    containerName,
		Namespace:     ns,
		MetricName:    metricName,
		PodName:       podName,
		PodOwnerName:  deployment,
		PodOwnerkind:  Deployment,
		ContainerName: containerName,
	}
}

func TestLocalAutoscalingWorkloadCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	testTime := time.Now().Unix()
	pkgconfigsetup.Datadog().SetWithoutSource("autoscaling.failover.enabled", true)
	defer cancel()
	lStore := GetWorkloadMetricStore(ctx)
	entities := make(map[*Entity]*EntityValue)
	entityOne := newEntity("container.cpu.usage", "workload-test-ns", "test-deployment", "pod1", "container-name1")
	entities[entityOne] = newEntityValue(testTime-30, 2.44e8)
	lStore.SetEntitiesValues(entities)
	entities[entityOne] = newEntityValue(testTime-15, 2.47e8)
	lStore.SetEntitiesValues(entities)

	entityTwo := newEntity("container.cpu.usage", "workload-test-ns", "test-deployment", "pod1", "container-name2")
	entities[entityTwo] = newEntityValue(testTime-30, 2.4e8)
	lStore.SetEntitiesValues(entities)
	entities[entityTwo] = newEntityValue(testTime-15, 2.45e8)
	lStore.SetEntitiesValues(entities)
	resp := GetAutoscalingWorkloadCheck(ctx)
	assert.NotNil(t, resp)
	workloadEntityList := resp.LocalAutoscalingWorkloadEntities
	assert.Len(t, workloadEntityList, 1)
	entity := workloadEntityList[0]
	assert.Equal(t, "workload-test-ns", entity["Namespace"].(string))
	assert.Equal(t, "test-deployment", entity["PodOwner"].(string))
	assert.Equal(t, "container.cpu.usage", entity["MetricName"].(string))
	assert.Equal(t, 2, entity["Datapoints(PodLevel)"].(int))
}

func TestDump(t *testing.T) {
	// Create test data
	entities := []LocalAutoscalingWorkloadEntity{
		{
			"Namespace":            "workload-apm",
			"PodOwner":             "go-app",
			"MetricName":           "container.memory.usage",
			"Datapoints(PodLevel)": 1,
		},
		{
			"Namespace":            "workload-redis",
			"PodOwner":             "redis-query",
			"MetricName":           "container.cpu.usage",
			"Datapoints(PodLevel)": 2,
		},
		{
			"Namespace":            "workload-nginx",
			"PodOwner":             "nginx",
			"MetricName":           "container.memory.usage",
			"Datapoints(PodLevel)": 3,
		},
	}

	response := &LocalWorkloadMetricStoreInfo{
		LocalAutoscalingWorkloadEntities: entities,
	}

	// Capture output
	var output strings.Builder
	response.Dump(&output)

	// Verify output format
	expectedOutput := `Namespace: workload-apm, PodOwner: go-app, MetricName: container.memory.usage, Datapoints: 1
Namespace: workload-redis, PodOwner: redis-query, MetricName: container.cpu.usage, Datapoints: 2
Namespace: workload-nginx, PodOwner: nginx, MetricName: container.memory.usage, Datapoints: 3
`

	assert.Equal(t, expectedOutput, output.String())
}
