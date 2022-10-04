// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package system

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	reader         *cgroups.Reader
	procPath       string
	baseController string
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
	containerID, err := cgroups.IdentiferFromCgroupReferences("/proc", "self", c.baseController, cgroups.ContainerFilter)
	return containerID, err
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
	var stats cgroups.Stats
	if err := cg.GetStats(&stats); err != nil {
		return nil, fmt.Errorf("cgroup parsing failed, incomplete data for containerID: %s, err: %w", cg.Identifier(), err)
	}

	cs := &provider.ContainerStats{
		Timestamp: time.Now(),
		Memory:    buildMemoryStats(stats.Memory),
		CPU:       buildCPUStats(stats.CPU),
		IO:        buildIOStats(c.procPath, stats.IO),
		PID:       buildPIDStats(stats.PID),
	}

	if cs.PID == nil {
		cs.PID = &provider.ContainerPIDStats{}
	}

	// Get PIDs
	var err error
	cs.PID.PIDs, err = cg.GetPIDs(cacheValidity)
	if err != nil {
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

	return cs
}

func buildCPUStats(cgs *cgroups.CPUStats) *provider.ContainerCPUStats {
	if cgs == nil {
		return nil
	}
	cs := &provider.ContainerCPUStats{}

	// Copy basid fields
	convertField(cgs.Total, &cs.Total)
	convertField(cgs.System, &cs.System)
	convertField(cgs.User, &cs.User)
	convertField(cgs.Shares, &cs.Shares)
	convertField(cgs.ElapsedPeriods, &cs.ElapsedPeriods)
	convertField(cgs.ThrottledPeriods, &cs.ThrottledPeriods)
	convertField(cgs.ThrottledTime, &cs.ThrottledTime)

	// Compute complex fields
	cs.Limit = computeCPULimitPct(cgs)

	return cs
}

func computeCPULimitPct(cgs *cgroups.CPUStats) *float64 {
	// Limit is computed using min(CPUSet, CFS CPU Quota)
	var limitPct float64
	if cgs.CPUCount != nil {
		limitPct = float64(*cgs.CPUCount) * 100
	}
	if cgs.SchedulerQuota != nil && cgs.SchedulerPeriod != nil {
		quotaLimitPct := 100 * (float64(*cgs.SchedulerQuota) / float64(*cgs.SchedulerPeriod))
		if quotaLimitPct < limitPct {
			limitPct = quotaLimitPct
		}
	}
	// If no limit is available, setting the limit to number of CPUs.
	// Always reporting a limit allows to compute CPU % accurately.
	if limitPct == 0 {
		limitPct = float64(systemutils.HostCPUCount()) * 100
	}
	return &limitPct
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
