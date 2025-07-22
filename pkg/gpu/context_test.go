// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func getTestSystemContext(t *testing.T, extraOpts ...systemContextOption) *systemContext {
	opts := []systemContextOption{
		withProcRoot(kernel.ProcFSRoot()),
		withWorkloadMeta(testutil.GetWorkloadMetaMock(t)),
		withTelemetry(testutil.GetTelemetryMock(t)),
	}

	opts = append(opts, extraOpts...) // Allow overriding the default options

	sysCtx, err := getSystemContext(opts...)
	require.NoError(t, err)
	require.NotNil(t, sysCtx)
	return sysCtx
}

func TestFilterDevicesForContainer(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	sysCtx := getTestSystemContext(t, withWorkloadMeta(wmetaMock))

	// Create a container with a single GPU and add it to the store
	containerID := "abcdef"
	deviceIndex := 2
	gpuUUID := testutil.GPUUUIDs[deviceIndex]
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerID,
		},
		ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
			{
				Name: string(gpuutil.GpuNvidiaGeneric),
				ID:   gpuUUID,
			},
		},
	}

	containerIDNoGpu := "abcdef2"
	containerNoGpu := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerIDNoGpu,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerIDNoGpu,
		},
		ResolvedAllocatedResources: nil,
	}

	containerIDGke := "gke-1234567890"
	containerGke := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerIDGke,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerIDGke,
		},
		ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
			{
				Name: string(gpuutil.GpuNvidiaGeneric),
				ID:   "nvidia0",
			},
		},
	}

	wmetaMock.Set(container)
	storeContainer, err := wmetaMock.GetContainer(containerID)
	require.NoError(t, err, "container should be found in the store")
	require.NotNil(t, storeContainer, "container should be found in the store")

	wmetaMock.Set(containerNoGpu)
	storeContainer, err = wmetaMock.GetContainer(containerIDNoGpu)
	require.NoError(t, err, "container should be found in the store")
	require.NotNil(t, storeContainer, "container should be found in the store")

	wmetaMock.Set(containerGke)
	storeContainer, err = wmetaMock.GetContainer(containerIDGke)
	require.NoError(t, err, "container should be found in the store")
	require.NotNil(t, storeContainer, "container should be found in the store")

	t.Run("NoContainer", func(t *testing.T) {
		filtered, err := sysCtx.filterDevicesForContainer(sysCtx.deviceCache.All(), "")
		require.NoError(t, err)
		nvmltestutil.RequireDeviceListsEqual(t, filtered, sysCtx.deviceCache.All()) // With no container, all devices should be returned
	})

	t.Run("NonExistentContainer", func(t *testing.T) {
		filtered, err := sysCtx.filterDevicesForContainer(sysCtx.deviceCache.All(), "non-existent-at-all")
		require.NoError(t, err)
		nvmltestutil.RequireDeviceListsEqual(t, filtered, sysCtx.deviceCache.All()) // If we can't find the container, all devices should be returned
	})

	t.Run("ContainerWithGPU", func(t *testing.T) {
		filtered, err := sysCtx.filterDevicesForContainer(sysCtx.deviceCache.All(), containerID)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		nvmltestutil.RequireDeviceListsEqual(t, filtered, sysCtx.deviceCache.All()[deviceIndex:deviceIndex+1])
	})

	t.Run("ContainerWithNoGPUs", func(t *testing.T) {
		_, err := sysCtx.filterDevicesForContainer(sysCtx.deviceCache.All(), containerIDNoGpu)
		require.Error(t, err, "expected an error when filtering a container with no GPUs")
	})

	t.Run("ContainerWithNoGPUsButOnlyOneDeviceInSystem", func(t *testing.T) {
		sysDevices := sysCtx.deviceCache.All()[:1]
		filtered, err := sysCtx.filterDevicesForContainer(sysDevices, containerIDNoGpu)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		nvmltestutil.RequireDeviceListsEqual(t, filtered, sysDevices)
	})

	t.Run("ContainerWithGKEPlugin", func(t *testing.T) {
		filtered, err := sysCtx.filterDevicesForContainer(sysCtx.deviceCache.All(), containerIDGke)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		nvmltestutil.RequireDeviceListsEqual(t, filtered, sysCtx.deviceCache.All()[0:1])
	})
}

func TestGetCurrentActiveGpuDevice(t *testing.T) {
	pidNoContainer := 1234
	pidNoContainerButEnv := 2235
	pidContainer := 3238
	pidContainerAndEnv := 3239

	envVisibleDevices := []int32{1, 2, 3}
	envVisibleDevicesStr := make([]string, len(envVisibleDevices))
	for i, idx := range envVisibleDevices {
		envVisibleDevicesStr[i] = strconv.Itoa(int(idx))
	}
	envVisibleDevicesValue := strings.Join(envVisibleDevicesStr, ",")

	procFs := uprobes.CreateFakeProcFS(t, []uprobes.FakeProcFSEntry{
		{Pid: uint32(pidNoContainer)},
		{Pid: uint32(pidContainer)},
		{Pid: uint32(pidContainerAndEnv), Env: map[string]string{"CUDA_VISIBLE_DEVICES": envVisibleDevicesValue}},
		{Pid: uint32(pidNoContainerButEnv), Env: map[string]string{"CUDA_VISIBLE_DEVICES": envVisibleDevicesValue}},
	})

	// MIG makes the device selection more complex, so we disable it for these tests
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	sysCtx := getTestSystemContext(t, withProcRoot(procFs), withWorkloadMeta(wmetaMock))

	// Create a container with a single GPU and add it to the store
	containerID := "abcdef"
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerID,
		},
	}

	containerDeviceIndexes := []int32{1, 2, 3, 4}
	for _, idx := range containerDeviceIndexes {
		gpuUUID := testutil.GPUUUIDs[idx]
		resource := workloadmeta.ContainerAllocatedResource{
			Name: string(gpuutil.GpuNvidiaGeneric),
			ID:   gpuUUID,
		}
		container.ResolvedAllocatedResources = append(container.ResolvedAllocatedResources, resource)
	}

	wmetaMock.Set(container)
	storeContainer, err := wmetaMock.GetContainer(containerID)
	require.NoError(t, err, "container should be found in the store")
	require.NotNil(t, storeContainer, "container should be found in the store")

	cases := []struct {
		name                string
		pid                 int
		containerID         string
		configuredDeviceIdx []int32
		expectedDeviceIdx   []int32
		updatedEnvVar       string
	}{
		{
			name:                "NoContainer",
			containerID:         "",
			pid:                 pidNoContainer,
			configuredDeviceIdx: []int32{1, 2},
			expectedDeviceIdx:   []int32{1, 2},
		},
		{
			name:                "NoContainerButEnv",
			containerID:         "",
			pid:                 pidNoContainerButEnv,
			configuredDeviceIdx: []int32{1, 2},
			expectedDeviceIdx:   []int32{envVisibleDevices[1], envVisibleDevices[2]},
		},
		{
			name:                "WithContainer",
			containerID:         containerID,
			pid:                 pidContainer,
			configuredDeviceIdx: []int32{1, 2},
			expectedDeviceIdx:   []int32{containerDeviceIndexes[1], containerDeviceIndexes[2]},
		},
		{
			name:                "WithContainerAndEnv",
			pid:                 pidContainerAndEnv,
			containerID:         containerID,
			configuredDeviceIdx: []int32{1, 2},
			expectedDeviceIdx:   []int32{containerDeviceIndexes[envVisibleDevices[1]], containerDeviceIndexes[envVisibleDevices[2]]},
		},
		{
			name:                "NoContainerAndRuntimeEnvVar",
			pid:                 pidNoContainer,
			configuredDeviceIdx: []int32{0},
			expectedDeviceIdx:   []int32{1},
			updatedEnvVar:       "1",
		},
		{
			name:                "NoContainerAndRuntimeUpdatedEnvVar",
			pid:                 pidNoContainerButEnv,
			configuredDeviceIdx: []int32{0},
			expectedDeviceIdx:   []int32{1},
			updatedEnvVar:       "1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.updatedEnvVar != "" {
				sysCtx.setUpdatedVisibleDevicesEnvVar(c.pid, c.updatedEnvVar)
				require.NotContains(t, sysCtx.visibleDevicesCache, c.pid, "cache not invalidated for process %d", c.pid)
			}

			for i, idx := range c.configuredDeviceIdx {
				sysCtx.setDeviceSelection(c.pid, c.pid+i, idx)
			}

			for i, idx := range c.expectedDeviceIdx {
				activeDevice, err := sysCtx.getCurrentActiveGpuDevice(c.pid, c.pid+i, func() string { return c.containerID })
				require.NoError(t, err)
				nvmltestutil.RequireDevicesEqual(t, sysCtx.deviceCache.All()[idx], activeDevice, "invalid device at index %d (real index is %d, selected index is %d)", i, idx, c.configuredDeviceIdx[i])
			}

			// Note: we're explicitly not resetting the caches, as we want to test
			// whether the functions correctly invalidate the caches when the
			// environment variable is updated.
		})
	}
}
