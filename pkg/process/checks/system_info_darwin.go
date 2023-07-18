// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package checks

import (
	"fmt"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/host"
	"github.com/DataDog/gopsutil/mem"
	"golang.org/x/sys/unix"
)

type statsProvider interface {
	getThreadCount() (int32, error)
}

type sysctlStatsProvider struct{}

func (_ *sysctlStatsProvider) getThreadCount() (int32, error) {
	threadCount, err := unix.SysctlUint32("machdep.cpu.thread_count")
	return int32(threadCount), err
}

var macosStatsProvider statsProvider = &sysctlStatsProvider{}

// patchCPUInfo returns the cpuInfo for the current host.
// On macOS, gopsutil returns incorrect results for the cpu count, so we need to patch it
// We do this in two steps, first we duplicate gopsutilCPUInfo[0] n times, where n is equal to the number of physical cores
// Then we retrieve the thread count and modify the copies using this new information.
func patchCPUInfo(gopsutilCPUInfo []cpu.InfoStat) ([]cpu.InfoStat, error) {
	// gopsutil only returns one cpu for macos, this is safe, and documented behavior
	cpuInfo := gopsutilCPUInfo[0]

	physicalCoreCount := int(cpuInfo.Cores)
	threadCount, err := macosStatsProvider.getThreadCount()
	if err != nil {
		return nil, fmt.Errorf("could not get thread count")
	}

	cpuStat := make([]cpu.InfoStat, 0, physicalCoreCount)
	for i := 0; i < physicalCoreCount; i++ {
		currentCpuInfo := cpuInfo
		currentCpuInfo.Cores = threadCount / int32(physicalCoreCount)
		cpuStat = append(cpuStat, currentCpuInfo)

	}
	return cpuStat, nil
}

// CollectSystemInfo collects a set of system-level information that will not
// change until a restart. This bit of information should be passed along with
// the process messages.
func CollectSystemInfo() (*model.SystemInfo, error) {
	hi, err := host.Info()
	if err != nil {
		return nil, err
	}

	cpuInfo, err := cpu.Info()
	if err != nil {
		return nil, err
	}
	cpuInfo, err = patchCPUInfo(cpuInfo)
	if err != nil {
		return nil, err
	}

	mi, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	cpus := make([]*model.CPUInfo, 0, len(cpuInfo))
	for _, c := range cpuInfo {
		cpus = append(cpus, &model.CPUInfo{
			Number:     c.CPU,
			Vendor:     c.VendorID,
			Family:     c.Family,
			Model:      c.Model,
			PhysicalId: c.PhysicalID,
			CoreId:     c.CoreID,
			Cores:      c.Cores,
			Mhz:        int64(c.Mhz),
			CacheSize:  c.CacheSize,
		})
	}

	return &model.SystemInfo{
		Uuid: hi.HostID,
		Os: &model.OSInfo{
			Name:          hi.OS,
			Platform:      hi.Platform,
			Family:        hi.PlatformFamily,
			Version:       hi.PlatformVersion,
			KernelVersion: hi.KernelVersion,
		},
		Cpus:        cpus,
		TotalMemory: int64(mi.Total),
	}, nil
}
