// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"fmt"
	"time"

	v1 "github.com/containerd/cgroups/stats/v1"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
)

func getContainerdStatsV1(metrics *v1.Metrics, container containers.Container, OCISpec *oci.Spec, processes []containerd.ProcessInfo) *provider.ContainerStats {
	if metrics == nil {
		return nil
	}

	currentTime := time.Now()

	return &provider.ContainerStats{
		Timestamp: currentTime,
		CPU:       getCPUStatsCgroupV1(metrics.CPU, currentTime, container.CreatedAt, OCISpec),
		Memory:    getMemoryStatsCgroupV1(metrics.Memory),
		IO:        getIOStatsCgroupV1(metrics.Blkio, processes),
	}
}

func getCPUStatsCgroupV1(cpuStat *v1.CPUStat, currentTime time.Time, startTime time.Time, OCISpec *oci.Spec) *provider.ContainerCPUStats {
	if cpuStat == nil {
		return nil
	}

	res := provider.ContainerCPUStats{}

	if cpuStat.Usage != nil {
		res.Total = util.UIntToFloatPtr(cpuStat.Usage.Total)
		res.System = util.UIntToFloatPtr(cpuStat.Usage.Kernel)
		res.User = util.UIntToFloatPtr(cpuStat.Usage.User)
	}

	if cpuStat.Throttling != nil {
		res.ThrottledPeriods = util.UIntToFloatPtr(cpuStat.Throttling.ThrottledPeriods)
		res.ThrottledTime = util.UIntToFloatPtr(cpuStat.Throttling.ThrottledTime)
	}

	res.Limit = getContainerdCPULimit(currentTime, startTime, OCISpec)

	return &res
}

func getMemoryStatsCgroupV1(memStat *v1.MemoryStat) *provider.ContainerMemStats {
	if memStat == nil {
		return nil
	}

	res := provider.ContainerMemStats{
		RSS:   util.UIntToFloatPtr(memStat.RSS),
		Cache: util.UIntToFloatPtr(memStat.Cache),
	}

	if memStat.Usage != nil {
		res.UsageTotal = util.UIntToFloatPtr(memStat.Usage.Usage)
		res.Limit = util.UIntToFloatPtr(memStat.Usage.Limit)
	}

	if memStat.Kernel != nil {
		res.KernelMemory = util.UIntToFloatPtr(memStat.Kernel.Usage)
	}

	if memStat.Swap != nil {
		res.Swap = util.UIntToFloatPtr(memStat.Swap.Usage)
	}

	return &res
}

func getIOStatsCgroupV1(blkioStat *v1.BlkIOStat, processes []containerd.ProcessInfo) *provider.ContainerIOStats {
	if blkioStat == nil {
		return nil
	}

	result := provider.ContainerIOStats{
		ReadBytes:       util.Float64Ptr(0),
		WriteBytes:      util.Float64Ptr(0),
		ReadOperations:  util.Float64Ptr(0),
		WriteOperations: util.Float64Ptr(0),
		Devices:         make(map[string]provider.DeviceIOStats),
	}

	for _, blkioStatEntry := range blkioStat.IoServiceBytesRecursive {
		deviceName := fmt.Sprintf("%d:%d", blkioStatEntry.Major, blkioStatEntry.Minor)
		device := result.Devices[deviceName]
		switch blkioStatEntry.Op {
		case "Read":
			device.ReadBytes = util.Float64Ptr(float64(blkioStatEntry.Value))
		case "Write":
			device.WriteBytes = util.Float64Ptr(float64(blkioStatEntry.Value))
		}
		result.Devices[deviceName] = device
	}

	for _, blkioStatEntry := range blkioStat.IoServicedRecursive {
		deviceName := fmt.Sprintf("%d:%d", blkioStatEntry.Major, blkioStatEntry.Minor)
		device := result.Devices[deviceName]
		switch blkioStatEntry.Op {
		case "Read":
			device.ReadOperations = util.Float64Ptr(float64(blkioStatEntry.Value))
		case "Write":
			device.WriteOperations = util.Float64Ptr(float64(blkioStatEntry.Value))
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
		BytesSent:   util.Float64Ptr(0),
		BytesRcvd:   util.Float64Ptr(0),
		PacketsSent: util.Float64Ptr(0),
		PacketsRcvd: util.Float64Ptr(0),
		Interfaces:  make(map[string]provider.InterfaceNetStats),
	}

	for _, stats := range networkStats {
		*containerNetworkStats.BytesSent += float64(stats.TxBytes)
		*containerNetworkStats.BytesRcvd += float64(stats.RxBytes)
		*containerNetworkStats.PacketsSent += float64(stats.TxPackets)
		*containerNetworkStats.PacketsRcvd += float64(stats.RxPackets)

		containerNetworkStats.Interfaces[stats.Name] = provider.InterfaceNetStats{
			BytesSent:   util.UIntToFloatPtr(stats.TxBytes),
			BytesRcvd:   util.UIntToFloatPtr(stats.RxBytes),
			PacketsSent: util.UIntToFloatPtr(stats.TxPackets),
			PacketsRcvd: util.UIntToFloatPtr(stats.RxPackets),
		}
	}

	return &containerNetworkStats
}
