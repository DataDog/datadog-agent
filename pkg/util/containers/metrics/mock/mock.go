// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mock

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
)

// MetricsProvider can be used to create tests
type MetricsProvider struct {
	collectors    map[string]provider.Collector
	metaCollector provider.MetaCollector
}

// NewMetricsProvider creates a mock provider
func NewMetricsProvider() *MetricsProvider {
	return &MetricsProvider{
		collectors: make(map[string]provider.Collector),
	}
}

// GetCollector emulates the MetricsProvider interface
func (mp *MetricsProvider) GetCollector(runtime string) provider.Collector {
	return mp.collectors[runtime]
}

// GetMetaCollector returns the registered MetaCollector
func (mp *MetricsProvider) GetMetaCollector() provider.MetaCollector {
	return mp.metaCollector
}

// RegisterCollector registers a collector
func (mp *MetricsProvider) RegisterCollector(collectorMeta provider.CollectorMetadata) {
	if collector, err := collectorMeta.Factory(); err != nil {
		mp.collectors[collectorMeta.ID] = collector
	}
}

// RegisterConcreteCollector registers a collector
func (mp *MetricsProvider) RegisterConcreteCollector(runtime string, c provider.Collector) {
	mp.collectors[runtime] = c
}

// RegisterMetaCollector registers the meta collector
func (mp *MetricsProvider) RegisterMetaCollector(c provider.MetaCollector) {
	mp.metaCollector = c
}

// RemoveCollector removes a collector
func (mp *MetricsProvider) RemoveCollector(runtime string) {
	delete(mp.collectors, runtime)
}

// Clear removes all collectors
func (mp *MetricsProvider) Clear() {
	mp.collectors = make(map[string]provider.Collector)
}

// ContainerEntry allows to customize mock responses
type ContainerEntry struct {
	ContainerStats *provider.ContainerStats
	NetworkStats   *provider.ContainerNetworkStats
	OpenFiles      *uint64
	Error          error
}

// Collector is a mocked collector
type Collector struct {
	id         string
	containers map[string]ContainerEntry
}

// NewCollector creates a MockCollector
func NewCollector(id string) *Collector {
	return &Collector{
		id:         id,
		containers: make(map[string]ContainerEntry),
	}
}

// ID returns collector ID
func (mp *Collector) ID() string {
	return mp.id
}

// SetContainerEntry sets an entry for a given containerID
func (mp *Collector) SetContainerEntry(containerID string, cEntry ContainerEntry) {
	mp.containers[containerID] = cEntry
}

// DeleteContainerEntry removes the entry for containerID
func (mp *Collector) DeleteContainerEntry(containerID string) {
	delete(mp.containers, containerID)
}

// Clear removes all entries
func (mp *Collector) Clear() {
	mp.containers = make(map[string]ContainerEntry)
}

// GetContainerStats returns stats from MockContainerEntry
func (mp *Collector) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	if entry, found := mp.containers[containerID]; found {
		return entry.ContainerStats, entry.Error
	}

	return nil, fmt.Errorf("container not found")
}

// GetContainerOpenFilesCount returns stats from MockContainerEntry
func (mp *Collector) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	if entry, found := mp.containers[containerID]; found {
		return entry.OpenFiles, entry.Error
	}

	return nil, fmt.Errorf("container not found")
}

// GetContainerNetworkStats returns stats from MockContainerEntry
func (mp *Collector) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	if entry, found := mp.containers[containerID]; found {
		return entry.NetworkStats, entry.Error
	}

	return nil, fmt.Errorf("container not found")
}

// GetContainerIDForPID returns a container ID for given PID.
func (mp *Collector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	return "", nil
}

// GetSelfContainerID returns the container ID for current container.
func (mp *Collector) GetSelfContainerID() (string, error) {
	return "", nil
}
