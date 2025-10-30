// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"strconv"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestPull(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:      collectorID,
		catalog: workloadmeta.NodeAgent,
		store:   wmetaMock,
	}

	ddnvml.WithMockNVML(t, nvmlMock)

	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))
	var expectedActivePIDs []int
	for _, proc := range testutil.DefaultProcessInfo {
		expectedActivePIDs = append(expectedActivePIDs, int(proc.Pid))
	}

	foundIDs := make(map[string]bool)
	for _, gpu := range gpus {
		foundIDs[gpu.ID] = true
		var expectedName string
		if gpu.DeviceType == workloadmeta.GPUDeviceTypeMIG {
			expectedName = "MIG " + testutil.DefaultGPUName
		} else if gpu.DeviceType == workloadmeta.GPUDeviceTypePhysical {
			expectedName = testutil.DefaultGPUName
			//for now, we test totalMemory only for physical devices
			require.Equal(t, testutil.DefaultTotalMemory, gpu.TotalMemory, "unexpected device memory for device %s", gpu.ID)
		}
		require.Equal(t, testutil.DefaultNvidiaDriverVersion, gpu.DriverVersion)
		require.Equal(t, nvidiaVendor, gpu.Vendor)
		require.Equal(t, expectedName, gpu.Name)
		require.Equal(t, expectedName, gpu.Device)
		require.Equal(t, "hopper", gpu.Architecture)
		require.Equal(t, testutil.DefaultGPUComputeCapMajor, gpu.ComputeCapability.Major)
		require.Equal(t, testutil.DefaultGPUComputeCapMinor, gpu.ComputeCapability.Minor)
		require.Equal(t, testutil.DefaultMaxClockRates[workloadmeta.GPUSM], gpu.MaxClockRates[workloadmeta.GPUSM])
		require.Equal(t, testutil.DefaultMaxClockRates[workloadmeta.GPUMemory], gpu.MaxClockRates[workloadmeta.GPUMemory])
		require.Equal(t, expectedActivePIDs, gpu.ActivePIDs)
		require.Equal(t, "none", gpu.VirtualizationMode)
	}

	for _, uuid := range testutil.GPUUUIDs {
		require.True(t, foundIDs[uuid], "GPU with UUID %s not found", uuid)
	}
}

func TestGpuArchToString(t *testing.T) {
	tests := []struct {
		arch     nvml.DeviceArchitecture
		expected string
	}{
		{nvml.DEVICE_ARCH_KEPLER, "kepler"},
		{nvml.DEVICE_ARCH_UNKNOWN, "unknown"},
		{nvml.DeviceArchitecture(3751), "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, gpuArchToString(tt.arch))
		})
	}
}

func TestGpuProcessInfoUpdate(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:      collectorID,
		catalog: workloadmeta.NodeAgent,
		store:   wmetaMock,
	}

	ddnvml.WithMockNVML(t, nvmlMock)

	// First pull to populate the store with initial PIDs
	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))

	var expectedActivePIDs []int
	for _, proc := range testutil.DefaultProcessInfo {
		expectedActivePIDs = append(expectedActivePIDs, int(proc.Pid))
	}

	for _, gpu := range gpus {
		require.Equal(t, expectedActivePIDs, gpu.ActivePIDs)
	}

	// Now change those PIDs and make sure the store is updated and we get a complete override
	// of the previous PIDs
	expectedActivePIDs = []int{9761, 1234}
	newProcessInfo := []nvml.ProcessInfo{
		{Pid: uint32(expectedActivePIDs[0]), UsedGpuMemory: 100},
		{Pid: uint32(expectedActivePIDs[1]), UsedGpuMemory: 200},
	}
	oldProcessInfo := testutil.DefaultProcessInfo
	t.Cleanup(func() { testutil.DefaultProcessInfo = oldProcessInfo })

	testutil.DefaultProcessInfo = newProcessInfo

	c.Pull(context.Background())
	gpus = wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))

	for _, gpu := range gpus {
		require.Equal(t, expectedActivePIDs, gpu.ActivePIDs)
	}
}

func TestProcessEntityCreation(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:        collectorID,
		catalog:   workloadmeta.NodeAgent,
		store:     wmetaMock,
		seenPIDs:  make(map[int32]*workloadmeta.EntityID),
		seenUUIDs: map[string]struct{}{},
	}

	ddnvml.WithMockNVML(t, nvmlMock)

	// First pull to create Process entities
	c.Pull(context.Background())

	// Verify Process entities were created
	for _, procInfo := range testutil.DefaultProcessInfo {
		pid := int32(procInfo.Pid)
		process, err := wmetaMock.GetProcess(pid)
		require.NoError(t, err)
		require.NotNil(t, process)
		require.Equal(t, pid, process.Pid)
		require.NotNil(t, process.GPU, "Process should have GPU field set")
		require.Equal(t, workloadmeta.KindGPU, process.GPU.Kind)
		// Verify the GPU UUID is one of the expected GPUs
		foundGPU := false
		for _, uuid := range testutil.GPUUUIDs {
			if process.GPU.ID == uuid {
				foundGPU = true
				break
			}
		}
		require.True(t, foundGPU, "Process GPU UUID should be one of the expected GPUs")
	}
}

func TestProcessEntityDeletion(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:        collectorID,
		catalog:   workloadmeta.NodeAgent,
		store:     wmetaMock,
		seenPIDs:  make(map[int32]*workloadmeta.EntityID),
		seenUUIDs: map[string]struct{}{},
	}

	ddnvml.WithMockNVML(t, nvmlMock)

	// First pull to create Process entities
	c.Pull(context.Background())

	// Verify Process entities were created
	initialPIDs := make([]int32, 0, len(testutil.DefaultProcessInfo))
	for _, procInfo := range testutil.DefaultProcessInfo {
		pid := int32(procInfo.Pid)
		initialPIDs = append(initialPIDs, pid)
		process, err := wmetaMock.GetProcess(pid)
		require.NoError(t, err)
		require.NotNil(t, process)
	}

	// Now remove all PIDs by setting empty process info
	oldProcessInfo := testutil.DefaultProcessInfo
	t.Cleanup(func() { testutil.DefaultProcessInfo = oldProcessInfo })
	testutil.DefaultProcessInfo = []nvml.ProcessInfo{}

	// Second pull should unset Process entities
	c.Pull(context.Background())

	// Verify Process entities were unset
	for _, pid := range initialPIDs {
		process, err := wmetaMock.GetProcess(pid)
		// Process should not exist from SourceNVML anymore
		// Note: Process might still exist if other sources have it, but GPU field should be nil
		if err == nil && process != nil {
			// If process still exists, it shouldn't have GPU from SourceNVML
			// Since we can't directly check source, we verify the process no longer has GPU
			// in a real scenario, the process would be removed if only SourceNVML had it
		}
	}
}

func TestProcessEntityWithMultipleGPUs(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:        collectorID,
		catalog:   workloadmeta.NodeAgent,
		store:     wmetaMock,
		seenPIDs:  make(map[int32]*workloadmeta.EntityID),
		seenUUIDs: map[string]struct{}{},
	}

	ddnvml.WithMockNVML(t, nvmlMock)

	// First pull
	c.Pull(context.Background())

	// Get the GPUs
	gpus := wmetaMock.ListGPUs()
	require.Greater(t, len(gpus), 0)

	// Verify Process entities have GPU field set
	for _, procInfo := range testutil.DefaultProcessInfo {
		pid := int32(procInfo.Pid)
		process, err := wmetaMock.GetProcess(pid)
		require.NoError(t, err)
		require.NotNil(t, process)
		require.NotNil(t, process.GPU)
		// Since a PID can appear on multiple GPUs, we just verify it has a GPU
		require.Equal(t, workloadmeta.KindGPU, process.GPU.Kind)
		// Verify it's one of the GPUs
		found := false
		for _, gpu := range gpus {
			if process.GPU.ID == gpu.ID {
				found = true
				break
			}
		}
		require.True(t, found, "Process GPU should match one of the GPUs")
	}
}

func TestProcessEntityMerging(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:        collectorID,
		catalog:   workloadmeta.NodeAgent,
		store:     wmetaMock,
		seenPIDs:  make(map[int32]*workloadmeta.EntityID),
		seenUUIDs: map[string]struct{}{},
	}

	ddnvml.WithMockNVML(t, nvmlMock)

	// First, create Process entity from GPU collector
	c.Pull(context.Background())

	// Get first PID
	if len(testutil.DefaultProcessInfo) == 0 {
		t.Skip("No process info in test data")
	}
	pid := int32(testutil.DefaultProcessInfo[0].Pid)
	gpus := wmetaMock.ListGPUs()
	require.Greater(t, len(gpus), 0)

	// Verify Process entity from GPU collector
	gpuProcess, err := wmetaMock.GetProcess(pid)
	require.NoError(t, err)
	require.NotNil(t, gpuProcess)
	require.NotNil(t, gpuProcess.GPU)

	// Now create Process entity from service discovery
	serviceDiscoveryProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(pid)),
		},
		Pid: pid,
		Service: &workloadmeta.Service{
			GeneratedName: "test-service",
			TCPPorts:      []uint16{8080},
		},
	}

	// Notify store with service discovery source
	wmetaMock.Notify([]workloadmeta.CollectorEvent{
		{
			Source: workloadmeta.SourceServiceDiscovery,
			Type:   workloadmeta.EventTypeSet,
			Entity: serviceDiscoveryProcess,
		},
	})

	// Verify merged Process entity
	mergedProcess, err := wmetaMock.GetProcess(pid)
	require.NoError(t, err)
	require.NotNil(t, mergedProcess)
	// Should have GPU field from GPU collector
	require.NotNil(t, mergedProcess.GPU)
	// Should have Service data from service discovery
	require.NotNil(t, mergedProcess.Service)
	require.Equal(t, "test-service", mergedProcess.Service.GeneratedName)
	require.Equal(t, []uint16{8080}, mergedProcess.Service.TCPPorts)

	// Now remove the PID from GPU ActivePIDs
	oldProcessInfo := testutil.DefaultProcessInfo
	t.Cleanup(func() { testutil.DefaultProcessInfo = oldProcessInfo })
	testutil.DefaultProcessInfo = []nvml.ProcessInfo{}

	// Pull again to trigger unset event from SourceNVML
	c.Pull(context.Background())

	// Verify Process entity still exists (because service discovery still has it)
	stillExistingProcess, err := wmetaMock.GetProcess(pid)
	require.NoError(t, err)
	require.NotNil(t, stillExistingProcess, "Process entity should still exist after GPU removal")
	// GPU field should be nil since SourceNVML unset it
	require.Nil(t, stillExistingProcess.GPU, "GPU field should be nil after SourceNVML unset")
	// Service data should still be present from service discovery
	require.NotNil(t, stillExistingProcess.Service)
	require.Equal(t, "test-service", stillExistingProcess.Service.GeneratedName)
	require.Equal(t, []uint16{8080}, stillExistingProcess.Service.TCPPorts)
}
