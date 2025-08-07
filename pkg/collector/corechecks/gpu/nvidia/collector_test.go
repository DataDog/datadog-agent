// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestCollectorsStillInitIfOneFails(t *testing.T) {
	succeedCollector := &mockCollector{}
	factorySucceeded := false

	// On the first call, this function returns correctly. On the second it fails.
	// We need this as we cannot rely on the order of the subsystems in the map.
	factory := func(_ ddnvml.Device) (Collector, error) {
		if !factorySucceeded {
			factorySucceeded = true
			return succeedCollector, nil
		}
		return nil, errors.New("failure")
	}

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	deps := &CollectorDependencies{DeviceCache: deviceCache}
	collectors, err := buildCollectors(deps, map[CollectorName]subsystemBuilder{"ok": factory, "fail": factory}, nil)
	require.NotNil(t, collectors)
	require.NoError(t, err)
}

func TestGetDeviceTagsMapping(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func() (*nvmlmock.Interface, taggermock.Mock)
		expected  func(t *testing.T, tagsMapping map[string][]string)
	}{
		{
			name: "Happy flow with 2 devices",
			mockSetup: func() (*nvmlmock.Interface, taggermock.Mock) {
				nvmlMock := &nvmlmock.Interface{
					DeviceGetCountFunc: func() (int, nvml.Return) {
						return 2, nvml.SUCCESS
					},
					DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
						return testutil.GetDeviceMock(index), nvml.SUCCESS
					},
				}
				fakeTagger := taggerfxmock.SetupFakeTagger(t)
				fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[0]), "foo", []string{"gpu_uuid=" + testutil.GPUUUIDs[0], "gpu_vendor=nvidia", "gpu_arch=pascal"}, nil, nil, nil)
				fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[1]), "foo", []string{"gpu_uuid=" + testutil.GPUUUIDs[1], "gpu_vendor=nvidia", "gpu_arch=turing"}, nil, nil, nil)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Len(t, tagsMapping, 2)
				tags, ok := tagsMapping[testutil.GPUUUIDs[0]]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=pascal", "gpu_uuid=" + testutil.GPUUUIDs[0]})

				tags, ok = tagsMapping[testutil.GPUUUIDs[1]]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=turing", "gpu_uuid=" + testutil.GPUUUIDs[1]})
			},
		},
		{
			name: "No available devices",
			mockSetup: func() (*nvmlmock.Interface, taggermock.Mock) {
				nvmlMock := &nvmlmock.Interface{
					DeviceGetCountFunc: func() (int, nvml.Return) {
						return 0, nvml.SUCCESS
					},
				}
				fakeTagger := taggerfxmock.SetupFakeTagger(t)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Nil(t, tagsMapping)
			},
		},
		{
			name: "Only one device successfully retrieved",
			mockSetup: func() (*nvmlmock.Interface, taggermock.Mock) {
				nvmlMock := &nvmlmock.Interface{
					DeviceGetCountFunc: func() (int, nvml.Return) {
						return 2, nvml.SUCCESS
					},
					DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
						if index == 0 {
							return testutil.GetDeviceMock(index), nvml.SUCCESS
						}
						return nil, nvml.ERROR_INVALID_ARGUMENT
					},
				}
				fakeTagger := taggerfxmock.SetupFakeTagger(t)
				fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[1]), "foo", []string{"gpu_vendor=nvidia", "gpu_arch=pascal", "gpu_uuid=" + testutil.GPUUUIDs[1]}, nil, nil, nil)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Len(t, tagsMapping, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			nvmlMock, fakeTagger := tc.mockSetup()
			ddnvml.WithMockNVML(t, nvmlMock)

			// Execute
			deviceCache, err := ddnvml.NewDeviceCache()
			require.NoError(t, err)
			tagsMapping := GetDeviceTagsMapping(deviceCache, fakeTagger)

			// Assert
			tc.expected(t, tagsMapping)
		})
	}
}

func TestAllCollectorsWork(t *testing.T) {
	// This test doesn't validate the results of the collectors, it only checks that they work with
	// the basic mock and we don't have any panics or anything.

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	deps := &CollectorDependencies{DeviceCache: deviceCache}
	collectors, err := BuildCollectors(deps, nil)
	require.NoError(t, err)
	require.NotNil(t, collectors)

	seenCollectors := make(map[CollectorName]struct{})

	for _, collector := range collectors {
		result, err := collector.Collect()
		require.NoError(t, err, "collector %s failed to collect", collector.Name())
		require.NotEmpty(t, result, "collector %s returned empty result", collector.Name())
		seenCollectors[collector.Name()] = struct{}{}
	}

	// We should have seen all the collectors
	for name := range factory {
		_, ok := seenCollectors[name]
		require.True(t, ok, "collector %s not seen", name)
	}
}

func TestRemoveDuplicateMetrics(t *testing.T) {
	t.Run("ComprehensiveScenario", func(t *testing.T) {
		// Test the exact scenario from function comment plus additional edge cases including zero priority
		allMetrics := map[CollectorName][]Metric{
			process: {
				{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1001"}},
				{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1002"}},
				{Name: "core.temp", Priority: 0}, // Zero priority (default)
			},
			device: {
				{Name: "memory.usage", Priority: 5, Tags: []string{"pid:1003"}},
				{Name: "fan.speed", Priority: 0}, // Zero priority (default)
				{Name: "power.draw", Priority: 3},
				{Name: "disk.usage", Priority: 0}, // Zero priority, unique metric
			},
			ebpf: {
				{Name: "core.temp", Priority: 7}, // Conflicts with CollectorA, higher priority beats zero
				{Name: "voltage", Priority: 2},
				{Name: "fan.speed", Priority: 0}, // Zero priority tie with CollectorB
			},
			samples: {}, // Empty collector
		}

		result := RemoveDuplicateMetrics(allMetrics)

		require.Len(t, result, 7) // 6 + 1 for fan.speed winner

		// Check all the deterministic results
		var memoryUsageCount, coreTempCount, powerDrawCount, voltageCount, diskUsageCount, fanSpeedCount int
		for _, metric := range result {
			switch metric.Name {
			case "memory.usage":
				require.Equal(t, 10, metric.Priority)
				memoryUsageCount++
			case "core.temp":
				require.Equal(t, 7, metric.Priority)
				coreTempCount++
			case "power.draw":
				require.Equal(t, 3, metric.Priority)
				powerDrawCount++
			case "voltage":
				require.Equal(t, 2, metric.Priority)
				voltageCount++
			case "disk.usage":
				require.Equal(t, 0, metric.Priority)
				diskUsageCount++
			case "fan.speed":
				require.Equal(t, 0, metric.Priority) // Zero priority tie winner
				fanSpeedCount++
			}
		}

		require.Equal(t, 2, memoryUsageCount) // Both from CollectorA
		require.Equal(t, 1, coreTempCount)    // CollectorC wins
		require.Equal(t, 1, powerDrawCount)   // CollectorB unique
		require.Equal(t, 1, voltageCount)     // CollectorC unique
		require.Equal(t, 1, diskUsageCount)   // CollectorB unique (zero priority)
		require.Equal(t, 1, fanSpeedCount)    // One collector wins the zero priority tie
	})

	t.Run("SingleCollectorMultipleSameName", func(t *testing.T) {
		// Ensure intra-collector preservation - no deduplication within same collector
		allMetrics := map[CollectorName][]Metric{
			process: {
				{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1001"}},
				{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1002"}},
				{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1003"}},
				{Name: "cpu.usage", Priority: 5},
			},
		}

		result := RemoveDuplicateMetrics(allMetrics)

		expected := []Metric{
			{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1001"}},
			{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1002"}},
			{Name: "memory.usage", Priority: 10, Tags: []string{"pid:1003"}},
			{Name: "cpu.usage", Priority: 5},
		}

		require.Len(t, result, 4)
		require.ElementsMatch(t, result, expected)
	})

	t.Run("PriorityTie", func(t *testing.T) {
		// Edge case: same metric name with same priority across collectors
		// First collector (in iteration order) should win
		allMetrics := map[CollectorName][]Metric{
			process: {
				{Name: "metric1", Priority: 5, Tags: []string{"tagA"}},
			},
			device: {
				{Name: "metric1", Priority: 5, Tags: []string{"tagB"}},
			},
		}

		result := RemoveDuplicateMetrics(allMetrics)

		// Should have exactly 1 metric (one collector wins the tie)
		require.Len(t, result, 1)
		require.Equal(t, "metric1", result[0].Name)
		require.Equal(t, 5, result[0].Priority)
		// Don't assert which specific tag wins since map iteration order is not guaranteed
	})

	t.Run("EmptyInputs", func(t *testing.T) {
		// Edge case: empty inputs
		t.Run("EmptyMap", func(t *testing.T) {
			result := RemoveDuplicateMetrics(map[CollectorName][]Metric{})
			require.Len(t, result, 0)
		})

		t.Run("EmptyCollectors", func(t *testing.T) {
			allMetrics := map[CollectorName][]Metric{
				process: {},
				ebpf:    {},
			}
			result := RemoveDuplicateMetrics(allMetrics)
			require.Len(t, result, 0)
		})

		t.Run("MixedEmptyAndNonEmpty", func(t *testing.T) {
			allMetrics := map[CollectorName][]Metric{
				process: {},
				device: {
					{Name: "metric1", Priority: 1},
				},
			}
			result := RemoveDuplicateMetrics(allMetrics)
			require.Len(t, result, 1)
			require.Equal(t, "metric1", result[0].Name)
		})
	})
}
