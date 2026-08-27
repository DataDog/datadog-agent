// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mockcontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

const (
	smActiveDelta               = 15.0
	gpuBurnerCollectionPasses   = 3
	gpuBurnerCollectionInterval = 5 * time.Second
)

func collectGPUBurnerMetrics(t *testing.T, passes int, interval time.Duration) map[string]map[string][]gpuspec.MetricObservation {
	t.Helper()
	require.Positive(t, passes)
	require.Positive(t, interval)

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
	require.NoError(t, checkInstance.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))
	t.Cleanup(checkInstance.Cancel)

	// Run once to initialize rate-derived collectors, then collect multiple
	// intervals while the burner is active. Keep every observation so callers
	// can validate how metric values evolve across the collection window.
	require.NoError(t, checkInstance.Run())
	for range passes {
		time.Sleep(interval)
		require.NoError(t, checkInstance.Run())
	}

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

func TestGPUBurnerSingleGPUDeviceSelection(t *testing.T) {
	testutil.RequireGPU(t)
	testutil.RequireSmi(t)
	env.SetFeatures(t, env.KubernetesDevicePlugins, env.NVML)

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)
	count, err := lib.DeviceGetCount()
	require.NoError(t, err)
	require.Positive(t, count)

	indices := []int{0}
	if count > 1 {
		indices = append(indices, 1)
	}
	for _, index := range indices {
		t.Run(fmt.Sprintf("one-gpu-%d", index), func(t *testing.T) {
			burner := StartGPUBurner(t, strconv.Itoa(index), 1, 100)
			assertBurnerDevicesActive(t, burner, 1, 100)
		})
	}
}

func TestGPUBurnerTwoGPUDeviceSelection(t *testing.T) {
	testutil.RequireGPU(t)
	testutil.RequireSmi(t)
	env.SetFeatures(t, env.KubernetesDevicePlugins, env.NVML)

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)
	count, err := lib.DeviceGetCount()
	require.NoError(t, err)
	if count < 2 {
		t.Skip("two-GPU device-selection tests require at least two physical GPUs")
	}
	deviceSets := [][]int{{0, 1}}
	if count >= 4 {
		deviceSets = append(deviceSets, []int{2, 3})
	}
	for _, devices := range deviceSets {
		t.Run(fmt.Sprintf("two-gpus-%d-%d", devices[0], devices[1]), func(t *testing.T) {
			burner := StartGPUBurner(t, fmt.Sprintf("%d,%d", devices[0], devices[1]), 2, 100)
			assertBurnerDevicesActive(t, burner, 2, 100)
		})
	}
}

func assertBurnerDevicesActive(t *testing.T, burner *GPUBurner, expectedWorkers int, targetSM float64) {
	t.Helper()

	metricsByUUID := collectGPUBurnerMetrics(t, gpuBurnerCollectionPasses, gpuBurnerCollectionInterval)
	status, err := burner.Status(t.Context())
	require.NoError(t, err)
	require.Len(t, status.Workers, expectedWorkers)
	for _, worker := range status.Workers {
		deviceMetrics := metricsByUUID[strings.ToLower(worker.GPUUUID)]
		require.NotEmpty(t, deviceMetrics, "no metrics emitted for gpu-burner worker GPU %s", worker.GPUUUID)
		require.NotEmpty(t, deviceMetrics["sm_active"], "sm_active was not emitted for gpu-burner worker GPU %s", worker.GPUUUID)
	}

	type smiResult struct {
		sample *testutil.SmiSample
		err    error
	}
	smiResults := make([]smiResult, len(status.Workers))
	var smiWG sync.WaitGroup
	for i, worker := range status.Workers {
		smiWG.Add(1)
		go func(i int, uuid string) {
			defer smiWG.Done()
			sample, err := testutil.CollectSmiSample(uuid)
			smiResults[i] = smiResult{sample: sample, err: err}
		}(i, worker.GPUUUID)
	}
	smiWG.Wait()
	for i, worker := range status.Workers {
		require.NotNil(t, worker.Metrics)
		deviceMetrics := metricsByUUID[strings.ToLower(worker.GPUUUID)]
		require.InDelta(t, targetSM, worker.Metrics.SMActive, smActiveDelta, "gpu-burner status SM activity differs from target")
		requireMetricNearValue(t, deviceMetrics, "sm_active", targetSM, smActiveDelta)
		require.NoError(t, smiResults[i].err, "collect nvidia-smi sample for GPU %s", worker.GPUUUID)
		requireMetricsMatchSmi(t, deviceMetrics, smiResults[i].sample)
	}
}

func requireMetricsMatchSmi(t *testing.T, deviceMetrics map[string][]gpuspec.MetricObservation, sample *testutil.SmiSample) {
	t.Helper()

	requireLatestMetricNearSmi(t, deviceMetrics, "sm_active", sample.SMUtilPct, 1, smActiveDelta)
	requireLatestMetricNearSmi(t, deviceMetrics, "temperature", sample.GPUTempC, 1, 5)
	requireLatestMetricNearSmi(t, deviceMetrics, "encoder_active", sample.EncoderPct, 1, 5)
	requireLatestMetricNearSmi(t, deviceMetrics, "decoder_active", sample.DecoderPct, 1, 5)
	requireLatestMetricNearSmi(t, deviceMetrics, "clock.speed.memory", sample.MemClockMHz, 1, 25)
	if sample.MemTempC != nil {
		requireLatestMetricNearSmi(t, deviceMetrics, "memory.temperature", sample.MemTempC, 1, 5)
	}
}

func requireMetricNearValue(t *testing.T, deviceMetrics map[string][]gpuspec.MetricObservation, name string, expected, delta float64) {
	t.Helper()

	observations := deviceMetrics[name]
	require.NotEmpty(t, observations, "%s was not emitted", name)
	latest := observations[len(observations)-1]
	require.NotNil(t, latest.Value, "%s was emitted without a value", name)
	require.InDelta(t, expected, *latest.Value, delta, "%s differs from gpu-burner status value", name)
}

func requireLatestMetricNearSmi(t *testing.T, deviceMetrics map[string][]gpuspec.MetricObservation, name string, smiValue *float64, scale, delta float64) {
	t.Helper()

	observations := deviceMetrics[name]
	require.NotEmpty(t, observations, "%s was not emitted for this device", name)
	latest := observations[len(observations)-1]
	require.NotNil(t, latest.Value, "%s was emitted without a value", name)
	require.NotNil(t, smiValue, "nvidia-smi value was blank for %s", name)
	require.InDelta(t, *smiValue*scale, *latest.Value, delta, "%s value %v differs from nvidia-smi reading %v", name, *latest.Value, *smiValue*scale)
}
