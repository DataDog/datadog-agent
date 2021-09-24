// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package metrics

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

const (
	systemCollectorID = "system"
)

func init() {
	metricsProvider.registerCollector(collectorMetadata{
		id:       systemCollectorID,
		priority: 0,
		runtimes: allLinuxRuntimes,
		factory: func() (Collector, error) {
			return newCgroupCollector()
		},
	})
}

type cgroupCollector struct {
	reader   *cgroups.Reader
	procPath string
}

func newCgroupCollector() (*cgroupCollector, error) {
	var err error
	var hostPrefix string

	procPath := config.Datadog.GetString("container_proc_root")
	if strings.HasPrefix(procPath, "/host") {
		hostPrefix = "/host"
	}

	reader, err := cgroups.NewReader(
		cgroups.WithCgroupV1BaseController("freezer"),
		cgroups.WithProcPath(procPath),
		cgroups.WithHostPrefix(hostPrefix),
		cgroups.WithReaderFilter(cgroups.ContainerFilter),
	)
	if err != nil {
		// Cgroup provider is pretty static. Except not having required mounts, it should always work.
		log.Errorf("Unable to initialize cgroup provider (cgroups not mounted?), err: %v", err)
		return nil, ErrPermaFail
	}

	return &cgroupCollector{
		reader:   reader,
		procPath: procPath,
	}, nil
}

func (c *cgroupCollector) ID() string {
	return systemCollectorID
}

func (c *cgroupCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	cg, err := c.getCgroup(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	var stats cgroups.Stats
	err = cg.GetStats(&stats)
	if err != nil {
		return nil, fmt.Errorf("cgroup parsing failed, incomplete data for containerID: %s, err: %w", containerID, err)
	}

	return c.buildContainerMetrics(stats), nil
}

func (c *cgroupCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration, networks map[string]string) (*ContainerNetworkStats, error) {
	cg, err := c.getCgroup(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	pidStats := &cgroups.PIDStats{}
	err = cg.GetPIDStats(pidStats)
	if err != nil {
		return nil, err
	}

	return buildNetworkStats(c.procPath, networks, pidStats)
}

func (c *cgroupCollector) getCgroup(containerID string, cacheValidity time.Duration) (cgroups.Cgroup, error) {
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

func (c *cgroupCollector) buildContainerMetrics(cgs cgroups.Stats) *ContainerStats {
	cs := &ContainerStats{
		Timestamp: time.Now(),
		Memory:    buildMemoryStats(cgs.Memory),
		CPU:       buildCPUStats(cgs.CPU),
		IO:        buildIOStats(c.procPath, cgs.IO),
		PID:       buildPIDStats(cgs.PID),
	}

	return cs
}

func buildMemoryStats(cgs *cgroups.MemoryStats) *ContainerMemStats {
	if cgs == nil {
		return nil
	}
	cs := &ContainerMemStats{}

	convertField(cgs.UsageTotal, &cs.UsageTotal)
	convertField(cgs.KernelMemory, &cs.KernelMemory)
	convertField(cgs.Limit, &cs.Limit)
	convertField(cgs.LowThreshold, &cs.Softlimit)
	convertField(cgs.RSS, &cs.RSS)
	convertField(cgs.Cache, &cs.Cache)
	convertField(cgs.Swap, &cs.Swap)
	convertField(cgs.OOMEvents, &cs.OOMEvents)

	return cs
}

func buildCPUStats(cgs *cgroups.CPUStats) *ContainerCPUStats {
	if cgs == nil {
		return nil
	}
	cs := &ContainerCPUStats{}

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
		limitPct = float64(system.HostCPUCount()) * 100
	}
	return &limitPct
}

func buildPIDStats(cgs *cgroups.PIDStats) *ContainerPIDStats {
	if cgs == nil {
		return nil
	}
	cs := &ContainerPIDStats{}

	cs.PIDs = cgs.PIDs
	convertField(cgs.HierarchicalThreadCount, &cs.ThreadCount)
	convertField(cgs.HierarchicalThreadLimit, &cs.ThreadLimit)

	return cs
}

func convertField(s *uint64, t **float64) {
	if s != nil {
		*t = util.Float64Ptr(float64(*s))
	}
}
