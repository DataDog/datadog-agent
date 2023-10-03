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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

const (
	systemCollectorID      = "system"
	cgroupV1BaseController = "memory"
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:       systemCollectorID,
		Priority: 0,
		Runtimes: provider.AllLinuxRuntimes,
		Factory: func() (provider.Collector, error) {
			return newSystemCollector()
		},
		DelegateCache: true,
	})
}

type systemCollector struct {
	reader              *cgroups.Reader
	procPath            string
	baseController      string
	hostCgroupNamespace bool
}

func newSystemCollector() (*systemCollector, error) {
	var err error
	var hostPrefix string

	if !config.IsHostProcAvailable() || !config.IsHostSysAvailable() {
		log.Debug("Container metrics system collector not available as host paths not mounted")
		return nil, provider.ErrPermaFail
	}

	procPath := config.Datadog.GetString("container_proc_root")
	if strings.HasPrefix(procPath, "/host") {
		hostPrefix = "/host"
	}

	reader, err := cgroups.NewReader(
		cgroups.WithCgroupV1BaseController(cgroupV1BaseController),
		cgroups.WithProcPath(procPath),
		cgroups.WithHostPrefix(hostPrefix),
		cgroups.WithReaderFilter(cgroups.ContainerFilter),
	)
	if err != nil {
		// Cgroup provider is pretty static. Except not having required mounts, it should always work.
		log.Errorf("Unable to initialize cgroup provider (cgroups not mounted?), err: %v", err)
		return nil, provider.ErrPermaFail
	}

	systemCollector := &systemCollector{
		reader:   reader,
		procPath: procPath,
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

	return systemCollector, nil
}

func (c *systemCollector) ID() string {
	return systemCollectorID
}

func (c *systemCollector) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	cg, err := c.getCgroup(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	return c.buildContainerMetrics(cg, cacheValidity)
}

func (c *systemCollector) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	cg, err := c.getCgroup(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	// Get PIDs
	pids, err := cg.GetPIDs(cacheValidity)
	if err != nil {
		return nil, fmt.Errorf("unable to get PIDs for cgroup id: %s. Unable to get count of open files", cg.Identifier())
	}

	ofCount, allFailed := systemutils.CountProcessesFileDescriptors(c.procPath, pids)
	if allFailed {
		return nil, fmt.Errorf("unable to read any PID open FDs for cgroup id: %s. Unable to get count of open files", cg.Identifier())
	}

	return &ofCount, nil
}

func (c *systemCollector) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	cg, err := c.getCgroup(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	pids, err := cg.GetPIDs(cacheValidity)
	if err != nil {
		return nil, err
	}

	return buildNetworkStats(c.procPath, pids)
}

func (c *systemCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	containerID, err := cgroups.IdentiferFromCgroupReferences(c.procPath, strconv.Itoa(pid), c.baseController, cgroups.ContainerFilter)
	return containerID, err
}

func (c *systemCollector) GetSelfContainerID() (string, error) {
	return getSelfContainerID(c.hostCgroupNamespace, c.reader.CgroupVersion(), c.baseController)
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

func (c *systemCollector) buildContainerMetrics(cg cgroups.Cgroup, cacheValidity time.Duration) (*provider.ContainerStats, error) {
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

	// Get PIDs
	var err error
	pids, err := cg.GetPIDs(cacheValidity)
	if err == nil {
		if cs.PID == nil {
			cs.PID = &provider.ContainerPIDStats{}
		}

		cs.PID.PIDs = pids
	} else {
		log.Debugf("Unable to get PIDs for cgroup id: %s. Metrics will be missing", cg.Identifier())
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
