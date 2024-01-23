// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"time"
)

// CollectorRef holds a collector interface reference with metadata
// T is always an interface. Collectors implementing this interface need
// to be comparable, often means that it should be a pointer receiver.
type CollectorRef[T comparable] struct {
	Collector T
	Priority  uint8
}

// MakeRef returns a CollectorRef[T] given a T and a priority
func MakeRef[T comparable](collector T, priority uint8) CollectorRef[T] {
	panic("not called")
}

func (pc CollectorRef[T]) bestCollector(runtime RuntimeMetadata, otherID string, oth CollectorRef[T]) CollectorRef[T] {
	panic("not called")
}

// Collectors is used by implementation to set their capabilities
// and by registry to store current version
// Priority fields: lowest gets higher priority (0 more prioritary than 1)
type Collectors struct {
	Stats          CollectorRef[ContainerStatsGetter]
	Network        CollectorRef[ContainerNetworkStatsGetter]
	OpenFilesCount CollectorRef[ContainerOpenFilesCountGetter]
	PIDs           CollectorRef[ContainerPIDsGetter]

	// These collectors are used in the meta collector
	ContainerIDForPID   CollectorRef[ContainerIDForPIDRetriever]
	ContainerIDForInode CollectorRef[ContainerIDForInodeRetriever]
	SelfContainerID     CollectorRef[SelfContainerIDRetriever]
}

func (c *Collectors) merge(runtime RuntimeMetadata, otherID string, oth *Collectors) {
	panic("not called")
}

// noImplCollector implements the Collector interface with all not implemented responses
type noImplCollector struct{}

func (noImplCollector) GetContainerStats(_, _ string, _ time.Duration) (*ContainerStats, error) {
	panic("not called")
}

func (noImplCollector) GetContainerNetworkStats(_, _ string, _ time.Duration) (*ContainerNetworkStats, error) {
	panic("not called")
}

func (noImplCollector) GetContainerOpenFilesCount(_, _ string, _ time.Duration) (*uint64, error) {
	panic("not called")
}

func (noImplCollector) GetPIDs(_, _ string, _ time.Duration) ([]int, error) {
	panic("not called")
}

// collectorImpl is used by provider for fast read calls
type collectorImpl struct {
	stats     ContainerStatsGetter
	network   ContainerNetworkStatsGetter
	openFiles ContainerOpenFilesCountGetter
	pids      ContainerPIDsGetter
}

func newCollectorImpl() *collectorImpl {
	panic("not called")
}

func fromCollectors(c *Collectors) *collectorImpl {
	panic("not called")
}

func (c *collectorImpl) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	panic("not called")
}

func (c *collectorImpl) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	panic("not called")
}

func (c *collectorImpl) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	panic("not called")
}

func (c *collectorImpl) GetPIDs(containerNS, containerID string, cacheValidity time.Duration) ([]int, error) {
	panic("not called")
}
