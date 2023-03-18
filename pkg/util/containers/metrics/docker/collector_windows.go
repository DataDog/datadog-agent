// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker && windows

package docker

import (
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

func convertContainerStats(stats *types.Stats) *provider.ContainerStats {
	return &provider.ContainerStats{
		Timestamp: stats.Read,
		CPU:       convertCPUStats(&stats.CPUStats),
		Memory:    convertMemoryStats(&stats.MemoryStats),
		IO:        convertIOStats(&stats.StorageStats),
		PID:       convertPIDStats(stats.NumProcs),
	}
}

func convertCPUStats(cpuStats *types.CPUStats) *provider.ContainerCPUStats {
	return &provider.ContainerCPUStats{
		// ContainerCPUStats expects CPU metrics in nanoseconds
		// *On Windows* (only) CPUStats units are 100’s of nanoseconds
		Total:  pointer.Ptr(100 * float64(cpuStats.CPUUsage.TotalUsage)),
		System: pointer.Ptr(100 * float64(cpuStats.CPUUsage.UsageInKernelmode)),
		User:   pointer.Ptr(100 * float64(cpuStats.CPUUsage.UsageInUsermode)),
	}
}

func convertMemoryStats(memStats *types.MemoryStats) *provider.ContainerMemStats {
	return &provider.ContainerMemStats{
		UsageTotal:        pointer.Ptr(float64(memStats.Commit)),
		PrivateWorkingSet: pointer.Ptr(float64(memStats.PrivateWorkingSet)),
		CommitBytes:       pointer.Ptr(float64(memStats.Commit)),
		CommitPeakBytes:   pointer.Ptr(float64(memStats.CommitPeak)),
	}
}

func convertIOStats(storageStats *types.StorageStats) *provider.ContainerIOStats {
	return &provider.ContainerIOStats{
		ReadBytes:       pointer.Ptr(float64(storageStats.ReadSizeBytes)),
		WriteBytes:      pointer.Ptr(float64(storageStats.WriteSizeBytes)),
		ReadOperations:  pointer.Ptr(float64(storageStats.ReadCountNormalized)),
		WriteOperations: pointer.Ptr(float64(storageStats.WriteCountNormalized)),
	}
}

func convertPIDStats(numProcs uint32) *provider.ContainerPIDStats {
	return &provider.ContainerPIDStats{
		ThreadCount: pointer.Ptr(float64(numProcs)),
	}
}

func computeCPULimit(containerStats *provider.ContainerStats, spec *types.ContainerJSON) {
	if spec == nil || spec.HostConfig == nil || containerStats.CPU == nil {
		return
	}

	// Compute CPU Limit from spec
	var cpuLimit float64
	switch {
	case spec.HostConfig.NanoCPUs > 0:
		cpuLimit = float64(spec.HostConfig.NanoCPUs) / 1e9 * 100
	case spec.HostConfig.CPUPercent > 0:
		cpuLimit = float64(spec.HostConfig.CPUPercent) * float64(system.HostCPUCount())
	case spec.HostConfig.CPUCount > 0:
		cpuLimit = float64(spec.HostConfig.CPUCount) * 100
	default:
		// If no limit is available, setting the limit to number of CPUs.
		// Always reporting a limit allows to compute CPU % accurately.
		cpuLimit = 100 * float64(system.HostCPUCount())
	}

	containerStats.CPU.Limit = &cpuLimit
}
