// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

const (
	collectorHighPriority  uint8 = 0
	collectorLowPriority   uint8 = 10
	systemCollectorID            = "system"
	cgroupV1BaseController       = "memory"
)

func init() {
	provider.RegisterCollector(provider.CollectorFactory{
		ID: systemCollectorID,
		Constructor: func(cache *provider.Cache, wlm optional.Option[workloadmeta.Component]) (provider.CollectorMetadata, error) {
			return newSystemCollector(cache, wlm)
		},
	})
}

type systemCollector struct {
	reader              *cgroups.Reader
	selfReader          *cgroups.Reader
	pidMapper           cgroups.StandalonePIDMapper
	procPath            string
	baseController      string
	hostCgroupNamespace bool
}

func newSystemCollector(cache *provider.Cache, wlm optional.Option[workloadmeta.Component]) (provider.CollectorMetadata, error) {
	var err error
	var hostPrefix string
	var collectorMetadata provider.CollectorMetadata
	var cf cgroups.ReaderFilter

	procPath := pkgconfigsetup.Datadog().GetString("container_proc_root")
	if strings.HasPrefix(procPath, "/host") {
		hostPrefix = "/host"
	}

	if useTrie := pkgconfigsetup.Datadog().GetBool("use_improved_cgroup_parser"); useTrie {
		var w workloadmeta.Component
		unwrapped, ok := wlm.Get()
		if ok {
			w = unwrapped
		}
		filter := newContainerFilter(w)
		go filter.start()
		cf = filter.ContainerFilter
	} else {
		cf = cgroups.ContainerFilter
	}
	reader, err := cgroups.NewReader(
		cgroups.WithCgroupV1BaseController(cgroupV1BaseController),
		cgroups.WithProcPath(procPath),
		cgroups.WithHostPrefix(hostPrefix),
		cgroups.WithReaderFilter(cf),
		cgroups.WithPIDMapper(pkgconfigsetup.Datadog().GetString("container_pid_mapper")),
	)
	if err != nil {
		// Cgroup provider is pretty static. Except not having required mounts, it should always work.
		log.Infof("Unable to initialize cgroup provider (cgroups not mounted?), err: %v", err)
		return collectorMetadata, provider.ErrPermaFail
	}

	selfReader, err := cgroups.NewSelfReader(
		procPath,
		env.IsContainerized(),
		cgroups.WithCgroupV1BaseController(cgroupV1BaseController),
	)
	if err != nil {
		// Cgroup provider is pretty static. Except not having required mounts, it should always work.
		log.Infof("Unable to initialize self cgroup reader, err: %v", err)
		return collectorMetadata, provider.ErrPermaFail
	}
	systemCollector := &systemCollector{
		reader:     reader,
		selfReader: selfReader,
		procPath:   procPath,
	}

	// Set base controller for cgroupV1 (remains empty for cgroupV2)
	if reader.CgroupVersion() == 1 {
		systemCollector.baseController = cgroupV1BaseController
	}

	// Check if we are in host cgroup namespace or not. Will be useful to determine the best way to retrieve self container ID.
	cgroupInode, err := systemutils.GetProcessNamespaceInode("/proc", "self", "cgroup")
	if err != nil {
		log.Warn("Unable to determine cgroup namespace id in system collector")
	} else {
		if isCgroupHost := cgroups.IsProcessHostCgroupNamespace(procPath, cgroupInode); isCgroupHost != nil {
			systemCollector.hostCgroupNamespace = *isCgroupHost
		}
	}

	// Build available Collectors based on environment
	var collectors *provider.Collectors
	if env.IsContainerized() {
		collectors = &provider.Collectors{}

		// When the Agent runs as a sidecar (e.g. Fargate) with shared PID namespace, we can use the system collector in some cases.
		// TODO: Check how we could detect shared PID namespace instead of makeing assumption
		isAgentSidecar := env.IsFeaturePresent(env.ECSFargate) || env.IsFeaturePresent(env.EKSFargate)

		// With sysfs we can always get cgroup stats
		if env.IsHostSysAvailable() {
			collectors.Stats = provider.MakeRef[provider.ContainerStatsGetter](systemCollector, collectorHighPriority)
		}

		// With host proc we can always get network stats and pids
		if env.IsHostProcAvailable() {
			collectors.Network = provider.MakeRef[provider.ContainerNetworkStatsGetter](systemCollector, collectorHighPriority)
			collectors.OpenFilesCount = provider.MakeRef[provider.ContainerOpenFilesCountGetter](systemCollector, collectorHighPriority)
			collectors.PIDs = provider.MakeRef[provider.ContainerPIDsGetter](systemCollector, collectorHighPriority)
			collectors.ContainerIDForPID = provider.MakeRef[provider.ContainerIDForPIDRetriever](systemCollector, collectorHighPriority)
			collectors.ContainerIDForInode = provider.MakeRef[provider.ContainerIDForInodeRetriever](systemCollector, collectorHighPriority)
		} else if isAgentSidecar {
			// When side car with sharedPIDNamespace, we can get the same data.
			// As we don't know if we are sharedPIDNamespace, adding as low priority.
			systemCollector.pidMapper = cgroups.NewStandalonePIDMapper(systemCollector.procPath, systemCollector.baseController, cgroups.ContainerFilter)

			collectors.Network = provider.MakeRef[provider.ContainerNetworkStatsGetter](systemCollector, collectorLowPriority)
			collectors.OpenFilesCount = provider.MakeRef[provider.ContainerOpenFilesCountGetter](systemCollector, collectorLowPriority)
			collectors.PIDs = provider.MakeRef[provider.ContainerPIDsGetter](systemCollector, collectorLowPriority)
			collectors.ContainerIDForPID = provider.MakeRef[provider.ContainerIDForPIDRetriever](systemCollector, collectorLowPriority)
		}

		// We can retrieve self PID with cgroupv1 or cgroupv2 with host ns
		// We may be able to get it in some case from cgroupv2 from mountinfo
		if reader.CgroupVersion() == 1 || systemCollector.hostCgroupNamespace {
			collectors.SelfContainerID = provider.MakeRef[provider.SelfContainerIDRetriever](systemCollector, collectorHighPriority)
		} else {
			collectors.SelfContainerID = provider.MakeRef[provider.SelfContainerIDRetriever](systemCollector, collectorLowPriority)
		}
	} else {
		// When not running in a container, we can use everything
		collectors = &provider.Collectors{
			Stats:               provider.MakeRef[provider.ContainerStatsGetter](systemCollector, collectorHighPriority),
			Network:             provider.MakeRef[provider.ContainerNetworkStatsGetter](systemCollector, collectorHighPriority),
			OpenFilesCount:      provider.MakeRef[provider.ContainerOpenFilesCountGetter](systemCollector, collectorHighPriority),
			PIDs:                provider.MakeRef[provider.ContainerPIDsGetter](systemCollector, collectorHighPriority),
			ContainerIDForPID:   provider.MakeRef[provider.ContainerIDForPIDRetriever](systemCollector, collectorHighPriority),
			ContainerIDForInode: provider.MakeRef[provider.ContainerIDForInodeRetriever](systemCollector, collectorHighPriority),
			SelfContainerID:     provider.MakeRef[provider.SelfContainerIDRetriever](systemCollector, collectorHighPriority),
		}
	}
	log.Debugf("Chosen system collectors: %+v", collectors)

	// Build metadata
	metadata := provider.CollectorMetadata{
		ID:         systemCollectorID,
		Collectors: make(provider.CollectorCatalog),
	}

	// Always cache results
	collectors = provider.MakeCached(systemCollectorID, cache, collectors)

	// Finally add to catalog
	for _, runtime := range provider.AllLinuxRuntimes {
		metadata.Collectors[provider.NewRuntimeMetadata(string(runtime), "")] = collectors
	}

	return metadata, nil
}

func (c *systemCollector) GetContainerStats(_, containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	cg, err := c.getCgroup(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	return c.buildContainerMetrics(cg, cacheValidity)
}

func (c *systemCollector) GetContainerOpenFilesCount(_, containerID string, cacheValidity time.Duration) (*uint64, error) {
	pids, err := c.getPIDs(containerID, cacheValidity)
	if err != nil {
		return nil, fmt.Errorf("unable to get PIDs for cgroup id: %s. Unable to get count of open files", containerID)
	}

	ofCount, allFailed := systemutils.CountProcessesFileDescriptors(c.procPath, pids)
	if allFailed {
		return nil, fmt.Errorf("unable to read any PID open FDs for cgroup id: %s. Unable to get count of open files", containerID)
	}

	return &ofCount, nil
}

func (c *systemCollector) GetContainerNetworkStats(_, containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	pids, err := c.getPIDs(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	return buildNetworkStats(c.procPath, pids)
}

func (c *systemCollector) GetPIDs(_, containerID string, cacheValidity time.Duration) ([]int, error) {
	return c.getPIDs(containerID, cacheValidity)
}

func (c *systemCollector) GetContainerIDForPID(pid int, _ time.Duration) (string, error) {
	containerID, err := cgroups.IdentiferFromCgroupReferences(c.procPath, strconv.Itoa(pid), c.baseController, cgroups.ContainerFilter)
	return containerID, err
}

func (c *systemCollector) GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error) {
	cg := c.reader.GetCgroupByInode(inode)
	if cg == nil {
		err := c.reader.RefreshCgroups(cacheValidity)
		if err != nil {
			return "", fmt.Errorf("containerID not found from inode %d and unable to refresh cgroups, err: %w", inode, err)
		}

		cg = c.reader.GetCgroupByInode(inode)
		if cg == nil {
			return "", fmt.Errorf("containerID not found from inode %d", inode)
		}
	}

	return cg.Identifier(), nil
}

func (c *systemCollector) GetSelfContainerID() (string, error) {
	cid, err := c.getSelfContainerIDFromInode()
	if cid != "" {
		return cid, nil
	}
	log.Debugf("unable to get self container ID from cgroup controller inode: %v", err)

	return getSelfContainerID(c.hostCgroupNamespace, c.reader.CgroupVersion(), c.baseController)
}

// getSelfContainerIDFromInode returns the container ID of the current process by using the inode of the cgroup
// controller. The `reader` must use a `cgroups.ContainerFilter`.
func (c *systemCollector) getSelfContainerIDFromInode() (string, error) {
	if c.selfReader == nil {
		return "", fmt.Errorf("self reader is not initialized")
	}
	selfCgroup := c.selfReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if selfCgroup == nil {
		return "", fmt.Errorf("unable to get self cgroup")
	}

	return c.GetContainerIDForInode(selfCgroup.Inode(), 0)
}

func (c *systemCollector) getCgroup(containerID string, cacheValidity time.Duration) (cgroups.Cgroup, error) {
	cg := c.reader.GetCgroup(containerID)
	if cg == nil {
		err := c.reader.RefreshCgroups(cacheValidity)
		if err != nil {
			return nil, fmt.Errorf("containerdID not found and unable to refresh cgroups, err: %w", err)
		}

		cg = c.reader.GetCgroup(containerID)
		if cg == nil {
			return nil, fmt.Errorf("containerID not found")
		}
	}

	return cg, nil
}

func (c *systemCollector) getPIDs(containerID string, cacheValidity time.Duration) ([]int, error) {
	if c.pidMapper == nil {
		cg, err := c.getCgroup(containerID, cacheValidity)
		if err != nil {
			return nil, err
		}

		return cg.GetPIDs(cacheValidity)
	}

	return c.pidMapper.GetPIDs(containerID, cacheValidity), nil
}

func (c *systemCollector) buildContainerMetrics(cg cgroups.Cgroup, _ time.Duration) (*provider.ContainerStats, error) {
	stats := &cgroups.Stats{}
	allFailed, errs := cgroups.GetStats(cg, stats)
	if allFailed {
		return nil, fmt.Errorf("cgroup parsing failed, no data for containerID: %s, err: %w", cg.Identifier(), multierror.Append(nil, errs...))
	} else if len(errs) > 0 {
		log.Debugf("Incomplete data when getting cgroup stats for cgroup id: %s, errs: %v", cg.Identifier(), errs)
	}

	parentCPUStatRetriever := func(parentCPUStats *cgroups.CPUStats) error {
		parentCg, err := cg.GetParent()
		if err != nil {
			return err
		}
		if parentCg == nil {
			return errors.New("no parent cgroup")
		}

		return parentCg.GetCPUStats(parentCPUStats)
	}

	cs := &provider.ContainerStats{
		Timestamp: time.Now(),
		Memory:    buildMemoryStats(stats.Memory),
		CPU:       buildCPUStats(stats.CPU, parentCPUStatRetriever),
		IO:        buildIOStats(c.procPath, stats.IO),
		PID:       buildPIDStats(stats.PID),
	}

	return cs, nil
}

func buildMemoryStats(cgs *cgroups.MemoryStats) *provider.ContainerMemStats {
	if cgs == nil {
		return nil
	}
	cs := &provider.ContainerMemStats{}

	convertField(cgs.UsageTotal, &cs.UsageTotal)
	convertField(cgs.KernelMemory, &cs.KernelMemory)
	convertField(cgs.Limit, &cs.Limit)
	convertField(cgs.LowThreshold, &cs.Softlimit)
	convertField(cgs.RSS, &cs.RSS)
	convertField(cgs.Cache, &cs.Cache)
	convertField(cgs.Swap, &cs.Swap)
	convertField(cgs.SwapLimit, &cs.SwapLimit)
	convertField(cgs.OOMEvents, &cs.OOMEvents)
	convertField(cgs.Peak, &cs.Peak)
	convertField(cgs.Pgfault, &cs.Pgfault)
	convertField(cgs.Pgmajfault, &cs.Pgmajfault)
	convertFieldAndUnit(cgs.PSISome.Total, &cs.PartialStallTime, float64(time.Microsecond))

	// Compute complex fields
	if cgs.UsageTotal != nil && cgs.InactiveFile != nil {
		cs.WorkingSet = pointer.Ptr(float64(*cgs.UsageTotal - *cgs.InactiveFile))
	}

	return cs
}

func buildCPUStats(cgs *cgroups.CPUStats, parentCPUStatsRetriever func(parentCPUStats *cgroups.CPUStats) error) *provider.ContainerCPUStats {
	if cgs == nil {
		return nil
	}
	cs := &provider.ContainerCPUStats{}

	// Copy basid fields
	convertField(cgs.Total, &cs.Total)
	convertField(cgs.System, &cs.System)
	convertField(cgs.User, &cs.User)
	convertField(cgs.Shares, &cs.Shares)
	convertField(cgs.Weight, &cs.Weight)
	convertField(cgs.ElapsedPeriods, &cs.ElapsedPeriods)
	convertField(cgs.ThrottledPeriods, &cs.ThrottledPeriods)
	convertField(cgs.ThrottledTime, &cs.ThrottledTime)
	convertFieldAndUnit(cgs.PSISome.Total, &cs.PartialStallTime, float64(time.Microsecond))

	// Compute complex fields
	cs.Limit, cs.DefaultedLimit = computeCPULimitPct(cgs, parentCPUStatsRetriever)

	return cs
}

func computeCPULimitPct(cgs *cgroups.CPUStats, parentCPUStatsRetriever func(parentCPUStats *cgroups.CPUStats) error) (*float64, bool) {
	limitPct := computeCgroupCPULimitPct(cgs)
	defaulted := false

	// Check parent cgroup as it's used on ECS
	if limitPct == nil {
		parentCPUStats := &cgroups.CPUStats{}
		if err := parentCPUStatsRetriever(parentCPUStats); err == nil {
			limitPct = computeCgroupCPULimitPct(parentCPUStats)
		}
	}

	// If no limit is available, setting the limit to number of CPUs.
	// Always reporting a limit allows to compute CPU % accurately.
	if limitPct == nil {
		limitPct = pointer.Ptr(float64(systemutils.HostCPUCount() * 100))
		defaulted = true
	}

	return limitPct, defaulted
}

func computeCgroupCPULimitPct(cgs *cgroups.CPUStats) *float64 {
	// Limit is computed using min(CPUSet, CFS CPU Quota)
	var limitPct *float64

	if cgs.CPUCount != nil && *cgs.CPUCount != uint64(systemutils.HostCPUCount()) {
		limitPct = pointer.Ptr(float64(*cgs.CPUCount) * 100.0)
	}

	if cgs.SchedulerQuota != nil && cgs.SchedulerPeriod != nil {
		quotaLimitPct := 100 * (float64(*cgs.SchedulerQuota) / float64(*cgs.SchedulerPeriod))
		if limitPct == nil || quotaLimitPct < *limitPct {
			limitPct = &quotaLimitPct
		}
	}

	return limitPct
}

func buildPIDStats(cgs *cgroups.PIDStats) *provider.ContainerPIDStats {
	if cgs == nil {
		return nil
	}
	cs := &provider.ContainerPIDStats{}

	convertField(cgs.HierarchicalThreadCount, &cs.ThreadCount)
	convertField(cgs.HierarchicalThreadLimit, &cs.ThreadLimit)

	return cs
}
