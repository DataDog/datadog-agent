// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"testing"
	"time"

	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl"
	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/system"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetaTesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
)

func TestGetContainerStats_Containerd(t *testing.T) {
	currentTime := time.Now()

	linuxCgroupV1Metrics := v1.Metrics{
		CPU: &v1.CPUStat{
			Usage: &v1.CPUUsage{
				Total:  10000,
				Kernel: 6000,
				User:   4000,
			},
			Throttling: &v1.Throttle{
				ThrottledPeriods: 1,
				ThrottledTime:    1000,
			},
		},
		Memory: &v1.MemoryStat{
			Cache: 20,
			RSS:   100,
			Usage: &v1.MemoryEntry{
				Limit: 2000,
				Usage: 1000,
			},
			Swap: &v1.MemoryEntry{
				Usage: 10,
			},
			Kernel: &v1.MemoryEntry{
				Usage: 500,
			},
		},
		Blkio: &v1.BlkIOStat{
			IoServiceBytesRecursive: []*v1.BlkIOEntry{
				{
					Major: 1,
					Minor: 1,
					Op:    "Read",
					Value: 10,
				},
				{
					Major: 1,
					Minor: 1,
					Op:    "Write",
					Value: 15,
				},
				{
					Major: 1,
					Minor: 2,
					Op:    "Read",
					Value: 50,
				},
				{
					Major: 1,
					Minor: 2,
					Op:    "Write",
					Value: 5,
				},
			},
			IoServicedRecursive: []*v1.BlkIOEntry{
				{
					Major: 1,
					Minor: 1,
					Op:    "Read",
					Value: 1,
				},
				{
					Major: 1,
					Minor: 1,
					Op:    "Write",
					Value: 2,
				},
				{
					Major: 1,
					Minor: 2,
					Op:    "Read",
					Value: 5,
				},
				{
					Major: 1,
					Minor: 2,
					Op:    "Write",
					Value: 1,
				},
			},
		},
	}
	linuxCgroupV1MetricsAny, err := typeurl.MarshalAny(&linuxCgroupV1Metrics)
	assert.NoError(t, err)

	linuxCgroupV2Metrics := v2.Metrics{
		CPU: &v2.CPUStat{
			UsageUsec:     10,
			UserUsec:      4,
			SystemUsec:    6,
			ThrottledUsec: 1,
			NrThrottled:   1,
		},
		Memory: &v2.MemoryStat{
			File:        20,
			Anon:        100,
			Usage:       1000,
			UsageLimit:  2000,
			SwapUsage:   10,
			Slab:        400,
			KernelStack: 100,
		},
		Io: &v2.IOStat{
			Usage: []*v2.IOEntry{
				{
					Major:  1,
					Minor:  1,
					Rbytes: 10,
					Wbytes: 15,
					Rios:   1,
					Wios:   2,
				},
				{
					Major:  1,
					Minor:  2,
					Rbytes: 50,
					Wbytes: 5,
					Rios:   5,
					Wios:   1,
				},
			},
		},
	}
	linuxCgroupV2MetricsAny, err := typeurl.MarshalAny(&linuxCgroupV2Metrics)
	assert.NoError(t, err)

	tests := []struct {
		name                   string
		containerdMetrics      *types.Metric
		expectedContainerStats *provider.ContainerStats
	}{
		{
			name: "Linux cgroup v1 metrics",
			containerdMetrics: &types.Metric{
				Data: linuxCgroupV1MetricsAny,
			},
			expectedContainerStats: &provider.ContainerStats{
				Timestamp: currentTime,
				CPU: &provider.ContainerCPUStats{
					Total:            pointer.Ptr(10000.0),
					System:           pointer.Ptr(6000.0),
					User:             pointer.Ptr(4000.0),
					ThrottledPeriods: pointer.Ptr(1.0),
					ThrottledTime:    pointer.Ptr(1000.0),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:   pointer.Ptr(1000.0),
					KernelMemory: pointer.Ptr(500.0),
					Limit:        pointer.Ptr(2000.0),
					RSS:          pointer.Ptr(100.0),
					Cache:        pointer.Ptr(20.0),
					Swap:         pointer.Ptr(10.0),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Ptr(60.0),
					WriteBytes:      pointer.Ptr(20.0),
					ReadOperations:  pointer.Ptr(6.0),
					WriteOperations: pointer.Ptr(3.0),
					Devices: map[string]provider.DeviceIOStats{
						"1:1": {
							ReadBytes:       pointer.Ptr(10.0),
							WriteBytes:      pointer.Ptr(15.0),
							ReadOperations:  pointer.Ptr(1.0),
							WriteOperations: pointer.Ptr(2.0),
						},
						"1:2": {
							ReadBytes:       pointer.Ptr(50.0),
							WriteBytes:      pointer.Ptr(5.0),
							ReadOperations:  pointer.Ptr(5.0),
							WriteOperations: pointer.Ptr(1.0),
						},
					},
				},
			},
		},
		{
			name: "Linux cgroup v2 metrics",
			containerdMetrics: &types.Metric{
				Data: linuxCgroupV2MetricsAny,
			},
			expectedContainerStats: &provider.ContainerStats{
				Timestamp: currentTime,
				CPU: &provider.ContainerCPUStats{
					Total:            pointer.Ptr(10000.0),
					System:           pointer.Ptr(6000.0),
					User:             pointer.Ptr(4000.0),
					ThrottledPeriods: pointer.Ptr(1.0),
					ThrottledTime:    pointer.Ptr(1000.0),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:   pointer.Ptr(1000.0),
					KernelMemory: pointer.Ptr(500.0),
					Limit:        pointer.Ptr(2000.0),
					RSS:          pointer.Ptr(100.0),
					Cache:        pointer.Ptr(20.0),
					Swap:         pointer.Ptr(10.0),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Ptr(60.0),
					WriteBytes:      pointer.Ptr(20.0),
					ReadOperations:  pointer.Ptr(6.0),
					WriteOperations: pointer.Ptr(3.0),
					Devices: map[string]provider.DeviceIOStats{
						"1:1": {
							ReadBytes:       pointer.Ptr(10.0),
							WriteBytes:      pointer.Ptr(15.0),
							ReadOperations:  pointer.Ptr(1.0),
							WriteOperations: pointer.Ptr(2.0),
						},
						"1:2": {
							ReadBytes:       pointer.Ptr(50.0),
							WriteBytes:      pointer.Ptr(5.0),
							ReadOperations:  pointer.Ptr(5.0),
							WriteOperations: pointer.Ptr(1.0),
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			containerID := "1"

			// The container needs to exist in the workloadmeta store and have a
			// namespace.
			workloadmetaStore := workloadmetaTesting.NewStore()
			workloadmetaStore.Set(&workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "test-namespace",
				},
			})

			collector := containerdCollector{
				client:            containerdClient(test.containerdMetrics),
				workloadmetaStore: workloadmetaStore,
			}

			// ID and cache TTL not relevant for these tests
			result, err := collector.GetContainerStats("", containerID, 10*time.Second)
			assert.NoError(t, err)

			result.CPU.Limit = nil         // Don't check this field. It's complex to calculate. Needs separate tests.
			result.Timestamp = currentTime // We have no control over it, so set it to avoid checking it.

			assert.Empty(t, cmp.Diff(test.expectedContainerStats, result))
		})
	}
}

func TestGetContainerNetworkStats_Containerd(t *testing.T) {
	linuxMetrics := v1.Metrics{
		Network: []*v1.NetworkStat{
			{
				Name:      "interface-1",
				RxBytes:   10,
				RxPackets: 1,
				TxBytes:   20,
				TxPackets: 2,
			},
			{
				Name:      "interface-2",
				RxBytes:   100,
				RxPackets: 10,
				TxBytes:   200,
				TxPackets: 20,
			},
		},
	}
	linuxMetricsAny, err := typeurl.MarshalAny(&linuxMetrics)
	assert.NoError(t, err)

	tests := []struct {
		name                 string
		containerdMetrics    *types.Metric
		interfaceMapping     map[string]string
		expectedNetworkStats *provider.ContainerNetworkStats
	}{
		{
			name: "Linux with no interface mapping",
			containerdMetrics: &types.Metric{
				Data: linuxMetricsAny,
			},
			expectedNetworkStats: &provider.ContainerNetworkStats{
				BytesSent:   pointer.Ptr(220.0),
				BytesRcvd:   pointer.Ptr(110.0),
				PacketsSent: pointer.Ptr(22.0),
				PacketsRcvd: pointer.Ptr(11.0),
				Interfaces: map[string]provider.InterfaceNetStats{
					"interface-1": {
						BytesSent:   pointer.Ptr(20.0),
						BytesRcvd:   pointer.Ptr(10.0),
						PacketsSent: pointer.Ptr(2.0),
						PacketsRcvd: pointer.Ptr(1.0),
					},
					"interface-2": {
						BytesSent:   pointer.Ptr(200.0),
						BytesRcvd:   pointer.Ptr(100.0),
						PacketsSent: pointer.Ptr(20.0),
						PacketsRcvd: pointer.Ptr(10.0),
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			containerID := "1"

			// The container needs to exist in the workloadmeta store and have a
			// namespace.
			workloadmetaStore := workloadmetaTesting.NewStore()
			workloadmetaStore.Set(&workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "test-namespace",
				},
			})

			collector := containerdCollector{
				client:            containerdClient(test.containerdMetrics),
				workloadmetaStore: workloadmetaStore,
			}

			// ID and cache TTL not relevant for these tests
			result, err := collector.GetContainerNetworkStats("", containerID, 10*time.Second)
			result.Timestamp = time.Time{} // We have no control over it, so set it to avoid checking it.

			assert.NoError(t, err)
			assert.Empty(t, cmp.Diff(test.expectedNetworkStats, result))
		})
	}
}

func Test_fillStatsFromSpec(t *testing.T) {
	tests := []struct {
		name     string
		spec     *oci.Spec
		expected *provider.ContainerStats
	}{
		{
			name: "Test CFS Quota",
			spec: &oci.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{
						CPU: &specs.LinuxCPU{
							Quota:  pointer.Ptr(int64(1000)),
							Period: pointer.Ptr(uint64(10000)),
						},
					},
				},
			},
			expected: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(10.0),
				},
			},
		},
		{
			name: "Test CFS No Period",
			spec: &oci.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{
						CPU: &specs.LinuxCPU{
							Quota: pointer.Ptr(int64(10000)),
						},
					},
				},
			},
			expected: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(10.0),
				},
			},
		},
		{
			name: "Test CPUSet",
			spec: &oci.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{
						CPU: &specs.LinuxCPU{
							Cpus: "1-3,5",
						},
					},
				},
			},
			expected: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(400.0),
				},
			},
		},
		{
			name: "Test no resources",
			spec: &oci.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{},
				},
			},
			expected: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(100 * float64(system.HostCPUCount())),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outContainerStats := &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{},
			}

			fillStatsFromSpec(outContainerStats, tt.spec)
			assert.Empty(t, cmp.Diff(tt.expected, outContainerStats))
		})
	}
}
