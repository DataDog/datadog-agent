// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"errors"
	"fmt"
	"strconv"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"
)

type workloadTagCacheEntry struct {
	tags  []string
	valid bool
}

// WorkloadTagCache encapsulates the logic for retrieving and caching workload
// tags for GPU monitoring metrics. The cache needs to be invalidated on each
// run of the check in order to avoid stale data. Invalidation does not entirely
// remove the data from the cache, as it might be useful to retrieve data for
// processes or containers that no longer exist but were still in the cache from
// the previous run.
type WorkloadTagCache struct {
	cache             map[workloadmeta.EntityID]*workloadTagCacheEntry
	tagger            tagger.Component
	wmeta             workloadmeta.Component
	containerProvider proccontainers.ContainerProvider // containerProvider is used as a fallback to get a PID -> CID mapping when workloadmeta does not have the process data
	pidToCid          map[int]string                   // pidToCid is the mapping of PIDs to container IDs, retrieved from the container provider until it is invalidated.
}

// NewWorkloadTagCache creates a new WorkloadTagCache
func NewWorkloadTagCache(tagger tagger.Component, wmeta workloadmeta.Component, containerProvider proccontainers.ContainerProvider) *WorkloadTagCache {
	return &WorkloadTagCache{
		cache:             make(map[workloadmeta.EntityID]*workloadTagCacheEntry),
		tagger:            tagger,
		wmeta:             wmeta,
		containerProvider: containerProvider,
	}
}

// GetWorkloadTags retrieves the tags for a workload from the cache or builds them if they are not in the cache.
// Returns an error if the workload kind is unsupported. If we cannot find the entity, we return "ErrNotFound".
// If an error happens, this function will return the previously cached tags if they exist, along with the error
// that happened when getting them.
func (c *WorkloadTagCache) GetWorkloadTags(workloadID workloadmeta.EntityID) ([]string, error) {
	cacheEntry, cacheEntryExists := c.cache[workloadID]
	if cacheEntryExists && cacheEntry.valid {
		return cacheEntry.tags, nil
	}

	var tags []string
	var err error

	switch workloadID.Kind {
	case workloadmeta.KindContainer:
		tags, err = c.buildContainerTags(workloadID.ID)
	case workloadmeta.KindProcess:
		tags, err = c.buildProcessTags(workloadID.ID)
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", workloadID.Kind)
	}

	// First, ensure we have a cache entry, to simplify the logic later
	if !cacheEntryExists {
		cacheEntry = &workloadTagCacheEntry{}
		c.cache[workloadID] = cacheEntry
	}

	if err == nil {
		// If no error happened, we can assume that the new tags are correct, so we store them
		cacheEntry.tags = tags
	} else if cacheEntryExists {
		// an error happened, so we return the previously cached tags
		tags = cacheEntry.tags
	}

	// Now we always mark the cache entry as valid. This is obvious in the case of no error, but
	// for the error case it's also useful to avoid re-trying an operation that already failed.
	// If the error was temporary, it will be retried after the next invalidation.
	cacheEntry.valid = true

	return tags, err
}

func (c *WorkloadTagCache) Invalidate() {
	for _, entry := range c.cache {
		// Mark entries as invalid, so that they are rebuilt on the next run, but can still
		// be used if we cannot find the entity later.
		entry.valid = false
	}
	c.pidToCid = nil
}

// buildContainerTags builds the tags for a container. Can return "ErrNotFound" if the container is not found.
func (c *WorkloadTagCache) buildContainerTags(containerID string) ([]string, error) {
	container, err := c.wmeta.GetContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("error getting container for workload %s: %w", containerID, err)
	}

	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)

	// we use orchestrator cardinality here to ensure we get the pod_name tag
	// ref: https://docs.datadoghq.com/containers/kubernetes/tag/?tab=datadogoperator#out-of-the-box-tags
	cardinality := taggertypes.OrchestratorCardinality
	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		// For Docker, we use high cardinality to get the container_name and container_id tags
		// that uniquely identify the container.
		// ref: https://docs.datadoghq.com/containers/docker/tag/#out-of-the-box-tagging
		cardinality = taggertypes.HighCardinality
	}

	return c.tagger.Tag(entityID, cardinality)
}

// buildProcessTags builds the tags for a process. Does not return "ErrNotFound", as it will try to get
// the data directly if workloadmeta does not have process data.
func (c *WorkloadTagCache) buildProcessTags(processID string) ([]string, error) {
	var multiErr error

	pidInt, err := strconv.Atoi(processID)
	if err != nil {
		return nil, fmt.Errorf("error converting process ID to int: %w", err)
	}
	pid := int32(pidInt)

	tags := []string{fmt.Sprintf("pid:%d", pid)}

	// Apart from PID, we try to add nspid and container-related tags if
	// available. Workloadmeta can provide this information, but it might not be
	// always available (e.g., a process hasn't still been detected by the
	// process collector, or it might have been removed)
	var nspid int32
	var containerID string
	process, err := c.wmeta.GetProcess(pid)
	if err == nil {
		nspid = process.NsPid
		if process.Owner != nil && process.Owner.Kind == workloadmeta.KindContainer {
			containerID = process.Owner.ID
		}
	} else if agenterrors.IsNotFound(err) {
		// If workloadmeta does not have the process data, we fall back to
		// retrieving the data directly.
		nspid, err = getNsPID(pid)
		if err != nil && !agenterrors.IsNotFound(err) {
			// A non-containerized process might not have a nspid, so ignore "NotFound" errors
			multiErr = errors.Join(multiErr, fmt.Errorf("error getting nspid for process %d: %w", pid, err))
		}

		containerID = c.getContainerID(pid)
	}

	if nspid == 0 {
		// default value for tags is nspid=pid if the process is not running in a container
		nspid = pid
	}
	tags = append(tags, fmt.Sprintf("nspid:%d", nspid))

	if containerID != "" {
		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		}
		// Use GetWorkloadTags so that we hit the cache, buildContainerTags would re-create the tags every time
		containerTags, err := c.GetWorkloadTags(entityID)
		if err != nil && !agenterrors.IsNotFound(err) {
			multiErr = errors.Join(multiErr, fmt.Errorf("error building container tags for process %d and container %s: %w", pid, containerID, err))
		}
		tags = append(tags, containerTags...)
	}

	return tags, multiErr
}

// note: given /proc/X/task/Y/status, we have no guarantee that tasks Y will all
// have the same NSpid values, specially in case of unusual pid namespace setups.
// As such, we attempt reading the nspid for only on the main thread (group leader)
// in /proc/X/task/X/status, or fail otherwise
func getNsPID(pid int32) (int32, error) {
	nspids, err := secutils.GetNsPids(uint32(pid), strconv.FormatUint(uint64(pid), 10))
	if err != nil && agenterrors.IsNotFound(err) {
		// (temporary) report agenterrors.IsNotFound directly, IsNotFound doesnot support wrapped errors so if we wrap here we'll lose the ability to check for it later
		// Remove this once IsNotFound supports wrapped errors
		return 0, err
	} else if err != nil {
		return 0, fmt.Errorf("failed reading nspids for host pid %d: %w", pid, err)
	}
	if len(nspids) == 0 {
		return 0, fmt.Errorf("found no nspids for host pid %d", pid)
	}

	return int32(nspids[len(nspids)-1]), nil
}

// getContainerID retrieves the container ID for a given PID from the container
// provider, used when workloadmeta does not have data for the process. We use
// the containerProvider as a fallback, however it also depends on workloadmeta
// for the container data. That dependency is not a problem for us: the reason we want
// the container is to get the container tags. If the container is not in workloadmeta
// we won't be able to get the tags anyway even if we got the container ID from another place
// that didn't depend at all on workloadmeta.
// Returns an empty string if the container ID is not found.
func (c *WorkloadTagCache) getContainerID(pid int32) string {
	if c.pidToCid == nil {
		if c.containerProvider == nil {
			// with no container provider, we cannot get the container ID, so return an empty string.
			// this might happen in tests that use the WorkloadTagCache without a container provider, where this logic is not needed
			// or under test but might cause panics if we don't have this check.
			return ""
		}

		// Get the PID -> CID mapping from the container provider with no cache validity, as we have already failed to hit the
		// workloadmeta cache for the process data. This mapping will be stored until it's invalidated for the
		// next run
		c.pidToCid = c.containerProvider.GetPidToCid(0)
	}

	containerID, exists := c.pidToCid[int(pid)]
	if !exists {
		return ""
	}

	return containerID
}
