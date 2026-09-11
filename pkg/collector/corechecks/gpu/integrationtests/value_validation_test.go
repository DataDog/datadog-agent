// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mockcontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

type nvidiaSmiCollection struct {
	wg      sync.WaitGroup
	mu      sync.Mutex
	results map[string]smiResult
}

type smiResult struct {
	sample *testutil.SmiSample
	err    error
}

type checkCollectionOptions struct {
	passes           int
	interval         time.Duration
	smiOptionsByUUID map[string][]testutil.SmiCollectionOption
	injectXIDDevices []safenvml.Device
}

func startNvidiaSmiCollection(optionsByUUID map[string][]testutil.SmiCollectionOption) *nvidiaSmiCollection {
	collection := &nvidiaSmiCollection{
		results: make(map[string]smiResult, len(optionsByUUID)),
	}

	// Each per-device dmon command takes about three seconds. Run them
	// concurrently so every SMI sample overlaps the same Agent check window.
	for uuid, options := range optionsByUUID {
		collection.wg.Add(1)
		go func() {
			defer collection.wg.Done()
			sample, err := testutil.CollectSmiSample(uuid, options...)
			collection.mu.Lock()
			collection.results[strings.ToLower(uuid)] = smiResult{sample: sample, err: err}
			collection.mu.Unlock()
		}()
	}
	return collection
}

func (c *nvidiaSmiCollection) wait(t *testing.T) map[string]*testutil.SmiSample {
	t.Helper()
	c.wg.Wait()

	samples := make(map[string]*testutil.SmiSample, len(c.results))
	for uuid, result := range c.results {
		require.NoError(t, result.err, "collect nvidia-smi sample for GPU %s", uuid)
		require.NotNil(t, result.sample, "no nvidia-smi sample for GPU %s", uuid)
		samples[uuid] = result.sample
	}
	return samples
}

func setupGPUCheck(t *testing.T) (*gpu.Check, *mocksender.MockSender) {
	t.Helper()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	gpu.SetupWorkloadmetaGPUs(t, wmetaMock, fakeTagger, gpuspec.DeviceModePhysical, false)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	checkInstance := gpu.NewCheck(fakeTagger, testutil.GetTelemetryMock(t), wmetaMock)
	mockSender := mocksender.NewMockSenderWithSenderManager(checkInstance.ID(), senderManager)
	mockSender.SetupAcceptAll()
	gpu.WithGPUConfigEnabled(t)

	checkInternal, ok := checkInstance.(*gpu.Check)
	require.True(t, ok)
	containerProvider := mockcontainers.NewMockContainerProvider(gomock.NewController(t))
	containerProvider.EXPECT().GetPidToCid(gomock.Any()).Return(map[int]string{}).AnyTimes()
	checkInternal.SetContainerProvider(containerProvider)
	require.NoError(t, checkInstance.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider"))
	t.Cleanup(checkInstance.Cancel)

	return checkInternal, mockSender
}

func collectCheckAndNvidiaSmiMetrics(t *testing.T, options checkCollectionOptions) (map[string]map[string][]gpuspec.MetricObservation, map[string]*testutil.SmiSample) {
	t.Helper()
	require.Positive(t, options.passes)
	require.Positive(t, options.interval)
	require.NotEmpty(t, options.smiOptionsByUUID)

	checkInternal, mockSender := setupGPUCheck(t)
	require.NoError(t, checkInternal.Run(), "initial Check.Run() should not return an error")

	for _, device := range options.injectXIDDevices {
		deviceUUID := device.GetDeviceInfo().UUID
		require.NoError(t, checkInternal.InjectXIDEventsForTest(deviceUUID, []safenvml.DeviceEventData{{
			DeviceUUID: deviceUUID,
			EventType:  nvml.EventTypeXidCriticalError,
			EventData:  31,
		}}))
	}

	var smiCollection *nvidiaSmiCollection
	for pass := range options.passes {
		if pass == options.passes-1 {
			// Compare SMI with the final Agent interval and return only that
			// interval's metrics. Earlier runs initialize rate collectors.
			smiCollection = startNvidiaSmiCollection(options.smiOptionsByUUID)
			mockSender.ResetCalls()
		}
		time.Sleep(options.interval)
		require.NoError(t, checkInternal.Run(), "Check.Run() pass %d should not return an error", pass+1)
	}
	require.NotNil(t, smiCollection)

	metricsByUUID := emittedMetricsByUUID(mockSender)
	require.NotEmpty(t, metricsByUUID)
	return metricsByUUID, smiCollection.wait(t)
}

func emittedMetricsByUUID(mockSender *mocksender.MockSender) map[string]map[string][]gpuspec.MetricObservation {
	metricsByUUID := make(map[string]map[string][]gpuspec.MetricObservation)
	for metricName, observations := range gpu.GetEmittedGPUMetrics(mockSender) {
		for _, observation := range observations {
			uuids := gpuspec.TagsToKeyValues(observation.Tags)["gpu_uuid"]
			if len(uuids) == 0 {
				continue
			}
			uuid := strings.ToLower(uuids[0])
			if metricsByUUID[uuid] == nil {
				metricsByUUID[uuid] = make(map[string][]gpuspec.MetricObservation)
			}
			metricsByUUID[uuid][metricName] = append(metricsByUUID[uuid][metricName], observation)
		}
	}
	return metricsByUUID
}

func validateCollectedMetrics(t *testing.T, specs *gpuspec.Specs, uuids []string, configsByUUID map[string]gpuspec.GPUConfig, metricsByUUID map[string]map[string][]gpuspec.MetricObservation, smiSamples map[string]*testutil.SmiSample, calibratedValuesByUUID map[string]map[string]*float64, baseOptions gpuspec.ValidationOptions) {
	t.Helper()

	for _, deviceUUID := range uuids {
		uuid := strings.ToLower(deviceUUID)
		t.Run("gpu="+uuid, func(t *testing.T) {
			config, found := configsByUUID[uuid]
			require.True(t, found, "could not find GPU config for %s", uuid)
			deviceMetrics := metricsByUUID[uuid]
			require.NotEmpty(t, deviceMetrics, "no metrics emitted for GPU %s", uuid)
			smiSample := smiSamples[uuid]
			require.NotNil(t, smiSample, "no nvidia-smi sample for GPU %s", uuid)

			options := baseOptions
			options.NvidiaSMIValues = smiSample.MetricValues()
			if calibratedValuesByUUID != nil {
				options.CalibratedWorkloadValues = calibratedValuesByUUID[uuid]
			}
			gpu.ValidateEmittedMetricsAgainstSpec(t, specs, config, deviceMetrics, nil, options)
		})
	}
}

func gpuConfigForPhysicalDevice(t *testing.T, specs *gpuspec.Specs, device safenvml.Device) gpuspec.GPUConfig {
	t.Helper()

	deviceInfo := device.GetDeviceInfo()
	architecture := gpuutil.ArchToString(deviceInfo.Architecture)
	require.NotContains(t, []string{"unknown", "invalid"}, architecture, "unsupported architecture enum %v", deviceInfo.Architecture)

	archSpec, ok := specs.Architectures.Architectures[architecture]
	require.True(t, ok, "architecture %s missing from architectures spec", architecture)
	require.True(t, gpuspec.IsModeSupportedByArchitecture(archSpec, gpuspec.DeviceModePhysical), "physical mode should be supported for architecture %s", architecture)

	capabilities := archSpec.EffectiveCapabilities(gpuspec.DeviceModePhysical)
	capabilities.NVLink = archSpec.SupportedNVLinkGeneration()
	if linkCount(t, device, "C2C", nvidia.GetC2CLinkCount) == 0 {
		capabilities.C2C = false
	}

	return gpuspec.GPUConfig{
		Architecture:    architecture,
		DeviceMode:      gpuspec.DeviceModePhysical,
		Capabilities:    capabilities,
		NVLinkLinkCount: deviceInfo.NVLinkLinkCount,
	}
}

func requiresGPM(architecture nvml.DeviceArchitecture) bool {
	return architecture >= nvml.DEVICE_ARCH_HOPPER
}

func physicalDeviceConfigs(t *testing.T, specs *gpuspec.Specs, devices []safenvml.Device) (map[string]gpuspec.GPUConfig, map[string][]testutil.SmiCollectionOption) {
	t.Helper()

	configsByUUID := make(map[string]gpuspec.GPUConfig, len(devices))
	smiOptionsByUUID := make(map[string][]testutil.SmiCollectionOption, len(devices))
	for _, device := range devices {
		uuid := strings.ToLower(device.GetDeviceInfo().UUID)
		deviceID, err := device.GetUUID()
		require.NoError(t, err, "get UUID for GPU %s", uuid)
		configsByUUID[uuid] = gpuConfigForPhysicalDevice(t, specs, device)
		smiOptionsByUUID[deviceID] = nil
		architecture, err := device.GetArchitecture()
		require.NoError(t, err, "get architecture for GPU %s", uuid)
		if requiresGPM(architecture) {
			smiOptionsByUUID[deviceID] = []testutil.SmiCollectionOption{testutil.WithGPM()}
		}
	}

	return configsByUUID, smiOptionsByUUID
}
