// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker && linux
// +build docker,linux

package docker

import (
	"fmt"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/system"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func convertContainerStats(stats *types.Stats) *provider.ContainerStats {
	return &provider.ContainerStats{
		Timestamp: time.Now(),
		CPU:       convertCPUStats(&stats.CPUStats),
		Memory:    convertMemoryStats(&stats.MemoryStats),
		IO:        convertIOStats(&stats.BlkioStats),
		PID:       convertPIDStats(&stats.PidsStats),
	}
}

func convertCPUStats(cpuStats *types.CPUStats) *provider.ContainerCPUStats {
	return &provider.ContainerCPUStats{
		Total:            pointer.UIntToFloatPtr(cpuStats.CPUUsage.TotalUsage),
		System:           pointer.UIntToFloatPtr(cpuStats.CPUUsage.UsageInKernelmode),
		User:             pointer.UIntToFloatPtr(cpuStats.CPUUsage.UsageInUsermode),
		ThrottledPeriods: pointer.UIntToFloatPtr(cpuStats.ThrottlingData.ThrottledPeriods),
		ThrottledTime:    pointer.UIntToFloatPtr(cpuStats.ThrottlingData.ThrottledTime),
	}
}

func convertMemoryStats(memStats *types.MemoryStats) *provider.ContainerMemStats {
	containerMemStats := &provider.ContainerMemStats{
		UsageTotal: pointer.UIntToFloatPtr(memStats.Usage),
		Limit:      pointer.UIntToFloatPtr(memStats.Limit),
		OOMEvents:  pointer.UIntToFloatPtr(memStats.Failcnt),
	}

	if rss, found := memStats.Stats["rss"]; found {
		containerMemStats.RSS = pointer.UIntToFloatPtr(rss)
	}

	if cache, found := memStats.Stats["cache"]; found {
		containerMemStats.Cache = pointer.UIntToFloatPtr(cache)
	}

	// `kernel_stack` and `slab`, which are used to compute `KernelMemory` are available only with cgroup v2
	if kernelStack, found := memStats.Stats["kernel_stack"]; found {
		if slab, found := memStats.Stats["slab"]; found {
			containerMemStats.KernelMemory = pointer.UIntToFloatPtr(kernelStack + slab)
		}
	}

	return containerMemStats
}

func convertIOStats(ioStats *types.BlkioStats) *provider.ContainerIOStats {
	containerIOStats := provider.ContainerIOStats{
		ReadBytes:       pointer.Float64Ptr(0),
		WriteBytes:      pointer.Float64Ptr(0),
		ReadOperations:  pointer.Float64Ptr(0),
		WriteOperations: pointer.Float64Ptr(0),
		Devices:         make(map[string]provider.DeviceIOStats),
	}

	procPath := config.Datadog.GetString("container_proc_root")
	deviceMapping, err := system.GetDiskDeviceMapping(procPath)
	if err != nil {
		log.Debugf("Error while getting disk mapping, no disk metric will be present, err: %w", err)
	}

	for _, blkioStatEntry := range ioStats.IoServiceBytesRecursive {
		deviceName, found := deviceMapping[fmt.Sprintf("%d:%d", blkioStatEntry.Major, blkioStatEntry.Minor)]

		var device provider.DeviceIOStats
		if found {
			device = containerIOStats.Devices[deviceName]
		}

		switch blkioStatEntry.Op {
		case "Read":
			device.ReadBytes = pointer.UIntToFloatPtr(blkioStatEntry.Value)
			*containerIOStats.ReadBytes += *device.ReadBytes
		case "Write":
			device.WriteBytes = pointer.UIntToFloatPtr(blkioStatEntry.Value)
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
			device.ReadOperations = pointer.UIntToFloatPtr(blkioStatEntry.Value)
			*containerIOStats.ReadOperations += *device.ReadOperations
		case "Write":
			device.WriteOperations = pointer.UIntToFloatPtr(blkioStatEntry.Value)
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
		ThreadCount: pointer.UIntToFloatPtr(pidStats.Current),
		ThreadLimit: pointer.UIntToFloatPtr(pidStats.Limit),
	}
}
