// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
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

func TestCheckRunMatchesSpecForPhysicalDevices(t *testing.T) {
	testutil.RequireGPU(t)
	env.SetFeatures(t, env.NVML)

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

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	gpu.SetupWorkloadmetaGPUs(t, wmetaMock, fakeTagger, gpuspec.DeviceModePhysical, false)

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

	// Run the check a second time so rate-derived field metrics such as NVLink
	// throughput have a previous sample to compare against and can be emitted.
	mockSender.ResetCalls()
	err = checkInstance.Run()
	require.NoError(t, err, "Second Check.Run() should not return an error")

	metricsByName := gpu.GetEmittedGPUMetrics(mockSender)
	require.NotEmpty(t, metricsByName)

	metricsByUUID := make(map[string]map[string][]gpuspec.MetricObservation, len(devices))
	for metricName, emittedSamples := range metricsByName {
		for _, sample := range emittedSamples {
			uuids := gpuspec.TagsToKeyValues(sample.Tags)["gpu_uuid"]
			if len(uuids) == 0 {
				continue
			}
			deviceUUID := strings.ToLower(uuids[0])
			if metricsByUUID[deviceUUID] == nil {
				metricsByUUID[deviceUUID] = make(map[string][]gpuspec.MetricObservation)
			}

			metricsByUUID[deviceUUID][metricName] = append(metricsByUUID[deviceUUID][metricName], sample)
		}
	}

	for _, device := range devices {
		deviceInfo := device.GetDeviceInfo()
		deviceUUID := strings.ToLower(deviceInfo.UUID)
		archName := gpuutil.ArchToString(deviceInfo.Architecture)
		if archName == "unknown" || archName == "invalid" {
			t.Logf("Skipping GPU %s with unsupported architecture enum %v", deviceUUID, deviceInfo.Architecture)
			continue
		}

		archSpec, ok := architecturesSpec.Architectures[archName]
		require.True(t, ok, "architecture %s missing from architectures spec", archName)
		require.True(t, gpuspec.IsModeSupportedByArchitecture(archSpec, gpuspec.DeviceModePhysical), "physical mode should be supported for architecture %s", archName)

		deviceMetrics := metricsByUUID[deviceUUID]
		require.NotEmpty(t, deviceMetrics, "expected emitted metrics for GPU %s", deviceUUID)

		gpuConfig := gpuspec.GPUConfig{Architecture: archName, DeviceMode: gpuspec.DeviceModePhysical}
		t.Run("gpu="+deviceUUID, func(t *testing.T) {
			gpu.ValidateEmittedMetricsAgainstSpec(t, metricsSpec, gpuConfig, false, deviceMetrics, nil)
		})
	}
}
