// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	return CollectorRef[T]{
		Collector: collector,
		Priority:  priority,
	}
}

func (pc CollectorRef[T]) bestCollector(runtime RuntimeMetadata, otherID string, oth CollectorRef[T]) CollectorRef[T] {
	// T is always an interface, so zero is nil, but currently we cannot express nillable in a constraint
	var zero T

	if oth.Collector != zero && (pc.Collector == zero || oth.Priority < pc.Priority) {
		log.Debugf("Using collector id: %s for type: %T and runtime: %s", otherID, pc, runtime.String())
		return oth
	}

	return pc
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
	ContainerIDForPID               CollectorRef[ContainerIDForPIDRetriever]
	ContainerIDForInode             CollectorRef[ContainerIDForInodeRetriever]
	SelfContainerID                 CollectorRef[SelfContainerIDRetriever]
	ContainerIDForPodUIDAndContName CollectorRef[ContainerIDForPodUIDAndContNameRetriever]
}

func (c *Collectors) merge(runtime RuntimeMetadata, otherID string, oth *Collectors) {
	c.Stats = c.Stats.bestCollector(runtime, otherID, oth.Stats)
	c.Network = c.Network.bestCollector(runtime, otherID, oth.Network)
	c.OpenFilesCount = c.OpenFilesCount.bestCollector(runtime, otherID, oth.OpenFilesCount)
	c.PIDs = c.PIDs.bestCollector(runtime, otherID, oth.PIDs)
	c.ContainerIDForPID = c.ContainerIDForPID.bestCollector(runtime, otherID, oth.ContainerIDForPID)
	c.ContainerIDForInode = c.ContainerIDForInode.bestCollector(runtime, otherID, oth.ContainerIDForInode)
	c.ContainerIDForPodUIDAndContName = c.ContainerIDForPodUIDAndContName.bestCollector(runtime, otherID, oth.ContainerIDForPodUIDAndContName)
	c.SelfContainerID = c.SelfContainerID.bestCollector(runtime, otherID, oth.SelfContainerID)
}

// noImplCollector implements the Collector interface with all not implemented responses
type noImplCollector struct{}

func (noImplCollector) GetContainerStats(_, _ string, _ time.Duration) (*ContainerStats, error) {
	return nil, nil
}

func (noImplCollector) GetContainerNetworkStats(_, _ string, _ time.Duration) (*ContainerNetworkStats, error) {
	return nil, nil
}

func (noImplCollector) GetContainerOpenFilesCount(_, _ string, _ time.Duration) (*uint64, error) {
	return nil, nil
}

func (noImplCollector) GetPIDs(_, _ string, _ time.Duration) ([]int, error) {
	return nil, nil
}

// collectorImpl is used by provider for fast read calls
type collectorImpl struct {
	stats     ContainerStatsGetter
	network   ContainerNetworkStatsGetter
	openFiles ContainerOpenFilesCountGetter
	pids      ContainerPIDsGetter
}

func newCollectorImpl() *collectorImpl {
	noImpl := noImplCollector{}
	return &collectorImpl{
		stats:     noImpl,
		network:   noImpl,
		openFiles: noImpl,
		pids:      noImpl,
	}
}

func fromCollectors(c *Collectors) *collectorImpl {
	collectorImpl := newCollectorImpl()
	if c.Stats.Collector != nil {
		collectorImpl.stats = c.Stats.Collector
	}

	if c.Network.Collector != nil {
		collectorImpl.network = c.Network.Collector
	}

	if c.OpenFilesCount.Collector != nil {
		collectorImpl.openFiles = c.OpenFilesCount.Collector
	}

	if c.PIDs.Collector != nil {
		collectorImpl.pids = c.PIDs.Collector
	}

	return collectorImpl
}

func (c *collectorImpl) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	return c.stats.GetContainerStats(containerNS, containerID, cacheValidity)
}

func (c *collectorImpl) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	return c.network.GetContainerNetworkStats(containerNS, containerID, cacheValidity)
}

func (c *collectorImpl) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	return c.openFiles.GetContainerOpenFilesCount(containerNS, containerID, cacheValidity)
}

func (c *collectorImpl) GetPIDs(containerNS, containerID string, cacheValidity time.Duration) ([]int, error) {
	return c.pids.GetPIDs(containerNS, containerID, cacheValidity)
}
