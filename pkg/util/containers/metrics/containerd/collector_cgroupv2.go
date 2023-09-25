// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"fmt"
	"time"

	v2 "github.com/containerd/cgroups/v3/cgroup2/stats"

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
		Total:            pointer.Ptr(float64(cpuStat.UsageUsec * uint64(time.Microsecond))),
		System:           pointer.Ptr(float64(cpuStat.SystemUsec * uint64(time.Microsecond))),
		User:             pointer.Ptr(float64(cpuStat.UserUsec * uint64(time.Microsecond))),
		ThrottledTime:    pointer.Ptr(float64(cpuStat.ThrottledUsec * uint64(time.Microsecond))),
		ThrottledPeriods: pointer.Ptr(float64(cpuStat.NrThrottled)),
	}
}

func getMemoryStatsCgroupV2(memStat *v2.MemoryStat, memEvents *v2.MemoryEvents) *provider.ContainerMemStats {
	if memStat == nil {
		return nil
	}

	res := provider.ContainerMemStats{
		UsageTotal:   pointer.Ptr(float64(memStat.Usage)),
		WorkingSet:   pointer.Ptr(float64(memStat.Usage - memStat.InactiveFile)),
		RSS:          pointer.Ptr(float64(memStat.Anon)),
		Cache:        pointer.Ptr(float64(memStat.File)),
		KernelMemory: pointer.Ptr(float64(memStat.Slab + memStat.KernelStack)),
		Limit:        pointer.Ptr(float64(memStat.UsageLimit)), // TODO: Check value if no limit
		Swap:         pointer.Ptr(float64(memStat.SwapUsage)),
	}

	if memEvents != nil {
		res.OOMEvents = pointer.Ptr(float64(memEvents.Oom))
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
			ReadBytes:       pointer.Ptr(float64(ioEntry.Rbytes)),
			WriteBytes:      pointer.Ptr(float64(ioEntry.Wbytes)),
			ReadOperations:  pointer.Ptr(float64(ioEntry.Rios)),
			WriteOperations: pointer.Ptr(float64(ioEntry.Wios)),
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
