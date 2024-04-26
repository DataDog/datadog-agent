// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"time"
)

// Individual interfaces, used to dynamically register available implementations

// ContainerStatsGetter interface
type ContainerStatsGetter interface {
	GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error)
}

// ContainerNetworkStatsGetter interface
type ContainerNetworkStatsGetter interface {
	GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error)
}

// ContainerOpenFilesCountGetter interface
type ContainerOpenFilesCountGetter interface {
	GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error)
}

// ContainerPIDsGetter interface
type ContainerPIDsGetter interface {
	GetPIDs(containerNS, containerID string, cacheValidity time.Duration) ([]int, error)
}

// ContainerIDForPIDRetriever interface
type ContainerIDForPIDRetriever interface {
	// GetContainerIDForPID returns a container ID for given PID.
	// ("", nil) will be returned if no error but the containerd ID was not found.
	GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error)
}

// ContainerIDForInodeRetriever interface
type ContainerIDForInodeRetriever interface {
	// GetContainerIDForInode returns a container ID for the given inode.
	// ("", nil) will be returned if no error but the containerd ID was not found.
	GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error)
}

// ContainerIDForPodUIDAndContNameRetriever interface
type ContainerIDForPodUIDAndContNameRetriever interface {
	// ContainerIDForPodUIDAndContName returns a container ID for the given pod uid
	// and container name. Returns ("", nil) if the containerd ID was not found.
	ContainerIDForPodUIDAndContName(podUID, contName string, initCont bool, cacheValidity time.Duration) (string, error)
}

// SelfContainerIDRetriever interface
type SelfContainerIDRetriever interface {
	// GetSelfContainerID returns the container ID for current container.
	// ("", nil) will be returned if not possible to get ID for current container.
	GetSelfContainerID() (string, error)
}

// Collector defines the public interface
type Collector interface {
	ContainerStatsGetter
	ContainerNetworkStatsGetter
	ContainerOpenFilesCountGetter
	ContainerPIDsGetter
}

// MetaCollector is a special collector that uses all available collectors, by priority order.
type MetaCollector interface {
	ContainerIDForPIDRetriever
	ContainerIDForInodeRetriever
	SelfContainerIDRetriever
	ContainerIDForPodUIDAndContNameRetriever
}
