// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/system"

	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func processContainerStats(containerID string, stats interface{}) (*provider.ContainerStats, error) {
	// Linux stats can be v1 or v2
	switch metricsVal := stats.(type) {
	case *v2.Metrics:
		return getContainerdStatsV2(metricsVal), nil
	case *v1.Metrics:
		return getContainerdStatsV1(metricsVal), nil
	default:
		return nil, fmt.Errorf("can't convert the metrics data (type %T) from container with ID %s", metricsVal, containerID)
	}
}

func processContainerNetworkStats(containerID string, stats interface{}) (*provider.ContainerNetworkStats, error) {
	switch metricsVal := stats.(type) {
	case *v1.Metrics:
		return getNetworkStatsCgroupV1(metricsVal.Network), nil
	case *v2.Metrics:
		// Network stats are not available on Linux cgroupv2
		return nil, nil
	default:
		return nil, fmt.Errorf("can't convert the metrics data (type %T) from container with ID %s", metricsVal, containerID)
	}
}

func fillStatsFromSpec(outContainerStats *provider.ContainerStats, spec *oci.Spec) {
	if spec == nil || spec.Linux == nil || outContainerStats.CPU == nil {
		return
	}

	if spec.Linux.Resources != nil && spec.Linux.Resources.CPU != nil {
		computeCPULimit(outContainerStats, spec.Linux.Resources.CPU)
	}

	// If no limit is available, setting the limit to number of CPUs.
	// Always reporting a limit allows to compute CPU % accurately.
	if outContainerStats.CPU.Limit == nil {
		outContainerStats.CPU.Limit = pointer.Ptr(100 * float64(system.HostCPUCount()))
	}
}

func computeCPULimit(containerStats *provider.ContainerStats, spec *specs.LinuxCPU) {
	switch {
	case spec.Cpus != "":
		containerStats.CPU.Limit = pointer.Ptr(100 * float64(cgroups.ParseCPUSetFormat(spec.Cpus)))
	case spec.Quota != nil && *spec.Quota > 0:
		period := 100000 // Default CFS Period
		if spec.Period != nil && *spec.Period > 0 {
			period = int(*spec.Period)
		}
		containerStats.CPU.Limit = pointer.Ptr(100 * float64(*spec.Quota) / float64(period))
	}
}
