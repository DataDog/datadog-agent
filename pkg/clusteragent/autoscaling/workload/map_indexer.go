// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

type mapIndexer struct {
	indexMap map[any]map[string]bool
}

func newMapIndexer() *mapIndexer {
	return &mapIndexer{
		indexMap: make(map[any]map[string]bool),
	}
}

// GetIDs returns all IDs for a given key
func (m *mapIndexer) GetIDs(key any) []string {
	ids := make([]string, 0, len(m.indexMap[key]))
	for id := range m.indexMap[key] {
		ids = append(ids, id)
	}
	return ids
}

// GetIndexKey returns the index key for a given object
func (m *mapIndexer) GetIndexKey(obj any) any {
	podAutoscaler, ok := obj.(model.PodAutoscalerInternal)
	if !ok {
		return nil
	}

	return podAutoscaler.GetOwnerReference()
}

// AddToIndex adds an ID to the index for a given key
func (m *mapIndexer) AddToIndex(key any, id string) {
	if key == nil {
		return
	}

	if m.indexMap[key] == nil {
		m.indexMap[key] = make(map[string]bool)
	}

	m.indexMap[key][id] = true
}

// RemoveFromIndex removes an ID from the index for a given key
func (m *mapIndexer) RemoveFromIndex(key any, id string) {
	if key == nil {
		return
	}

	delete(m.indexMap[key], id)
	if len(m.indexMap[key]) == 0 {
		delete(m.indexMap, key)
	}
}
