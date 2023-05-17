// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker && linux

package docker

import (
	"fmt"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/system"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

func convertContainerStats(stats *types.Stats) *provider.ContainerStats {
	return &provider.ContainerStats{
		Timestamp: stats.Read,
		CPU:       convertCPUStats(&stats.CPUStats),
		Memory:    convertMemoryStats(&stats.MemoryStats),
		IO:        convertIOStats(&stats.BlkioStats),
		PID:       convertPIDStats(&stats.PidsStats),
	}
}

func convertCPUStats(cpuStats *types.CPUStats) *provider.ContainerCPUStats {
	return &provider.ContainerCPUStats{
		Total:            pointer.Ptr(float64(cpuStats.CPUUsage.TotalUsage)),
		System:           pointer.Ptr(float64(cpuStats.CPUUsage.UsageInKernelmode)),
		User:             pointer.Ptr(float64(cpuStats.CPUUsage.UsageInUsermode)),
		ThrottledPeriods: pointer.Ptr(float64(cpuStats.ThrottlingData.ThrottledPeriods)),
		ThrottledTime:    pointer.Ptr(float64(cpuStats.ThrottlingData.ThrottledTime)),
	}
}

func convertMemoryStats(memStats *types.MemoryStats) *provider.ContainerMemStats {
	containerMemStats := &provider.ContainerMemStats{
		UsageTotal: pointer.Ptr(float64(memStats.Usage)),
		Limit:      pointer.Ptr(float64(memStats.Limit)),
		OOMEvents:  pointer.Ptr(float64(memStats.Failcnt)),
	}

	if rss, found := memStats.Stats["rss"]; found {
		containerMemStats.RSS = pointer.Ptr(float64(rss))
	}

	if cache, found := memStats.Stats["cache"]; found {
		containerMemStats.Cache = pointer.Ptr(float64(cache))
	}

	// `kernel_stack` and `slab`, which are used to compute `KernelMemory` are available only with cgroup v2
	if kernelStack, found := memStats.Stats["kernel_stack"]; found {
		if slab, found := memStats.Stats["slab"]; found {
			containerMemStats.KernelMemory = pointer.Ptr(float64(kernelStack + slab))
		}
	}

	return containerMemStats
}

func convertIOStats(ioStats *types.BlkioStats) *provider.ContainerIOStats {
	containerIOStats := provider.ContainerIOStats{
		ReadBytes:       pointer.Ptr(0.0),
		WriteBytes:      pointer.Ptr(0.0),
		ReadOperations:  pointer.Ptr(0.0),
		WriteOperations: pointer.Ptr(0.0),
		Devices:         make(map[string]provider.DeviceIOStats),
	}

	procPath := config.Datadog.GetString("container_proc_root")
	deviceMapping, err := system.GetDiskDeviceMapping(procPath)
	if err != nil {
		log.Debugf("Error while getting disk mapping, no disk metric will be present, err: %v", err)
	}

	for _, blkioStatEntry := range ioStats.IoServiceBytesRecursive {
		deviceName, found := deviceMapping[fmt.Sprintf("%d:%d", blkioStatEntry.Major, blkioStatEntry.Minor)]

		var device provider.DeviceIOStats
		if found {
			device = containerIOStats.Devices[deviceName]
		}

		switch blkioStatEntry.Op {
		case "Read":
			device.ReadBytes = pointer.Ptr(float64(blkioStatEntry.Value))
			*containerIOStats.ReadBytes += *device.ReadBytes
		case "Write":
			device.WriteBytes = pointer.Ptr(float64(blkioStatEntry.Value))
			*containerIOStats.WriteBytes += *device.WriteBytes
		}

		if found {
			containerIOStats.Devices[deviceName] = device
		}
	}

	for _, blkioStatEntry := range ioStats.IoServicedRecursive {
		deviceName, found := deviceMapping[fmt.Sprintf("%d:%d", blkioStatEntry.Major, blkioStatEntry.Minor)]

		var device provider.DeviceIOStats
		if found {
			device = containerIOStats.Devices[deviceName]
		}

		switch blkioStatEntry.Op {
		case "Read":
			device.ReadOperations = pointer.Ptr(float64(blkioStatEntry.Value))
			*containerIOStats.ReadOperations += *device.ReadOperations
		case "Write":
			device.WriteOperations = pointer.Ptr(float64(blkioStatEntry.Value))
			*containerIOStats.WriteOperations += *device.WriteOperations
		}

		if found {
			containerIOStats.Devices[deviceName] = device
		}
	}

	return &containerIOStats
}

func convertPIDStats(pidStats *types.PidsStats) *provider.ContainerPIDStats {
	return &provider.ContainerPIDStats{
		ThreadCount: pointer.Ptr(float64(pidStats.Current)),
		ThreadLimit: pointer.Ptr(float64(pidStats.Limit)),
	}
}

func computeCPULimit(containerStats *provider.ContainerStats, spec *types.ContainerJSON) {
	if spec == nil || spec.HostConfig == nil || containerStats.CPU == nil {
		return
	}

	var cpuLimit float64
	switch {
	case spec.HostConfig.NanoCPUs > 0:
		cpuLimit = float64(spec.HostConfig.NanoCPUs) / 1e9 * 100
	case spec.HostConfig.CpusetCpus != "":
		cpuLimit = 100 * float64(cgroups.ParseCPUSetFormat(spec.HostConfig.CpusetCpus))
	case spec.HostConfig.CPUQuota > 0:
		period := spec.HostConfig.CPUPeriod
		if period == 0 {
			period = 100000 // Default CFS Period
		}
		cpuLimit = 100 * float64(spec.HostConfig.CPUQuota) / float64(period)
	default:
		// If no limit is available, setting the limit to number of CPUs.
		// Always reporting a limit allows to compute CPU % accurately.
		cpuLimit = 100 * float64(systemutils.HostCPUCount())
	}

	containerStats.CPU.Limit = &cpuLimit
}
