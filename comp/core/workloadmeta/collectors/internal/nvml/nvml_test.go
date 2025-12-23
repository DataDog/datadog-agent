// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"slices"
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

	c := newCollector(wmetaMock, nil)

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

	for _, migChildrenUUIDs := range testutil.MIGChildrenUUIDs {
		for _, migChildUUID := range migChildrenUUIDs {
			require.True(t, foundIDs[migChildUUID], "MIG child GPU %s not found", migChildUUID)
		}
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

	c := newCollector(wmetaMock, nil)

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

func TestProcessEntities(t *testing.T) {
	processInfo := make(map[string][]nvml.ProcessInfo)

	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithProcessInfoCallback(func(uuid string) ([]nvml.ProcessInfo, nvml.Return) {
		return processInfo[uuid], nvml.SUCCESS
	}))

	c := newCollector(wmetaMock, nil)
	c.integrateWithWorkloadmetaProcesses = true

	ddnvml.WithMockNVML(t, nvmlMock)

	// Pull first, we have no process info so we should have no Process entities
	c.Pull(context.Background())

	processes := wmetaMock.ListProcesses()
	require.Equal(t, 0, len(processes))

	// Add process info for the first GPU
	pid0 := int32(1234)
	processInfo[testutil.GPUUUIDs[0]] = []nvml.ProcessInfo{
		{Pid: uint32(pid0), UsedGpuMemory: 100},
	}

	// Pull again, we should have one Process entity
	c.Pull(context.Background())
	processes = wmetaMock.ListProcesses()
	require.Equal(t, 1, len(processes))
	require.Equal(t, testutil.GPUUUIDs[0], processes[0].GPUs[0].ID)
	require.Equal(t, pid0, processes[0].Pid)

	// Add a new process that's using the second and third GPUs, while the one for the first GPU is still present
	pid1 := int32(5678)
	processInfo[testutil.GPUUUIDs[1]] = []nvml.ProcessInfo{
		{Pid: uint32(pid1), UsedGpuMemory: 200},
	}
	processInfo[testutil.GPUUUIDs[2]] = []nvml.ProcessInfo{
		{Pid: uint32(pid1), UsedGpuMemory: 300},
	}

	// Pull again, we should have two Process entities, one for the first GPU and one for the second and third GPUs
	c.Pull(context.Background())
	processes = wmetaMock.ListProcesses()
	require.Equal(t, 2, len(processes))

	foundPid0, foundPid1 := false, false
	for _, process := range processes {
		if process.Pid == pid0 {
			foundPid0 = true
			require.Equal(t, 1, len(process.GPUs))
			require.Equal(t, testutil.GPUUUIDs[0], process.GPUs[0].ID)
		} else if process.Pid == pid1 {
			foundPid1 = true
			require.Equal(t, 2, len(process.GPUs))
			require.True(t, slices.Contains(testutil.GPUUUIDs, process.GPUs[0].ID))
			require.True(t, slices.Contains(testutil.GPUUUIDs, process.GPUs[1].ID))
		}
	}
	require.True(t, foundPid0, "Process with PID %d not found", pid0)
	require.True(t, foundPid1, "Process with PID %d not found", pid1)

	// Now remove the process info for the first GPU
	processInfo[testutil.GPUUUIDs[0]] = []nvml.ProcessInfo{}

	// Pull again, we should have one Process entity, for the second and third GPUs
	c.Pull(context.Background())
	processes = wmetaMock.ListProcesses()
	require.Equal(t, 1, len(processes))
	require.Equal(t, testutil.GPUUUIDs[1], processes[0].GPUs[0].ID)
	require.Equal(t, pid1, processes[0].Pid)
	require.Equal(t, 2, len(processes[0].GPUs))
	require.True(t, slices.Contains(testutil.GPUUUIDs, processes[0].GPUs[0].ID))
	require.True(t, slices.Contains(testutil.GPUUUIDs, processes[0].GPUs[1].ID))

	// Now remove the process info for the second and third GPUs
	processInfo[testutil.GPUUUIDs[1]] = []nvml.ProcessInfo{}
	processInfo[testutil.GPUUUIDs[2]] = []nvml.ProcessInfo{}

	// Pull again, we should have no Process entities
	c.Pull(context.Background())
	processes = wmetaMock.ListProcesses()
	require.Equal(t, 0, len(processes))
}

func TestProcessEntityMerging(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	pid := int32(1234)
	procinfo := []nvml.ProcessInfo{
		{Pid: uint32(pid), UsedGpuMemory: 100},
	}
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithDeviceCount(1),
		testutil.WithProcessInfoCallback(func(_ string) ([]nvml.ProcessInfo, nvml.Return) {
			return procinfo, nvml.SUCCESS
		}))

	c := newCollector(wmetaMock, nil)
	c.integrateWithWorkloadmetaProcesses = true

	ddnvml.WithMockNVML(t, nvmlMock)

	// First, create Process entity from GPU collector
	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Len(t, gpus, 1)

	// Verify Process entity from GPU collector
	gpuProcess, err := wmetaMock.GetProcess(pid)
	require.NoError(t, err)
	require.NotNil(t, gpuProcess)
	require.NotEmpty(t, gpuProcess.GPUs)

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
	require.NotEmpty(t, mergedProcess.GPUs)
	// Should have Service data from service discovery
	require.NotNil(t, mergedProcess.Service)
	require.Equal(t, "test-service", mergedProcess.Service.GeneratedName)
	require.Equal(t, []uint16{8080}, mergedProcess.Service.TCPPorts)

	// Now remove the PID from GPU ActivePIDs
	procinfo = []nvml.ProcessInfo{}

	// Pull again to trigger unset event from SourceNVML
	c.Pull(context.Background())

	// Verify Process entity still exists (because service discovery still has it)
	stillExistingProcess, err := wmetaMock.GetProcess(pid)
	require.NoError(t, err)
	require.NotNil(t, stillExistingProcess, "Process entity should still exist after GPU removal")
	// GPU field should be nil since SourceNVML unset it
	require.Empty(t, stillExistingProcess.GPUs, "GPU field should be empty after SourceNVML unset")
	// Service data should still be present from service discovery
	require.NotNil(t, stillExistingProcess.Service)
	require.Equal(t, "test-service", stillExistingProcess.Service.GeneratedName)
	require.Equal(t, []uint16{8080}, stillExistingProcess.Service.TCPPorts)

	// Check that the process gets removed from the store once the unset from service discovery is sent
	wmetaMock.Notify([]workloadmeta.CollectorEvent{
		{
			Source: workloadmeta.SourceServiceDiscovery,
			Type:   workloadmeta.EventTypeUnset,
			Entity: serviceDiscoveryProcess,
		},
	})
	processes := wmetaMock.ListProcesses()
	require.Equal(t, 0, len(processes))
}

func TestPullWithMIGDevices(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := newCollector(wmetaMock, nil)

	ddnvml.WithMockNVML(t, nvmlMock)

	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))

	// Build a map of parent UUID to child UUIDs for validation
	parentToChildren := make(map[string][]string)
	for deviceIdx, childUUIDs := range testutil.MIGChildrenUUIDs {
		parentUUID := testutil.GPUUUIDs[deviceIdx]
		parentToChildren[parentUUID] = childUUIDs
	}

	// Separate physical and MIG devices
	physicalDevices := make(map[string]*workloadmeta.GPU)
	migDevices := make(map[string]*workloadmeta.GPU)

	for _, gpu := range gpus {
		if gpu.DeviceType == workloadmeta.GPUDeviceTypePhysical {
			physicalDevices[gpu.ID] = gpu
		} else if gpu.DeviceType == workloadmeta.GPUDeviceTypeMIG {
			migDevices[gpu.ID] = gpu
		}
	}

	// Verify we have the expected number of physical and MIG devices
	expectedPhysicalCount := len(testutil.GPUUUIDs)
	expectedMIGCount := 0
	for _, count := range testutil.MIGChildrenPerDevice {
		expectedMIGCount += count
	}
	require.Equal(t, expectedPhysicalCount, len(physicalDevices), "unexpected number of physical devices")
	require.Equal(t, expectedMIGCount, len(migDevices), "unexpected number of MIG devices")

	// Verify each MIG device has the correct parent and properties
	for parentUUID, childUUIDs := range parentToChildren {
		parentGPU, ok := physicalDevices[parentUUID]
		require.True(t, ok, "parent GPU %s not found", parentUUID)

		// Verify parent device properties
		require.Equal(t, workloadmeta.GPUDeviceTypePhysical, parentGPU.DeviceType)
		require.Empty(t, parentGPU.ParentGPUUUID, "physical device should not have a parent")

		// Verify each child MIG device
		for _, childUUID := range childUUIDs {
			migGPU, ok := migDevices[childUUID]
			require.True(t, ok, "MIG device %s not found", childUUID)

			// Verify MIG device properties
			require.Equal(t, workloadmeta.GPUDeviceTypeMIG, migGPU.DeviceType)
			require.Equal(t, parentUUID, migGPU.ParentGPUUUID, "MIG device %s should have parent %s", childUUID, parentUUID)
			require.Equal(t, "MIG "+testutil.DefaultGPUName, migGPU.Name)
			require.Equal(t, "MIG "+testutil.DefaultGPUName, migGPU.Device)
			require.Equal(t, testutil.DefaultNvidiaDriverVersion, migGPU.DriverVersion)
			require.Equal(t, nvidiaVendor, migGPU.Vendor)
			require.Equal(t, "hopper", migGPU.Architecture)
			require.Equal(t, testutil.DefaultGPUComputeCapMajor, migGPU.ComputeCapability.Major)
			require.Equal(t, testutil.DefaultGPUComputeCapMinor, migGPU.ComputeCapability.Minor)

			// Verify MIG device has cores (should be a fraction of parent's cores)
			require.Greater(t, migGPU.TotalCores, 0, "MIG device should have cores")
			require.Less(t, migGPU.TotalCores, parentGPU.TotalCores, "MIG device should have fewer cores than parent")

			// Verify MIG device has memory (should be a fraction of parent's memory)
			require.Greater(t, migGPU.TotalMemory, uint64(0), "MIG device should have memory")
			require.Less(t, migGPU.TotalMemory, parentGPU.TotalMemory, "MIG device should have less memory than parent")

			// Verify MIG device has process info
			var expectedActivePIDs []int
			for _, proc := range testutil.DefaultProcessInfo {
				expectedActivePIDs = append(expectedActivePIDs, int(proc.Pid))
			}
			require.Equal(t, expectedActivePIDs, migGPU.ActivePIDs)
		}
	}

	// Verify all physical devices without MIG children have no MIG devices
	for _, uuid := range testutil.GPUUUIDs {
		if _, hasMIGChildren := parentToChildren[uuid]; !hasMIGChildren {
			physicalGPU, ok := physicalDevices[uuid]
			require.True(t, ok, "physical GPU %s not found", uuid)
			require.Equal(t, workloadmeta.GPUDeviceTypePhysical, physicalGPU.DeviceType)
			require.Empty(t, physicalGPU.ParentGPUUUID)
		}
	}
}
