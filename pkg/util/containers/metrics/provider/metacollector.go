// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"sort"
	"sync"
	"time"
)

// MetaCollector is a special collector that uses all available collectors, by priority order.
type metaCollector struct {
	lock                                      sync.RWMutex
	selfContainerIDcollectors                 []CollectorRef[SelfContainerIDRetriever]
	containerIDFromPIDcollectors              []CollectorRef[ContainerIDForPIDRetriever]
	containerIDFromInodeCollectors            []CollectorRef[ContainerIDForInodeRetriever]
	ContainerIDForPodUIDAndContNameCollectors []CollectorRef[ContainerIDForPodUIDAndContNameRetriever]
}

func newMetaCollector() *metaCollector {
	return &metaCollector{}
}

func (mc *metaCollector) collectorsUpdatedCallback(collectorsCatalog CollectorCatalog) {
	mc.lock.Lock()
	defer mc.lock.Unlock()

	mc.selfContainerIDcollectors = buildUniqueCollectors(collectorsCatalog, func(c *Collectors) CollectorRef[SelfContainerIDRetriever] { return c.SelfContainerID })
	mc.containerIDFromPIDcollectors = buildUniqueCollectors(collectorsCatalog, func(c *Collectors) CollectorRef[ContainerIDForPIDRetriever] { return c.ContainerIDForPID })
	mc.containerIDFromInodeCollectors = buildUniqueCollectors(collectorsCatalog, func(c *Collectors) CollectorRef[ContainerIDForInodeRetriever] { return c.ContainerIDForInode })
	mc.ContainerIDForPodUIDAndContNameCollectors = buildUniqueCollectors(collectorsCatalog, func(c *Collectors) CollectorRef[ContainerIDForPodUIDAndContNameRetriever] {
		return c.ContainerIDForPodUIDAndContName
	})
}

// GetSelfContainerID returns the container ID for current container.
func (mc *metaCollector) GetSelfContainerID() (string, error) {
	mc.lock.RLock()
	defer mc.lock.RUnlock()

	for _, collectorRef := range mc.selfContainerIDcollectors {
		val, err := collectorRef.Collector.GetSelfContainerID()
		if err != nil {
			return "", err
		}

		if val != "" {
			return val, nil
		}
	}

	return "", nil
}

// GetContainerIDForPID returns a container ID for given PID.
func (mc *metaCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	mc.lock.RLock()
	defer mc.lock.RUnlock()

	for _, collectorRef := range mc.containerIDFromPIDcollectors {
		val, err := collectorRef.Collector.GetContainerIDForPID(pid, cacheValidity)
		if err != nil {
			return "", err
		}

		if val != "" {
			return val, nil
		}
	}

	return "", nil
}

// GetContainerIDForInode returns a container ID for the given inode.
// ("", nil) will be returned if no error but the containerd ID was not found.
func (mc *metaCollector) GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error) {
	mc.lock.RLock()
	defer mc.lock.RUnlock()

	for _, collectorRef := range mc.containerIDFromInodeCollectors {
		val, err := collectorRef.Collector.GetContainerIDForInode(inode, cacheValidity)
		if err != nil {
			return "", err
		}

		if val != "" {
			return val, nil
		}
	}

	return "", nil
}

// ContainerIDForPodUIDAndContName returns a container ID for the given pod uid
// and container name. Returns ("", nil) if the containerd ID was not found.
func (mc *metaCollector) ContainerIDForPodUIDAndContName(podUID, contName string, initCont bool, cacheValidity time.Duration) (string, error) {
	mc.lock.RLock()
	defer mc.lock.RUnlock()

	for _, collectorRef := range mc.ContainerIDForPodUIDAndContNameCollectors {
		val, err := collectorRef.Collector.ContainerIDForPodUIDAndContName(podUID, contName, initCont, cacheValidity)
		if err != nil {
			return "", err
		}

		if val != "" {
			return val, nil
		}
	}

	return "", nil
}

func buildUniqueCollectors[T comparable](collectorsCatalog CollectorCatalog, getter func(*Collectors) CollectorRef[T]) []CollectorRef[T] {
	// We don't need to optimize performances too much as this is called only a handful of times
	var zero T
	uniqueCollectors := make(map[CollectorRef[T]]struct{})
	var sortedCollectors []CollectorRef[T]

	for _, collectors := range collectorsCatalog {
		if collectors != nil {
			collectorRef := getter(collectors)
			if collectorRef.Collector != zero {
				uniqueCollectors[collectorRef] = struct{}{}
			}
		}
	}

	for collectorRef := range uniqueCollectors {
		sortedCollectors = append(sortedCollectors, collectorRef)
	}

	sort.Slice(sortedCollectors, func(i, j int) bool {
		return sortedCollectors[i].Priority < sortedCollectors[j].Priority
	})

	return sortedCollectors
}
