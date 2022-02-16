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

	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
)

func getContainerdStatsV2(metrics *v2.Metrics, container containers.Container, OCISpec *oci.Spec, processes []containerd.ProcessInfo) *provider.ContainerStats {
	if metrics == nil {
		return nil
	}

	currentTime := time.Now()

	return &provider.ContainerStats{
		Timestamp: currentTime,
		CPU:       getCPUStatsCgroupV2(metrics.CPU, currentTime, container.CreatedAt, OCISpec),
		Memory:    getMemoryStatsCgroupV2(metrics.Memory, metrics.MemoryEvents),
		IO:        getIOStatsCgroupV2(metrics.Io),
	}
}

func getCPUStatsCgroupV2(cpuStat *v2.CPUStat, currentTime time.Time, startTime time.Time, OCISpec *oci.Spec) *provider.ContainerCPUStats {
	if cpuStat == nil {
		return nil
	}

	// Unfortunately the underlying struct does not provide a way to know if metrics are set or not
	return &provider.ContainerCPUStats{
		Total:            util.UIntToFloatPtr(cpuStat.UsageUsec * uint64(time.Microsecond)),
		System:           util.UIntToFloatPtr(cpuStat.SystemUsec * uint64(time.Microsecond)),
		User:             util.UIntToFloatPtr(cpuStat.UserUsec * uint64(time.Microsecond)),
		ThrottledTime:    util.UIntToFloatPtr(cpuStat.ThrottledUsec * uint64(time.Microsecond)),
		ThrottledPeriods: util.UIntToFloatPtr(cpuStat.NrThrottled),
		Limit:            getContainerdCPULimit(currentTime, startTime, OCISpec),
	}
}

func getMemoryStatsCgroupV2(memStat *v2.MemoryStat, memEvents *v2.MemoryEvents) *provider.ContainerMemStats {
	if memStat == nil {
		return nil
	}

	res := provider.ContainerMemStats{
		UsageTotal:   util.UIntToFloatPtr(memStat.Usage),
		RSS:          util.UIntToFloatPtr(memStat.Anon),
		Cache:        util.UIntToFloatPtr(memStat.File),
		KernelMemory: util.UIntToFloatPtr(memStat.Slab + memStat.KernelStack),
		Limit:        util.UIntToFloatPtr(memStat.UsageLimit), // TODO: Check value if no limit
		Swap:         util.UIntToFloatPtr(memStat.SwapUsage),
	}

	if memEvents != nil {
		res.OOMEvents = util.UIntToFloatPtr(memEvents.Oom)
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
			ReadBytes:       util.UIntToFloatPtr(ioEntry.Rbytes),
			WriteBytes:      util.UIntToFloatPtr(ioEntry.Wbytes),
			ReadOperations:  util.UIntToFloatPtr(ioEntry.Rios),
			WriteOperations: util.UIntToFloatPtr(ioEntry.Wios),
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
