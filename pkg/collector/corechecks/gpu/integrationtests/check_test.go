// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

type CheckTestSuite struct {
	suite.Suite
	devices    []safenvml.Device
	metrics    map[string]map[string][]gpuspec.MetricObservation
	smiSamples map[string]*testutil.SmiSample
}

func (suite *CheckTestSuite) SetupTest() {
	t := suite.T()

	testutil.RequireGPU(t)
	testutil.RequireSmi(t)
	env.SetFeatures(t, env.KubernetesDevicePlugins, env.NVML)

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NoError(t, cache.Refresh())

	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)
	suite.devices = devices

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
	checkInternal.SetContainerProvider(mock_containers.NewMockContainerProvider(gomock.NewController(t)))

	err = checkInstance.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider")
	require.NoError(t, err)
	t.Cleanup(func() { checkInstance.Cancel() })

	// Collect an nvidia-smi sample for each device concurrently with the
	// collector runs below, so both observe the (idle) device over the same
	// wall-clock window. nvidia-smi dmon takes ~3 seconds (three cycles), which
	// overlaps the two check runs and the 1s sleep between them.
	type smiResult struct {
		uuid   string
		sample *testutil.SmiSample
		err    error
	}
	smiResults := make([]smiResult, len(devices))
	var smiWG sync.WaitGroup
	for i, device := range devices {
		deviceID, err := device.GetUUID()
		require.NoError(t, err, "could not get the device ID")
		uuid := strings.ToLower(device.GetDeviceInfo().UUID)

		smiWG.Add(1)
		go func(i int, deviceID, uuid string) {
			defer smiWG.Done()
			sample, err := testutil.CollectSmiSample(deviceID)
			smiResults[i] = smiResult{uuid: uuid, sample: sample, err: err}
		}(i, deviceID, uuid)
	}

	err = checkInstance.Run()
	require.NoError(t, err, "Check.Run() should not return an error")

	// Inject XID events for each device to ensure the errors.xid.total metric is emitted.
	for _, device := range devices {
		deviceUUID := device.GetDeviceInfo().UUID
		require.NoError(t, checkInternal.InjectXIDEventsForTest(deviceUUID, []safenvml.DeviceEventData{{
			DeviceUUID: deviceUUID,
			EventType:  nvml.EventTypeXidCriticalError,
			EventData:  31,
		}}))
	}

	// Run the check a second time so rate-derived field metrics such as NVLink
	// throughput have a previous sample to compare against and can be emitted.
	// We need a one second interval before the second call to ensure new
	// metrics are available for the sample.
	time.Sleep(1 * time.Second)
	mockSender.ResetCalls()
	err = checkInstance.Run()
	require.NoError(t, err, "Second Check.Run() should not return an error")

	// Wait for the concurrent nvidia-smi collection to finish and assert its results on the test goroutine.
	smiWG.Wait()
	suite.smiSamples = make(map[string]*testutil.SmiSample, len(smiResults))
	for _, res := range smiResults {
		require.NoError(t, res.err, "could not collect nvidia-smi sample for GPU %s", res.uuid)
		suite.smiSamples[res.uuid] = res.sample
	}

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
	suite.metrics = metricsByUUID
}

func (suite *CheckTestSuite) TestCheckRunMatchesSpecForPhysicalDevices() {
	t := suite.T()

	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)

	for _, device := range suite.devices {
		deviceInfo := device.GetDeviceInfo()
		deviceUUID := strings.ToLower(deviceInfo.UUID)
		archName := gpuutil.ArchToString(deviceInfo.Architecture)
		if archName == "unknown" || archName == "invalid" {
			t.Logf("Skipping GPU %s with unsupported architecture enum %v", deviceUUID, deviceInfo.Architecture)
			continue
		}

		archSpec, ok := specs.Architectures.Architectures[archName]
		require.True(t, ok, "architecture %s missing from architectures spec", archName)
		require.True(t, gpuspec.IsModeSupportedByArchitecture(archSpec, gpuspec.DeviceModePhysical), "physical mode should be supported for architecture %s", archName)

		deviceMetrics := suite.metrics[deviceUUID]
		require.NotEmpty(t, deviceMetrics, "expected emitted metrics for GPU %s", deviceUUID)

		capabilities := archSpec.EffectiveCapabilities(gpuspec.DeviceModePhysical)
		capabilities.NVLink = archSpec.SupportedNVLinkGeneration()
		nvlinkLinkCount := linkCount(t, device, "NVLink", nvidia.GetNVLinkCount)
		if linkCount(t, device, "C2C", nvidia.GetC2CLinkCount) == 0 {
			capabilities.C2C = false
		}
		gpuConfig := gpuspec.GPUConfig{Architecture: archName, DeviceMode: gpuspec.DeviceModePhysical, Capabilities: capabilities, NVLinkLinkCount: nvlinkLinkCount}
		validationOptions := gpuspec.ValidationOptions{
			WorkloadActive: false,
			IgnoreMetrics:  map[string]bool{"fan_speed": true, "memory.temperature": true}, // not all devices have fans or memory temperature sensors
		}
		t.Run("gpu="+deviceUUID, func(t *testing.T) {
			gpu.ValidateEmittedMetricsAgainstSpec(t, specs, gpuConfig, deviceMetrics, nil, validationOptions)
		})
	}
}

func (suite *CheckTestSuite) TestCheckMetricValuesMatchSmi() {
	t := suite.T()

	for _, device := range suite.devices {
		arch, err := device.GetArchitecture()
		require.NoError(t, err, "could not get device architecture")

		deviceInfo := device.GetDeviceInfo()
		deviceUUID := strings.ToLower(deviceInfo.UUID)
		deviceMetrics := suite.metrics[deviceUUID]

		sample := suite.smiSamples[deviceUUID]
		require.NotNil(t, sample, "no nvidia-smi sample collected for GPU %s", deviceUUID)

		// Power is recored in watts from nvidia-smi, but milliwatts from our collector. We need to scale it up and also
		// give a larger margin of error to account for that.
		requireMetricNearSmi(t, deviceMetrics, "power.usage", sample.PowerWatts, 1000, 1000)
		requireMetricNearSmi(t, deviceMetrics, "temperature", sample.GPUTempC, 1, 5)
		requireMetricNearSmi(t, deviceMetrics, "encoder_active", sample.EncoderPct, 1, 5)
		requireMetricNearSmi(t, deviceMetrics, "decoder_active", sample.DecoderPct, 1, 5)
		requireMetricNearSmi(t, deviceMetrics, "clock.speed.memory", sample.MemClockMHz, 1, 5)
		requireMetricNearSmi(t, deviceMetrics, "clock.speed.graphics", sample.ProcClockMHz, 1, 5)

		// Not all GPUs have a memory temporature sensors.
		if sample.MemTempC != nil {
			requireMetricNearSmi(t, deviceMetrics, "memory.temperature", sample.MemTempC, 1, 5)
		}

		// Skip gpm metrics on older architectures.
		if arch < nvml.DEVICE_ARCH_HOPPER {
			continue
		}

		// GPM metrics (nvidia-smi --gpm-metrics) are only available on Hopper and newer GPUs.
		requireMetricNearSmi(t, deviceMetrics, "gr_engine_active", sample.GraphicsActivity, 1, 5)
	}
}

func TestCheckTestSuite(t *testing.T) {
	suite.Run(t, new(CheckTestSuite))
}

func linkCount(t *testing.T, device safenvml.Device, name string, countFunc func(safenvml.Device) (int, error)) int {
	t.Helper()

	count, err := countFunc(device)
	if err != nil {
		if safenvml.IsAPIUnsupportedOnDevice(err, device) {
			return 0
		}
		require.NoError(t, err, "%s link count probe failed for GPU %s", name, device.GetDeviceInfo().UUID)
		return 0
	}
	return count
}

func getFloatValue(t *testing.T, in *float64) float64 {
	t.Helper()
	require.NotNil(t, in, "float value does not exist")
	return *in
}

// requireMetricNearSmi asserts that the first emitted sample of the named
// metric is within delta of the nvidia-smi reading, after applying scale to
// convert the nvidia-smi units into the agent's units (e.g. watts to
// milliwatts).
func requireMetricNearSmi(t *testing.T, deviceMetrics map[string][]gpuspec.MetricObservation, name string, smiValue *float64, scale, delta float64) {
	t.Helper()

	observations, ok := deviceMetrics[name]
	require.True(t, ok, "could not find %s for device", name)
	require.GreaterOrEqual(t, len(observations), 1, "%s was not emitted for this device", name)
	actual := getFloatValue(t, observations[0].Value)

	require.NotNil(t, smiValue, "nividia-smi value was blank for %s", name)
	expected := *smiValue * scale

	t.Logf("checking %s with value %f against nividia-smi value %f", name, actual, expected)
	require.InDelta(t, expected, actual, delta, "%s value %v differs from nvidia-smi reading %v", name, actual, expected)
}
