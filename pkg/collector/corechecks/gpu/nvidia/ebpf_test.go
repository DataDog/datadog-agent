// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"slices"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
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
					Metrics: []model.StatsTuple{
						{
							Key: model.StatsKey{
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
	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.StatsTuple{
		{
			Key: model.StatsKey{
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

	// Should have 4 metrics: 2 usage + 2 limit
	assert.Len(t, metrics, 4)

	// Verify usage metrics
	coreUsage := findMetric(metrics, "core.usage")
	require.NotNil(t, coreUsage)
	assert.Equal(t, float64(50), coreUsage.Value)
	assert.Equal(t, []string{"pid:123"}, coreUsage.Tags)

	memoryUsage := findMetric(metrics, "memory.usage")
	require.NotNil(t, memoryUsage)
	assert.Equal(t, float64(1024), memoryUsage.Value)
	assert.Equal(t, []string{"pid:123"}, memoryUsage.Tags)

	// Verify limit metrics have aggregated PID tags
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	assert.Equal(t, float64(testutil.DefaultGpuCores), coreLimit.Value)
	assert.Equal(t, []string{"pid:123"}, coreLimit.Tags)

	memoryLimit := findMetric(metrics, "memory.limit")
	require.NotNil(t, memoryLimit)
	assert.Equal(t, float64(testutil.DefaultTotalMemory), memoryLimit.Value)
	assert.Equal(t, []string{"pid:123"}, memoryLimit.Tags)
}

func testCollectWithMultipleActiveProcesses(t *testing.T) {
	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.StatsTuple{
		{
			Key: model.StatsKey{
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
			Key: model.StatsKey{
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

	// Should have 6 metrics: 4 usage (2 per process) + 2 limit
	assert.Len(t, metrics, 6)

	// Verify limit metrics have aggregated PID tags
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	expectedPidTags := []string{"pid:123", "pid:456"}
	slices.Sort(coreLimit.Tags)
	slices.Sort(expectedPidTags)
	assert.Equal(t, expectedPidTags, coreLimit.Tags)

	memoryLimit := findMetric(metrics, "memory.limit")
	require.NotNil(t, memoryLimit)
	slices.Sort(memoryLimit.Tags)
	assert.Equal(t, expectedPidTags, memoryLimit.Tags)
}

func testCollectWithInactiveProcesses(t *testing.T) {
	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.StatsTuple{
		{
			Key: model.StatsKey{
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
	assert.Len(t, metrics, 4)

	// Now collect with empty stats (process became inactive)
	cache.stats = &model.GPUStats{Metrics: []model.StatsTuple{}}

	metrics, err = collector.Collect()
	require.NoError(t, err)

	// Should have 4 metrics: 2 zero usage + 2 limit
	assert.Len(t, metrics, 4)

	// Verify zero usage metrics for inactive process
	coreUsage := findMetric(metrics, "core.usage")
	require.NotNil(t, coreUsage)
	assert.Equal(t, float64(0), coreUsage.Value)
	assert.Equal(t, []string{"pid:123"}, coreUsage.Tags)

	memoryUsage := findMetric(metrics, "memory.usage")
	require.NotNil(t, memoryUsage)
	assert.Equal(t, float64(0), memoryUsage.Value)
	assert.Equal(t, []string{"pid:123"}, memoryUsage.Tags)

	// Verify limit metrics still include inactive process PID
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	assert.Equal(t, []string{"pid:123"}, coreLimit.Tags)
}

func testCollectFiltersByDeviceUUID(t *testing.T) {
	device1UUID := "device-1-uuid"
	device2UUID := "device-2-uuid"

	device := createMockDevice(t, device1UUID)
	cache := createMockCacheWithStats([]model.StatsTuple{
		{
			Key: model.StatsKey{
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
			Key: model.StatsKey{
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

	// Should only have metrics for device1UUID (4 metrics: 2 usage + 2 limit)
	assert.Len(t, metrics, 4)

	// All metrics should be for PID 123 only
	for _, metric := range metrics {
		assert.Equal(t, []string{"pid:123"}, metric.Tags)
	}
}

func testCollectAggregatesPidTagsForLimits(t *testing.T) {
	device := createMockDevice(t, testutil.DefaultGpuUUID)
	cache := createMockCacheWithStats([]model.StatsTuple{
		{
			Key: model.StatsKey{
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
			Key: model.StatsKey{
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
			Key: model.StatsKey{
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

	// Should have 8 metrics: 6 usage (2 per process) + 2 limit
	assert.Len(t, metrics, 8)

	// Verify limit metrics have all PID tags aggregated
	coreLimit := findMetric(metrics, "core.limit")
	require.NotNil(t, coreLimit)
	expectedPidTags := []string{"pid:123", "pid:456", "pid:789"}
	slices.Sort(coreLimit.Tags)
	slices.Sort(expectedPidTags)
	assert.Equal(t, expectedPidTags, coreLimit.Tags)

	memoryLimit := findMetric(metrics, "memory.limit")
	require.NotNil(t, memoryLimit)
	slices.Sort(memoryLimit.Tags)
	assert.Equal(t, expectedPidTags, memoryLimit.Tags)

	// Verify usage metrics have individual PID tags
	usageMetrics := findAllMetricsWithName(metrics, "core.usage")
	assert.Len(t, usageMetrics, 3)
	for _, metric := range usageMetrics {
		assert.Len(t, metric.Tags, 1)
		assert.Contains(t, metric.Tags[0], "pid:")
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

	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)

	devices := deviceCache.AllPhysicalDevices()
	require.Len(t, devices, 1)

	return devices[0]
}

func createMockCacheWithStats(statsTuples []model.StatsTuple) *SystemProbeCache {
	cache := NewSystemProbeCache()
	cache.stats = &model.GPUStats{
		Metrics: statsTuples,
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
