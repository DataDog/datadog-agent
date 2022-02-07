// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"time"
)

// MockMetricsProvider can be used to create tests
type MockMetricsProvider struct {
	collectors map[string]Collector
}

// NewMockMetricsProvider creates a mock provider
func NewMockMetricsProvider() *MockMetricsProvider {
	return &MockMetricsProvider{
		collectors: make(map[string]Collector),
	}
}

// GetCollector emulates the MetricsProvider interface
func (mp *MockMetricsProvider) GetCollector(runtime string) Collector {
	return mp.collectors[runtime]
}

// RegisterCollector registers a collector
func (mp *MockMetricsProvider) RegisterCollector(collectorMeta CollectorMetadata) {
	if collector, err := collectorMeta.Factory(); err != nil {
		mp.collectors[collectorMeta.ID] = collector
	}
}

// RegisterConcreteCollector registers a collector
func (mp *MockMetricsProvider) RegisterConcreteCollector(runtime string, c Collector) {
	mp.collectors[runtime] = c
}

// RemoveCollector removes a collector
func (mp *MockMetricsProvider) RemoveCollector(runtime string) {
	delete(mp.collectors, runtime)
}

// Clear removes all collectors
func (mp *MockMetricsProvider) Clear() {
	mp.collectors = make(map[string]Collector)
}

// MockContainerEntry allows to customize mock responses
type MockContainerEntry struct {
	ContainerStats ContainerStats
	NetworkStats   ContainerNetworkStats
	Error          error
}

// MockCollector is a mocked collector
type MockCollector struct {
	id         string
	containers map[string]MockContainerEntry
}

// NewMockCollector creates a MockCollector
func NewMockCollector(id string) *MockCollector {
	return &MockCollector{
		id:         id,
		containers: make(map[string]MockContainerEntry),
	}
}

// ID returns collector ID
func (mp *MockCollector) ID() string {
	return mp.id
}

// SetContainerEntry sets an entry for a given containerID
func (mp *MockCollector) SetContainerEntry(containerID string, cEntry MockContainerEntry) {
	mp.containers[containerID] = cEntry
}

// DeleteContainerEntry removes the entry for containerID
func (mp *MockCollector) DeleteContainerEntry(containerID string) {
	delete(mp.containers, containerID)
}

// Clear removes all entries
func (mp *MockCollector) Clear() {
	mp.containers = make(map[string]MockContainerEntry)
}

// GetContainerStats returns stats from MockContainerEntry
func (mp *MockCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	if entry, found := mp.containers[containerID]; found {
		return &entry.ContainerStats, entry.Error
	}

	return nil, fmt.Errorf("container not found")
}

// GetContainerNetworkStats returns stats from MockContainerEntry
func (mp *MockCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	if entry, found := mp.containers[containerID]; found {
		return &entry.NetworkStats, entry.Error
	}

	return nil, fmt.Errorf("container not found")
}
