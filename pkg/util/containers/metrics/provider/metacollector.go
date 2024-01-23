// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"sync"
	"time"
)

// MetaCollector is a special collector that uses all available collectors, by priority order.
type metaCollector struct {
	lock                           sync.RWMutex
	selfContainerIDcollectors      []CollectorRef[SelfContainerIDRetriever]
	containerIDFromPIDcollectors   []CollectorRef[ContainerIDForPIDRetriever]
	containerIDFromInodeCollectors []CollectorRef[ContainerIDForInodeRetriever]
}

func newMetaCollector() *metaCollector {
	return &metaCollector{}
}

func (mc *metaCollector) collectorsUpdatedCallback(collectorsCatalog CollectorCatalog) {
	panic("not called")
}

// GetSelfContainerID returns the container ID for current container.
func (mc *metaCollector) GetSelfContainerID() (string, error) {
	panic("not called")
}

// GetContainerIDForPID returns a container ID for given PID.
func (mc *metaCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	panic("not called")
}

// GetContainerIDForInode returns a container ID for given Inode.
// ("", nil) will be returned if no error but the containerd ID was not found.
func (mc *metaCollector) GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error) {
	panic("not called")
}

func buildUniqueCollectors[T comparable](collectorsCatalog CollectorCatalog, getter func(*Collectors) CollectorRef[T]) []CollectorRef[T] {
	panic("not called")
}
