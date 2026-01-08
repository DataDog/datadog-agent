// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

// TestNVMLDeviceEnumeration tests that NVML can enumerate GPU devices on the current system.
// This validates the check's ability to discover and interact with GPUs.
func TestNVMLDeviceEnumeration(t *testing.T) {
	testutil.RequireGPU(t)

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err, "NVML library should be available")
	require.NotNil(t, lib, "NVML library should not be nil")

	deviceCount, err := lib.DeviceGetCount()
	require.NoError(t, err, "Should be able to get device count")
	require.Greater(t, deviceCount, 0, "Should have at least one GPU")

	for i := 0; i < deviceCount; i++ {
		device, err := lib.DeviceGetHandleByIndex(i)
		require.NoError(t, err, "Should be able to get device handle for index %d", i)
		require.NotNil(t, device, "Device handle should not be nil")

		name, err := device.GetName()
		require.NoError(t, err, "Should be able to get device name")
		t.Logf("GPU %d: %s", i, name)

		uuid, err := device.GetUUID()
		require.NoError(t, err, "Should be able to get device UUID")
		t.Logf("GPU %d UUID: %s", i, uuid)
	}
}

// valueInterval represents a valid range for a metric value
type valueInterval struct {
	min float64
	max float64
}

// metricTestCase defines a test case for validating a metric
type metricTestCase struct {
	name string // Metric name (e.g., "gpu.sm_active")
	// getExpectedValue returns the expected value based on device info.
	// If set, ExpectedValue is ignored.
	getExpectedValue func(*safenvml.DeviceInfo) float64
	interval         *valueInterval // Valid range for the value (nil to skip interval check)
}

// extractDeviceUUID extracts the GPU UUID from metric tags
func extractDeviceUUID(tags []string) string {
	for _, tag := range tags {
		if len(tag) > 9 && tag[:9] == "gpu_uuid:" {
			return tag[9:]
		}
	}
	return ""
}

// assertMetricCase validates a metric against its test case
func assertMetricCase(t *testing.T, metricsByName map[string][]mock.Call, tc metricTestCase, deviceCache safenvml.DeviceCache) {
	t.Helper()

	calls, ok := metricsByName[tc.name]
	if !assert.True(t, ok, "%s metric should be present", tc.name) || !assert.NotEmpty(t, calls, "No calls found for metric %s", tc.name) {
		return
	}

	for _, call := range calls {
		value := call.Arguments[1].(float64)
		tags := call.Arguments[3].([]string)
		deviceUUID := extractDeviceUUID(tags)

		device, err := deviceCache.GetByUUID(deviceUUID)
		require.NoError(t, err, "Should be able to get device by UUID %s", deviceUUID)
		deviceInfo := device.GetDeviceInfo()
		require.NotNil(t, deviceInfo, "Device info should not be nil")

		if tc.getExpectedValue != nil {
			expectedValue := tc.getExpectedValue(deviceInfo)
			assert.Equal(t, expectedValue, value,
				"%s should match expected value for device %s", tc.name, deviceUUID)
		}

		if tc.interval != nil {
			assert.GreaterOrEqual(t, value, tc.interval.min,
				"%s should be >= %v", tc.name, tc.interval.min)
			assert.LessOrEqual(t, value, tc.interval.max,
				"%s should be <= %v", tc.name, tc.interval.max)
		}
	}
}

// TestCheckRunWithRealHardware tests the full check run against real GPU hardware
// and validates that expected metrics are collected with reasonable values.
func TestCheckRunWithRealHardware(t *testing.T) {
	testutil.RequireGPU(t)

	// Get device info for validation
	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NoError(t, cache.Refresh())

	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)

	// Set up the check with mocked agent components
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	checkInstance := gpu.NewCheck(fakeTagger, testutil.GetTelemetryMock(t), wmetaMock)

	mockSender := mocksender.NewMockSenderWithSenderManager(checkInstance.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// Enable GPU check
	gpu.WithGPUConfigEnabled(t)

	// Configure the check - need to set container provider before Configure
	checkInternal, ok := checkInstance.(*gpu.Check)
	require.True(t, ok)
	checkInternal.SetContainerProvider(mock_containers.NewMockContainerProvider(gomock.NewController(t)))

	err = checkInstance.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test")
	require.NoError(t, err)
	t.Cleanup(func() { checkInstance.Cancel() })

	// Run the check
	err = checkInstance.Run()
	require.NoError(t, err, "Check.Run() should not return an error")

	// Collect all metrics that were sent
	calls := mockSender.Calls
	metricsByName := make(map[string][]mock.Call)
	for _, call := range calls {
		if call.Method == "GaugeWithTimestamp" || call.Method == "CountWithTimestamp" {
			metricName := call.Arguments[0].(string)
			metricsByName[metricName] = append(metricsByName[metricName], call)
		}
	}

	// Define test cases
	maxPCIeThroughput := 64 * 1024 * 1024 * 1024.0
	testCases := []metricTestCase{
		{
			name:     "gpu.sm_active",
			interval: &valueInterval{min: 0.0, max: 100.0},
		},
		{
			name: "gpu.core.limit",
			getExpectedValue: func(d *safenvml.DeviceInfo) float64 {
				return float64(d.CoreCount)
			},
		},
		{
			name: "gpu.memory.limit",
			getExpectedValue: func(d *safenvml.DeviceInfo) float64 {
				return float64(d.Memory)
			},
		},
		{
			name:     "gpu.temperature",
			interval: &valueInterval{min: 0.0, max: 100.0},
		},
		{
			name:     "gpu.power.usage",
			interval: &valueInterval{min: 0.0, max: 1000000.0}, // 0-1000W in milliwatts
		},
		{
			name:     "gpu.pci.throughput.tx",
			interval: &valueInterval{min: 0.0, max: maxPCIeThroughput},
		},
		{
			name:     "gpu.pci.throughput.rx",
			interval: &valueInterval{min: 0.0, max: maxPCIeThroughput},
		},
	}

	// Run all test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assertMetricCase(t, metricsByName, tc, cache)
		})
	}
}
