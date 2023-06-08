// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && windows

package containerd

import (
	"testing"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/system"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetaTesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"

	wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestGetContainerStats_Containerd(t *testing.T) {
	currentTime := time.Now()

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
			name: "Windows metrics",
			containerdMetrics: &types.Metric{
				Data: windowsMetricsAny,
			},
			expectedContainerStats: &provider.ContainerStats{
				Timestamp: currentTime,
				CPU: &provider.ContainerCPUStats{
					Total:  pointer.Ptr(1000.0),
					System: pointer.Ptr(600.0),
					User:   pointer.Ptr(400.0),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:        pointer.Ptr(1000.0),
					PrivateWorkingSet: pointer.Ptr(100.0),
					CommitBytes:       pointer.Ptr(1000.0),
					CommitPeakBytes:   pointer.Ptr(1500.0),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Ptr(20.0),
					WriteBytes:      pointer.Ptr(10.0),
					ReadOperations:  pointer.Ptr(2.0),
					WriteOperations: pointer.Ptr(1.0),
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
			result, err := collector.GetContainerStats("test-namespace", containerID, 10*time.Second)
			assert.NoError(t, err)

			result.CPU.Limit = nil         // Don't check this field. It's complex to calculate. Needs separate tests.
			result.Timestamp = currentTime // We have no control over it, so set it to avoid checking it.

			assert.Empty(t, cmp.Diff(test.expectedContainerStats, result))
		})
	}
}

func TestGetContainerNetworkStats_Containerd(t *testing.T) {
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
			result, err := collector.GetContainerNetworkStats("test-namespace", containerID, 10*time.Second)

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
			name: "Test CPU Count",
			spec: &oci.Spec{
				Windows: &specs.Windows{
					Resources: &specs.WindowsResources{
						CPU: &specs.WindowsCPUResources{
							Count: pointer.Ptr(uint64(5)),
						},
					},
				},
			},
			expected: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(500.0),
				},
				Memory: &provider.ContainerMemStats{},
			},
		},
		{
			name: "Test CPU Maximum",
			spec: &oci.Spec{
				Windows: &specs.Windows{
					Resources: &specs.WindowsResources{
						CPU: &specs.WindowsCPUResources{
							Maximum: pointer.Ptr(uint16(5000)),
						},
					},
				},
			},
			expected: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(50 * float64(system.HostCPUCount())),
				},
				Memory: &provider.ContainerMemStats{},
			},
		},
		{
			name: "Test memory, no CPU",
			spec: &oci.Spec{
				Windows: &specs.Windows{
					Resources: &specs.WindowsResources{
						Memory: &specs.WindowsMemoryResources{
							Limit: pointer.Ptr(uint64(500)),
						},
					},
				},
			},
			expected: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(100 * float64(system.HostCPUCount())),
				},
				Memory: &provider.ContainerMemStats{
					Limit: pointer.Ptr(500.0),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outContainerStats := &provider.ContainerStats{
				CPU:    &provider.ContainerCPUStats{},
				Memory: &provider.ContainerMemStats{},
			}

			fillStatsFromSpec(outContainerStats, tt.spec)
			assert.Empty(t, cmp.Diff(tt.expected, outContainerStats))
		})
	}
}
