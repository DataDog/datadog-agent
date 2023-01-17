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

	v2 "github.com/containerd/cgroups/v2/stats"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func getContainerdStatsV2(metrics *v2.Metrics) *provider.ContainerStats {
	if metrics == nil {
		return nil
	}

	currentTime := time.Now()

	return &provider.ContainerStats{
		Timestamp: currentTime,
		CPU:       getCPUStatsCgroupV2(metrics.CPU),
		Memory:    getMemoryStatsCgroupV2(metrics.Memory, metrics.MemoryEvents),
		IO:        getIOStatsCgroupV2(metrics.Io),
	}
}

func getCPUStatsCgroupV2(cpuStat *v2.CPUStat) *provider.ContainerCPUStats {
	if cpuStat == nil {
		return nil
	}

	// Unfortunately the underlying struct does not provide a way to know if metrics are set or not
	return &provider.ContainerCPUStats{
		Total:            pointer.UIntToFloatPtr(cpuStat.UsageUsec * uint64(time.Microsecond)),
		System:           pointer.UIntToFloatPtr(cpuStat.SystemUsec * uint64(time.Microsecond)),
		User:             pointer.UIntToFloatPtr(cpuStat.UserUsec * uint64(time.Microsecond)),
		ThrottledTime:    pointer.UIntToFloatPtr(cpuStat.ThrottledUsec * uint64(time.Microsecond)),
		ThrottledPeriods: pointer.UIntToFloatPtr(cpuStat.NrThrottled),
	}
}

func getMemoryStatsCgroupV2(memStat *v2.MemoryStat, memEvents *v2.MemoryEvents) *provider.ContainerMemStats {
	if memStat == nil {
		return nil
	}

	res := provider.ContainerMemStats{
		UsageTotal:   pointer.UIntToFloatPtr(memStat.Usage),
		RSS:          pointer.UIntToFloatPtr(memStat.Anon),
		Cache:        pointer.UIntToFloatPtr(memStat.File),
		KernelMemory: pointer.UIntToFloatPtr(memStat.Slab + memStat.KernelStack),
		Limit:        pointer.UIntToFloatPtr(memStat.UsageLimit), // TODO: Check value if no limit
		Swap:         pointer.UIntToFloatPtr(memStat.SwapUsage),
	}

	if memEvents != nil {
		res.OOMEvents = pointer.UIntToFloatPtr(memEvents.Oom)
	}

	return &res
}

func getIOStatsCgroupV2(ioStat *v2.IOStat) *provider.ContainerIOStats {
	if ioStat == nil || len(ioStat.Usage) == 0 {
		return nil
	}

	result := provider.ContainerIOStats{
		Devices: make(map[string]provider.DeviceIOStats, len(ioStat.Usage)),
	}

	var sumRBytes, sumROps, sumWBytes, sumWOps float64
	for _, ioEntry := range ioStat.Usage {
		sumRBytes += float64(ioEntry.Rbytes)
		sumROps += float64(ioEntry.Rios)
		sumWBytes += float64(ioEntry.Wbytes)
		sumWOps += float64(ioEntry.Wios)

		outIOStats := provider.DeviceIOStats{
			ReadBytes:       pointer.UIntToFloatPtr(ioEntry.Rbytes),
			WriteBytes:      pointer.UIntToFloatPtr(ioEntry.Wbytes),
			ReadOperations:  pointer.UIntToFloatPtr(ioEntry.Rios),
			WriteOperations: pointer.UIntToFloatPtr(ioEntry.Wios),
		}
		deviceName := fmt.Sprintf("%d:%d", ioEntry.Major, ioEntry.Minor)
		result.Devices[deviceName] = outIOStats
	}

	result.ReadBytes = &sumRBytes
	result.ReadOperations = &sumROps
	result.WriteBytes = &sumWBytes
	result.WriteOperations = &sumWOps

	return &result
}
