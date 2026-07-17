// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const workloadTagCacheTelemetrySubsystem = consts.GpuTelemetryModule + "__workload_tag_cache"

type workloadTagCacheEntry struct {
	tags  []string
	stale bool
}

// WorkloadTagCache encapsulates the logic for retrieving and caching workload
// tags for GPU monitoring metrics. The cache needs to be invalidated on each
// run of the check in order to avoid stale data. Invalidation does not entirely
// remove the data from the cache, as it might be useful to retrieve data for
// processes or containers that no longer exist but were still in the cache from
// the previous run.
type WorkloadTagCache struct {
	cache             *simplelru.LRU[workloadmeta.EntityID, *workloadTagCacheEntry]
	tagger            tagger.Component
	wmeta             workloadmeta.Component
	containerProvider proccontainers.ContainerProvider // containerProvider is used as a fallback to get a PID -> CID mapping when workloadmeta does not have the process data
	pidToSlurm        map[int]model.SlurmInfo          // pidToSlurm maps PIDs to the owning Slurm job identity, resolved system-probe-side; nil/empty means no Slurm tagging.
	pidToCid          map[int]string                   // pidToCid is the mapping of PIDs to container IDs, retrieved from the container provider until it is invalidated.
	telemetry         *workloadTagCacheTelemetry       // telemetry is the telemetry component for the workload tag cache
}

type workloadTagCacheTelemetry struct {
	cacheHits        telemetry.Counter
	cacheMisses      telemetry.Counter
	buildErrors      telemetry.Counter
	staleEntriesUsed telemetry.Counter
	processFallbacks telemetry.Counter
	cacheEvictions   telemetry.Counter
	cacheSize        telemetry.Gauge
}

func newWorkloadTagCacheTelemetry(tm telemetry.Component) *workloadTagCacheTelemetry {
	return &workloadTagCacheTelemetry{
		cacheHits:        tm.NewCounter(workloadTagCacheTelemetrySubsystem, "hits", []string{"entity_kind"}, "Number of cache hits"),
		cacheMisses:      tm.NewCounter(workloadTagCacheTelemetrySubsystem, "misses", []string{"entity_kind"}, "Number of cache misses"),
		cacheEvictions:   tm.NewCounter(workloadTagCacheTelemetrySubsystem, "evictions", []string{"entity_kind"}, "Number of cache evictions"),
		staleEntriesUsed: tm.NewCounter(workloadTagCacheTelemetrySubsystem, "stale_entries_used", []string{"entity_kind"}, "Number of stale cache used"),
		cacheSize:        tm.NewGauge(workloadTagCacheTelemetrySubsystem, "size", []string{}, "Cache size"),
		buildErrors:      tm.NewCounter(workloadTagCacheTelemetrySubsystem, "build_errors", []string{"entity_kind"}, "Number of errors building workload tags"),
		processFallbacks: tm.NewCounter(workloadTagCacheTelemetrySubsystem, "process_fallbacks", []string{}, "Counter with the number of times we had to fall back to getting process data directly, instead of through workloadmeta"),
	}
}

// NewWorkloadTagCache creates a new WorkloadTagCache
func NewWorkloadTagCache(tagger tagger.Component, wmeta workloadmeta.Component, containerProvider proccontainers.ContainerProvider, tm telemetry.Component, cacheSize int) (*WorkloadTagCache, error) {
	c := &WorkloadTagCache{
		tagger:            tagger,
		wmeta:             wmeta,
		containerProvider: containerProvider,
		telemetry:         newWorkloadTagCacheTelemetry(tm),
	}

	var err error
	c.cache, err = simplelru.NewLRU(cacheSize, c.onLRUEvicted)
	if err != nil {
		return nil, fmt.Errorf("error creating LRU cache: %w", err)
	}

	return c, nil
}

// Size returns the number of entries in the cache
func (c *WorkloadTagCache) Size() int {
	return c.cache.Len()
}

// GetOrCreateWorkloadTags retrieves the tags for a workload from the cache or builds them if they are not in the cache.
// Returns an error if the workload kind is unsupported. If we cannot find the entity, we return "ErrNotFound".
// If an error happens, this function will return the previously cached tags if they exist, along with the error
// that happened when getting them.
func (c *WorkloadTagCache) GetOrCreateWorkloadTags(workloadID workloadmeta.EntityID) ([]string, error) {
	cacheEntry, cacheEntryExists := c.cache.Get(workloadID)
	if cacheEntryExists && !cacheEntry.stale {
		c.telemetry.cacheHits.Inc(string(workloadID.Kind))
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
		c.cache.Add(workloadID, cacheEntry)
	}

	if err == nil {
		// If no error happened, we can assume that the new tags are correct, so we store them
		cacheEntry.tags = tags
		c.telemetry.cacheMisses.Inc(string(workloadID.Kind))
	} else if cacheEntryExists {
		// an error happened, so we return the previously cached tags
		tags = cacheEntry.tags
		c.telemetry.staleEntriesUsed.Inc(string(workloadID.Kind))
	} else {
		// This is the worst case, we had an error and no previous tags, so we cannot return anything
		cacheEntry.tags = tags // Because we had nothing, store whatever we got, processes for example returns partial tags.
		c.telemetry.buildErrors.Inc(string(workloadID.Kind))
	}

	// Now we always mark the cache entry as not stale. This is obvious in the case of no error, but
	// for the error case it's also useful to avoid re-trying an operation that already failed.
	// If the error was temporary, it will be retried after the next run.
	cacheEntry.stale = false

	return tags, err
}

// SetContainerProvider sets the container provider after construction.
func (c *WorkloadTagCache) SetContainerProvider(p proccontainers.ContainerProvider) {
	c.containerProvider = p
	c.pidToCid = nil
}

// slurmInfoFromStats converts the system-probe GPU stats payload's PID -> Slurm identity map
// (keyed by host PID as uint32) into the int-keyed map used by the tag cache, keeping only
// processes that resolved to an actual Slurm job. Returns nil for a nil/empty payload so the
// common (no-Slurm) case allocates nothing.
func slurmInfoFromStats(stats *model.GPUStats) map[int]model.SlurmInfo {
	if stats == nil || len(stats.SlurmInfoByPID) == 0 {
		return nil
	}

	pidToSlurm := make(map[int]model.SlurmInfo, len(stats.SlurmInfoByPID))
	for pid, info := range stats.SlurmInfoByPID {
		if info.JobID == "" {
			continue
		}
		pidToSlurm[int(pid)] = info
	}
	if len(pidToSlurm) == 0 {
		return nil
	}
	return pidToSlurm
}

// SetSlurmInfo replaces the PID -> Slurm job identity mapping for the current run. The mapping is
// built from the system-probe GPU stats payload (which resolves Slurm identity where it already
// holds SYS_PTRACE). An empty or nil map means no process has a resolved Slurm identity, so no
// Slurm tags are added.
func (c *WorkloadTagCache) SetSlurmInfo(pidToSlurm map[int]model.SlurmInfo) {
	c.pidToSlurm = pidToSlurm
}

// MarkStale marks all entries in the cache as stale. That way, on the next calls to GetWorkloadTags, we will
// try to rebuild them, anf if we can't we will return stale data.
func (c *WorkloadTagCache) MarkStale() {
	for entry := range c.cache.ValuesIter() {
		entry.stale = true
	}

	// Invalidate the PID -> CID mapping, so that it's refreshed on the next run
	c.pidToCid = nil

	// Update the telemetry metrics with the current state of the cache.
	c.telemetry.cacheSize.Set(float64(c.cache.Len()))
}

// buildContainerTags builds the tags for a container. Can return "ErrNotFound"
// (coming from the workloadmeta component) if the container is not found.
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

// buildProcessTags builds the tags for a process. Can return "ErrNotFound" if the process
// is not found in workloadmeta and is not running.
func (c *WorkloadTagCache) buildProcessTags(processID string) ([]string, error) {
	var multiErr error

	pidInt, err := strconv.ParseInt(processID, 10, 32)
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
	}

	// Fallbacks in case workloadmeta does not have the data we need
	var contErr, nspidErr error
	usedFallbacks := false
	if containerID == "" {
		usedFallbacks = true

		containerID, contErr = c.getContainerID(pid)
		if contErr != nil && !agenterrors.IsNotFound(contErr) {
			multiErr = errors.Join(multiErr, fmt.Errorf("error getting container ID for process %d: %w", pid, contErr))
		}
	}

	if nspid == 0 {
		usedFallbacks = true

		nspid, nspidErr = getNsPID(uint32(pid))
		if nspidErr != nil && !errors.Is(nspidErr, secutils.ErrNoNSPid) {
			multiErr = errors.Join(multiErr, fmt.Errorf("error getting nspid for process %d: %w", pid, nspidErr))
		}

		// default value for tags is nspid=pid if the process is not running in a container
		if nspid == 0 {
			nspid = pid
		}
	}
	tags = append(tags, fmt.Sprintf("nspid:%d", nspid))

	if contErr != nil && nspidErr != nil && !kernel.ProcessExists(int(pid)) {
		// The process does not exist anymore, so return a "NotFound" error so that we can return stale data.
		return tags, agenterrors.NewNotFound(pid)
	}

	// Mark here and not before, to avoid incrementing the counter if the process does not exist anymore.
	if usedFallbacks {
		c.telemetry.processFallbacks.Inc()
	}

	if containerID != "" {
		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		}
		// Use GetWorkloadTags so that we hit the cache, buildContainerTags would re-create the tags every time
		containerTags, err := c.GetOrCreateWorkloadTags(entityID)
		if err != nil && !agenterrors.IsNotFound(err) {
			multiErr = errors.Join(multiErr, fmt.Errorf("error building container tags for process %d and container %s: %w", pid, containerID, err))
		}
		tags = append(tags, containerTags...)
	}

	// Slurm job identity is resolved system-probe-side and delivered via the GPU stats payload
	// (see SetSlurmInfo); the core agent only looks it up here, so no SYS_PTRACE is needed here.
	if info, ok := c.pidToSlurm[int(pid)]; ok && info.JobID != "" {
		tags = append(tags, "slurm_job_id:"+info.JobID)
		if info.JobName != "" {
			tags = append(tags, "slurm_job_name:"+info.JobName)
		}
		if info.Partition != "" {
			tags = append(tags, "slurm_job_partition:"+info.Partition)
		}
	}

	return tags, multiErr
}

// note: given /proc/X/task/Y/status, we have no guarantee that tasks Y will all
// have the same NSpid values, specially in case of unusual pid namespace setups.
// As such, we attempt reading the nspid for only on the main thread (group leader)
// in /proc/X/task/X/status, or fail otherwise
func getNsPID(pid uint32) (int32, error) {
	nspids, err := secutils.GetNsPids(pid, strconv.FormatUint(uint64(pid), 10))
	if err != nil {
		return 0, fmt.Errorf("could not get nspid for pid %d: %w", pid, err)
	}
	if len(nspids) == 0 {
		return 0, secutils.ErrNoNSPid
	}

	nspid := nspids[len(nspids)-1]
	if nspid > math.MaxInt32 {
		return 0, fmt.Errorf("nspid %d is too large to fit in int32", nspid)
	}

	return int32(nspid), nil
}

// getContainerID retrieves the container ID for a given PID from the container
// provider, used when workloadmeta does not have data for the process. We use
// the containerProvider as a fallback, however it also depends on workloadmeta
// for the container data. That dependency is not a problem for us: the reason
// we want the container is to get the container tags. If the container is not
// in workloadmeta we won't be able to get the tags anyway even if we got the
// container ID from another place that didn't depend at all on workloadmeta.
//
// Returns an empty string and a "ErrNotFound" error if the container is not
// found
func (c *WorkloadTagCache) getContainerID(pid int32) (string, error) {
	if c.pidToCid == nil {
		if c.containerProvider == nil {
			return "", errors.New("no container provider available")
		}

		// Get the PID -> CID mapping from the container provider with no cache validity, as we have already failed to hit the
		// workloadmeta cache for the process data. This mapping will be stored until it's invalidated for the
		// next run
		c.pidToCid = c.containerProvider.GetPidToCid(0)
	}

	containerID, exists := c.pidToCid[int(pid)]
	if !exists {
		return "", agenterrors.NewNotFound(pid)
	}

	return containerID, nil
}

func (c *WorkloadTagCache) onLRUEvicted(workloadID workloadmeta.EntityID, _ *workloadTagCacheEntry) {
	c.telemetry.cacheEvictions.Inc(string(workloadID.Kind))
}

// NewWorkloadTagCacheWithSubsystem creates a WorkloadTagCache that registers
// its telemetry counters under "<subsystemPrefix>__workload_tag_cache". The
// prefix must be unique per cache instance in the agent process.
func NewWorkloadTagCacheWithSubsystem(subsystemPrefix string, tagger tagger.Component, wmeta workloadmeta.Component, containerProvider proccontainers.ContainerProvider, tm telemetry.Component, cacheSize int) (*WorkloadTagCache, error) {
	c := &WorkloadTagCache{
		tagger:            tagger,
		wmeta:             wmeta,
		containerProvider: containerProvider,
		telemetry:         newWorkloadTagCacheTelemetryWithSubsystem(subsystemPrefix, tm),
	}

	var err error
	c.cache, err = simplelru.NewLRU(cacheSize, c.onLRUEvicted)
	if err != nil {
		return nil, fmt.Errorf("error creating LRU cache: %w", err)
	}

	return c, nil
}

func newWorkloadTagCacheTelemetryWithSubsystem(subsystemPrefix string, tm telemetry.Component) *workloadTagCacheTelemetry {
	subsystem := subsystemPrefix + "__workload_tag_cache"
	return &workloadTagCacheTelemetry{
		cacheHits:        tm.NewCounter(subsystem, "hits", []string{"entity_kind"}, "Number of cache hits"),
		cacheMisses:      tm.NewCounter(subsystem, "misses", []string{"entity_kind"}, "Number of cache misses"),
		cacheEvictions:   tm.NewCounter(subsystem, "evictions", []string{"entity_kind"}, "Number of cache evictions"),
		staleEntriesUsed: tm.NewCounter(subsystem, "stale_entries_used", []string{"entity_kind"}, "Number of stale cache used"),
		cacheSize:        tm.NewGauge(subsystem, "size", []string{}, "Cache size"),
		buildErrors:      tm.NewCounter(subsystem, "build_errors", []string{"entity_kind"}, "Number of errors building workload tags"),
		processFallbacks: tm.NewCounter(subsystem, "process_fallbacks", []string{}, "Counter with the number of times we had to fall back to getting process data directly, instead of through workloadmeta"),
	}
}
