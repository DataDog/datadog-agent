// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

const samples int = 1000000

var weightedPrefixes = map[string]int{
	"container_image_metadata": 60,
	"container_id":             60,
	"ecs_task":                 5,
	"host":                     5,
	"deployment":               15,
	"kubernetes_metadata":      30,
	"kubernetes_pod_uid":       30,
	"process":                  30,
}

// getWeightedPrefix selects a prefix based on the provided weights.
func getNextPrefix() types.EntityIDPrefix {
	totalWeight := 0
	for _, weight := range weightedPrefixes {
		totalWeight += weight
	}

	randomWeight := rand.Intn(totalWeight)

	// Iterate through the prefixes and select one based on the random weight
	cumulativeWeight := 0
	for prefix, weight := range weightedPrefixes {
		cumulativeWeight += weight
		if randomWeight < cumulativeWeight {
			return types.EntityIDPrefix(prefix)
		}
	}

	return "" // This line should never be reached if the weights are set up correctly
}

func initStore(store types.ObjectStore[int]) {
	for i := range samples {
		entityID := types.NewEntityID(getNextPrefix(), fmt.Sprintf("%d", i))
		store.Set(entityID, i)
	}
}

// Mock ApplyFunc for testing purposes
func mockApplyFunc[T any](_ types.EntityID, _ T) {}

func BenchmarkDefaultObjectStore_Set(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", false)
	store := NewObjectStore[int](cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		initStore(store)
	}
}

func BenchmarkCompositeObjectStore_Set(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", true)
	store := NewObjectStore[int](cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		initStore(store)
	}
}

func BenchmarkDefaultObjectStore_Get(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", false)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entityID := types.NewEntityID(getNextPrefix(), fmt.Sprintf("%d", i))
		_, _ = store.Get(entityID)
	}
}

func BenchmarkCompositeObjectStore_Get(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", true)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entityID := types.NewEntityID(getNextPrefix(), fmt.Sprintf("%d", i))
		_, _ = store.Get(entityID)
	}
}

func BenchmarkDefaultObjectStore_Unset(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", false)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entityID := types.NewEntityID(getNextPrefix(), fmt.Sprintf("%d", i))
		store.Unset(entityID)
		store.Set(entityID, i) // reset the state for the next iteration
	}
}

func BenchmarkCompositeObjectStore_Unset(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", true)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entityID := types.NewEntityID(getNextPrefix(), fmt.Sprintf("%d", i))
		store.Unset(entityID)
		store.Set(entityID, i) // reset the state for the next iteration
	}
}

func BenchmarkDefaultObjectStore_Size(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", false)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.Size()
	}
}

func BenchmarkCompositeObjectStore_Size(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", true)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.Size()
	}
}

func BenchmarkDefaultObjectStore_ForEach(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", false)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ForEach(mockApplyFunc[int])
	}
}

func BenchmarkCompositeObjectStore_ForEach(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", true)
	store := NewObjectStore[int](cfg)
	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ForEach(mockApplyFunc[int])
	}
}

func BenchmarkDefaultObjectStore_ListAll(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", false)
	store := NewObjectStore[int](cfg)

	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.ListObjects()
	}
}

func BenchmarkCompositeObjectStore_ListAll(b *testing.B) {
	cfg := configmock.New(b)
	cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", true)
	store := NewObjectStore[int](cfg)

	initStore(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.ListObjects()
	}
}
