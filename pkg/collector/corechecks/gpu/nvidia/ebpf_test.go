// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestSystemProbeCache(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "new_cache_is_invalid",
			testFunc: func(t *testing.T) {
				cache := NewSystemProbeCache()
				assert.False(t, cache.IsValid())
				assert.Nil(t, cache.GetStats())
			},
		},
		{
			name: "cache_validity_after_refresh",
			testFunc: func(t *testing.T) {
				cache := NewSystemProbeCache()

				// Mock successful refresh by manually setting stats
				testStats := &model.GPUStats{
					ProcessMetrics: []model.ProcessStatsTuple{
						{
							Key: model.ProcessStatsKey{
								PID:        123,
								DeviceUUID: testutil.DefaultGpuUUID,
							},
							UtilizationMetrics: model.UtilizationMetrics{
								UsedCores: 50,
								Memory: model.MemoryMetrics{
									CurrentBytes: 1024,
								},
							},
						},
					},
				}
				cache.stats = testStats

				assert.True(t, cache.IsValid())
				assert.Equal(t, testStats, cache.GetStats())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func TestEbpfCollectorCollect(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name:     "collect_with_invalid_cache",
			testFunc: testCollectWithInvalidCache,
		},
		{
			name:     "collect_with_single_active_process",
			testFunc: testCollectWithSingleActiveProcess,
		},
		{
			name:     "collect_with_multiple_active_processes",
			testFunc: testCollectWithMultipleActiveProcesses,
		},
		{
			name:     "collect_with_inactive_processes",
			testFunc: testCollectWithInactiveProcesses,
		},
		{
			name:     "collect_filters_by_device_uuid",
			testFunc: testCollectFiltersByDeviceUUID,
		},
		{
			name:     "collect_aggregates_pid_tags_for_limits",
			testFunc: testCollectAggregatesPidTagsForLimits,
		},
		{
			name:     "collect_emits_sm_active_metrics",
			testFunc: testCollectEmitsSmActiveMetrics,
		},
		{
			name:     "collect_emits_device_utilization_metrics",
			testFunc: testCollectEmitsDeviceSmActiveMetric,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testCollectWithInvalidCache(t *testing.T) {
	dev := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := NewSystemProbeCache()

	collector, err := newEbpfCollector(dev, cache)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)
	assert.Empty(t, metrics)
}

func testCollectWithSingleActiveProcess(t *testing.T) {
	exe := "/bin/test"
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{{Pid: 123, NsPid: 3, Cmdline: exe, Command: exe, Exe: exe}})
	kernel.WithFakeProcFS(t, procRoot)

	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.ProcessStatsTuple{
		{
			Key: model.ProcessStatsKey{
				PID:        123,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 50,
				Memory: model.MemoryMetrics{
					CurrentBytes: 1024,
				},
			},
		},
	})

	collector, err := newEbpfCollector(device, cache)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)

	// Should have 5 metrics: 3 usage (core, memory, sm_active) + 2 limit
	assert.Len(t, metrics, 5)

	// Verify usage metrics
	coreUsage := findMetric(metrics, "process.core.usage")
	require.NotNil(t, coreUsage)
	assert.Equal(t, float64(50), coreUsage.Value)
	require.Len(t, coreUsage.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(coreUsage.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", coreUsage.AssociatedWorkloads[0].ID)

	memoryUsage := findMetric(metrics, "process.memory.usage")
	require.NotNil(t, memoryUsage)
	assert.Equal(t, float64(1024), memoryUsage.Value)
	require.Len(t, memoryUsage.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(memoryUsage.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", memoryUsage.AssociatedWorkloads[0].ID)

	// Verify limit metrics have aggregated workloads
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	assert.Equal(t, float64(testutil.DefaultGpuCores), coreLimit.Value)
	require.Len(t, coreLimit.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(coreLimit.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", coreLimit.AssociatedWorkloads[0].ID)

	memoryLimit := findMetric(metrics, "memory.limit")
	require.NotNil(t, memoryLimit)
	assert.Equal(t, float64(testutil.DefaultTotalMemory), memoryLimit.Value)
	require.Len(t, memoryLimit.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(memoryLimit.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", memoryLimit.AssociatedWorkloads[0].ID)
}

func testCollectWithMultipleActiveProcesses(t *testing.T) {
	exe := "/bin/test"
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 123, NsPid: 3, Cmdline: exe, Command: exe, Exe: exe},
		{Pid: 456, NsPid: 33, Cmdline: exe, Command: exe, Exe: exe},
	})
	kernel.WithFakeProcFS(t, procRoot)

	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.ProcessStatsTuple{
		{
			Key: model.ProcessStatsKey{
				PID:        123,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 50,
				Memory: model.MemoryMetrics{
					CurrentBytes: 1024,
				},
			},
		},
		{
			Key: model.ProcessStatsKey{
				PID:        456,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 30,
				Memory: model.MemoryMetrics{
					CurrentBytes: 512,
				},
			},
		},
	})

	collector, err := newEbpfCollector(device, cache)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)

	// Should have 8 metrics: 6 usage (3 per process: core, memory, sm_active) + 2 limit
	assert.Len(t, metrics, 8)

	// Verify limit metrics have aggregated workloads
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	require.Len(t, coreLimit.AssociatedWorkloads, 2)
	workloadIDs := []string{coreLimit.AssociatedWorkloads[0].ID, coreLimit.AssociatedWorkloads[1].ID}
	assert.ElementsMatch(t, []string{"123", "456"}, workloadIDs)

	memoryLimit := findMetric(metrics, "memory.limit")
	require.NotNil(t, memoryLimit)
	require.Len(t, memoryLimit.AssociatedWorkloads, 2)
	workloadIDs = []string{memoryLimit.AssociatedWorkloads[0].ID, memoryLimit.AssociatedWorkloads[1].ID}
	assert.ElementsMatch(t, []string{"123", "456"}, workloadIDs)
}

func testCollectWithInactiveProcesses(t *testing.T) {
	exe := "/bin/test"
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{{Pid: 123, NsPid: 5, Cmdline: exe, Command: exe, Exe: exe}})
	kernel.WithFakeProcFS(t, procRoot)

	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.ProcessStatsTuple{
		{
			Key: model.ProcessStatsKey{
				PID:        123,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 50,
				Memory: model.MemoryMetrics{
					CurrentBytes: 1024,
				},
			},
		},
	})

	collector, err := newEbpfCollector(device, cache)
	require.NoError(t, err)

	// First collect with process 123
	metrics, err := collector.Collect()
	require.NoError(t, err)
	assert.Len(t, metrics, 5)

	// Now collect with empty stats (process became inactive)
	cache.stats = &model.GPUStats{ProcessMetrics: []model.ProcessStatsTuple{}}

	metrics, err = collector.Collect()
	require.NoError(t, err)

	// Should have 5 metrics: 3 zero usage (core, memory, sm_active) + 2 limit
	assert.Len(t, metrics, 5)

	// Verify zero usage metrics for inactive process
	coreUsage := findMetric(metrics, "process.core.usage")
	require.NotNil(t, coreUsage)
	assert.Equal(t, float64(0), coreUsage.Value)
	require.Len(t, coreUsage.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(coreUsage.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", coreUsage.AssociatedWorkloads[0].ID)

	memoryUsage := findMetric(metrics, "process.memory.usage")
	require.NotNil(t, memoryUsage)
	assert.Equal(t, float64(0), memoryUsage.Value)
	require.Len(t, memoryUsage.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(memoryUsage.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", memoryUsage.AssociatedWorkloads[0].ID)

	// Verify limit metrics still include inactive process workload
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	require.Len(t, coreLimit.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(coreLimit.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", coreLimit.AssociatedWorkloads[0].ID)
}

func testCollectFiltersByDeviceUUID(t *testing.T) {
	exe := "/bin/test"
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 123, NsPid: 7, Cmdline: exe, Command: exe, Exe: exe},
		{Pid: 456, NsPid: 77, Cmdline: exe, Command: exe, Exe: exe},
	})
	kernel.WithFakeProcFS(t, procRoot)

	device1UUID := "device-1-uuid"
	device2UUID := "device-2-uuid"

	device := createMockDevice(t, device1UUID)
	cache := createMockCacheWithStats([]model.ProcessStatsTuple{
		{
			Key: model.ProcessStatsKey{
				PID:        123,
				DeviceUUID: device1UUID, // This device
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 50,
				Memory: model.MemoryMetrics{
					CurrentBytes: 1024,
				},
			},
		},
		{
			Key: model.ProcessStatsKey{
				PID:        456,
				DeviceUUID: device2UUID, // Different device
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 30,
				Memory: model.MemoryMetrics{
					CurrentBytes: 512,
				},
			},
		},
	})

	collector, err := newEbpfCollector(device, cache)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)

	// Should only have metrics for device1UUID (5 metrics: 3 usage + 2 limit)
	assert.Len(t, metrics, 5)

	// All metrics should be for PID 123 only
	for _, metric := range metrics {
		require.Len(t, metric.AssociatedWorkloads, 1)
		assert.Equal(t, "process", string(metric.AssociatedWorkloads[0].Kind))
		assert.Equal(t, "123", metric.AssociatedWorkloads[0].ID)
	}
}

func testCollectAggregatesPidTagsForLimits(t *testing.T) {
	exe := "/bin/test"
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 123, NsPid: 1, Cmdline: exe, Command: exe, Exe: exe},
		{Pid: 456, NsPid: 11, Cmdline: exe, Command: exe, Exe: exe},
		{Pid: 789, NsPid: 111, Cmdline: exe, Command: exe, Exe: exe},
	})
	kernel.WithFakeProcFS(t, procRoot)

	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.ProcessStatsTuple{
		{
			Key: model.ProcessStatsKey{
				PID:        123,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 50,
				Memory: model.MemoryMetrics{
					CurrentBytes: 1024,
				},
			},
		},
		{
			Key: model.ProcessStatsKey{
				PID:        456,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 30,
				Memory: model.MemoryMetrics{
					CurrentBytes: 512,
				},
			},
		},
		{
			Key: model.ProcessStatsKey{
				PID:        789,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 20,
				Memory: model.MemoryMetrics{
					CurrentBytes: 256,
				},
			},
		},
	})

	collector, err := newEbpfCollector(device, cache)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)

	// Should have 11 metrics: 9 usage (3 per process: core, memory, sm_active) + 2 limit
	assert.Len(t, metrics, 11)

	// Verify limit metrics have all workloads aggregated
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	require.Len(t, coreLimit.AssociatedWorkloads, 3)
	workloadIDs := []string{
		coreLimit.AssociatedWorkloads[0].ID,
		coreLimit.AssociatedWorkloads[1].ID,
		coreLimit.AssociatedWorkloads[2].ID,
	}
	assert.ElementsMatch(t, []string{"123", "456", "789"}, workloadIDs)

	memoryLimit := findMetric(metrics, "memory.limit")
	require.NotNil(t, memoryLimit)
	require.Len(t, memoryLimit.AssociatedWorkloads, 3)
	workloadIDs = []string{
		memoryLimit.AssociatedWorkloads[0].ID,
		memoryLimit.AssociatedWorkloads[1].ID,
		memoryLimit.AssociatedWorkloads[2].ID,
	}
	assert.ElementsMatch(t, []string{"123", "456", "789"}, workloadIDs)

	// Verify usage metrics have individual workloads
	usageMetrics := findAllMetricsWithName(metrics, "process.core.usage")
	assert.Len(t, usageMetrics, 3)
	for _, metric := range usageMetrics {
		require.Len(t, metric.AssociatedWorkloads, 1)
		assert.Equal(t, "process", string(metric.AssociatedWorkloads[0].Kind))
	}
}

func testCollectEmitsSmActiveMetrics(t *testing.T) {
	exe := "/bin/test"
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{{Pid: 123, NsPid: 3, Cmdline: exe, Command: exe, Exe: exe}})
	kernel.WithFakeProcFS(t, procRoot)

	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.ProcessStatsTuple{
		{
			Key: model.ProcessStatsKey{
				PID:        123,
				DeviceUUID: testutil.DefaultGpuUUID,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores:     50,
				ActiveTimePct: 75.5,
				Memory: model.MemoryMetrics{
					CurrentBytes: 1024,
				},
			},
		},
	})

	collector, err := newEbpfCollector(device, cache)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)

	// Should have 5 metrics: 3 usage (core, memory, sm_active) + 2 limit
	assert.Len(t, metrics, 5)

	// Verify process.sm_active metric
	smActive := findMetric(metrics, "process.sm_active")
	require.NotNil(t, smActive, "process.sm_active metric not found")
	assert.Equal(t, 75.5, smActive.Value)
	assert.Equal(t, Low, smActive.Priority, "process.sm_active should have Low priority")
	require.Len(t, smActive.AssociatedWorkloads, 1)
	assert.Equal(t, "process", string(smActive.AssociatedWorkloads[0].Kind))
	assert.Equal(t, "123", smActive.AssociatedWorkloads[0].ID)
}

func testCollectEmitsDeviceSmActiveMetric(t *testing.T) {
	exe := "/bin/test"
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 123, NsPid: 3, Cmdline: exe, Command: exe, Exe: exe},
		{Pid: 456, NsPid: 4, Cmdline: exe, Command: exe, Exe: exe},
	})
	kernel.WithFakeProcFS(t, procRoot)

	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := &SystemProbeCache{
		stats: &model.GPUStats{
			ProcessMetrics: []model.ProcessStatsTuple{
				{
					Key: model.ProcessStatsKey{
						PID:        123,
						DeviceUUID: testutil.DefaultGpuUUID,
					},
					UtilizationMetrics: model.UtilizationMetrics{
						UsedCores:     50,
						ActiveTimePct: 60.0,
						Memory: model.MemoryMetrics{
							CurrentBytes: 1024,
						},
					},
				},
				{
					Key: model.ProcessStatsKey{
						PID:        456,
						DeviceUUID: testutil.DefaultGpuUUID,
					},
					UtilizationMetrics: model.UtilizationMetrics{
						UsedCores:     30,
						ActiveTimePct: 40.0,
						Memory: model.MemoryMetrics{
							CurrentBytes: 512,
						},
					},
				},
			},
			DeviceMetrics: []model.DeviceStatsTuple{
				{
					DeviceUUID: testutil.DefaultGpuUUID,
					Metrics: model.DeviceUtilizationMetrics{
						ActiveTimePct: 85.0, // Device-wide active time (may be less than sum due to overlaps)
					},
				},
			},
		},
	}

	collector, err := newEbpfCollector(device, cache)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)

	// Should have 10 metrics: 6 usage (3 per process: core, memory, sm_active) + 2 limit + 2 device metrics (sm_active, gr_engine_active)
	assert.Len(t, metrics, 10)

	// Verify device-level sm_active metric
	deviceSmActive := findMetric(metrics, "sm_active")
	require.NotNil(t, deviceSmActive, "sm_active metric not found")
	assert.Equal(t, 85.0, deviceSmActive.Value)
	assert.Equal(t, Low, deviceSmActive.Priority, "sm_active should have Low priority")
	assert.Empty(t, deviceSmActive.AssociatedWorkloads, "device-level sm_active should not have associated workloads")

	// Verify device-level gr_engine_active metric
	deviceGrEngineActive := findMetric(metrics, "gr_engine_active")
	require.NotNil(t, deviceGrEngineActive, "gr_engine_active metric not found")
	assert.Equal(t, 85.0, deviceGrEngineActive.Value)
	assert.Equal(t, Low, deviceGrEngineActive.Priority, "gr_engine_active should have Low priority")
	assert.Empty(t, deviceGrEngineActive.AssociatedWorkloads, "device-level gr_engine_active should not have associated workloads")

	// Verify per-process sm_active metrics
	processSmActiveMetrics := findAllMetricsWithName(metrics, "process.sm_active")
	assert.Len(t, processSmActiveMetrics, 2)
	for _, metric := range processSmActiveMetrics {
		assert.Equal(t, Low, metric.Priority)
		require.Len(t, metric.AssociatedWorkloads, 1)
	}
}

// Helper functions

func createMockDevice(t *testing.T, deviceUUID string) ddnvml.Device {
	nvmlMock := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 1, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			mockDevice := testutil.GetDeviceMock(index)
			// Override UUID if different from default
			if deviceUUID != testutil.DefaultGpuUUID {
				mockDevice.GetUUIDFunc = func() (string, nvml.Return) {
					return deviceUUID, nvml.SUCCESS
				}
			}
			return mockDevice, nvml.SUCCESS
		},
	}
	ddnvml.WithMockNVML(t, nvmlMock)

	deviceCache := ddnvml.NewDeviceCache()
	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)
	require.Len(t, devices, 1)

	return devices[0]
}

func createMockCacheWithStats(statsTuples []model.ProcessStatsTuple) *SystemProbeCache {
	cache := NewSystemProbeCache()
	cache.stats = &model.GPUStats{
		ProcessMetrics: statsTuples,
	}
	return cache
}

func findMetric(metrics []Metric, name string) *Metric {
	for _, metric := range metrics {
		if metric.Name == name {
			return &metric
		}
	}
	return nil
}

func findAllMetricsWithName(metrics []Metric, name string) []Metric {
	var result []Metric
	for _, metric := range metrics {
		if metric.Name == name {
			result = append(result, metric)
		}
	}
	return result
}
