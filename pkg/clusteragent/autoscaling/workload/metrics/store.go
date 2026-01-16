// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"sync"

	"k8s.io/client-go/tools/cache"
)

// PodAutoscalerMetricsStore stores structured metrics for DatadogPodAutoscaler objects
// It implements a subset of the cache.Store interface pattern from kube-state-metrics
type PodAutoscalerMetricsStore struct {
	metrics             sync.Map // map[string]StructuredMetrics
	generateMetricsFunc func(interface{}) StructuredMetrics
}

// NewPodAutoscalerMetricsStore creates a new metrics store
func NewPodAutoscalerMetricsStore(generateFunc func(interface{}) StructuredMetrics) *PodAutoscalerMetricsStore {
	return &PodAutoscalerMetricsStore{
		generateMetricsFunc: generateFunc,
	}
}

// Add adds or updates metrics for an object
func (m *PodAutoscalerMetricsStore) Add(key string, obj interface{}) {
	m.Update(key, obj)
}

// Update generates and stores metrics for an object
func (m *PodAutoscalerMetricsStore) Update(key string, obj interface{}) {
	metrics := m.generateMetricsFunc(obj)
	m.metrics.Store(key, metrics)
}

// Delete removes metrics for an object
func (m *PodAutoscalerMetricsStore) Delete(key string, _obj interface{}) {
	m.metrics.Delete(key)
}

// Replace clears the store and adds all provided objects
func (m *PodAutoscalerMetricsStore) Replace(list []interface{}, resourceVersion string) error {
	// Clear existing metrics
	m.metrics.Range(func(key, value interface{}) bool {
		m.metrics.Delete(key)
		return true
	})

	// Add all objects from list
	for _, obj := range list {
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			return err
		}
		m.Add(key, obj)
	}

	return nil
}

// List returns nil (not needed for metrics use case)
func (m *PodAutoscalerMetricsStore) List() []interface{} {
	return nil
}

// ListKeys returns nil (not needed for metrics use case)
func (m *PodAutoscalerMetricsStore) ListKeys() []string {
	return nil
}

// Get returns nil (not needed for metrics use case)
func (m *PodAutoscalerMetricsStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

// GetByKey returns nil (not needed for metrics use case)
func (m *PodAutoscalerMetricsStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

// Resync is a no-op
func (m *PodAutoscalerMetricsStore) Resync() error {
	return nil
}
