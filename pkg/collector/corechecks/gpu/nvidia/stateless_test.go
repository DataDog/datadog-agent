// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestSramEccErrorStatusSample(t *testing.T) {
	mockDevice := setupMockDevice(t,
		testutil.WithArchitecture("ampere"),
		testutil.WithCustomHook(func(device *mock.Device) {
			device.GetSramEccErrorStatusFunc = func() (nvml.EccSramErrorStatus, nvml.Return) {
				return nvml.EccSramErrorStatus{
					AggregateCor:            11,
					AggregateUncParity:      13,
					AggregateUncSecDed:      17,
					AggregateUncBucketL2:    19,
					AggregateUncBucketSm:    23,
					AggregateUncBucketPcie:  29,
					AggregateUncBucketMcu:   31,
					AggregateUncBucketOther: 37,
					BThresholdExceeded:      1,
				}, nvml.SUCCESS
			}
		}),
	)

	metricsOut, _, err := sramEccErrorStatusSample(mockDevice)
	require.NoError(t, err)
	require.Len(t, metricsOut, 9)

	assertMetric := func(name string, value float64, tags ...string) {
		t.Helper()
		for _, metric := range metricsOut {
			if metric.Name == name && slices.Equal(metric.Tags, tags) {
				require.Equal(t, value, metric.Value)
				require.Equal(t, metrics.GaugeType, metric.Type)
				return
			}
		}
		require.Failf(t, "metric not found", "expected metric %s with tags %v", name, tags)
	}

	assertMetric("errors.ecc.corrected.total", 11, "memory_location:sram")
	assertMetric("errors.ecc.sram.uncorrected_by_subtype.total", 13, "memory_location:sram", "error_subtype:parity")
	assertMetric("errors.ecc.sram.uncorrected_by_subtype.total", 17, "memory_location:sram", "error_subtype:secded")
	assertMetric("errors.ecc.uncorrected.total", 19, "memory_location:l2_cache")
	assertMetric("errors.ecc.uncorrected.total", 23, "memory_location:sm")
	assertMetric("errors.ecc.uncorrected.total", 29, "memory_location:pcie")
	assertMetric("errors.ecc.uncorrected.total", 31, "memory_location:microcontroller")
	assertMetric("errors.ecc.uncorrected.total", 37, "memory_location:other")
	assertMetric("errors.ecc.sram.threshold_exceeded", 1)
}

func TestSramEccErrorStatusSampleArchitectureSupport(t *testing.T) {
	device := setupMockDevice(t, testutil.WithArchitecture("turing"))

	metricsOut, _, err := sramEccErrorStatusSample(device)
	require.Error(t, err)
	require.Empty(t, metricsOut)
	require.True(t, safenvml.IsUnsupported(err))
}

func TestLegacyEccMetricOverlapRules(t *testing.T) {
	ampereDevice := setupMockDevice(t, testutil.WithArchitecture("ampere"))

	require.True(t, shouldSkipLegacyEccMetric(ampereDevice, nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.MEMORY_LOCATION_SRAM))
	require.True(t, shouldSkipLegacyEccMetric(ampereDevice, nvml.MEMORY_ERROR_TYPE_UNCORRECTED, nvml.MEMORY_LOCATION_SRAM))
	require.True(t, shouldSkipLegacyEccMetric(ampereDevice, nvml.MEMORY_ERROR_TYPE_UNCORRECTED, nvml.MEMORY_LOCATION_L2_CACHE))
	require.False(t, shouldSkipLegacyEccMetric(ampereDevice, nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.MEMORY_LOCATION_L2_CACHE))
	require.False(t, shouldSkipLegacyEccMetric(ampereDevice, nvml.MEMORY_ERROR_TYPE_UNCORRECTED, nvml.MEMORY_LOCATION_DEVICE_MEMORY))

	preAmpereDevice := setupMockDevice(t, testutil.WithArchitecture("turing"))

	require.False(t, shouldSkipLegacyEccMetric(preAmpereDevice, nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.MEMORY_LOCATION_SRAM))
	require.False(t, shouldSkipLegacyEccMetric(preAmpereDevice, nvml.MEMORY_ERROR_TYPE_UNCORRECTED, nvml.MEMORY_LOCATION_L2_CACHE))
}

// TestNewStatelessCollector tests stateless collector-specific initialization with dynamic API creation
func TestNewStatelessCollector(t *testing.T) {
	device := setupMockDevice(t, testutil.WithMockAllFunctions())

	// Test that the stateless collector creates the expected dynamic API set
	collector, err := newStatelessCollector(device, &CollectorDependencies{Workloadmeta: testutil.GetWorkloadMetaMockWithDefaultGPUs(t)})
	require.NoError(t, err)
	require.NotNil(t, collector)

	// Verify it's a baseCollector with the expected name
	bc := collector.(*baseCollector)
	require.Equal(t, stateless, bc.name)
	require.NotEmpty(t, bc.supportedAPIs) // Should have at least some supported APIs

	// Test collection works
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.NotEmpty(t, metrics) // Should have some metrics
}

// TestCollectProcessMemory tests the process memory collection with different process scenarios
func TestCollectProcessMemory(t *testing.T) {
	tests := []struct {
		name          string
		processes     testutil.MockProcessInfoList
		expectedCount int
	}{
		{
			name:          "NoProcesses",
			processes:     testutil.MockProcessInfoList{},
			expectedCount: 1, // Only memory.limit
		},
		{
			name: "SingleProcess",
			processes: testutil.MockProcessInfoList{
				{Pid: 1234, UsedGpuMemory: 536870912}, // 512MB
			},
			expectedCount: 2, // process.memory.usage + memory.limit
		},
		{
			name: "MultipleProcesses",
			processes: testutil.MockProcessInfoList{
				{Pid: 1001, UsedGpuMemory: 1073741824}, // 1GB
				{Pid: 1002, UsedGpuMemory: 2147483648}, // 2GB
				{Pid: 1003, UsedGpuMemory: 536870912},  // 512MB
			},
			expectedCount: 4, // 3 process.memory.usage + 1 memory.limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override API factory to only include process memory
			originalFactory := statelessAPIFactory
			defer func() { statelessAPIFactory = originalFactory }()

			statelessAPIFactory = func(_ *CollectorDependencies) []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "process_memory_usage",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							return processMemorySample(device)
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, testutil.WithProcessData(tt.processes, nvml.SUCCESS))

			collector, err := newStatelessCollector(mockDevice, &CollectorDependencies{})
			require.NoError(t, err)

			processMetrics, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, processMetrics, tt.expectedCount)
		})
	}
}

// TestCollectProcessMemory_Error tests error handling with API failures
func TestCollectProcessMemory_Error(t *testing.T) {
	// Override API factory to only include process memory
	originalFactory := statelessAPIFactory
	defer func() { statelessAPIFactory = originalFactory }()

	statelessAPIFactory = func(_ *CollectorDependencies) []apiCallInfo {
		return []apiCallInfo{
			{
				Name: "process_memory_usage",
				Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
					return processMemorySample(device)
				},
			},
		}
	}

	mockDevice := setupMockDevice(t, testutil.WithProcessData(nil, nvml.ERROR_UNKNOWN))

	collector, err := newStatelessCollector(mockDevice, &CollectorDependencies{})
	require.NoError(t, err)

	processMetrics, err := collector.Collect()

	// Should get error but still have some metrics (memory.limit from processMemorySample)
	require.Error(t, err)
	require.Greater(t, len(processMetrics), 0) // Should still get memory.limit metric
}

// TestProcessMemoryMetricTags tests that process memory metrics have correct tags and priorities
func TestProcessMemoryMetricTags(t *testing.T) {
	// Override API factory to only include process memory
	originalFactory := statelessAPIFactory
	defer func() { statelessAPIFactory = originalFactory }()

	statelessAPIFactory = func(_ *CollectorDependencies) []apiCallInfo {
		return []apiCallInfo{
			{
				Name: "process_memory_usage",
				Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
					return processMemorySample(device)
				},
			},
		}
	}

	processes := testutil.MockProcessInfoList{
		{Pid: 1001, UsedGpuMemory: 1073741824}, // 1GB
		{Pid: 1002, UsedGpuMemory: 2147483648}, // 2GB
	}
	mockDevice := setupMockDevice(t, testutil.WithProcessData(processes, nvml.SUCCESS))

	collector, err := newStatelessCollector(mockDevice, &CollectorDependencies{})
	require.NoError(t, err)

	processMetrics, err := collector.Collect()
	require.NoError(t, err)

	// Should have exactly 3 metrics: 2 process.memory.usage + 1 memory.limit
	require.Len(t, processMetrics, 3)

	// Check process.memory.usage metrics have associated workloads
	processMemoryMetrics := 0
	for _, metric := range processMetrics {
		if metric.Name == "process.memory.usage" {
			processMemoryMetrics++
			require.Len(t, metric.AssociatedWorkloads, 1, "process.memory.usage should have exactly one workload")
			require.Equal(t, "process", string(metric.AssociatedWorkloads[0].Kind), "process.memory.usage workload should be of kind process")
			require.Equal(t, Medium, metric.Priority, "process.memory.usage should have High priority")
		}
		if metric.Name == "memory.limit" {
			require.Len(t, metric.AssociatedWorkloads, 2, "memory.limit should have workloads for all processes")
			require.Equal(t, Medium, metric.Priority, "memory.limit should have High priority")
		}
	}
	require.Equal(t, 2, processMemoryMetrics, "Should have process.memory.usage for each process")
}

// TestNVLinkCollector_Initialization tests NVLink collector initialization (migrated from nvlink_test.go)
func TestNVLinkCollector_Initialization(t *testing.T) {
	tests := []struct {
		name      string
		mockOpts  []testutil.NvmlMockOption
		wantError bool
		wantLinks int
	}{
		{
			name:      "Unsupported device",
			mockOpts:  []testutil.NvmlMockOption{testutil.WithFieldValuesReturn(nvml.ERROR_NOT_SUPPORTED)},
			wantError: true,
		},
		{
			name:      "Unknown error",
			mockOpts:  []testutil.NvmlMockOption{testutil.WithFieldValuesReturn(nvml.ERROR_UNKNOWN)},
			wantError: false,
		},
		{
			name: "Success with 4 links",
			mockOpts: []testutil.NvmlMockOption{testutil.WithCustomHook(func(device *mock.Device) {
				device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
					require.Len(t, values, 1, "Expected one field value for total number of links, got %d", len(values))
					require.Equal(t, values[0].FieldId, uint32(nvml.FI_DEV_NVLINK_LINK_COUNT), "Expected field ID to be FI_DEV_NVLINK_LINK_COUNT, got %d", values[0].FieldId)
					require.Equal(t, values[0].ScopeId, uint32(0), "Expected scope ID to be 0, got %d", values[0].ScopeId)
					testutil.ApplyMockFieldValue(&values[0], testutil.NewFieldValue(4))
					return nvml.SUCCESS
				}
			})},
			wantError: false,
			wantLinks: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override API factory to only include NVLink
			originalFactory := statelessAPIFactory
			defer func() { statelessAPIFactory = originalFactory }()

			statelessAPIFactory = func(_ *CollectorDependencies) []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "nvlink_metrics",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							// Test the API first (like the original TestFunc)
							fields := []nvml.FieldValue{
								{
									FieldId: nvml.FI_DEV_NVLINK_LINK_COUNT,
									ScopeId: 0,
								},
							}
							if err := device.GetFieldValues(fields); err != nil {
								return nil, 0, err
							}
							// If test passes, return empty metrics for this test
							return []Metric{}, 0, nil
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, tt.mockOpts...)
			c, err := newStatelessCollector(mockDevice, &CollectorDependencies{})

			if tt.wantError {
				require.Error(t, err)
				require.Nil(t, c)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, c)
		})
	}
}

// TestNVLinkCollector_Collection tests NVLink collector collection (migrated from nvlink_test.go)
func TestNVLinkCollector_Collection(t *testing.T) {
	tests := []struct {
		name             string
		nvlinkStates     []nvml.EnableState
		nvlinkErrors     []error
		expectedActive   int
		expectedInactive int
		expectError      bool
	}{
		{
			name: "All links active",
			nvlinkStates: []nvml.EnableState{
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_ENABLED,
			},
			nvlinkErrors:     []error{nil, nil, nil},
			expectedActive:   3,
			expectedInactive: 0,
			expectError:      false,
		},
		{
			name: "Mixed active and inactive links",
			nvlinkStates: []nvml.EnableState{
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_DISABLED,
				nvml.FEATURE_ENABLED,
			},
			nvlinkErrors:     []error{nil, nil, nil},
			expectedActive:   2,
			expectedInactive: 1,
			expectError:      false,
		},
		{
			name: "Error getting link state",
			nvlinkStates: []nvml.EnableState{
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_ENABLED,
			},
			nvlinkErrors:     []error{nil, errors.New("unknown error")},
			expectedActive:   1,
			expectedInactive: 0,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override API factory to only include NVLink with full implementation
			originalFactory := statelessAPIFactory
			defer func() { statelessAPIFactory = originalFactory }()

			statelessAPIFactory = func(_ *CollectorDependencies) []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "nvlink_metrics",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							return nvlinkSample(device)
						},
					},
				}
			}

			// Create collector with mock device
			stateErrors := make(map[int]nvml.Return)
			for link, linkErr := range tt.nvlinkErrors {
				if linkErr != nil {
					stateErrors[link] = nvml.ERROR_UNKNOWN
				}
			}
			mockDevice := setupMockDevice(t,
				testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1}),
				testutil.WithNVLinkStates(tt.nvlinkStates, stateErrors),
			)

			collector, err := newStatelessCollector(mockDevice, &CollectorDependencies{})
			require.NoError(t, err)

			// Collect metrics
			allMetrics, err := collector.Collect()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify metrics, as we still expect to have all 3 metrics even if some errors were returned
			require.Len(t, allMetrics, 3)

			// Check total links metric
			require.Equal(t, float64(len(tt.nvlinkStates)), allMetrics[0].Value)
			require.Equal(t, metrics.GaugeType, allMetrics[0].Type)

			// Check active links metric
			require.Equal(t, float64(tt.expectedActive), allMetrics[1].Value)
			require.Equal(t, metrics.GaugeType, allMetrics[1].Type)

			// Check inactive links metric
			require.Equal(t, float64(tt.expectedInactive), allMetrics[2].Value)
			require.Equal(t, metrics.GaugeType, allMetrics[2].Type)
		})
	}
}

// TestProcessMemoryMetricValues tests that the correct memory metric values are emitted
// depending on the device architecture and API availability. On Hopper+, the process_detail_list
// API should win (higher priority). On older architectures, process_memory_usage is the only source.
// When GetRunningProcessDetailList fails, we should fall back to GetComputeRunningProcesses data
// including memory.limit.
func TestProcessMemoryMetricValues(t *testing.T) {
	const (
		legacyPid    = uint32(1001)
		legacyMemory = uint64(100)

		detailPid    = uint32(2002)
		detailMemory = uint64(200)
	)

	detailProc := nvml.ProcessDetail_v1{
		Pid:           detailPid,
		UsedGpuMemory: detailMemory,
	}

	tests := []struct {
		name             string
		architecture     string
		detailListErr    nvml.Return
		expectPid        uint32
		expectMemory     float64
		expectCollectErr bool
	}{
		{
			name:          "PreHopper uses GetComputeRunningProcesses",
			architecture:  "ampere",
			detailListErr: nvml.ERROR_ARGUMENT_VERSION_MISMATCH,
			expectPid:     legacyPid,
			expectMemory:  float64(legacyMemory),
		},
		{
			name:          "Hopper uses GetRunningProcessDetailList",
			architecture:  "hopper",
			detailListErr: nvml.SUCCESS,
			expectPid:     detailPid,
			expectMemory:  float64(detailMemory),
		},
		{
			name:             "Hopper fallback on detail list error",
			architecture:     "hopper",
			detailListErr:    nvml.ERROR_UNKNOWN,
			expectPid:        legacyPid,
			expectMemory:     float64(legacyMemory),
			expectCollectErr: true,
		},
		{
			name:             "Hopper fallback on detail list insufficient size",
			architecture:     "hopper",
			detailListErr:    nvml.ERROR_INSUFFICIENT_SIZE,
			expectPid:        legacyPid,
			expectMemory:     float64(legacyMemory),
			expectCollectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFactory := statelessAPIFactory
			defer func() { statelessAPIFactory = originalFactory }()

			statelessAPIFactory = func(_ *CollectorDependencies) []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "process_memory_usage",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							return processMemorySample(device)
						},
					},
					{
						Name: "process_detail_list",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							return processDetailListSample(device)
						},
					},
				}
			}

			mockDevice := setupMockDevice(t,
				testutil.WithArchitecture(tt.architecture),
				testutil.WithProcessData(testutil.MockProcessInfoList{
					{Pid: legacyPid, UsedGpuMemory: legacyMemory},
				}, nvml.SUCCESS),
				testutil.WithProcessDetailList([]nvml.ProcessDetail_v1{detailProc}, tt.detailListErr),
			)

			collector, err := newStatelessCollector(mockDevice, &CollectorDependencies{})
			require.NoError(t, err)

			allMetrics, err := collector.Collect()
			if tt.expectCollectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Deduplicate across the two API handlers, just like the real collector pipeline does
			deduped := RemoveDuplicateMetrics(map[CollectorName][]*Metric{
				collector.Name(): allMetrics,
			})

			// Find the process.memory.usage metric and verify its value
			var foundProcessMemory bool
			var foundMemoryLimit bool
			for _, m := range deduped {
				switch m.Name {
				case "process.memory.usage":
					foundProcessMemory = true
					require.Equal(t, tt.expectMemory, m.Value, "process.memory.usage value mismatch")
					require.Len(t, m.AssociatedWorkloads, 1)
					require.Equal(t, strconv.Itoa(int(tt.expectPid)), m.AssociatedWorkloads[0].ID)
				case "memory.limit":
					require.False(t, foundMemoryLimit, "memory.limit should be emitted only once")
					foundMemoryLimit = true
					require.Equal(t, float64(testutil.DefaultTotalMemory), m.Value, "memory.limit should equal total device memory")
					require.Len(t, m.AssociatedWorkloads, 1, "memory.limit should have one associated workload")
					require.Equal(t, strconv.Itoa(int(tt.expectPid)), m.AssociatedWorkloads[0].ID, "memory.limit workload should match the winning process source")
				}
			}

			require.True(t, foundProcessMemory, "expected process.memory.usage metric")
			require.True(t, foundMemoryLimit, "expected memory.limit metric")
		})
	}
}

func TestProcessDetailListArchitectureSupport(t *testing.T) {
	tests := []struct {
		name      string
		supported bool
	}{
		{
			name:      "hopper",
			supported: true,
		},
		{
			name:      "blackwell",
			supported: true,
		},
		{
			name:      "ampere",
			supported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			detailListRet := nvml.ERROR_ARGUMENT_VERSION_MISMATCH
			if tt.supported {
				detailListRet = nvml.SUCCESS
			}
			mockDevice := setupMockDevice(t,
				testutil.WithArchitecture(tt.name),
				testutil.WithMockAllFunctions(),
				testutil.WithProcessDetailList(nil, detailListRet))

			wmeta := testutil.GetWorkloadMetaMockWithDefaultGPUs(t) // used only to avoid nil pointer dereferences in initialization
			collector, err := newStatelessCollector(mockDevice, &CollectorDependencies{Workloadmeta: wmeta})
			require.NoError(t, err)

			baseColl, ok := collector.(*baseCollector)
			require.True(t, ok)

			hasProcessDetailAPICall := slices.ContainsFunc(baseColl.supportedAPIs, func(api apiCallInfo) bool {
				return api.Name == "process_detail_list"
			})
			require.Equal(t, tt.supported, hasProcessDetailAPICall)
		})
	}
}

func TestDeviceUnhealthyMetricFeatureGate(t *testing.T) {
	tests := []struct {
		name          string
		features      []env.Feature
		expectMetrics bool
		expectError   bool
	}{
		{
			name:          "not emitted when kubernetes device plugins feature is absent",
			features:      nil,
			expectMetrics: false,
			expectError:   true,
		},
		{
			name:          "emitted when kubernetes device plugins feature is present",
			features:      []env.Feature{env.KubernetesDevicePlugins},
			expectMetrics: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env.SetFeatures(t, tt.features...)

			device := setupMockDevice(t)
			wmeta := testutil.GetWorkloadMetaMockWithDefaultGPUs(t)
			apis := createStatelessAPIs(&CollectorDependencies{Workloadmeta: wmeta})
			deviceUnhealthyAPI := findAPICallByName(t, apis, "device_unhealthy_count")

			gotMetrics, _, err := deviceUnhealthyAPI.Handler(device, 0)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tt.expectMetrics {
				require.Len(t, gotMetrics, 1)
				require.Equal(t, "device.unhealthy", gotMetrics[0].Name)
			} else {
				require.Empty(t, gotMetrics)
			}
		})
	}
}

func TestPCIELinkBytesPerSecond(t *testing.T) {
	// This table was generated from https://en.wikipedia.org/wiki/PCI_Express#Hardware_protocol_summary:~:text=and%20other%20features.-,Comparison%20table,-%5Bedit%5D
	tests := map[string]struct {
		gen      int
		width    int
		wantErr  bool
		expected float64
	}{
		// PCIe 1.0 — 2.5 GT/s, 8b/10b
		"gen1 x1":  {gen: 1, width: 1, expected: 0.25e9},
		"gen1 x2":  {gen: 1, width: 2, expected: 0.5e9},
		"gen1 x4":  {gen: 1, width: 4, expected: 1e9},
		"gen1 x8":  {gen: 1, width: 8, expected: 2e9},
		"gen1 x16": {gen: 1, width: 16, expected: 4e9},

		// PCIe 2.0 — 5.0 GT/s, 8b/10b
		"gen2 x1":  {gen: 2, width: 1, expected: 0.5e9},
		"gen2 x2":  {gen: 2, width: 2, expected: 1e9},
		"gen2 x4":  {gen: 2, width: 4, expected: 2e9},
		"gen2 x8":  {gen: 2, width: 8, expected: 4e9},
		"gen2 x16": {gen: 2, width: 16, expected: 8e9},

		// PCIe 3.0 — 8.0 GT/s, 128b/130b
		"gen3 x1":  {gen: 3, width: 1, expected: 0.985e9},
		"gen3 x2":  {gen: 3, width: 2, expected: 1.969e9},
		"gen3 x4":  {gen: 3, width: 4, expected: 3.938e9},
		"gen3 x8":  {gen: 3, width: 8, expected: 7.877e9},
		"gen3 x16": {gen: 3, width: 16, expected: 15.754e9},

		// PCIe 4.0 — 16.0 GT/s, 128b/130b
		"gen4 x1":  {gen: 4, width: 1, expected: 1.969e9},
		"gen4 x2":  {gen: 4, width: 2, expected: 3.938e9},
		"gen4 x4":  {gen: 4, width: 4, expected: 7.877e9},
		"gen4 x8":  {gen: 4, width: 8, expected: 15.754e9},
		"gen4 x16": {gen: 4, width: 16, expected: 31.508e9},

		// PCIe 5.0 — 32.0 GT/s, 128b/130b
		"gen5 x1":  {gen: 5, width: 1, expected: 3.938e9},
		"gen5 x2":  {gen: 5, width: 2, expected: 7.877e9},
		"gen5 x4":  {gen: 5, width: 4, expected: 15.754e9},
		"gen5 x8":  {gen: 5, width: 8, expected: 31.508e9},
		"gen5 x16": {gen: 5, width: 16, expected: 63.015e9},

		// PCIe 6.0 — 64.0 GT/s, PAM4 + 242B/256B FLIT
		"gen6 x1":  {gen: 6, width: 1, expected: 7.563e9},
		"gen6 x2":  {gen: 6, width: 2, expected: 15.125e9},
		"gen6 x4":  {gen: 6, width: 4, expected: 30.25e9},
		"gen6 x8":  {gen: 6, width: 8, expected: 60.5e9},
		"gen6 x16": {gen: 6, width: 16, expected: 121e9},

		// Error cases
		"unknown gen 0":  {gen: 0, width: 16, wantErr: true},
		"unknown gen 7":  {gen: 7, width: 16, wantErr: true},
		"negative gen":   {gen: -1, width: 16, wantErr: true},
		"zero width":     {gen: 4, width: 0, wantErr: true},
		"negative width": {gen: 4, width: -1, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := pcieLinkBytesPerSecond(test.gen, test.width)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.InDelta(t, test.expected, actual, 1e6) // tolerate 1 MB/s drift
		})
	}
}

func TestPCIELinkMetrics(t *testing.T) {
	tests := map[string]struct {
		currentWidth       int
		maxWidth           int
		currentGeneration  int
		maxGeneration      int
		expectedMetricVals map[string]float64
		expectedErr        string
	}{
		"matching link": {
			currentWidth:      16,
			maxWidth:          16,
			currentGeneration: 4,
			maxGeneration:     4,
			expectedMetricVals: map[string]float64{
				"pci.link.width.current":  16,
				"pci.link.width.max":      16,
				"pci.link.width.degraded": 0,
				"pci.link.speed.current":  31.50769230769231e9,
				"pci.link.speed.max":      31.50769230769231e9,
				"pci.link.speed.degraded": 0,
			},
		},
		"degraded link": {
			currentWidth:      8,
			maxWidth:          16,
			currentGeneration: 4,
			maxGeneration:     4,
			expectedMetricVals: map[string]float64{
				"pci.link.width.current":  8,
				"pci.link.width.max":      16,
				"pci.link.width.degraded": 1,
				"pci.link.speed.current":  15.753846153846155e9,
				"pci.link.speed.max":      31.50769230769231e9,
				"pci.link.speed.degraded": 1,
			},
		},
		"current width error": {
			currentWidth:      -1,
			maxWidth:          16,
			currentGeneration: 4,
			maxGeneration:     4,
			expectedErr:       "get current PCIe link width",
		},
		"max width error emits current width": {
			currentWidth:      16,
			maxWidth:          -1,
			currentGeneration: 4,
			maxGeneration:     4,
			expectedMetricVals: map[string]float64{
				"pci.link.width.current": 16,
			},
			expectedErr: "get max PCIe link width",
		},
		"current generation error emits width metrics": {
			currentWidth:      8,
			maxWidth:          16,
			currentGeneration: -1,
			maxGeneration:     4,
			expectedMetricVals: map[string]float64{
				"pci.link.width.current":  8,
				"pci.link.width.max":      16,
				"pci.link.width.degraded": 1,
			},
			expectedErr: "get current PCIe link generation",
		},
		"max generation error emits current speed": {
			currentWidth:      8,
			maxWidth:          16,
			currentGeneration: 4,
			maxGeneration:     -1,
			expectedMetricVals: map[string]float64{
				"pci.link.width.current":  8,
				"pci.link.width.max":      16,
				"pci.link.width.degraded": 1,
				"pci.link.speed.current":  15.753846153846155e9,
			},
			expectedErr: "get max PCIe link generation",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			device := setupMockDevice(t, testutil.WithCustomHook(func(device *mock.Device) {
				device.GetCurrPcieLinkWidthFunc = func() (int, nvml.Return) {
					if test.currentWidth == -1 {
						return 0, nvml.ERROR_UNKNOWN
					}
					return test.currentWidth, nvml.SUCCESS
				}
				device.GetMaxPcieLinkWidthFunc = func() (int, nvml.Return) {
					if test.maxWidth == -1 {
						return 0, nvml.ERROR_UNKNOWN
					}
					return test.maxWidth, nvml.SUCCESS
				}
				device.GetCurrPcieLinkGenerationFunc = func() (int, nvml.Return) {
					if test.currentGeneration == -1 {
						return 0, nvml.ERROR_UNKNOWN
					}
					return test.currentGeneration, nvml.SUCCESS
				}
				device.GetMaxPcieLinkGenerationFunc = func() (int, nvml.Return) {
					if test.maxGeneration == -1 {
						return 0, nvml.ERROR_UNKNOWN
					}
					return test.maxGeneration, nvml.SUCCESS
				}
			}))

			metricsOut, _, err := pcieLinkMetrics(device)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.expectedErr)
			}
			require.Len(t, metricsOut, len(test.expectedMetricVals))
			metricsByName := metricValuesToPointers(metricsOut)
			for metricName, expectedValue := range test.expectedMetricVals {
				metric := findMetric(metricsByName, metricName)
				require.NotNil(t, metric, "expected metric %s", metricName)
				require.InDelta(t, expectedValue, metric.Value, 1e6)
				require.Equal(t, metrics.GaugeType, metric.Type)
			}
		})
	}
}

func TestClockThrottleReasonMetrics(t *testing.T) {
	tests := map[string]struct {
		reasons                      uint64
		expectedReason               string
		expectedThrottledWhileActive float64
	}{
		"none": {
			reasons:                      nvml.ClocksEventReasonNone,
			expectedReason:               "none",
			expectedThrottledWhileActive: 0,
		},
		"gpu idle": {
			reasons:                      nvml.ClocksEventReasonGpuIdle,
			expectedReason:               "gpu_idle",
			expectedThrottledWhileActive: 0,
		},
		"applications clocks setting": {
			reasons:                      nvml.ClocksEventReasonApplicationsClocksSetting,
			expectedReason:               "applications_clocks_setting",
			expectedThrottledWhileActive: 1,
		},
		"sw power cap": {
			reasons:                      nvml.ClocksEventReasonSwPowerCap,
			expectedReason:               "sw_power_cap",
			expectedThrottledWhileActive: 1,
		},
		"hw slowdown": {
			reasons:                      nvml.ClocksThrottleReasonHwSlowdown,
			expectedReason:               "hw_slowdown",
			expectedThrottledWhileActive: 1,
		},
		"sync boost": {
			reasons:                      nvml.ClocksEventReasonSyncBoost,
			expectedReason:               "sync_boost",
			expectedThrottledWhileActive: 1,
		},
		"sw thermal slowdown": {
			reasons:                      nvml.ClocksEventReasonSwThermalSlowdown,
			expectedReason:               "sw_thermal_slowdown",
			expectedThrottledWhileActive: 1,
		},
		"hw thermal slowdown": {
			reasons:                      nvml.ClocksThrottleReasonHwThermalSlowdown,
			expectedReason:               "hw_thermal_slowdown",
			expectedThrottledWhileActive: 1,
		},
		"hw power brake slowdown": {
			reasons:                      nvml.ClocksThrottleReasonHwPowerBrakeSlowdown,
			expectedReason:               "hw_power_brake_slowdown",
			expectedThrottledWhileActive: 1,
		},
		"display clock setting": {
			reasons:                      nvml.ClocksEventReasonDisplayClockSetting,
			expectedReason:               "display_clock_setting",
			expectedThrottledWhileActive: 1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			metricsOut := clockThrottleReasonMetrics(test.reasons)

			for _, metric := range metricsOut {
				if strings.HasPrefix(metric.Name, "clock.throttle_reasons.") {
					metricReason := strings.TrimPrefix(metric.Name, "clock.throttle_reasons.")

					if metricReason == test.expectedReason {
						require.Equal(t, 1.0, metric.Value, "expected metric %s to be 1.0", metric.Name)
					} else {
						require.Equal(t, 0.0, metric.Value, "expected metric %s to be 0.0", metric.Name)
					}

				} else if metric.Name == "clock.throttled_while_active" {
					require.Equal(t, test.expectedThrottledWhileActive, metric.Value, "expected metric %s to be %f", metric.Name, test.expectedThrottledWhileActive)

					expectedTag := throttleReasonTag + ":" + notThrottledReason
					if test.expectedThrottledWhileActive > 0 {
						expectedTag = throttleReasonTag + ":" + test.expectedReason
					}

					require.Len(t, metric.Tags, 1)
					require.Equal(t, expectedTag, metric.Tags[0], "expected metric %s to have tag %s", metric.Name, expectedTag)
				} else {
					require.Failf(t, "unexpected metric", "received unknown metric %s", metric.Name)
				}
			}
		})
	}
}

func TestNeedsRecoverySample(t *testing.T) {
	tests := []struct {
		name          string
		action        nvml.DeviceGpuRecoveryAction
		expectedValue float64
		expectedTag   string
	}{
		{name: "none", action: nvml.GPU_RECOVERY_ACTION_NONE, expectedValue: 0, expectedTag: "recovery_action:none"},
		{name: "reset", action: nvml.GPU_RECOVERY_ACTION_GPU_RESET, expectedValue: 1, expectedTag: "recovery_action:reset"},
		{name: "reboot", action: nvml.GPU_RECOVERY_ACTION_NODE_REBOOT, expectedValue: 1, expectedTag: "recovery_action:reboot"},
		{name: "drain", action: nvml.GPU_RECOVERY_ACTION_DRAIN_P2P, expectedValue: 1, expectedTag: "recovery_action:drain"},
		{name: "drain_and_reset", action: nvml.GPU_RECOVERY_ACTION_DRAIN_AND_RESET, expectedValue: 1, expectedTag: "recovery_action:drain_and_reset"},
		{name: "unknown_future_action", action: nvml.DeviceGpuRecoveryAction(99), expectedValue: 1, expectedTag: "recovery_action:unknown_99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := setupMockDevice(t, testutil.WithFieldValuesPartialOverride(map[uint32]testutil.MockFieldValue{
				nvml.FI_DEV_GET_GPU_RECOVERY_ACTION: testutil.NewFieldValue(uint64(tt.action)),
			}))

			metricsOut, _, err := needsRecoverySample(device)
			require.NoError(t, err)
			require.Len(t, metricsOut, 1)

			metric := metricsOut[0]
			require.Equal(t, "device.needs_recovery", metric.Name)
			require.Equal(t, metrics.GaugeType, metric.Type)
			require.Equal(t, tt.expectedValue, metric.Value)
			require.Equal(t, []string{tt.expectedTag}, metric.Tags)

		})
	}
}

func TestNeedsRecoverySampleUnsupported(t *testing.T) {
	device := setupMockDevice(t, testutil.WithUnsupportedFields(nvml.FI_DEV_GET_GPU_RECOVERY_ACTION))

	_, _, err := needsRecoverySample(device)
	require.Error(t, err)
	require.True(t, safenvml.IsAPIUnsupportedOnDevice(err, device))
}

func findAPICallByName(t *testing.T, apis []apiCallInfo, name string) apiCallInfo {
	t.Helper()
	for _, api := range apis {
		if api.Name == name {
			return api
		}
	}

	require.FailNowf(t, "api call not found", "expected API call %q to exist", name)
	return apiCallInfo{}
}
