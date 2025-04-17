// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func TestProcessScaleUp(t *testing.T) {
	testTime := time.Now().Unix()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// setup podwatcher
	pw := workload.NewPodWatcher(nil, nil)
	pw.HandleEvent(newFakeWLMPodEvent("default", "test-deployment", "pod1", []string{"container-name1", "container-name2"}))

	// setup store
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	store.Set("default/autoscaler1", newAutoscaler(true), "")
	store.Set("default/autoscaler2", newAutoscaler(false), "")

	// setup loadstore
	lStore := loadstore.GetWorkloadMetricStore(ctx)
	entities := make(map[*loadstore.Entity]*loadstore.EntityValue)
	entityOne := newEntity("container.cpu.usage", "default", "test-deployment", "pod1", "container-name1")
	entities[entityOne] = newEntityValue(testTime-30, 2.44e8)
	lStore.SetEntitiesValues(entities)
	entities[entityOne] = newEntityValue(testTime-15, 2.47e8)
	lStore.SetEntitiesValues(entities)

	entityTwo := newEntity("container.cpu.usage", "default", "test-deployment", "pod1", "container-name2")
	entities[entityTwo] = newEntityValue(testTime-30, 2.4e8)
	lStore.SetEntitiesValues(entities)
	entities[entityTwo] = newEntityValue(testTime-15, 2.45e8)
	lStore.SetEntitiesValues(entities)
	queryResult := lStore.GetMetricsRaw("container.cpu.usage", "default", "test-deployment", "")
	assert.Len(t, queryResult.Results, 1)

	// test
	recommender := NewRecommender(pw, store)
	recommender.process(ctx)
	pai, found := store.Get("default/autoscaler1")
	assert.True(t, found)
	assert.Nil(t, pai.FallbackScalingValues().HorizontalError)
	assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerLocalValueSource, pai.FallbackScalingValues().Horizontal.Source)
	assert.Equal(t, int32(2), pai.FallbackScalingValues().Horizontal.Replicas) // currently 1 replica, recommending scale up to 2
	assert.Equal(t, (testTime - 30), pai.FallbackScalingValues().Horizontal.Timestamp.Unix())

	// check that autoscalers without fallback enabled are not processed
	pai2, found := store.Get("default/autoscaler2")
	assert.True(t, found)
	assert.Nil(t, pai2.FallbackScalingValues().Horizontal)

	resetWorkloadMetricStore()
}

func TestProcessScaleDown(t *testing.T) {
	testTime := time.Now().Unix()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// setup podwatcher
	pw := workload.NewPodWatcher(nil, nil)
	pw.HandleEvent(newFakeWLMPodEvent("default", "test-deployment", "pod1", []string{"container-name1", "container-name2"}))
	pw.HandleEvent(newFakeWLMPodEvent("default", "test-deployment", "pod2", []string{"container-name1"}))
	pw.HandleEvent(newFakeWLMPodEvent("default", "test-deployment", "pod3", []string{"container-name1"}))

	// setup store
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	store.Set("default/autoscaler1", newAutoscaler(true), "")
	store.Set("default/autoscaler2", newAutoscaler(false), "")

	// setup loadstore
	lStore := loadstore.GetWorkloadMetricStore(ctx)
	entities := make(map[*loadstore.Entity]*loadstore.EntityValue)
	podOneContainerOneEntity := newEntity("container.cpu.usage", "default", "test-deployment", "pod1", "container-name1")
	entities[podOneContainerOneEntity] = newEntityValue(testTime-30, 1.2e8)
	lStore.SetEntitiesValues(entities)
	entities[podOneContainerOneEntity] = newEntityValue(testTime-15, 1.14e8)
	lStore.SetEntitiesValues(entities)

	podOneContainerTwoEntity := newEntity("container.cpu.usage", "default", "test-deployment", "pod1", "container-name2")
	entities[podOneContainerTwoEntity] = newEntityValue(testTime-30, 1.9e8)
	lStore.SetEntitiesValues(entities)
	entities[podOneContainerTwoEntity] = newEntityValue(testTime-15, 1.7e8)
	lStore.SetEntitiesValues(entities)

	podTwoEntity := newEntity("container.cpu.usage", "default", "test-deployment", "pod2", "container-name1")
	entities[podTwoEntity] = newEntityValue(testTime-30, 1.5e8)
	lStore.SetEntitiesValues(entities)
	entities[podTwoEntity] = newEntityValue(testTime-15, 1.4e8)
	lStore.SetEntitiesValues(entities)

	podThreeEntity := newEntity("container.cpu.usage", "default", "test-deployment", "pod3", "container-name1")
	entities[podThreeEntity] = newEntityValue(testTime-30, 1.5e8)
	lStore.SetEntitiesValues(entities)
	entities[podThreeEntity] = newEntityValue(testTime-15, 1.45e8)
	lStore.SetEntitiesValues(entities)

	queryResult := lStore.GetMetricsRaw("container.cpu.usage", "default", "test-deployment", "")
	assert.Len(t, queryResult.Results, 3)

	// test
	recommender := NewRecommender(pw, store)
	recommender.process(ctx)
	pai, found := store.Get("default/autoscaler1")
	assert.True(t, found)
	assert.Nil(t, pai.FallbackScalingValues().HorizontalError)
	assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerLocalValueSource, pai.FallbackScalingValues().Horizontal.Source)
	assert.Equal(t, int32(2), pai.FallbackScalingValues().Horizontal.Replicas) // currently 3 replicas, recommending scale down to 2
	assert.Equal(t, (testTime - 30), pai.FallbackScalingValues().Horizontal.Timestamp.Unix())

	// check that autoscalers without fallback enabled are not processed
	pai2, found := store.Get("default/autoscaler2")
	assert.True(t, found)
	assert.Nil(t, pai2.FallbackScalingValues().Horizontal)

	// cleanup
	resetWorkloadMetricStore()
}
