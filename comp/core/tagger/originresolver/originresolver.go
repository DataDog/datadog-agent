// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package originresolver resolves the container that origin detection data comes from.
//
// It is shared by the consumers of origin detection data (e.g. the tagger and the
// DogStatsD workload filter) so that the container ID is resolved the same way
// everywhere, and only once: a producer can resolve the origin with Resolve and
// attach the returned ResolvedOrigin to the OriginInfo, which the other consumers
// then reuse instead of doing the same lookups again.
package originresolver

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
)

// CacheValidity is the cache validity used for the meta collector lookups.
const CacheValidity = time.Second

var containerIDFromSocketCutIndex = len(types.ContainerID) + types.GetSeparatorLength()

// ContainerIDFromSocket returns the container ID of an origin resolved with Unix Domain
// Socket origin detection ("container_id://<id>"), or an empty string if there is none.
func ContainerIDFromSocket(containerIDFromSocket string) string {
	if len(containerIDFromSocket) <= containerIDFromSocketCutIndex {
		return ""
	}
	return containerIDFromSocket[containerIDFromSocketCutIndex:]
}

// ContainerIDForInode returns the container ID for the given cgroup inode.
// ("", nil) is returned when the inode is unset or the container is not found.
func ContainerIDForInode(inode uint64, retriever provider.ContainerIDForInodeRetriever) (string, error) {
	if inode == 0 || retriever == nil {
		return "", nil
	}
	return retriever.GetContainerIDForInode(inode, CacheValidity)
}

// ContainerIDForExternalData returns the container ID for the given external data.
// ("", nil) is returned when the external data is incomplete or the container is not found.
func ContainerIDForExternalData(externalData origindetection.ExternalData, retriever provider.ContainerIDForPodUIDAndContNameRetriever) (string, error) {
	if externalData.PodUID == "" || externalData.ContainerName == "" || retriever == nil {
		return "", nil
	}
	return retriever.ContainerIDForPodUIDAndContName(externalData.PodUID, externalData.ContainerName, externalData.Init, CacheValidity)
}

// InodeContainerID returns the container ID for the origin inode, reusing
// origin.Resolved when the inode was already resolved.
func InodeContainerID(origin taggertypes.OriginInfo, retriever provider.ContainerIDForInodeRetriever) (string, error) {
	if origin.Resolved != nil && origin.Resolved.InodeDone {
		return origin.Resolved.InodeContainerID, nil
	}
	return ContainerIDForInode(origin.LocalData.Inode, retriever)
}

// ExternalDataContainerID returns the container ID for the origin external data,
// reusing origin.Resolved when the external data was already resolved.
func ExternalDataContainerID(origin taggertypes.OriginInfo, retriever provider.ContainerIDForPodUIDAndContNameRetriever) (string, error) {
	if origin.Resolved != nil && origin.Resolved.ExternalDataDone {
		return origin.Resolved.ExternalDataContainerID, nil
	}
	return ContainerIDForExternalData(origin.ExternalData, retriever)
}

// Resolve returns the container the origin comes from, by order of reliability:
// UDS origin, client container ID, inode, then external data.
//
// The inode and external data lookups are only done when the previous sources don't
// give a container ID. The lookups that were done, including failed ones, are returned
// as a ResolvedOrigin (nil if no lookup was done) to attach to the origin so that
// other consumers don't do them again. It must not be modified once attached.
func Resolve(origin taggertypes.OriginInfo, metaCollector provider.MetaCollector) (string, *taggertypes.ResolvedOrigin) {
	if containerID := ContainerIDFromSocket(origin.ContainerIDFromSocket); containerID != "" {
		return containerID, nil
	}
	if origin.LocalData.ContainerID != "" {
		return origin.LocalData.ContainerID, nil
	}
	// Without a meta collector, nothing is resolved: don't report lookups as done
	// so that other consumers can still do them.
	if metaCollector == nil {
		return "", nil
	}

	var resolved *taggertypes.ResolvedOrigin
	if origin.LocalData.Inode != 0 {
		resolved = &taggertypes.ResolvedOrigin{InodeDone: true}
		resolved.InodeContainerID, _ = ContainerIDForInode(origin.LocalData.Inode, metaCollector)
		if resolved.InodeContainerID != "" {
			return resolved.InodeContainerID, resolved
		}
	}

	if origin.ExternalData.PodUID != "" && origin.ExternalData.ContainerName != "" {
		if resolved == nil {
			resolved = &taggertypes.ResolvedOrigin{}
		}
		resolved.ExternalDataDone = true
		resolved.ExternalDataContainerID, _ = ContainerIDForExternalData(origin.ExternalData, metaCollector)
		return resolved.ExternalDataContainerID, resolved
	}

	return "", resolved
}
