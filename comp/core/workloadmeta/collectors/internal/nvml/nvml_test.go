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

func TestExtractGPUType(t *testing.T) {
	tests := []struct {
		deviceName string
		expected   string
	}{
		// instances: g4dn.12xlarge, g4dn.16xlarge, g4dn.2xlarge, g4dn.4xlarge, g4dn.8xlarge, g4dn.metal, g4dn.xlarge, standard_nc16as_t4_v3, standard_nc4as_t4_v3, standard_nc64as_t4_v3, standard_nc8as_t4_v3
		{deviceName: "Tesla T4", expected: "t4"},
		// instances: g4dn.12xlarge, g4dn.16xlarge, g4dn.2xlarge, g4dn.4xlarge, g4dn.8xlarge, g4dn.metal, g4dn.xlarge, standard_nc16as_t4_v3, standard_nc4as_t4_v3, standard_nc64as_t4_v3, standard_nc8as_t4_v3
		{deviceName: "Tesla_T4", expected: "t4"},
		// instances: g5g.16xlarge, g5g.2xlarge, g5g.4xlarge, g5g.8xlarge, g5g.metal, g5g.xlarge
		{deviceName: "NVIDIA T4G", expected: "t4g"},
		// instances: g5g.16xlarge, g5g.2xlarge, g5g.4xlarge, g5g.8xlarge, g5g.metal, g5g.xlarge
		{deviceName: "NVIDIA_T4G", expected: "t4g"},
		// instances: standard_nc16ads_a10_v4, standard_nc32ads_a10_v4, standard_nc8ads_a10_v4, standard_nv12ads_a10_v5, standard_nv18ads_a10_v5, standard_nv36adms_a10_v5, standard_nv36ads_a10_v5, standard_nv6ads_a10_v5, standard_nv72ads_a10_v5
		{deviceName: "NVIDIA A10", expected: "a10"},
		// instances: standard_nc16ads_a10_v4, standard_nc32ads_a10_v4, standard_nc8ads_a10_v4, standard_nv12ads_a10_v5, standard_nv18ads_a10_v5, standard_nv36adms_a10_v5, standard_nv36ads_a10_v5, standard_nv6ads_a10_v5, standard_nv72ads_a10_v5
		{deviceName: "NVIDIA_A10", expected: "a10"},
		// instances: NVadsA10_v5 family
		{deviceName: "NVIDIA A10-4Q", expected: "a10"},
		// instances: g5.12xlarge, g5.16xlarge, g5.24xlarge, g5.2xlarge, g5.48xlarge, g5.4xlarge, g5.8xlarge, g5.xlarge
		{deviceName: "NVIDIA A10G", expected: "a10g"},
		// instances: g5.12xlarge, g5.16xlarge, g5.24xlarge, g5.2xlarge, g5.48xlarge, g5.4xlarge, g5.8xlarge, g5.xlarge
		{deviceName: "NVIDIA_A10G", expected: "a10g"},
		// instances: a2-highgpu-1g, a2-highgpu-2g, a2-highgpu-4g, a2-highgpu-8g, a2-megagpu-16g, a2-ultragpu-1g, a2-ultragpu-2g, a2-ultragpu-4g, a2-ultragpu-8g, p4d.24xlarge, p4de.24xlarge, standard_nc24ads_a100_v4, standard_nc48ads_a100_v4, standard_nc96ads_a100_v4, standard_nd96amsr_a100_v4, standard_nd96asr_v4
		{deviceName: "NVIDIA A100-SXM4-40GB", expected: "a100"},
		// instances: a2-highgpu-1g, a2-highgpu-2g, a2-highgpu-4g, a2-highgpu-8g, a2-megagpu-16g, a2-ultragpu-1g, a2-ultragpu-2g, a2-ultragpu-4g, a2-ultragpu-8g, p4d.24xlarge, p4de.24xlarge, standard_nc24ads_a100_v4, standard_nc48ads_a100_v4, standard_nc96ads_a100_v4, standard_nd96amsr_a100_v4, standard_nd96asr_v4
		{deviceName: "NVIDIA_A100-SXM4-40GB", expected: "a100"},
		{deviceName: "NVIDIA A100 80GB PCIe MIG 3g.40gb", expected: "a100"},
		// instances: a3-edgegpu-8g, a3-edgegpu-8g-nolssd, a3-highgpu-1g, a3-highgpu-2g, a3-highgpu-4g, a3-highgpu-8g, a3-megagpu-8g, p5.48xlarge, p5.4xlarge, standard_nc40ads_h100_v5, standard_nc80adis_h100_v5, standard_ncc40ads_h100_v5, standard_nd96isr_h100_v5
		{deviceName: "NVIDIA H100-PCIE", expected: "h100"},
		// instances: a3-edgegpu-8g, a3-edgegpu-8g-nolssd, a3-highgpu-1g, a3-highgpu-2g, a3-highgpu-4g, a3-highgpu-8g, a3-megagpu-8g, p5.48xlarge, p5.4xlarge, standard_nc40ads_h100_v5, standard_nc80adis_h100_v5, standard_ncc40ads_h100_v5, standard_nd96isr_h100_v5
		{deviceName: "NVIDIA_H100-PCIE", expected: "h100"},
		{deviceName: "NVIDIA H100 NVL MIG 3g.47gb", expected: "h100"},
		// instances: a3-ultragpu-8g, a3-ultragpu-8g-nolssd, p5en.48xlarge, standard_nd96isr_h200_v5
		{deviceName: "NVIDIA H200", expected: "h200"},
		// instances: a3-ultragpu-8g, a3-ultragpu-8g-nolssd, p5en.48xlarge, standard_nd96isr_h200_v5
		{deviceName: "NVIDIA_H200", expected: "h200"},
		// instances: p3.16xlarge, p3.2xlarge, p3.8xlarge, p3dn.24xlarge, standard_nc12s_v3, standard_nc24rs_v3, standard_nc24s_v3, standard_nc6s_v3, standard_nd40rs_v2
		{deviceName: "NVIDIA V100-32GB", expected: "v100"},
		// instances: p3.16xlarge, p3.2xlarge, p3.8xlarge, p3dn.24xlarge, standard_nc12s_v3, standard_nc24rs_v3, standard_nc24s_v3, standard_nc6s_v3, standard_nd40rs_v2
		{deviceName: "NVIDIA_V100-32GB", expected: "v100"},
		// instances: g2-standard-12, g2-standard-16, g2-standard-24, g2-standard-32, g2-standard-4, g2-standard-48, g2-standard-8, g2-standard-96, g6.12xlarge, g6.16xlarge, g6.24xlarge, g6.2xlarge, g6.48xlarge, g6.4xlarge, g6.8xlarge, g6.xlarge, g6f.2xlarge, g6f.4xlarge, g6f.large, g6f.xlarge, gr6.4xlarge, gr6.8xlarge, gr6f.4xlarge
		{deviceName: "NVIDIA L4", expected: "l4"},
		// instances: g2-standard-12, g2-standard-16, g2-standard-24, g2-standard-32, g2-standard-4, g2-standard-48, g2-standard-8, g2-standard-96, g6.12xlarge, g6.16xlarge, g6.24xlarge, g6.2xlarge, g6.48xlarge, g6.4xlarge, g6.8xlarge, g6.xlarge, g6f.2xlarge, g6f.4xlarge, g6f.large, g6f.xlarge, gr6.4xlarge, gr6.8xlarge, gr6f.4xlarge
		{deviceName: "NVIDIA_L4", expected: "l4"},
		{deviceName: " NVIDIA L4 ", expected: "l4"},
		{deviceName: "NVIDIA-L4", expected: "l4"},
		{deviceName: "L4", expected: ""},
		{deviceName: "l4", expected: ""},
		{deviceName: "NVIDIA: L4", expected: "l4"},
		{deviceName: "NVIDIA   L4", expected: "l4"},
		{deviceName: "NVIDIA__L4", expected: "l4"},
		{deviceName: "NVIDIA--L4", expected: "l4"},
		{deviceName: "NVIDIA L4 24GB", expected: "l4"},
		{deviceName: "NVIDIA L4-24GB", expected: "l4"},
		{deviceName: "NVIDIA L4 (rev.2)", expected: "l4"},
		{deviceName: "\"NVIDIA L4\"", expected: "l4"},
		{deviceName: "'NVIDIA L4'", expected: "l4"},
		{deviceName: "NVIDIA GeForce-RTX-3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA GeForce RTX_3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA GeForce   RTX 3090", expected: "rtx_3090"},
		// instances: g6e.12xlarge, g6e.16xlarge, g6e.24xlarge, g6e.2xlarge, g6e.48xlarge, g6e.4xlarge, g6e.8xlarge, g6e.xlarge
		{deviceName: "NVIDIA L40S", expected: "l40s"},
		// instances: g6e.12xlarge, g6e.16xlarge, g6e.24xlarge, g6e.2xlarge, g6e.48xlarge, g6e.4xlarge, g6e.8xlarge, g6e.xlarge
		{deviceName: "NVIDIA_L40S", expected: "l40s"},
		// instances: standard_nv12s_v2, standard_nv12s_v3, standard_nv24s_v2, standard_nv24s_v3, standard_nv48s_v3, standard_nv6s_v2
		{deviceName: "Tesla M60", expected: "m60"},
		// instances: standard_nv12s_v2, standard_nv12s_v3, standard_nv24s_v2, standard_nv24s_v3, standard_nv48s_v3, standard_nv6s_v2
		{deviceName: "Tesla_M60", expected: "m60"},
		// instances: p6-b200.48xlarge
		{deviceName: "NVIDIA B200-96GB", expected: "b200"},
		// instances: p6-b200.48xlarge
		{deviceName: "NVIDIA_B200-96GB", expected: "b200"},
		{deviceName: "NVIDIA RTX A6000", expected: "rtx_a6000"},
		{deviceName: "NVIDIA_RTX_A6000", expected: "rtx_a6000"},
		{deviceName: "NVIDIA RTX 6000 Ada Generation", expected: "rtx_6000"},
		{deviceName: "NVIDIA_RTX_6000_Ada_Generation", expected: "rtx_6000"},
		{deviceName: "NVIDIA GeForce RTX 3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA_GeForce_RTX_3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA GeForce RTX 4090", expected: "rtx_4090"},
		{deviceName: "NVIDIA_GeForce_RTX_4090", expected: "rtx_4090"},
		{deviceName: "", expected: ""},
		{deviceName: "Unknown GPU", expected: ""},
		{deviceName: "Unknown_GPU", expected: ""},
		{deviceName: "nViDiA a100", expected: "a100"},
	}

	for _, tt := range tests {
		t.Run(tt.deviceName, func(t *testing.T) {
			require.Equal(t, tt.expected, extractGPUType(tt.deviceName))
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
