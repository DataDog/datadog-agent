// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux
// +build containerd,linux

package containerd

import (
	"fmt"
	"time"

	v1 "github.com/containerd/cgroups/stats/v1"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func getContainerdStatsV1(metrics *v1.Metrics) *provider.ContainerStats {
	if metrics == nil {
		return nil
	}

	currentTime := time.Now()

	return &provider.ContainerStats{
		Timestamp: currentTime,
		CPU:       getCPUStatsCgroupV1(metrics.CPU),
		Memory:    getMemoryStatsCgroupV1(metrics.Memory),
		IO:        getIOStatsCgroupV1(metrics.Blkio),
	}
}

func getCPUStatsCgroupV1(cpuStat *v1.CPUStat) *provider.ContainerCPUStats {
	if cpuStat == nil {
		return nil
	}

	res := provider.ContainerCPUStats{}

	if cpuStat.Usage != nil {
		res.Total = pointer.UIntToFloatPtr(cpuStat.Usage.Total)
		res.System = pointer.UIntToFloatPtr(cpuStat.Usage.Kernel)
		res.User = pointer.UIntToFloatPtr(cpuStat.Usage.User)
	}

	if cpuStat.Throttling != nil {
		res.ThrottledPeriods = pointer.UIntToFloatPtr(cpuStat.Throttling.ThrottledPeriods)
		res.ThrottledTime = pointer.UIntToFloatPtr(cpuStat.Throttling.ThrottledTime)
	}

	return &res
}

func getMemoryStatsCgroupV1(memStat *v1.MemoryStat) *provider.ContainerMemStats {
	if memStat == nil {
		return nil
	}

	res := provider.ContainerMemStats{
		RSS:   pointer.UIntToFloatPtr(memStat.RSS),
		Cache: pointer.UIntToFloatPtr(memStat.Cache),
	}

	if memStat.Usage != nil {
		res.UsageTotal = pointer.UIntToFloatPtr(memStat.Usage.Usage)
		res.Limit = pointer.UIntToFloatPtr(memStat.Usage.Limit)
	}

	if memStat.Kernel != nil {
		res.KernelMemory = pointer.UIntToFloatPtr(memStat.Kernel.Usage)
	}

	if memStat.Swap != nil {
		res.Swap = pointer.UIntToFloatPtr(memStat.Swap.Usage)
	}

	return &res
}

func getIOStatsCgroupV1(blkioStat *v1.BlkIOStat) *provider.ContainerIOStats {
	if blkioStat == nil {
		return nil
	}

	result := provider.ContainerIOStats{
		ReadBytes:       pointer.Float64Ptr(0),
		WriteBytes:      pointer.Float64Ptr(0),
		ReadOperations:  pointer.Float64Ptr(0),
		WriteOperations: pointer.Float64Ptr(0),
		Devices:         make(map[string]provider.DeviceIOStats),
	}

	for _, blkioStatEntry := range blkioStat.IoServiceBytesRecursive {
		deviceName := fmt.Sprintf("%d:%d", blkioStatEntry.Major, blkioStatEntry.Minor)
		device := result.Devices[deviceName]
		switch blkioStatEntry.Op {
		case "Read":
			device.ReadBytes = pointer.Float64Ptr(float64(blkioStatEntry.Value))
		case "Write":
			device.WriteBytes = pointer.Float64Ptr(float64(blkioStatEntry.Value))
		}
		result.Devices[deviceName] = device
	}

	for _, blkioStatEntry := range blkioStat.IoServicedRecursive {
		deviceName := fmt.Sprintf("%d:%d", blkioStatEntry.Major, blkioStatEntry.Minor)
		device := result.Devices[deviceName]
		switch blkioStatEntry.Op {
		case "Read":
			device.ReadOperations = pointer.Float64Ptr(float64(blkioStatEntry.Value))
		case "Write":
			device.WriteOperations = pointer.Float64Ptr(float64(blkioStatEntry.Value))
		}
		result.Devices[deviceName] = device
	}

	for _, device := range result.Devices {
		if device.ReadBytes != nil {
			*result.ReadBytes += *device.ReadBytes
		}
		if device.WriteBytes != nil {
			*result.WriteBytes += *device.WriteBytes
		}
		if device.ReadOperations != nil {
			*result.ReadOperations += *device.ReadOperations
		}
		if device.WriteOperations != nil {
			*result.WriteOperations += *device.WriteOperations
		}
	}

	return &result
}

func getNetworkStatsCgroupV1(networkStats []*v1.NetworkStat) *provider.ContainerNetworkStats {
	containerNetworkStats := provider.ContainerNetworkStats{
		Timestamp:   time.Now(),
		BytesSent:   pointer.Float64Ptr(0),
		BytesRcvd:   pointer.Float64Ptr(0),
		PacketsSent: pointer.Float64Ptr(0),
		PacketsRcvd: pointer.Float64Ptr(0),
		Interfaces:  make(map[string]provider.InterfaceNetStats),
	}

	for _, stats := range networkStats {
		*containerNetworkStats.BytesSent += float64(stats.TxBytes)
		*containerNetworkStats.BytesRcvd += float64(stats.RxBytes)
		*containerNetworkStats.PacketsSent += float64(stats.TxPackets)
		*containerNetworkStats.PacketsRcvd += float64(stats.RxPackets)

		containerNetworkStats.Interfaces[stats.Name] = provider.InterfaceNetStats{
			BytesSent:   pointer.UIntToFloatPtr(stats.TxBytes),
			BytesRcvd:   pointer.UIntToFloatPtr(stats.RxBytes),
			PacketsSent: pointer.UIntToFloatPtr(stats.TxPackets),
			PacketsRcvd: pointer.UIntToFloatPtr(stats.RxPackets),
		}
	}

	return &containerNetworkStats
}
