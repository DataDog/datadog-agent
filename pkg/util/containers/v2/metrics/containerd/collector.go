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

	wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/DataDog/datadog-agent/pkg/config"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/system"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	containerdCollectorID = "containerd"
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:       containerdCollectorID,
		Priority: 1, // Less than the "system" collector, so we can rely on cgroups directly if possible
		Runtimes: []string{provider.RuntimeNameContainerd},
		Factory: func() (provider.Collector, error) {
			return newContainerdCollector()
		},
		DelegateCache: true,
	})
}

type containerdCollector struct {
	client            cutil.ContainerdItf
	workloadmetaStore workloadmeta.Store
}

func newContainerdCollector() (*containerdCollector, error) {
	if !config.IsFeaturePresent(config.Containerd) {
		return nil, provider.ErrPermaFail
	}

	client, err := cutil.NewContainerdUtil()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &containerdCollector{
		client:            client,
		workloadmetaStore: workloadmeta.GetGlobalStore(),
	}, nil
}

// ID returns the collector ID.
func (c *containerdCollector) ID() string {
	return containerdCollectorID
}

// GetContainerStats returns stats by container ID.
func (c *containerdCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	namespace, err := c.containerNamespace(containerID)
	if err != nil {
		return nil, err
	}
	c.client.SetCurrentNamespace(namespace)

	metrics, err := c.getContainerdMetrics(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	if winStats, ok := metrics.(*wstats.Statistics); ok {
		windowsMetrics := winStats.GetWindows()

		if windowsMetrics == nil {
			return nil, fmt.Errorf("error getting Windows metrics for container with ID %s: %s", containerID, err)
		}

		return getContainerdStatsWindows(windowsMetrics), nil
	}

	container, err := c.client.Container(containerID)
	if err != nil {
		return nil, fmt.Errorf("could not get container with ID %s: %w", containerID, err)
	}

	OCISpec, err := c.client.Spec(container)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve OCI Spec from container with ID %s: %w", containerID, err)
	}

	info, err := c.client.Info(container)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve the metadata of the container with ID %s: %w", containerID, err)
	}

	processes, err := c.client.TaskPids(container)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve the processes of the container with ID %s: %w", containerID, err)
	}

	// Linux stats can be v1 or v2
	switch metricsVal := metrics.(type) {
	case *v2.Metrics:
		return getContainerdStatsV2(metricsVal, info, OCISpec, processes), nil
	case *v1.Metrics:
		return getContainerdStatsV1(metricsVal, info, OCISpec, processes), nil
	default:
		return nil, fmt.Errorf("can't convert the metrics data (type %T) from container with ID %s", metricsVal, containerID)
	}
}

// GetContainerNetworkStats returns network stats by container ID.
func (c *containerdCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	namespace, err := c.containerNamespace(containerID)
	if err != nil {
		return nil, err
	}
	c.client.SetCurrentNamespace(namespace)

	metrics, err := c.getContainerdMetrics(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	switch metricsVal := metrics.(type) {
	case *v1.Metrics:
		return getNetworkStatsCgroupV1(metricsVal.Network), nil
	case *v2.Metrics:
		// Network stats are not available on Linux cgroupv2
		return nil, nil
	case *wstats.Statistics:
		// Network stats are not available on Windows
		return nil, nil
	default:
		return nil, fmt.Errorf("can't convert the metrics data (type %T) from container with ID %s", metricsVal, containerID)
	}
}

// This method returns interface{} because the metrics could be an instance of
// v1.Metrics (for Linux) or stats.Statistics (Windows) and they don't share a
// common interface.
func (c *containerdCollector) getContainerdMetrics(containerID string, cacheValidity time.Duration) (interface{}, error) {
	container, err := c.client.Container(containerID)
	if err != nil {
		return nil, fmt.Errorf("could not get container with ID %s: %s", containerID, err)
	}

	metricTask, errTask := c.client.TaskMetrics(container)
	if errTask != nil {
		return nil, fmt.Errorf("could not get metrics for container with ID %s: %s", containerID, err)
	}

	metrics, err := typeurl.UnmarshalAny(metricTask.Data)
	if err != nil {
		return nil, fmt.Errorf("could not convert the metrics data from container with ID %s: %s", containerID, err)
	}

	return metrics, nil
}

func getContainerdCPULimit(currentTime time.Time, startTime time.Time, OCISpec *oci.Spec) *float64 {
	timeDiff := float64(currentTime.Sub(startTime).Nanoseconds()) // cpu.total is in nanoseconds

	if timeDiff <= 0 {
		return nil
	}

	var cpuLimits *specs.LinuxCPU
	if OCISpec != nil && OCISpec.Linux != nil && OCISpec.Linux.Resources != nil {
		cpuLimits = OCISpec.Linux.Resources.CPU
	}

	cpuLimitPct := float64(system.HostCPUCount())
	if cpuLimits != nil && cpuLimits.Period != nil && *cpuLimits.Period > 0 && cpuLimits.Quota != nil && *cpuLimits.Quota > 0 {
		cpuLimitPct = float64(*cpuLimits.Quota) / float64(*cpuLimits.Period)
	}

	limit := cpuLimitPct * timeDiff
	return &limit
}

func (c *containerdCollector) containerNamespace(containerID string) (string, error) {
	container, err := c.workloadmetaStore.GetContainer(containerID)
	if err != nil {
		return "", err
	}

	return container.Namespace, nil
}
