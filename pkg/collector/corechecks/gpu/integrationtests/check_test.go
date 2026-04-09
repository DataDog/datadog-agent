// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
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

var expectedLiveDeviceMetrics = []string{
	"core.limit",
	"memory.limit",
	"sm_active",
	"temperature",
	"power.usage",
	"pci.throughput.rx",
	"pci.throughput.tx",
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

func gpuArchToSpecName(arch nvml.DeviceArchitecture) string {
	switch arch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "kepler"
	case nvml.DEVICE_ARCH_MAXWELL:
		return "maxwell"
	case nvml.DEVICE_ARCH_PASCAL:
		return "pascal"
	case nvml.DEVICE_ARCH_VOLTA:
		return "volta"
	case nvml.DEVICE_ARCH_TURING:
		return "turing"
	case nvml.DEVICE_ARCH_AMPERE:
		return "ampere"
	case nvml.DEVICE_ARCH_ADA:
		return "ada"
	case nvml.DEVICE_ARCH_HOPPER:
		return "hopper"
	case 10:
		return "blackwell"
	case nvml.DEVICE_ARCH_UNKNOWN:
		return "unknown"
	default:
		return "invalid"
	}
}

func seedPhysicalGPUEntities(t *testing.T, fakeTagger taggermock.Mock, wmetaMock workloadmetamock.Mock, devices []safenvml.Device, driverVersion string) {
	t.Helper()

	for _, device := range devices {
		deviceInfo := device.GetDeviceInfo()
		gpuEntity := &workloadmeta.GPU{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindGPU,
				ID:   deviceInfo.UUID,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name: deviceInfo.Name,
			},
			Vendor:             "nvidia",
			Device:             deviceInfo.Name,
			DriverVersion:      driverVersion,
			Index:              deviceInfo.Index,
			Architecture:       gpuArchToSpecName(deviceInfo.Architecture),
			TotalCores:         deviceInfo.CoreCount,
			TotalMemory:        deviceInfo.Memory,
			DeviceType:         workloadmeta.GPUDeviceTypePhysical,
			VirtualizationMode: "none",
		}
		wmetaMock.Set(gpuEntity)
		fakeTagger.SetTags(
			taggertypes.NewEntityID(taggertypes.GPU, deviceInfo.UUID),
			"integrationtests",
			[]string{
				"gpu_uuid:" + strings.ToLower(deviceInfo.UUID),
				"gpu_device:" + strings.ToLower(strings.ReplaceAll(deviceInfo.Name, " ", "_")),
				"gpu_vendor:nvidia",
				"gpu_driver_version:" + driverVersion,
			},
			nil,
			nil,
			nil,
		)
	}
}

// assertMetricCase validates a metric against its test case
func assertMetricCase(t *testing.T, metricsByName map[string][]mock.Call, tc metricTestCase, deviceCache safenvml.DeviceCache) {
	t.Helper()

	calls, ok := metricsByName[tc.name]
	require.True(t, ok, "%s metric should be present", tc.name)
	require.NotEmpty(t, calls, "No calls found for metric %s", tc.name)

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

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NoError(t, cache.Refresh())

	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	checkInstance := gpu.NewCheck(fakeTagger, testutil.GetTelemetryMock(t), wmetaMock)

	mockSender := mocksender.NewMockSenderWithSenderManager(checkInstance.ID(), senderManager)
	mockSender.SetupAcceptAll()

	gpu.WithGPUConfigEnabled(t)

	checkInternal, ok := checkInstance.(*gpu.Check)
	require.True(t, ok)
	checkInternal.SetContainerProvider(mock_containers.NewMockContainerProvider(gomock.NewController(t)))

	err = checkInstance.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider")
	require.NoError(t, err)
	t.Cleanup(func() { checkInstance.Cancel() })

	err = checkInstance.Run()
	require.NoError(t, err, "Check.Run() should not return an error")

	calls := mockSender.Calls
	metricsByName := make(map[string][]mock.Call)
	for _, call := range calls {
		if call.Method == "GaugeWithTimestamp" || call.Method == "CountWithTimestamp" {
			metricName := call.Arguments[0].(string)
			metricsByName[metricName] = append(metricsByName[metricName], call)
		}
	}

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
			interval: &valueInterval{min: 0.0, max: 1000000.0},
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assertMetricCase(t, metricsByName, tc, cache)
		})
	}
}

func TestCheckRunMatchesSpecForPhysicalDevices(t *testing.T) {
	testutil.RequireGPU(t)

	metricsSpec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)
	architecturesSpec, err := gpuspec.LoadArchitecturesSpec()
	require.NoError(t, err)

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NoError(t, cache.Refresh())

	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)

	driverVersion, err := lib.SystemGetDriverVersion()
	require.NoError(t, err)

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	seedPhysicalGPUEntities(t, fakeTagger, wmetaMock, devices, driverVersion)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	checkInstance := gpu.NewCheck(fakeTagger, testutil.GetTelemetryMock(t), wmetaMock)
	mockSender := mocksender.NewMockSenderWithSenderManager(checkInstance.ID(), senderManager)
	mockSender.SetupAcceptAll()

	gpu.WithGPUConfigEnabled(t)

	checkInternal, ok := checkInstance.(*gpu.Check)
	require.True(t, ok)
	checkInternal.SetContainerProvider(mock_containers.NewMockContainerProvider(gomock.NewController(t)))

	err = checkInstance.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider")
	require.NoError(t, err)
	t.Cleanup(func() { checkInstance.Cancel() })

	err = checkInstance.Run()
	require.NoError(t, err, "Check.Run() should not return an error")

	metricsByName := gpu.GetEmittedGPUMetrics(mockSender)
	require.NotEmpty(t, metricsByName)

	metricsByUUID := make(map[string]map[string][]gpuspec.EmittedMetric, len(devices))
	for metricName, emittedSamples := range metricsByName {
		for _, sample := range emittedSamples {
			deviceUUID := strings.ToLower(extractDeviceUUID(sample.Tags))
			if deviceUUID == "" {
				continue
			}
			if metricsByUUID[deviceUUID] == nil {
				metricsByUUID[deviceUUID] = make(map[string][]gpuspec.EmittedMetric)
			}

			metricsByUUID[deviceUUID][metricName] = append(metricsByUUID[deviceUUID][metricName], sample)
		}
	}

	for _, device := range devices {
		deviceInfo := device.GetDeviceInfo()
		deviceUUID := strings.ToLower(deviceInfo.UUID)
		archName := gpuArchToSpecName(deviceInfo.Architecture)
		if archName == "unknown" || archName == "invalid" {
			t.Logf("Skipping GPU %s with unsupported architecture enum %v", deviceUUID, deviceInfo.Architecture)
			continue
		}

		archSpec, ok := architecturesSpec.Architectures[archName]
		require.True(t, ok, "architecture %s missing from architectures spec", archName)
		require.True(t, gpuspec.IsModeSupportedByArchitecture(archSpec, gpuspec.DeviceModePhysical), "physical mode should be supported for architecture %s", archName)

		deviceMetrics := metricsByUUID[deviceUUID]
		require.NotEmpty(t, deviceMetrics, "expected emitted metrics for GPU %s", deviceUUID)

		t.Run("gpu="+deviceUUID, func(t *testing.T) {
			emittedNames := make([]string, 0, len(deviceMetrics))
			for metricName, emittedSamples := range deviceMetrics {
				emittedNames = append(emittedNames, metricName)

				metricSpec, ok := metricsSpec.Metrics[metricName]
				require.True(t, ok, "metric emitted by check is missing from spec: %s", metricName)
				require.True(t, metricSpec.SupportsArchitecture(archName), "metric %s emitted on unsupported architecture %s", metricName, archName)
				require.True(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModePhysical), "metric %s emitted in unsupported physical mode", metricName)
				gpuspec.ValidateMetricTagsAgainstSpec(t, metricsSpec, metricName, metricSpec, emittedSamples, nil)
			}

			slices.Sort(emittedNames)
			t.Logf("GPU %s (%s) emitted %d spec metrics", deviceUUID, archName, len(emittedNames))

			for _, metricName := range expectedLiveDeviceMetrics {
				metricSpec, ok := metricsSpec.Metrics[metricName]
				require.True(t, ok, "baseline metric %s missing from spec", metricName)
				require.True(t, metricSpec.SupportsArchitecture(archName), "baseline metric %s unsupported on architecture %s", metricName, archName)
				require.True(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModePhysical), "baseline metric %s unsupported in physical mode", metricName)
				require.Contains(t, deviceMetrics, metricName, "baseline device metric %s should be emitted for GPU %s on architecture %s", metricName, deviceUUID, archName)
			}
		})
	}
}
