// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"strconv"
	"time"
)

const (
	contCoreStatsCachePrefix           = "cs-"
	contOpenFilesCachePrefix           = "of-"
	contNetStatsCachePrefix            = "cns-"
	contPidToCidCachePrefix            = "pid-"
	contInodeToCidCachePrefix          = "in-"
	contPodUIDContNameToCidCachePrefix = "pc-"
	contPidsCachePrefix                = "pids-"
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
	collectorCache := &collectorCache{
		providerID: providerID,
		cache:      cache,
		collectors: collectors,
	}

	return &Collectors{
		Stats:                           makeCached(collectors.Stats, ContainerStatsGetter(collectorCache)),
		Network:                         makeCached(collectors.Network, ContainerNetworkStatsGetter(collectorCache)),
		OpenFilesCount:                  makeCached(collectors.OpenFilesCount, ContainerOpenFilesCountGetter(collectorCache)),
		PIDs:                            makeCached(collectors.PIDs, ContainerPIDsGetter(collectorCache)),
		ContainerIDForPID:               makeCached(collectors.ContainerIDForPID, ContainerIDForPIDRetriever(collectorCache)),
		ContainerIDForInode:             makeCached(collectors.ContainerIDForInode, ContainerIDForInodeRetriever(collectorCache)),
		SelfContainerID:                 makeCached(collectors.SelfContainerID, SelfContainerIDRetriever(collectorCache)),
		ContainerIDForPodUIDAndContName: makeCached(collectors.ContainerIDForPodUIDAndContName, ContainerIDForPodUIDAndContNameRetriever(collectorCache)),
	}
}

// GetContainerStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	cacheKey := cc.providerID + "-" + contCoreStatsCachePrefix + containerNS + containerID

	return getOrFallback(cc.cache, cacheKey, cacheValidity, func() (*ContainerStats, error) {
		return cc.collectors.Stats.Collector.GetContainerStats(containerNS, containerID, cacheValidity)
	})
}

// GetContainerNetworkStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	cacheKey := cc.providerID + "-" + contNetStatsCachePrefix + containerNS + containerID

	return getOrFallback(cc.cache, cacheKey, cacheValidity, func() (*ContainerNetworkStats, error) {
		return cc.collectors.Network.Collector.GetContainerNetworkStats(containerNS, containerID, cacheValidity)
	})
}

// GetContainerOpenFilesCount returns the count of open files if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	cacheKey := cc.providerID + "-" + contOpenFilesCachePrefix + containerNS + containerID

	return getOrFallback(cc.cache, cacheKey, cacheValidity, func() (*uint64, error) {
		return cc.collectors.OpenFilesCount.Collector.GetContainerOpenFilesCount(containerNS, containerID, cacheValidity)
	})
}

// GetPIDs returns the container ID for given PID
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetPIDs(containerNS, containerID string, cacheValidity time.Duration) ([]int, error) {
	cacheKey := cc.providerID + "-" + contPidsCachePrefix + containerNS + containerID

	return getOrFallback(cc.cache, cacheKey, cacheValidity, func() ([]int, error) {
		return cc.collectors.PIDs.Collector.GetPIDs(containerNS, containerID, cacheValidity)
	})
}

// GetContainerIDForPID returns the container ID for given PID
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	cacheKey := cc.providerID + "-" + contPidToCidCachePrefix + strconv.FormatInt(int64(pid), 10)

	return getOrFallback(cc.cache, cacheKey, cacheValidity, func() (string, error) {
		return cc.collectors.ContainerIDForPID.Collector.GetContainerIDForPID(pid, cacheValidity)
	})
}

// GetContainerIDForInode returns a container ID for the given inode.
// ("", nil) will be returned if no error but the containerd ID was not found.
func (cc *collectorCache) GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error) {
	cacheKey := cc.providerID + "-" + contInodeToCidCachePrefix + strconv.FormatUint(inode, 10)

	return getOrFallback(cc.cache, cacheKey, cacheValidity, func() (string, error) {
		return cc.collectors.ContainerIDForInode.Collector.GetContainerIDForInode(inode, cacheValidity)
	})
}

// ContainerIDForPodUIDAndContName returns a container ID for the given pod uid
// and container name. Returns ("", nil) if the containerd ID was not found.
func (cc *collectorCache) ContainerIDForPodUIDAndContName(podUID, contName string, initCont bool, cacheValidity time.Duration) (string, error) {
	initPrefix := ""
	if initCont {
		initPrefix = "i-"
	}
	cacheKey := cc.providerID + "-" + contPodUIDContNameToCidCachePrefix + podUID + "/" + initPrefix + contName

	return getOrFallback(cc.cache, cacheKey, cacheValidity, func() (string, error) {
		return cc.collectors.ContainerIDForPodUIDAndContName.Collector.ContainerIDForPodUIDAndContName(podUID, contName, initCont, cacheValidity)
	})
}

// GetSelfContainerID returns current process container ID
// No caching as it's not supposed to change
func (cc *collectorCache) GetSelfContainerID() (string, error) {
	if cc.selfContainerIDCache != "" {
		return cc.selfContainerIDCache, nil
	}

	selfID, err := cc.collectors.SelfContainerID.Collector.GetSelfContainerID()
	if err == nil {
		cc.selfContainerIDCache = selfID
	}

	return selfID, err
}

func makeCached[T comparable](cr CollectorRef[T], cache T) CollectorRef[T] {
	var zero T
	if cr.Collector != zero {
		cr.Collector = cache
	}

	return cr
}

func getOrFallback[T any](cache *Cache, cacheKey string, cacheValidity time.Duration, collect func() (T, error)) (T, error) {
	currentTime := time.Now()

	entry, found, err := cache.Get(currentTime, cacheKey, cacheValidity)
	if found {
		if err != nil {
			return *new(T), err
		}

		return entry.(T), nil
	}

	// No cache, cacheValidity is 0 or too old value
	val, err := collect()
	if err != nil {
		cache.Store(currentTime, cacheKey, nil, err)
		return *new(T), err
	}

	cache.Store(currentTime, cacheKey, val, nil)
	return val, nil
}
