// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package metrics

import (
	"testing"
	"time"

	wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	v1 "github.com/containerd/cgroups/stats/v1"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util"
	containerdutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
)

type mockedContainerdClient struct {
	containerdutil.ContainerdItf
	mockContainer   func(id string) (containerd.Container, error)
	mockInfo        func(ctn containerd.Container) (containers.Container, error)
	mockSpec        func(ctn containerd.Container) (*oci.Spec, error)
	mockTaskMetrics func(ctn containerd.Container) (*types.Metric, error)
	mockTaskPids    func(ctn containerd.Container) ([]containerd.ProcessInfo, error)
}

func (m *mockedContainerdClient) Container(id string) (containerd.Container, error) {
	return m.mockContainer(id)
}

func (m *mockedContainerdClient) Info(ctn containerd.Container) (containers.Container, error) {
	return m.mockInfo(ctn)
}

func (m *mockedContainerdClient) Spec(ctn containerd.Container) (*oci.Spec, error) {
	return m.mockSpec(ctn)
}

func (m *mockedContainerdClient) TaskMetrics(ctn containerd.Container) (*types.Metric, error) {
	return m.mockTaskMetrics(ctn)
}

func (m *mockedContainerdClient) TaskPids(ctn containerd.Container) ([]containerd.ProcessInfo, error) {
	return m.mockTaskPids(ctn)
}

type mockedContainer struct {
	containerd.Container
}

func TestGetContainerStats_Containerd(t *testing.T) {
	currentTime := time.Now()

	linuxMetrics := v1.Metrics{
		CPU: &v1.CPUStat{
			Usage: &v1.CPUUsage{
				Total:  1000,
				Kernel: 600,
				User:   400,
			},
			Throttling: &v1.Throttle{
				ThrottledPeriods: 1,
				ThrottledTime:    100,
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
	linuxMetricsAny, err := typeurl.MarshalAny(&linuxMetrics)
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
		expectedContainerStats *ContainerStats
	}{
		{
			name: "Linux metrics",
			containerdMetrics: &types.Metric{
				Data: linuxMetricsAny,
			},
			expectedContainerStats: &ContainerStats{
				Timestamp: currentTime,
				CPU: &ContainerCPUStats{
					Total:            util.Float64Ptr(1000),
					System:           util.Float64Ptr(600),
					User:             util.Float64Ptr(400),
					ThrottledPeriods: util.Float64Ptr(1),
					ThrottledTime:    util.Float64Ptr(100),
				},
				Memory: &ContainerMemStats{
					UsageTotal:   util.Float64Ptr(1000),
					KernelMemory: util.Float64Ptr(500),
					Limit:        util.Float64Ptr(2000),
					RSS:          util.Float64Ptr(100),
					Cache:        util.Float64Ptr(20),
					Swap:         util.Float64Ptr(10),
				},
				IO: &ContainerIOStats{
					ReadBytes:       util.Float64Ptr(60),
					WriteBytes:      util.Float64Ptr(20),
					ReadOperations:  util.Float64Ptr(6),
					WriteOperations: util.Float64Ptr(3),
					Devices: map[string]DeviceIOStats{
						"1:1": {
							ReadBytes:       util.Float64Ptr(10),
							WriteBytes:      util.Float64Ptr(15),
							ReadOperations:  util.Float64Ptr(1),
							WriteOperations: util.Float64Ptr(2),
						},
						"1:2": {
							ReadBytes:       util.Float64Ptr(50),
							WriteBytes:      util.Float64Ptr(5),
							ReadOperations:  util.Float64Ptr(5),
							WriteOperations: util.Float64Ptr(1),
						},
					},
					OpenFiles: util.Float64Ptr(0), // Not checked in this test
				},
			},
		},
		{
			name: "Windows metrics",
			containerdMetrics: &types.Metric{
				Data: windowsMetricsAny,
			},
			expectedContainerStats: &ContainerStats{
				Timestamp: currentTime,
				CPU: &ContainerCPUStats{
					Total:  util.Float64Ptr(1000),
					System: util.Float64Ptr(600),
					User:   util.Float64Ptr(400),
				},
				Memory: &ContainerMemStats{
					PrivateWorkingSet: util.Float64Ptr(100),
					CommitBytes:       util.Float64Ptr(1000),
					CommitPeakBytes:   util.Float64Ptr(1500),
				},
				IO: &ContainerIOStats{
					ReadBytes:       util.Float64Ptr(20),
					WriteBytes:      util.Float64Ptr(10),
					ReadOperations:  util.Float64Ptr(2),
					WriteOperations: util.Float64Ptr(1),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector := containerdCollector{client: containerdClient(test.containerdMetrics)}

			// ID and cache TTL not relevant for these tests
			result, err := collector.GetContainerStats("1", 10*time.Second)
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
		expectedNetworkStats *ContainerNetworkStats
	}{
		{
			name: "Linux with no interface mapping",
			containerdMetrics: &types.Metric{
				Data: linuxMetricsAny,
			},
			expectedNetworkStats: &ContainerNetworkStats{
				BytesSent:   util.Float64Ptr(220),
				BytesRcvd:   util.Float64Ptr(110),
				PacketsSent: util.Float64Ptr(22),
				PacketsRcvd: util.Float64Ptr(11),
				Interfaces: map[string]InterfaceNetStats{
					"interface-1": {
						BytesSent:   util.Float64Ptr(20),
						BytesRcvd:   util.Float64Ptr(10),
						PacketsSent: util.Float64Ptr(2),
						PacketsRcvd: util.Float64Ptr(1),
					},
					"interface-2": {
						BytesSent:   util.Float64Ptr(200),
						BytesRcvd:   util.Float64Ptr(100),
						PacketsSent: util.Float64Ptr(20),
						PacketsRcvd: util.Float64Ptr(10),
					},
				},
			},
		},
		{
			name: "Linux with interface mapping",
			containerdMetrics: &types.Metric{
				Data: linuxMetricsAny,
			},
			interfaceMapping: map[string]string{
				"interface-1": "custom-1",
				"interface-2": "custom-2",
			},
			expectedNetworkStats: &ContainerNetworkStats{
				BytesSent:   util.Float64Ptr(220),
				BytesRcvd:   util.Float64Ptr(110),
				PacketsSent: util.Float64Ptr(22),
				PacketsRcvd: util.Float64Ptr(11),
				Interfaces: map[string]InterfaceNetStats{
					"custom-1": {
						BytesSent:   util.Float64Ptr(20),
						BytesRcvd:   util.Float64Ptr(10),
						PacketsSent: util.Float64Ptr(2),
						PacketsRcvd: util.Float64Ptr(1),
					},
					"custom-2": {
						BytesSent:   util.Float64Ptr(200),
						BytesRcvd:   util.Float64Ptr(100),
						PacketsSent: util.Float64Ptr(20),
						PacketsRcvd: util.Float64Ptr(10),
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
			collector := containerdCollector{client: containerdClient(test.containerdMetrics)}

			// ID and cache TTL not relevant for these tests
			result, err := collector.GetContainerNetworkStats("1", 10*time.Second, test.interfaceMapping)

			assert.NoError(t, err)
			assert.Empty(t, cmp.Diff(test.expectedNetworkStats, result))
		})
	}
}

// Returns a fake containerd client for testing.
// For these tests we need 2 things:
//   - 1) Being able to control the metrics returned by the TaskMetrics
//   function.
//   - 2) Define functions like Info, Spec, etc. so they don't return errors.
func containerdClient(metrics *types.Metric) *mockedContainerdClient {
	return &mockedContainerdClient{
		mockTaskMetrics: func(ctn containerd.Container) (*types.Metric, error) {
			return metrics, nil
		},
		mockContainer: func(id string) (containerd.Container, error) {
			return mockedContainer{}, nil
		},
		mockInfo: func(ctn containerd.Container) (containers.Container, error) {
			return containers.Container{}, nil
		},
		mockSpec: func(ctn containerd.Container) (*oci.Spec, error) {
			return nil, nil
		},
		mockTaskPids: func(ctn containerd.Container) ([]containerd.ProcessInfo, error) {
			return nil, nil
		},
	}
}
