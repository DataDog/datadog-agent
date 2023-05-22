// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && windows

package containerd

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/system"

	wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/containerd/containerd/oci"
)

func processContainerStats(containerID string, stats interface{}) (*provider.ContainerStats, error) {
	winStats, ok := stats.(*wstats.Statistics)
	if !ok {
		return nil, fmt.Errorf("unknown type received, type: %T, obj: %v", stats, stats)
	}

	windowsMetrics := winStats.GetWindows()
	if windowsMetrics == nil {
		return nil, fmt.Errorf("error getting Windows metrics for container with ID %s", containerID)
	}

	return getContainerdStatsWindows(windowsMetrics), nil
}

func processContainerNetworkStats(containerID string, stats interface{}) (*provider.ContainerNetworkStats, error) {
	// Not available on Windows
	return nil, nil
}

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
		Total:  pointer.Ptr(float64(procStats.TotalRuntimeNS)),
		System: pointer.Ptr(float64(procStats.RuntimeKernelNS)),
		User:   pointer.Ptr(float64(procStats.RuntimeUserNS)),
	}
}

func getContainerdMemoryStatsWindows(memStats *wstats.WindowsContainerMemoryStatistics) *provider.ContainerMemStats {
	if memStats == nil {
		return nil
	}

	return &provider.ContainerMemStats{
		UsageTotal:        pointer.Ptr(float64(memStats.MemoryUsageCommitBytes)),
		PrivateWorkingSet: pointer.Ptr(float64(memStats.MemoryUsagePrivateWorkingSetBytes)),
		CommitBytes:       pointer.Ptr(float64(memStats.MemoryUsageCommitBytes)),
		CommitPeakBytes:   pointer.Ptr(float64(memStats.MemoryUsageCommitPeakBytes)),
	}
}

func getContainerdIOStatsWindows(ioStats *wstats.WindowsContainerStorageStatistics) *provider.ContainerIOStats {
	if ioStats == nil {
		return nil
	}

	return &provider.ContainerIOStats{
		ReadBytes:       pointer.Ptr(float64(ioStats.ReadSizeBytes)),
		WriteBytes:      pointer.Ptr(float64(ioStats.WriteSizeBytes)),
		ReadOperations:  pointer.Ptr(float64(ioStats.ReadCountNormalized)),
		WriteOperations: pointer.Ptr(float64(ioStats.WriteCountNormalized)),
	}
}

func fillStatsFromSpec(outContainerStats *provider.ContainerStats, spec *oci.Spec) {
	if spec == nil || spec.Windows == nil {
		return
	}

	// These fields seem to be mostly filled from Kubernetes, or at least we should expect the meaning to be aligned:
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/kuberuntime/kuberuntime_container_windows.go#L48
	// CPU.Count has priority over CPU.Maximum
	if spec.Windows.Resources != nil {
		// Setting CPU Limit from Spec
		if spec.Windows.Resources.CPU != nil && outContainerStats.CPU != nil {
			if spec.Windows.Resources.CPU.Count != nil && *spec.Windows.Resources.CPU.Count > 0 {
				outContainerStats.CPU.Limit = pointer.Ptr(float64(*spec.Windows.Resources.CPU.Count) * 100)
			} else if spec.Windows.Resources.CPU.Maximum != nil && *spec.Windows.Resources.CPU.Maximum > 0 {
				// CPU Maximum is a 0-10000 value that gets computed as 10000 * (cpuLimit.MilliValue() / 1000) / runtime.NumCPU()
				outContainerStats.CPU.Limit = pointer.Ptr(float64(*spec.Windows.Resources.CPU.Maximum) / 100 * float64(system.HostCPUCount()))
			}
		}

		// Setting Memory Limit from Spec
		if spec.Windows.Resources.Memory != nil && outContainerStats.Memory != nil {
			if spec.Windows.Resources.Memory.Limit != nil && *spec.Windows.Resources.Memory.Limit > 0 {
				outContainerStats.Memory.Limit = pointer.UIntPtrToFloatPtr(spec.Windows.Resources.Memory.Limit)
			}
		}
	}

	// If no limit is available, setting the limit to number of CPUs.
	// Always reporting a limit allows to compute CPU % accurately.
	if outContainerStats.CPU != nil && outContainerStats.CPU.Limit == nil {
		outContainerStats.CPU.Limit = pointer.Ptr(100 * float64(system.HostCPUCount()))
	}
}
