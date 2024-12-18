// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestFilterDevicesForContainer(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot(), wmetaMock)
	require.NotNil(t, sysCtx)
	require.NoError(t, err)

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
		AllocatedResources: []workloadmeta.ContainerAllocatedResource{
			{
				Name: nvidiaResourceName,
				ID:   gpuUUID,
			},
		},
	}

	wmetaMock.Set(container)
	storeContainer, err := wmetaMock.GetContainer(containerID)
	require.NoError(t, err, "container should be found in the store")
	require.NotNil(t, storeContainer, "container should be found in the store")

	t.Run("NoContainer", func(t *testing.T) {
		filtered, err := sysCtx.filterDevicesForContainer(sysCtx.gpuDevices, "")
		require.NoError(t, err)
		testutil.RequireDeviceListsEqual(t, filtered, sysCtx.gpuDevices) // With no container, all devices should be returned
	})

	t.Run("NonExistentContainer", func(t *testing.T) {
		filtered, err := sysCtx.filterDevicesForContainer(sysCtx.gpuDevices, "non-existent-at-all")
		require.NoError(t, err)
		testutil.RequireDeviceListsEqual(t, filtered, sysCtx.gpuDevices) // If we can't find the container, all devices should be returned
	})

	t.Run("ContainerWithGPU", func(t *testing.T) {
		filtered, err := sysCtx.filterDevicesForContainer(sysCtx.gpuDevices, containerID)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		testutil.RequireDeviceListsEqual(t, filtered, sysCtx.gpuDevices[deviceIndex:deviceIndex+1])
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

	wmetaMock := testutil.GetWorkloadMetaMock(t)
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), procFs, wmetaMock)
	require.NotNil(t, sysCtx)
	require.NoError(t, err)

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
			Name: nvidiaResourceName,
			ID:   gpuUUID,
		}
		container.AllocatedResources = append(container.AllocatedResources, resource)
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i, idx := range c.configuredDeviceIdx {
				sysCtx.setDeviceSelection(c.pid, c.pid+i, idx)
			}

			for i, idx := range c.expectedDeviceIdx {
				activeDevice, err := sysCtx.getCurrentActiveGpuDevice(c.pid, c.pid+i, c.containerID)
				require.NoError(t, err)
				testutil.RequireDevicesEqual(t, sysCtx.gpuDevices[idx], activeDevice, "invalid device at index %d (real index is %d, selected index is %d)", i, idx, c.configuredDeviceIdx[i])
			}
		})
	}
}
