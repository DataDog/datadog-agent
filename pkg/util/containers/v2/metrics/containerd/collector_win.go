// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"

	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func getContainerdStatsWindows(windowsStats *wstats.WindowsContainerStatistics) *provider.ContainerStats {
	if windowsStats == nil {
		return nil
	}

	return &provider.ContainerStats{
		Timestamp: windowsStats.Timestamp,
		CPU:       getContainerdCPUStatsWindows(windowsStats.Processor),
		Memory:    getContainerdMemoryStatsWindows(windowsStats.Memory),
		IO:        getContainerdIOStatsWindows(windowsStats.Storage),
	}
}

func getContainerdCPUStatsWindows(procStats *wstats.WindowsContainerProcessorStatistics) *provider.ContainerCPUStats {
	if procStats == nil {
		return nil
	}

	return &provider.ContainerCPUStats{
		Total:  pointer.UIntToFloatPtr(procStats.TotalRuntimeNS),
		System: pointer.UIntToFloatPtr(procStats.RuntimeKernelNS),
		User:   pointer.UIntToFloatPtr(procStats.RuntimeUserNS),
	}
}

func getContainerdMemoryStatsWindows(memStats *wstats.WindowsContainerMemoryStatistics) *provider.ContainerMemStats {
	if memStats == nil {
		return nil
	}

	return &provider.ContainerMemStats{
		PrivateWorkingSet: pointer.UIntToFloatPtr(memStats.MemoryUsagePrivateWorkingSetBytes),
		CommitBytes:       pointer.UIntToFloatPtr(memStats.MemoryUsageCommitBytes),
		CommitPeakBytes:   pointer.UIntToFloatPtr(memStats.MemoryUsageCommitPeakBytes),
	}
}

func getContainerdIOStatsWindows(ioStats *wstats.WindowsContainerStorageStatistics) *provider.ContainerIOStats {
	if ioStats == nil {
		return nil
	}

	return &provider.ContainerIOStats{
		ReadBytes:       pointer.UIntToFloatPtr(ioStats.ReadSizeBytes),
		WriteBytes:      pointer.UIntToFloatPtr(ioStats.WriteSizeBytes),
		ReadOperations:  pointer.UIntToFloatPtr(ioStats.ReadCountNormalized),
		WriteOperations: pointer.UIntToFloatPtr(ioStats.WriteCountNormalized),
	}
}
