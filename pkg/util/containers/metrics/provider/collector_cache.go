// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"time"
)

const (
	contCoreStatsCachePrefix  = "cs-"
	contOpenFilesCachePrefix  = "of-"
	contNetStatsCachePrefix   = "cns-"
	contPidToCidCachePrefix   = "pid-"
	contInodeToCidCachePrefix = "in-"
	contPidsCachePrefix       = "pids-"
)

// collectorCache is a wrapper handling cache for collectors.
//
// The underlying cache is fully synchronized to avoid locking at this layer.
// It means that if a method is called multiple times in parallel for the same cachey key,
// it may result in multiple calls to the underlying collector.
//
// The collectors referenced in *Collectors may be changed, no synchronization is done.
type collectorCache struct {
	providerID           string
	collectors           *Collectors
	cache                *Cache
	selfContainerIDCache string
}

// Make sure all methods are implemented
var (
	_ Collector     = &collectorCache{}
	_ MetaCollector = &collectorCache{}
)

// MakeCached modifies `collectors` to go through a caching layer
func MakeCached(providerID string, cache *Cache, collectors *Collectors) *Collectors {
	panic("not called")
}

// GetContainerStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	panic("not called")
}

// GetContainerNetworkStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	panic("not called")
}

// GetContainerOpenFilesCount returns the count of open files if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	panic("not called")
}

// GetPIDs returns the container ID for given PID
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetPIDs(containerNS, containerID string, cacheValidity time.Duration) ([]int, error) {
	panic("not called")
}

// GetContainerIDForPID returns the container ID for given PID
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	panic("not called")
}

// GetContainerIDForInode returns the container ID for given Inode
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error) {
	panic("not called")
}

// GetSelfContainerID returns current process container ID
// No caching as it's not supposed to change
func (cc *collectorCache) GetSelfContainerID() (string, error) {
	panic("not called")
}

func makeCached[T comparable](cr CollectorRef[T], cache T) CollectorRef[T] {
	panic("not called")
}

func getOrFallback[T any](cache *Cache, cacheKey string, cacheValidity time.Duration, collect func() (T, error)) (T, error) {
	panic("not called")
}
