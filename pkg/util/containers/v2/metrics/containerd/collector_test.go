// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetaTesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"

	wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

type mockedContainer struct {
	containerd.Container

	id string
}

func (mc mockedContainer) ID() string {
	return mc.id
}

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

	windowsMetrics := wstats.Statistics{
		Container: &wstats.Statistics_Windows{
			Windows: &wstats.WindowsContainerStatistics{
				Timestamp: currentTime,
				Processor: &wstats.WindowsContainerProcessorStatistics{
					TotalRuntimeNS:  1000,
					RuntimeUserNS:   400,
					RuntimeKernelNS: 600,
				},
				Memory: &wstats.WindowsContainerMemoryStatistics{
					MemoryUsageCommitBytes:            1000,
					MemoryUsageCommitPeakBytes:        1500,
					MemoryUsagePrivateWorkingSetBytes: 100,
				},
				Storage: &wstats.WindowsContainerStorageStatistics{
					ReadCountNormalized:  2,
					ReadSizeBytes:        20,
					WriteCountNormalized: 1,
					WriteSizeBytes:       10,
				},
			},
		},
	}
	windowsMetricsAny, err := typeurl.MarshalAny(&windowsMetrics)
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
					Total:            pointer.Float64Ptr(10000),
					System:           pointer.Float64Ptr(6000),
					User:             pointer.Float64Ptr(4000),
					ThrottledPeriods: pointer.Float64Ptr(1),
					ThrottledTime:    pointer.Float64Ptr(1000),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:   pointer.Float64Ptr(1000),
					KernelMemory: pointer.Float64Ptr(500),
					Limit:        pointer.Float64Ptr(2000),
					RSS:          pointer.Float64Ptr(100),
					Cache:        pointer.Float64Ptr(20),
					Swap:         pointer.Float64Ptr(10),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Float64Ptr(60),
					WriteBytes:      pointer.Float64Ptr(20),
					ReadOperations:  pointer.Float64Ptr(6),
					WriteOperations: pointer.Float64Ptr(3),
					Devices: map[string]provider.DeviceIOStats{
						"1:1": {
							ReadBytes:       pointer.Float64Ptr(10),
							WriteBytes:      pointer.Float64Ptr(15),
							ReadOperations:  pointer.Float64Ptr(1),
							WriteOperations: pointer.Float64Ptr(2),
						},
						"1:2": {
							ReadBytes:       pointer.Float64Ptr(50),
							WriteBytes:      pointer.Float64Ptr(5),
							ReadOperations:  pointer.Float64Ptr(5),
							WriteOperations: pointer.Float64Ptr(1),
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
					Total:            pointer.Float64Ptr(10000),
					System:           pointer.Float64Ptr(6000),
					User:             pointer.Float64Ptr(4000),
					ThrottledPeriods: pointer.Float64Ptr(1),
					ThrottledTime:    pointer.Float64Ptr(1000),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:   pointer.Float64Ptr(1000),
					KernelMemory: pointer.Float64Ptr(500),
					Limit:        pointer.Float64Ptr(2000),
					RSS:          pointer.Float64Ptr(100),
					Cache:        pointer.Float64Ptr(20),
					Swap:         pointer.Float64Ptr(10),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Float64Ptr(60),
					WriteBytes:      pointer.Float64Ptr(20),
					ReadOperations:  pointer.Float64Ptr(6),
					WriteOperations: pointer.Float64Ptr(3),
					Devices: map[string]provider.DeviceIOStats{
						"1:1": {
							ReadBytes:       pointer.Float64Ptr(10),
							WriteBytes:      pointer.Float64Ptr(15),
							ReadOperations:  pointer.Float64Ptr(1),
							WriteOperations: pointer.Float64Ptr(2),
						},
						"1:2": {
							ReadBytes:       pointer.Float64Ptr(50),
							WriteBytes:      pointer.Float64Ptr(5),
							ReadOperations:  pointer.Float64Ptr(5),
							WriteOperations: pointer.Float64Ptr(1),
						},
					},
				},
			},
		},
		{
			name: "Windows metrics",
			containerdMetrics: &types.Metric{
				Data: windowsMetricsAny,
			},
			expectedContainerStats: &provider.ContainerStats{
				Timestamp: currentTime,
				CPU: &provider.ContainerCPUStats{
					Total:  pointer.Float64Ptr(1000),
					System: pointer.Float64Ptr(600),
					User:   pointer.Float64Ptr(400),
				},
				Memory: &provider.ContainerMemStats{
					PrivateWorkingSet: pointer.Float64Ptr(100),
					CommitBytes:       pointer.Float64Ptr(1000),
					CommitPeakBytes:   pointer.Float64Ptr(1500),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Float64Ptr(20),
					WriteBytes:      pointer.Float64Ptr(10),
					ReadOperations:  pointer.Float64Ptr(2),
					WriteOperations: pointer.Float64Ptr(1),
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
			result, err := collector.GetContainerStats(containerID, 10*time.Second)
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

	windowsMetrics := wstats.Statistics{
		Container: &wstats.Statistics_Windows{
			Windows: &wstats.WindowsContainerStatistics{},
		},
	}
	windowsMetricsAny, err := typeurl.MarshalAny(&windowsMetrics)
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
				BytesSent:   pointer.Float64Ptr(220),
				BytesRcvd:   pointer.Float64Ptr(110),
				PacketsSent: pointer.Float64Ptr(22),
				PacketsRcvd: pointer.Float64Ptr(11),
				Interfaces: map[string]provider.InterfaceNetStats{
					"interface-1": {
						BytesSent:   pointer.Float64Ptr(20),
						BytesRcvd:   pointer.Float64Ptr(10),
						PacketsSent: pointer.Float64Ptr(2),
						PacketsRcvd: pointer.Float64Ptr(1),
					},
					"interface-2": {
						BytesSent:   pointer.Float64Ptr(200),
						BytesRcvd:   pointer.Float64Ptr(100),
						PacketsSent: pointer.Float64Ptr(20),
						PacketsRcvd: pointer.Float64Ptr(10),
					},
				},
			},
		},
		{
			name: "Windows",
			containerdMetrics: &types.Metric{
				Data: windowsMetricsAny,
			},
			expectedNetworkStats: nil, // Does not return anything on Windows
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
			result, err := collector.GetContainerNetworkStats(containerID, 10*time.Second)

			assert.NoError(t, err)
			assert.Empty(t, cmp.Diff(test.expectedNetworkStats, result))
		})
	}
}

func TestGetContainerIDForPID(t *testing.T) {
	pidMap := map[string][]containerd.ProcessInfo{
		"cID1": {containerd.ProcessInfo{Pid: 10}},
	}

	fakeClient := fake.MockedContainerdClient{
		MockContainers: func() ([]containerd.Container, error) {
			return []containerd.Container{
				mockedContainer{id: "cID1"},
				mockedContainer{id: "cID2"},
			}, nil
		},

		MockTaskPids: func(ctn containerd.Container) ([]containerd.ProcessInfo, error) {
			return pidMap[ctn.ID()], nil
		},
	}

	collector := containerdCollector{
		client:   &fakeClient,
		pidCache: provider.NewCache(pidCacheGCInterval),
	}

	// Cache is empty, will trigger a full refresh
	cID1, err := collector.GetContainerIDForPID(10, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "cID1", cID1)

	// Add an entry for PID 20, should not be picked up because full refresh is recent enough
	pidMap["cID2"] = []containerd.ProcessInfo{{Pid: 20}}
	cID2, err := collector.GetContainerIDForPID(20, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "", cID2)

	cID2, err = collector.GetContainerIDForPID(20, 0)
	assert.NoError(t, err)
	assert.Equal(t, "cID2", cID2)
}

// Returns a fake containerd client for testing.
// For these tests we need 2 things:
//   - 1) Being able to control the metrics returned by the TaskMetrics
//   function.
//   - 2) Define functions like Info, Spec, etc. so they don't return errors.
func containerdClient(metrics *types.Metric) *fake.MockedContainerdClient {
	return &fake.MockedContainerdClient{
		MockTaskMetrics: func(ctn containerd.Container) (*types.Metric, error) {
			return metrics, nil
		},
		MockContainer: func(id string) (containerd.Container, error) {
			return mockedContainer{}, nil
		},
		MockInfo: func(ctn containerd.Container) (containers.Container, error) {
			return containers.Container{}, nil
		},
		MockSpec: func(ctn containerd.Container) (*oci.Spec, error) {
			return nil, nil
		},
		MockTaskPids: func(ctn containerd.Container) ([]containerd.ProcessInfo, error) {
			return nil, nil
		},
		MockSetCurrentNamespace: func(namespace string) {},
	}
}
