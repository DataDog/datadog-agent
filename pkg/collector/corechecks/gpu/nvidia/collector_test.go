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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestCollectorsStillInitIfOneFails(t *testing.T) {
	succeedCollector := &mockCollector{}
	factorySucceeded := false

	// On the first call, this function returns correctly. On the second it fails.
	// We need this as we cannot rely on the order of the subsystems in the map.
	factory := func(_ ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
		if !factorySucceeded {
			factorySucceeded = true
			return succeedCollector, nil
		}
		return nil, errors.New("failure")
	}

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)
	deps := &CollectorDependencies{}
	collectors, err := buildCollectors(devices, deps, map[CollectorName]subsystemBuilder{"ok": factory, "fail": factory}, nil)
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
			deviceCache := ddnvml.NewDeviceCache()
			tagsMapping := GetDeviceTagsMapping(deviceCache, fakeTagger)

			// Assert
			tc.expected(t, tagsMapping)
		})
	}
}

func TestAllCollectorsWork(t *testing.T) {
	// This test doesn't validate the results of the collectors, it only checks that they work with
	// the basic mock, and we don't have any panics or anything.

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	eventsGatherer := NewDeviceEventsGatherer()
	require.NoError(t, eventsGatherer.Start())
	t.Cleanup(func() { require.NoError(t, eventsGatherer.Stop()) })
	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)

	deps := &CollectorDependencies{
		DeviceEventsGatherer: eventsGatherer,
		Workloadmeta:         testutil.GetWorkloadMetaMockWithDefaultGPUs(t),
	}
	collectors, err := BuildCollectors(devices, deps, nil)
	require.NoError(t, err)
	require.NotNil(t, collectors)

	seenCollectors := make(map[CollectorName]struct{})

	for _, collector := range collectors {
		result, err := collector.Collect()
		require.NoError(t, err, "collector %s failed to collect", collector.Name())
		if collector.Name() != deviceEvents {
			require.NotEmpty(t, result, "collector %s returned empty result", collector.Name())
		}
		seenCollectors[collector.Name()] = struct{}{}
	}

	// We should have seen all the collectors
	for name := range factory {
		_, ok := seenCollectors[name]
		require.True(t, ok, "collector %s not seen", name)
	}
}

func TestDisabledCollectors(t *testing.T) {
	tests := []struct {
		name                   string
		disabledCollectors     []string
		expectedCollectorCount int
		expectedCollectorNames []CollectorName
		unexpectedNames        []CollectorName
	}{
		{
			name:                   "no collectors disabled",
			disabledCollectors:     []string{},
			expectedCollectorCount: 5, // stateless, sampling, fields, gpm, device_events
			expectedCollectorNames: []CollectorName{stateless, sampling, field, gpm, deviceEvents},
		},
		{
			name:                   "disable gpm collector",
			disabledCollectors:     []string{"gpm"},
			expectedCollectorCount: 4,
			expectedCollectorNames: []CollectorName{stateless, sampling, field, deviceEvents},
			unexpectedNames:        []CollectorName{gpm},
		},
		{
			name:                   "disable multiple collectors",
			disabledCollectors:     []string{"gpm", "fields"},
			expectedCollectorCount: 3,
			expectedCollectorNames: []CollectorName{stateless, sampling, deviceEvents},
			unexpectedNames:        []CollectorName{gpm, field},
		},
		{
			name:                   "disable all collectors",
			disabledCollectors:     []string{"stateless", "sampling", "fields", "gpm", "device_events"},
			expectedCollectorCount: 0,
			expectedCollectorNames: []CollectorName{},
		},
		{
			name:                   "disable non-existent collector",
			disabledCollectors:     []string{"non_existent"},
			expectedCollectorCount: 5,
			expectedCollectorNames: []CollectorName{stateless, sampling, field, gpm, deviceEvents},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup NVML mock
			nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithDeviceCount(1), testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
			ddnvml.WithMockNVML(t, nvmlMock)
			deviceCache := ddnvml.NewDeviceCache()
			devices, err := deviceCache.AllPhysicalDevices()
			require.NoError(t, err)

			// Setup dependencies
			eventsGatherer := NewDeviceEventsGatherer()
			require.NoError(t, eventsGatherer.Start())
			t.Cleanup(func() { require.NoError(t, eventsGatherer.Stop()) })

			deps := &CollectorDependencies{
				DeviceEventsGatherer: eventsGatherer,
				Workloadmeta:         testutil.GetWorkloadMetaMockWithDefaultGPUs(t),
			}

			// Build collectors with disabled list
			collectors, err := BuildCollectors(devices, deps, tt.disabledCollectors)
			require.NoError(t, err)

			// Verify the correct number of collectors were created
			require.Equal(t, tt.expectedCollectorCount, len(collectors),
				"expected %d collectors, got %d", tt.expectedCollectorCount, len(collectors))

			// Verify the correct collectors were created
			collectorNames := make(map[CollectorName]bool)
			for _, collector := range collectors {
				collectorNames[collector.Name()] = true
			}

			for _, expectedName := range tt.expectedCollectorNames {
				require.True(t, collectorNames[expectedName],
					"expected collector %s to be created", expectedName)
			}

			// Verify disabled collectors were not created
			for _, unexpectedName := range tt.unexpectedNames {
				require.False(t, collectorNames[unexpectedName],
					"collector %s should not be created", unexpectedName)
			}
		})
	}
}

func TestDisabledCollectorsWithSystemProbe(t *testing.T) {
	// Setup NVML mock
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)

	// Setup dependencies with system-probe cache
	eventsGatherer := NewDeviceEventsGatherer()
	require.NoError(t, eventsGatherer.Start())
	t.Cleanup(func() { require.NoError(t, eventsGatherer.Stop()) })

	spCache := &SystemProbeCache{}

	deps := &CollectorDependencies{
		DeviceEventsGatherer: eventsGatherer,
		SystemProbeCache:     spCache,
		Workloadmeta:         testutil.GetWorkloadMetaMockWithDefaultGPUs(t),
	}

	// Build collectors with ebpf disabled
	collectors, err := BuildCollectors(devices, deps, []string{"ebpf"})
	require.NoError(t, err)

	// Verify no ebpf collectors were created
	for _, collector := range collectors {
		require.NotEqual(t, ebpf, collector.Name(),
			"ebpf collector should not be created when disabled")
	}

	// Verify other collectors were created
	require.Greater(t, len(collectors), 0, "should have created some collectors")

	// Now test without disabling ebpf - should create ebpf collectors
	collectors, err = BuildCollectors(devices, deps, []string{})
	require.NoError(t, err)

	// Verify ebpf collectors were created
	foundEbpf := false
	for _, collector := range collectors {
		if collector.Name() == ebpf {
			foundEbpf = true
			break
		}
	}
	require.True(t, foundEbpf, "ebpf collector should be created when not disabled")
}

func TestRemoveDuplicateMetrics(t *testing.T) {
	t.Run("ComprehensiveScenario", func(t *testing.T) {
		// Test the exact scenario from function comment plus additional edge cases including zero priority
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1001"}},
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1002"}},
				{Name: "core.temp", Priority: Low}, // Zero priority (default)
			},
			stateless: {
				{Name: "memory.usage", Priority: Low, Tags: []string{"pid:1003"}},
				{Name: "fan.speed", Priority: Low}, // Zero priority (default)
				{Name: "power.draw", Priority: Low},
				{Name: "disk.usage", Priority: Low}, // Zero priority, unique metric
			},
			ebpf: {
				{Name: "core.temp", Priority: Medium}, // Conflicts with CollectorA, higher priority beats zero
				{Name: "voltage", Priority: Low},
				{Name: "fan.speed", Priority: Low}, // Zero priority tie with CollectorB
			},
			field: {}, // Empty collector
		}

		result := RemoveDuplicateMetrics(allMetrics)

		require.Len(t, result, 7) // 6 + 1 for fan.speed winner

		// Check all the deterministic results
		var memoryUsageCount, coreTempCount, powerDrawCount, voltageCount, diskUsageCount, fanSpeedCount int
		for _, metric := range result {
			switch metric.Name {
			case "memory.usage":
				require.Equal(t, Medium, metric.Priority)
				require.NotContains(t, metric.Tags, "pid:1003")
				memoryUsageCount++
			case "core.temp":
				require.Equal(t, Medium, metric.Priority)
				coreTempCount++
			case "power.draw":
				require.Equal(t, Low, metric.Priority)
				powerDrawCount++
			case "voltage":
				require.Equal(t, Low, metric.Priority)
				voltageCount++
			case "disk.usage":
				require.Equal(t, Low, metric.Priority)
				diskUsageCount++
			case "fan.speed":
				require.Equal(t, Low, metric.Priority) // Zero priority tie winner
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
			sampling: {
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1001"}},
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1002"}},
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1003"}},
				{Name: "cpu.usage", Priority: Low},
			},
		}

		result := RemoveDuplicateMetrics(allMetrics)

		expected := []Metric{
			{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1001"}},
			{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1002"}},
			{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1003"}},
			{Name: "cpu.usage", Priority: Low},
		}

		require.Len(t, result, 4)
		require.ElementsMatch(t, result, expected)
	})

	t.Run("PriorityTie", func(t *testing.T) {
		// Edge case: same metric name with same priority across collectors
		// First collector (in iteration order) should win
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "metric1", Priority: Low, Tags: []string{"tagA"}},
			},
			stateless: {
				{Name: "metric1", Priority: Low, Tags: []string{"tagB"}},
			},
		}

		result := RemoveDuplicateMetrics(allMetrics)

		// Should have exactly 1 metric (one collector wins the tie)
		require.Len(t, result, 1)
		require.Equal(t, Low, result[0].Priority)
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
				sampling: {},
				ebpf:     {},
			}
			result := RemoveDuplicateMetrics(allMetrics)
			require.Len(t, result, 0)
		})

		t.Run("MixedEmptyAndNonEmpty", func(t *testing.T) {
			allMetrics := map[CollectorName][]Metric{
				sampling: {},
				stateless: {
					{Name: "metric1", Priority: Low},
				},
			}
			result := RemoveDuplicateMetrics(allMetrics)
			require.Len(t, result, 1)
			require.Equal(t, "metric1", result[0].Name)
		})
	})

	t.Run("PreservedTags", func(t *testing.T) {
		tags := []string{"pid:1001", "pid:1002"}
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "memory.limit", Priority: Medium, Tags: tags},
			},
			ebpf: {
				{Name: "memory.limit", Priority: Low, Tags: nil},
			},
		}
		result := RemoveDuplicateMetrics(allMetrics)
		require.Len(t, result, 1)
		require.ElementsMatch(t, result[0].Tags, tags)
	})

	t.Run("DifferentPrioritySameCollector", func(t *testing.T) {
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "memory.limit", Priority: Medium, Tags: []string{"pid:1001"}},
				{Name: "memory.limit", Priority: Low, Tags: []string{""}},
			},
		}
		result := RemoveDuplicateMetrics(allMetrics)
		require.Len(t, result, 1)
		require.ElementsMatch(t, result[0].Tags, []string{"pid:1001"})
	})
}

// TestConfiguredMetricPriority ensures that the priority is as defined for certain critical metrics
func TestConfiguredMetricPriority(t *testing.T) {
	const pid = 123
	device := setupMockDeviceWithLibOpts(t, func(device *nvmlmock.Device) *nvmlmock.Device {
		device.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
			return []nvml.ProcessUtilizationSample{
				{
					Pid:    pid,
					SmUtil: 50,
				},
			}, nvml.SUCCESS
		}
		device.GetSamplesFunc = func(_ nvml.SamplingType, lastTimestamp uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
			return nvml.VALUE_TYPE_UNSIGNED_INT, []nvml.Sample{
				{TimeStamp: lastTimestamp + 100, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 1}},
				{TimeStamp: lastTimestamp + 200, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 2}},
			}, nvml.SUCCESS
		}
		return device
	}, testutil.WithMockAllFunctions())
	deviceUUID := device.GetDeviceInfo().UUID

	spCache := &SystemProbeCache{
		stats: &model.GPUStats{
			ProcessMetrics: []model.ProcessStatsTuple{
				{
					Key: model.ProcessStatsKey{
						PID:        123,
						DeviceUUID: deviceUUID,
					},
					UtilizationMetrics: model.UtilizationMetrics{
						UsedCores: 50,
						Memory: model.MemoryMetrics{
							CurrentBytes: 1024,
						},
						ActiveTimePct: 50,
					},
				},
			},
			DeviceMetrics: []model.DeviceStatsTuple{
				{
					DeviceUUID: deviceUUID,
					Metrics: model.DeviceUtilizationMetrics{
						ActiveTimePct: 50,
					},
				},
			},
		},
	}

	deps := &CollectorDependencies{
		SystemProbeCache: spCache,
		Workloadmeta:     testutil.GetWorkloadMetaMockWithDefaultGPUs(t),
	}

	// Build collectors with deviceEvents disabled (not useful for this test)
	collectors, err := BuildCollectors([]ddnvml.Device{device}, deps, []string{string(deviceEvents)})
	require.NoError(t, err)

	// Set up the expected metric order. The first collector in the list should have the highest priority over the rest.
	desiredMetricPriority := map[string][]CollectorName{
		"sm_active":         {sampling, ebpf},
		"gr_engine_active":  {gpm, sampling, ebpf},
		"process.sm_active": {sampling, ebpf},
	}

	metricsByCollector := make(map[string]map[CollectorName]Metric)

	for metricName := range desiredMetricPriority {
		metricsByCollector[metricName] = make(map[CollectorName]Metric)
	}

	for _, collector := range collectors {
		metrics, err := collector.Collect()
		require.NoError(t, err)
		for _, metric := range metrics {
			metricMap, ok := metricsByCollector[metric.Name]
			if ok {
				require.NotContains(t, metricMap, collector.Name(), "each collector should only emit one %s metric with the same name", metric.Name)
				metricMap[collector.Name()] = metric
			}
		}
	}

	for metricName, metricMap := range metricsByCollector {
		t.Run(metricName, func(t *testing.T) {
			require.Contains(t, desiredMetricPriority, metricName) // sanity check
			collectorOrder := desiredMetricPriority[metricName]

			require.Len(t, metricMap, len(collectorOrder))
			for _, collectorName := range collectorOrder {
				require.Contains(t, metricMap, collectorName, "each collector should emit the metric")
			}

			for i := range len(collectorOrder) - 1 {
				higherPriorityCollector := collectorOrder[i]
				lowerPriorityCollector := collectorOrder[i+1]
				require.Greater(t, metricMap[higherPriorityCollector].Priority, metricMap[lowerPriorityCollector].Priority, "collector %s should have higher priority than collector %s", higherPriorityCollector, lowerPriorityCollector)
			}
		})
	}
}
