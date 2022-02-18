// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker && windows
// +build docker,windows

package docker

import (
	"time"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func convertContainerStats(stats *types.Stats) *provider.ContainerStats {
	return &provider.ContainerStats{
		Timestamp: time.Now(),
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
		Total:  pointer.UIntToFloatPtr(100 * cpuStats.CPUUsage.TotalUsage),
		System: pointer.UIntToFloatPtr(100 * cpuStats.CPUUsage.UsageInKernelmode),
		User:   pointer.UIntToFloatPtr(100 * cpuStats.CPUUsage.UsageInUsermode),
	}
}

func convertMemoryStats(memStats *types.MemoryStats) *provider.ContainerMemStats {
	return &provider.ContainerMemStats{
		UsageTotal:        pointer.UIntToFloatPtr(memStats.Usage),
		Limit:             pointer.UIntToFloatPtr(memStats.Limit),
		PrivateWorkingSet: pointer.UIntToFloatPtr(memStats.PrivateWorkingSet),
		CommitBytes:       pointer.UIntToFloatPtr(memStats.Commit),
		CommitPeakBytes:   pointer.UIntToFloatPtr(memStats.CommitPeak),
	}
}

func convertIOStats(storageStats *types.StorageStats) *provider.ContainerIOStats {
	return &provider.ContainerIOStats{
		ReadBytes:       pointer.UIntToFloatPtr(storageStats.ReadSizeBytes),
		WriteBytes:      pointer.UIntToFloatPtr(storageStats.WriteSizeBytes),
		ReadOperations:  pointer.UIntToFloatPtr(storageStats.ReadCountNormalized),
		WriteOperations: pointer.UIntToFloatPtr(storageStats.WriteCountNormalized),
	}
}

func convertPIDStats(numProcs uint32) *provider.ContainerPIDStats {
	return &provider.ContainerPIDStats{
		ThreadCount: pointer.UIntToFloatPtr(uint64(numProcs)),
	}
}
