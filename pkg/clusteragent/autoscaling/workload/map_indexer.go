// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

type MapIndexer struct {
	indexMap map[any]map[string]bool
}

func NewMapIndexer() *MapIndexer {
	return &MapIndexer{
		indexMap: make(map[any]map[string]bool),
	}
}

func (m *MapIndexer) GetIDs(key any) []string {
	ids := make([]string, 0, len(m.indexMap[key]))
	for id := range m.indexMap[key] {
		ids = append(ids, id)
	}
	return ids
}

func (m *MapIndexer) GetIndexKey(obj any) any {
	podAutoscaler, ok := obj.(model.PodAutoscalerInternal)
	if !ok {
		return nil
	}

	return model.OwnerReference{
		Namespace:  podAutoscaler.Namespace(),
		Name:       podAutoscaler.Spec().TargetRef.Name,
		Kind:       podAutoscaler.Spec().TargetRef.Kind,
		APIVersion: podAutoscaler.Spec().TargetRef.APIVersion,
	}
}

func (m *MapIndexer) AddToIndex(key any, id string) {
	if key == nil {
		return
	}

	if m.indexMap[key] == nil {
		m.indexMap[key] = make(map[string]bool)
	}

	m.indexMap[key][id] = true
}

func (m *MapIndexer) RemoveFromIndex(key any, id string) {
	if key == nil {
		return
	}

	delete(m.indexMap[key], id)
	if len(m.indexMap[key]) == 0 {
		delete(m.indexMap, key)
	}
}
