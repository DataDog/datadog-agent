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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

const (
	smActiveDelta               = 15.0
	gpuBurnerCollectionPasses   = 3
	gpuBurnerCollectionInterval = 5 * time.Second
)

func TestGPUBurnerSingleGPUDeviceSelection(t *testing.T) {
	testutil.RequireGPU(t)
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
	expectedUUIDsByIndex := make(map[int][]string, len(indices))
	for _, index := range indices {
		expectedUUIDsByIndex[index] = gpuUUIDsForIndices(t, lib, []int{index})
	}
	for _, index := range indices {
		t.Run(fmt.Sprintf("one-gpu-%d", index), func(t *testing.T) {
			burner := StartGPUBurner(t, strconv.Itoa(index), 1, 100)
			assertBurnerDeviceSelection(t, burner, expectedUUIDsByIndex[index])
		})
	}
}

func TestGPUBurnerTwoGPUDeviceSelection(t *testing.T) {
	testutil.RequireGPU(t)
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
	expectedUUIDsBySet := make([][]string, len(deviceSets))
	for i, devices := range deviceSets {
		expectedUUIDsBySet[i] = gpuUUIDsForIndices(t, lib, devices)
	}
	for i, devices := range deviceSets {
		t.Run(fmt.Sprintf("two-gpus-%d-%d", devices[0], devices[1]), func(t *testing.T) {
			burner := StartGPUBurner(t, fmt.Sprintf("%d,%d", devices[0], devices[1]), 2, 100)
			assertBurnerDeviceSelection(t, burner, expectedUUIDsBySet[i])
		})
	}
}

func gpuUUIDsForIndices(t *testing.T, lib safenvml.SafeNVML, indices []int) []string {
	t.Helper()

	uuids := make([]string, 0, len(indices))
	for _, index := range indices {
		device, err := lib.DeviceGetHandleByIndex(index)
		require.NoError(t, err, "get NVML device handle for index %d", index)
		uuid, err := device.GetUUID()
		require.NoError(t, err, "get NVML device UUID for index %d", index)
		uuids = append(uuids, uuid)
	}
	return uuids
}

func assertBurnerDeviceSelection(t *testing.T, burner *GPUBurner, expectedUUIDs []string) *GPUBurnerStatus {
	t.Helper()

	status, err := burner.Status(t.Context())
	require.NoError(t, err)
	require.Len(t, status.Workers, len(expectedUUIDs))
	actualUUIDs := make([]string, 0, len(status.Workers))
	for _, worker := range status.Workers {
		actualUUIDs = append(actualUUIDs, strings.ToLower(worker.GPUUUID))
	}
	expectedUUIDKeys := make([]string, 0, len(expectedUUIDs))
	for _, uuid := range expectedUUIDs {
		expectedUUIDKeys = append(expectedUUIDKeys, strings.ToLower(uuid))
	}
	require.ElementsMatch(t, expectedUUIDKeys, actualUUIDs, "gpu-burner workers do not match the selected CUDA-visible devices")

	return &status
}

func startCalibratedWorkload(t *testing.T, lib safenvml.SafeNVML, indices []int, targetSM float64) map[string]map[string]*float64 {
	t.Helper()

	expectedUUIDs := gpuUUIDsForIndices(t, lib, indices)
	visibleDevices := make([]string, len(indices))
	for i, index := range indices {
		visibleDevices[i] = strconv.Itoa(index)
	}
	burner := StartGPUBurner(t, strings.Join(visibleDevices, ","), len(indices), int(targetSM))
	status := assertBurnerDeviceSelection(t, burner, expectedUUIDs)

	valuesByUUID := make(map[string]map[string]*float64, len(status.Workers))
	for _, worker := range status.Workers {
		uuid := strings.ToLower(worker.GPUUUID)
		require.NotNil(t, worker.Metrics)
		require.NotNil(t, worker.Metrics.SMActive, "gpu-burner did not report SM activity for GPU %s", worker.GPUUUID)
		require.InDelta(t, targetSM, *worker.Metrics.SMActive, smActiveDelta, "gpu-burner status SM activity differs from target")
		valuesByUUID[uuid] = worker.Metrics.MetricValues()
	}
	return valuesByUUID
}
