// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func newTestCollector(t *testing.T, store workloadmeta.Component) *collector {
	t.Helper()

	config := config.NewMock(t)
	config.SetInTest("gpu.enabled", true)

	return newCollector(store, config)
}

func TestStartDisabledWhenGPUMonitoringDisabled(t *testing.T) {
	env.SetFeatures(t, env.NVML)

	c := newCollector(nil, config.NewMock(t))
	err := c.Start(context.Background(), nil)

	require.Equal(t, dderrors.NewDisabled(componentName, "GPU monitoring is disabled"), err)
}

func TestPull(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := newTestCollector(t, wmetaMock)

	ddnvml.WithMockNVML(t, nvmlMock)

	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))
	expectedActivePIDs := testutil.DefaultActivePIDs()
	expectedPhysicalActivePIDs := slices.Clone(expectedActivePIDs)
	expectedPhysicalActivePIDs = append(expectedPhysicalActivePIDs, 1234)
	slices.Sort(expectedPhysicalActivePIDs)

	foundIDs := make(map[string]bool)
	for _, gpu := range gpus {
		foundIDs[gpu.ID] = true
		var expectedName string
		expectedGPUActivePIDs := expectedActivePIDs
		if gpu.DeviceType == workloadmeta.GPUDeviceTypeMIG {
			expectedName = testutil.DefaultGPUName + " MIG 3g.40gb"
		} else if gpu.DeviceType == workloadmeta.GPUDeviceTypePhysical {
			expectedName = testutil.DefaultGPUName
			expectedGPUActivePIDs = expectedPhysicalActivePIDs
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
		require.Equal(t, testutil.DefaultMaxClockRates[nvml.CLOCK_SM], gpu.MaxClockRates[workloadmeta.GPUSM])
		require.Equal(t, testutil.DefaultMaxClockRates[nvml.CLOCK_MEM], gpu.MaxClockRates[workloadmeta.GPUMemory])
		require.ElementsMatch(t, expectedGPUActivePIDs, gpu.ActivePIDs)
		require.Equal(t, "none", gpu.VirtualizationMode)
		require.Equal(t, "0000:00:1e.0", gpu.PCIBusID)
		require.Empty(t, gpu.FabricClusterUUID)
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

func TestPullNVLinkVersion(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1, NvLinkLinkCount: 1}),
	)
	c := newTestCollector(t, wmetaMock)
	ddnvml.WithMockNVML(t, nvmlMock)

	c.Pull(context.Background())

	for _, gpu := range wmetaMock.ListGPUs() {
		expectedVersion := "1.0"
		if gpu.DeviceType == workloadmeta.GPUDeviceTypeMIG {
			// MIG devices do not have NVLink ports, even when their parent does.
			expectedVersion = "not_nvlink_capable"
		}
		require.Equalf(t, expectedVersion, gpu.NVLinkVersion, "unexpected NVLink version for GPU %s", gpu.ID)
	}
}

func TestPullWithoutNVLink(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithNVLinkLinkCount(0),
	)
	c := newTestCollector(t, wmetaMock)
	ddnvml.WithMockNVML(t, nvmlMock)

	c.Pull(context.Background())

	for _, gpu := range wmetaMock.ListGPUs() {
		require.Equalf(t, "not_nvlink_capable", gpu.NVLinkVersion, "unexpected NVLink version for GPU %s", gpu.ID)
	}
}

func TestFabricInfoToTags(t *testing.T) {
	clusterUUID := [16]uint8{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	tests := []struct {
		name              string
		fabricInfo        nvml.GpuFabricInfo_v2
		expectedClusterID string
		expectedCliqueID  uint32
		expectedAvailable bool
	}{
		{
			name: "completed fabric with cluster UUID",
			fabricInfo: nvml.GpuFabricInfo_v2{
				State:       nvml.GPU_FABRIC_STATE_COMPLETED,
				Status:      uint32(nvml.SUCCESS),
				CliqueId:    42,
				ClusterUuid: clusterUUID,
			},
			expectedClusterID: "00112233-4455-6677-8899-aabbccddeeff",
			expectedCliqueID:  42,
			expectedAvailable: true,
		},
		{
			name: "fabric initialization incomplete",
			fabricInfo: nvml.GpuFabricInfo_v2{
				State:       nvml.GPU_FABRIC_STATE_IN_PROGRESS,
				Status:      uint32(nvml.SUCCESS),
				CliqueId:    42,
				ClusterUuid: clusterUUID,
			},
		},
		{
			name: "fabric status failed",
			fabricInfo: nvml.GpuFabricInfo_v2{
				State:       nvml.GPU_FABRIC_STATE_COMPLETED,
				Status:      uint32(nvml.ERROR_UNKNOWN),
				CliqueId:    42,
				ClusterUuid: clusterUUID,
			},
		},
		{
			name: "cluster UUID is unavailable",
			fabricInfo: nvml.GpuFabricInfo_v2{
				State:    nvml.GPU_FABRIC_STATE_COMPLETED,
				Status:   uint32(nvml.SUCCESS),
				CliqueId: 42,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterID, cliqueID, available := fabricInfoToTags(tt.fabricInfo)
			require.Equal(t, tt.expectedClusterID, clusterID)
			require.Equal(t, tt.expectedCliqueID, cliqueID)
			require.Equal(t, tt.expectedAvailable, available)
		})
	}
}

func TestFabricClusterUUIDFromNVMLInfo(t *testing.T) {
	clusterUUID := [16]uint8{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

	require.Equal(t, "00112233-4455-6677-8899-aabbccddeeff", fabricClusterUUIDFromNVMLInfo(clusterUUID))
}

func TestPCIBusIDFromNVMLInfo(t *testing.T) {
	tests := []struct {
		name     string
		pciInfo  nvml.PciInfo
		expected string
	}{
		{
			name: "typical linux BDF",
			pciInfo: nvml.PciInfo{
				Domain: 0,
				Bus:    0x65,
				Device: 0,
			},
			expected: "0000:65:00.0",
		},
		{
			name: "domain wider than four hex digits",
			pciInfo: nvml.PciInfo{
				Domain: 0x12345,
				Bus:    0xab,
				Device: 0x1e,
			},
			expected: "12345:ab:1e.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, pciBusIDFromNVMLInfo(tt.pciInfo))
		})
	}
}

func TestGpuProcessInfoUpdate(t *testing.T) {
	// Seed the callback with the default process info so the first pull mirrors
	// the package defaults without mutating any global state.
	processInfo := slices.Clone(testutil.DefaultProcessInfo)
	expectedActivePIDs := testutil.DefaultActivePIDs()

	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithProcessDataCallback(func(_ string) (testutil.MockProcessInfoList, nvml.Return) {
			return processInfo, nvml.SUCCESS
		}),
	)

	c := newTestCollector(t, wmetaMock)

	ddnvml.WithMockNVML(t, nvmlMock)

	// First pull to populate the store with initial PIDs
	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))

	for _, gpu := range gpus {
		require.ElementsMatch(t, expectedActivePIDs, gpu.ActivePIDs)
	}

	// Now change those PIDs and make sure the store is updated and we get a complete override
	// of the previous PIDs
	expectedActivePIDs = []int{9761, 1234}
	processInfo = testutil.MockProcessInfoList{
		{Pid: uint32(expectedActivePIDs[0]), UsedGpuMemory: 100},
		{Pid: uint32(expectedActivePIDs[1]), UsedGpuMemory: 200},
	}

	c.Pull(context.Background())
	gpus = wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))

	for _, gpu := range gpus {
		require.ElementsMatch(t, expectedActivePIDs, gpu.ActivePIDs)
	}
}

func TestProcessEntities(t *testing.T) {
	processInfo := make(map[string]testutil.MockProcessInfoList)

	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithProcessDataCallback(func(uuid string) (testutil.MockProcessInfoList, nvml.Return) {
		return processInfo[uuid], nvml.SUCCESS
	}))

	c := newTestCollector(t, wmetaMock)
	c.integrateWithWorkloadmetaProcesses = true

	ddnvml.WithMockNVML(t, nvmlMock)

	// Pull first, we have no process info so we should have no Process entities
	c.Pull(context.Background())

	processes := wmetaMock.ListProcesses()
	require.Equal(t, 0, len(processes))

	// Add process info for the first GPU
	pid0 := int32(1234)
	processInfo[testutil.GPUUUIDs[0]] = testutil.MockProcessInfoList{
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
	processInfo[testutil.GPUUUIDs[1]] = testutil.MockProcessInfoList{
		{Pid: uint32(pid1), UsedGpuMemory: 200},
	}
	processInfo[testutil.GPUUUIDs[2]] = testutil.MockProcessInfoList{
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
	processInfo[testutil.GPUUUIDs[0]] = testutil.MockProcessInfoList{}

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
	processInfo[testutil.GPUUUIDs[1]] = testutil.MockProcessInfoList{}
	processInfo[testutil.GPUUUIDs[2]] = testutil.MockProcessInfoList{}

	// Pull again, we should have no Process entities
	c.Pull(context.Background())
	processes = wmetaMock.ListProcesses()
	require.Equal(t, 0, len(processes))
}

func TestProcessEntityMerging(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	pid := int32(1234)
	procinfo := testutil.MockProcessInfoList{
		{Pid: uint32(pid), UsedGpuMemory: 100},
	}
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithDeviceCount(1),
		testutil.WithProcessDataCallback(func(_ string) (testutil.MockProcessInfoList, nvml.Return) {
			return procinfo, nvml.SUCCESS
		}),
	)
	c := newTestCollector(t, wmetaMock)
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
	procinfo = testutil.MockProcessInfoList{}

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

	c := newTestCollector(t, wmetaMock)

	ddnvml.WithMockNVML(t, nvmlMock)

	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, testutil.GetTotalExpectedDevices(), len(gpus))

	// Build a map of parent UUID to child UUIDs for validation
	parentToChildren := make(map[string][]string)
	for deviceIdx, childUUIDs := range testutil.MIGChildrenUUIDs {
		parentUUID := testutil.GPUUUIDs[deviceIdx]
		parentToChildren[parentUUID] = slices.Collect(maps.Values(childUUIDs))
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
	for _, childrenUUIDs := range testutil.MIGChildrenUUIDs {
		expectedMIGCount += len(childrenUUIDs)
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
			require.Equal(t, testutil.DefaultGPUName+" MIG 3g.40gb", migGPU.Name)
			require.Equal(t, testutil.DefaultGPUName+" MIG 3g.40gb", migGPU.Device)
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
			require.ElementsMatch(t, testutil.DefaultActivePIDs(), migGPU.ActivePIDs)
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

func TestCDIClaimUID(t *testing.T) {
	// The claim UID is a Kubernetes UUID, which contains hyphens. Splitting the
	// CDI device name at the first hyphen truncates it to 8 characters and the
	// spec-file lookup then misses silently -- the failure this test pins down.
	const uid = "c8593c85-440d-4156-b199-aea592ff83df"

	tests := []struct {
		name    string
		cdiName string
		want    string
	}{
		{
			name:    "MIG device, real name observed on an H200 node",
			cdiName: "k8s.gpu.nvidia.com/claim=" + uid + "-gpu-0-mig-1g18gb-19-0",
			want:    uid,
		},
		{
			name:    "whole card",
			cdiName: "k8s.gpu.nvidia.com/claim=" + uid + "-gpu-0",
			want:    uid,
		},
		{
			name:    "no device suffix",
			cdiName: "k8s.gpu.nvidia.com/claim=" + uid,
			want:    uid,
		},
		{
			name:    "not a claim device",
			cdiName: "nvidia.com/gpu=all",
			want:    "",
		},
		{
			name:    "truncated, shorter than a UUID",
			cdiName: "k8s.gpu.nvidia.com/claim=c8593c85",
			want:    "",
		},
		{
			// The UID is interpolated into a spec file path.
			name:    "a path separator in the UID is rejected",
			cdiName: "k8s.gpu.nvidia.com/claim=../../../etc/passwd-aaaaaaaaaaaaaaaaaaaa",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, cdiClaimUID(cdiDeviceKey(tt.cdiName)))
		})
	}
}

// fakeDeviceCache is a DeviceCache that returns a fixed device list. Only All()
// is exercised by resolveMIGUUID; the rest satisfy the interface.
type fakeDeviceCache struct {
	devices []ddnvml.Device
}

func (f *fakeDeviceCache) Refresh() error                               { return nil }
func (f *fakeDeviceCache) GetByUUID(string) (ddnvml.Device, error)      { return nil, nil }
func (f *fakeDeviceCache) GetByIndex(int) (ddnvml.Device, error)        { return nil, nil }
func (f *fakeDeviceCache) Count() (int, error)                          { return len(f.devices), nil }
func (f *fakeDeviceCache) SMVersionSet() (map[uint32]struct{}, error)   { return nil, nil }
func (f *fakeDeviceCache) All() ([]ddnvml.Device, error)                { return f.devices, nil }
func (f *fakeDeviceCache) AllPhysicalDevices() ([]ddnvml.Device, error) { return f.devices, nil }
func (f *fakeDeviceCache) AllMigDevices() ([]ddnvml.Device, error)      { return nil, nil }
func (f *fakeDeviceCache) Cores(string) (uint64, error)                 { return 0, nil }

func migChild(uuid string, gi, ci int) *ddnvml.MIGDevice {
	return &ddnvml.MIGDevice{
		DeviceInfo:        ddnvml.DeviceInfo{UUID: uuid},
		MIGInstanceID:     gi,
		ComputeInstanceID: ci,
	}
}

func TestResolveMIGUUIDMatchesComputeInstance(t *testing.T) {
	// A GPU instance can hold several compute instances (e.g. 3g.71gb split
	// into 3x 1c.3g). Matching on the GPU instance alone returns whichever CI
	// NVML happens to enumerate first, attributing every one of those
	// containers to the same MIG device.
	const (
		ci0UUID = "MIG-00000000-0000-0000-0000-000000000000"
		ci1UUID = "MIG-11111111-1111-1111-1111-111111111111"
		gi2UUID = "MIG-22222222-2222-2222-2222-222222222222"
	)

	cache := &fakeDeviceCache{devices: []ddnvml.Device{
		&ddnvml.PhysicalDevice{
			DeviceInfo: ddnvml.DeviceInfo{UUID: "GPU-parent", Index: 0},
			MIGChildren: []*ddnvml.MIGDevice{
				migChild(ci0UUID, 1, 0),
				migChild(ci1UUID, 1, 1),
				migChild(gi2UUID, 2, 0),
			},
		},
	}}

	tests := []struct {
		name         string
		gpu, gi, ci  int
		want         string
		wantResolved bool
	}{
		{name: "first compute instance", gpu: 0, gi: 1, ci: 0, want: ci0UUID, wantResolved: true},
		{name: "second compute instance of the same GPU instance", gpu: 0, gi: 1, ci: 1, want: ci1UUID, wantResolved: true},
		{name: "different GPU instance", gpu: 0, gi: 2, ci: 0, want: gi2UUID, wantResolved: true},
		{name: "compute instance that does not exist", gpu: 0, gi: 2, ci: 1, wantResolved: false},
		{name: "GPU instance that does not exist", gpu: 0, gi: 9, ci: 0, wantResolved: false},
		{name: "physical device that does not exist", gpu: 1, gi: 1, ci: 0, wantResolved: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveMIGUUID(cache, tt.gpu, tt.gi, tt.ci)
			require.Equal(t, tt.wantResolved, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

// cdiSpecMIG is the shape observed on a Nebius H200 node running dynamic MIG:
// the claim pins the GPU instance's and the compute instance's capability
// devices, each as a path/minor pair.
const cdiSpecMIG = `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: c8593c85-440d-4156-b199-aea592ff83df-gpu-0-mig-1g18gb-19-0
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia-caps/nvidia-cap102
      hostPath: /dev/nvidia-caps/nvidia-cap102
      permissions: rw
      major: 239
      minor: 102
    - path: /dev/nvidia-caps/nvidia-cap103
      permissions: rw
      major: 239
      minor: 103
`

const cdiSpecWholeCard = `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: c8593c85-440d-4156-b199-aea592ff83df-gpu-0
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia0
      permissions: rw
    - path: /dev/nvidiactl
      permissions: rw
`

func writeCDISpec(t *testing.T, uid, contents string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "k8s.gpu.nvidia.com-claim_"+uid+".yaml"), []byte(contents), 0o600))
	old := cdiSpecDirs
	cdiSpecDirs = []string{dir}
	t.Cleanup(func() { cdiSpecDirs = old })
}

func TestCDIDeviceNodes(t *testing.T) {
	const uid = "c8593c85-440d-4156-b199-aea592ff83df"

	t.Run("MIG claim yields both capability devices with their minors", func(t *testing.T) {
		writeCDISpec(t, uid, cdiSpecMIG)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0-mig-1g18gb-19-0")
		require.NoError(t, err)
		require.Equal(t, []cdiDeviceNode{
			{path: "/dev/nvidia-caps/nvidia-cap102", minor: 102},
			{path: "/dev/nvidia-caps/nvidia-cap103", minor: 103},
		}, nodes)
	})

	t.Run("whole-card claim yields the device node with no minor", func(t *testing.T) {
		writeCDISpec(t, uid, cdiSpecWholeCard)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0")
		require.NoError(t, err)
		// A path entry with no minor must not absorb the next entry's minor,
		// which is what a plain forward scan would do.
		require.Equal(t, []cdiDeviceNode{
			{path: "/dev/nvidia0", minor: -1},
			{path: "/dev/nvidiactl", minor: -1},
		}, nodes)
	})

	t.Run("missing spec is an error, not an empty result", func(t *testing.T) {
		writeCDISpec(t, uid, cdiSpecMIG)

		_, err := cdiDeviceNodes("00000000-0000-0000-0000-000000000000", "irrelevant")
		require.Error(t, err)
	})

	t.Run("a multi-device claim yields only the requested device", func(t *testing.T) {
		// One claim, two MIG devices, one spec file. Reading every node in the
		// file attributes both devices to a container holding one of them.
		writeCDISpec(t, uid, `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: `+uid+`-gpu-0-mig-1g18gb-19-0
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia-caps/nvidia-cap102
      minor: 102
    - path: /dev/nvidia-caps/nvidia-cap103
      minor: 103
- name: `+uid+`-gpu-0-mig-1g18gb-19-1
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia-caps/nvidia-cap104
      minor: 104
    - path: /dev/nvidia-caps/nvidia-cap105
      minor: 105
`)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0-mig-1g18gb-19-1")
		require.NoError(t, err)
		require.Equal(t, []cdiDeviceNode{
			{path: "/dev/nvidia-caps/nvidia-cap104", minor: 104},
			{path: "/dev/nvidia-caps/nvidia-cap105", minor: 105},
		}, nodes)
	})

	t.Run("an unmatched device name falls back to the whole spec", func(t *testing.T) {
		writeCDISpec(t, uid, cdiSpecMIG)

		nodes, err := cdiDeviceNodes(uid, "some-name-the-driver-did-not-use")
		require.NoError(t, err)
		require.Len(t, nodes, 2)
	})

	t.Run("the minor is found however many keys precede it", func(t *testing.T) {
		// A device node carries an optional "type" field. A fixed-width
		// lookahead loses the minor as soon as the driver emits one more key,
		// and the container then silently loses attribution.
		writeCDISpec(t, uid, `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: `+uid+`-gpu-0-mig-1g18gb-19-0
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia-caps/nvidia-cap102
      hostPath: /dev/nvidia-caps/nvidia-cap102
      type: c
      permissions: rw
      uid: 0
      gid: 0
      major: 239
      minor: 102
`)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0-mig-1g18gb-19-0")
		require.NoError(t, err)
		require.Equal(t, []cdiDeviceNode{
			{path: "/dev/nvidia-caps/nvidia-cap102", minor: 102},
		}, nodes)
	})
}

func TestCDIDeviceNodesUsesTheParsedSpec(t *testing.T) {
	const uid = "c8593c85-440d-4156-b199-aea592ff83df"

	t.Run("key order does not matter", func(t *testing.T) {
		// The line scan only looks forward from "path", so a minor written
		// before it is invisible to that path. Nothing forbids this order.
		writeCDISpec(t, uid, `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: `+uid+`-gpu-0-mig-1g18gb-19-0
  containerEdits:
    deviceNodes:
    - minor: 102
      major: 239
      path: /dev/nvidia-caps/nvidia-cap102
`)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0-mig-1g18gb-19-0")
		require.NoError(t, err)
		require.Equal(t, []cdiDeviceNode{
			{path: "/dev/nvidia-caps/nvidia-cap102", minor: 102},
		}, nodes)
	})

	t.Run("nodes of another device in the same claim are not read", func(t *testing.T) {
		// The parsed path knows which entry a node belongs to; the scan can
		// only infer it from the most recent "- name:" line.
		writeCDISpec(t, uid, `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: `+uid+`-gpu-0-mig-1g18gb-19-0
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia-caps/nvidia-cap102
      minor: 102
- name: `+uid+`-gpu-0-mig-1g18gb-19-1
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia-caps/nvidia-cap104
      minor: 104
containerEdits:
  deviceNodes:
  - path: /dev/nvidiactl
`)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0-mig-1g18gb-19-0")
		require.NoError(t, err)
		// Neither the sibling device's node nor the spec-wide /dev/nvidiactl.
		require.Equal(t, []cdiDeviceNode{
			{path: "/dev/nvidia-caps/nvidia-cap102", minor: 102},
		}, nodes)
	})

	t.Run("a document the struct does not fit falls back to the scan", func(t *testing.T) {
		// devices as a mapping rather than a sequence: unmarshalling fails, but
		// the path/minor lines are still readable. The fallback exists so that
		// a driver emitting an unexpected shape degrades to the behaviour
		// verified on hardware instead of losing attribution outright.
		writeCDISpec(t, uid, `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
  someDevice:
    containerEdits:
      deviceNodes:
      - path: /dev/nvidia-caps/nvidia-cap102
        minor: 102
`)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0-mig-1g18gb-19-0")
		require.NoError(t, err)
		require.Equal(t, []cdiDeviceNode{
			{path: "/dev/nvidia-caps/nvidia-cap102", minor: 102},
		}, nodes)
	})

	t.Run("an entry with no nvidia device node falls back rather than reporting none", func(t *testing.T) {
		// An empty result would silently drop the container's attribution, so
		// it is treated as "this struct did not fit" instead.
		writeCDISpec(t, uid, `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: `+uid+`-gpu-0
  containerEdits:
    env:
    - FOO=bar
containerEdits:
  deviceNodes:
  - path: /dev/nvidia0
`)

		nodes, err := cdiDeviceNodes(uid, uid+"-gpu-0")
		require.NoError(t, err)
		require.Equal(t, []cdiDeviceNode{{path: "/dev/nvidia0", minor: -1}}, nodes)
	})
}

func TestIsMIGCapabilityDevice(t *testing.T) {
	// IMEX channel nodes also contain "nvidia-cap" but are numbered in a
	// different minor space, so treating one as a capability minor can resolve
	// to a MIG instance the container does not own.
	assert.True(t, isMIGCapabilityDevice("/dev/nvidia-caps/nvidia-cap102"))
	assert.False(t, isMIGCapabilityDevice("/dev/nvidia-caps-imex-channels/channel102"))
	assert.False(t, isMIGCapabilityDevice("/dev/nvidia0"))
}

func TestPhysicalDeviceIndex(t *testing.T) {
	tests := []struct {
		path      string
		wantIndex int
		wantOK    bool
	}{
		{path: "/dev/nvidia0", wantIndex: 0, wantOK: true},
		{path: "/dev/nvidia11", wantIndex: 11, wantOK: true},
		// Control and capability nodes share the prefix but are not devices.
		{path: "/dev/nvidiactl", wantOK: false},
		{path: "/dev/nvidia-uvm", wantOK: false},
		{path: "/dev/nvidia-caps/nvidia-cap102", wantOK: false},
		{path: "/dev/dri/card0", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			index, ok := physicalDeviceMinor(tt.path)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantIndex, index)
		})
	}
}

func TestMIGMinorsToInstances(t *testing.T) {
	// Real shape of /proc/driver/nvidia-caps/mig-minors: two GPU instances on
	// gpu0, the first holding two compute instances.
	const migMinors = `config 1
monitor 2
gpu0/gi11/access 102
gpu0/gi11/ci0/access 103
gpu0/gi11/ci1/access 104
gpu0/gi12/access 105
gpu0/gi12/ci0/access 106
`
	dir := t.TempDir()
	path := filepath.Join(dir, "mig-minors")
	require.NoError(t, os.WriteFile(path, []byte(migMinors), 0o600))
	old := migMinorsPath
	migMinorsPath = path
	t.Cleanup(func() { migMinorsPath = old })

	tests := []struct {
		name   string
		minors []int
		want   []migInstance
	}{
		{
			name:   "one MIG device: GPU instance minor plus compute instance minor",
			minors: []int{102, 103},
			want:   []migInstance{{gpu: 0, gi: 11, ci: 0}},
		},
		{
			name: "two compute instances of one GPU instance stay distinct",
			// Matching on the GPU instance alone collapses these onto one.
			minors: []int{102, 103, 104},
			want:   []migInstance{{gpu: 0, gi: 11, ci: 0}, {gpu: 0, gi: 11, ci: 1}},
		},
		{
			name:   "a claim holding two MIG devices yields both",
			minors: []int{102, 103, 105, 106},
			want:   []migInstance{{gpu: 0, gi: 11, ci: 0}, {gpu: 0, gi: 12, ci: 0}},
		},
		{
			name: "GPU instance minor alone resolves to nothing",
			// The gi access device carries no compute instance, so there is no
			// full tuple to report.
			minors: []int{102},
			want:   nil,
		},
		{
			name:   "no minors",
			minors: nil,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, migMinorsToInstances(tt.minors))
		})
	}
}

func TestResolveMIGUUIDFallsBackWhenComputeInstanceUnknown(t *testing.T) {
	// GetComputeInstanceId is a non-critical NVML API. A driver that does not
	// export it leaves ComputeInstanceID at -1, and the lookup must still
	// resolve on the GPU instance rather than failing closed -- that would drop
	// MIG attribution entirely on such a driver.
	const uuid = "MIG-33333333-3333-3333-3333-333333333333"

	cache := &fakeDeviceCache{devices: []ddnvml.Device{
		&ddnvml.PhysicalDevice{
			DeviceInfo:  ddnvml.DeviceInfo{UUID: "GPU-parent", Index: 0},
			MIGChildren: []*ddnvml.MIGDevice{migChild(uuid, 1, -1)},
		},
	}}

	for _, ci := range []int{0, 1} {
		got, ok := resolveMIGUUID(cache, 0, 1, ci)
		require.True(t, ok)
		require.Equal(t, uuid, got)
	}
}

func TestResolvePhysicalUUID(t *testing.T) {
	// Whole-card DRA claims pin /dev/nvidiaN, which resolves with no MIG hop.
	// Without this, a container holding both a MIG device and a whole card
	// would have only the MIG device published.
	cache := &fakeDeviceCache{devices: []ddnvml.Device{
		&ddnvml.PhysicalDevice{DeviceInfo: ddnvml.DeviceInfo{UUID: "GPU-zero", Index: 0}, MinorNumber: 0},
		&ddnvml.PhysicalDevice{DeviceInfo: ddnvml.DeviceInfo{UUID: "GPU-one", Index: 1}, MinorNumber: 1},
		// A MIG child must never be returned for a physical device node.
		migChild("MIG-child", 1, 0),
	}}

	got, ok := resolvePhysicalUUID(cache, 1)
	require.True(t, ok)
	require.Equal(t, "GPU-one", got)

	_, ok = resolvePhysicalUUID(cache, 7)
	require.False(t, ok)
}

// TestResolvePhysicalUUIDMatchesMinorNotIndex pins the distinction that makes
// this correct. /dev/nvidiaN encodes the minor number; NVML's Index is its own
// enumeration order. They agree on ordinary hardware, so matching the wrong one
// passes every test where they coincide and then attributes a container to
// another card in the field -- silently, with a plausible UUID.
func TestResolvePhysicalUUIDMatchesMinorNotIndex(t *testing.T) {
	cache := &fakeDeviceCache{devices: []ddnvml.Device{
		&ddnvml.PhysicalDevice{DeviceInfo: ddnvml.DeviceInfo{UUID: "GPU-minor-7", Index: 0}, MinorNumber: 7},
		&ddnvml.PhysicalDevice{DeviceInfo: ddnvml.DeviceInfo{UUID: "GPU-minor-0", Index: 7}, MinorNumber: 0},
	}}

	got, ok := resolvePhysicalUUID(cache, 7)
	require.True(t, ok)
	require.Equal(t, "GPU-minor-7", got, "/dev/nvidia7 must resolve by minor number, not by NVML index")

	got, ok = resolvePhysicalUUID(cache, 0)
	require.True(t, ok)
	require.Equal(t, "GPU-minor-0", got)
}

// TestResolvePhysicalUUIDSkipsUnknownMinor guards the -1 sentinel. When the
// driver did not expose GetMinorNumber there is nothing to match on, and
// falling back to Index would be the very confusion this matching avoids.
func TestResolvePhysicalUUIDSkipsUnknownMinor(t *testing.T) {
	cache := &fakeDeviceCache{devices: []ddnvml.Device{
		&ddnvml.PhysicalDevice{DeviceInfo: ddnvml.DeviceInfo{UUID: "GPU-unknown", Index: 0}, MinorNumber: -1},
	}}

	_, ok := resolvePhysicalUUID(cache, 0)
	require.False(t, ok, "a device with no known minor must not be matched against a device node")
}

// writeMIGMinors points migMinorsPath at a fixture for the duration of a test.
func writeMIGMinors(t *testing.T, contents string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mig-minors")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	old := migMinorsPath
	migMinorsPath = path
	t.Cleanup(func() { migMinorsPath = old })
}

// TestResolveCDIToGPUsIgnoresTheParentOfAMIGEntry pins the distinction between
// a device a claim allocates and a device it merely needs access to. A MIG
// entry pins the parent card's node alongside the capability devices
// [verified on an H200]; resolving that parent as a whole card ties the
// container to every other workload on the card, and does it silently -- with
// a real UUID rather than an error -- whenever the MIG hop fails.
func TestResolveCDIToGPUsIgnoresTheParentOfAMIGEntry(t *testing.T) {
	const (
		uid      = "c8593c85-440d-4156-b199-aea592ff83df"
		migUUID  = "MIG-10c78edc-04e2-5dc7-a77e-3417825cf9ee"
		cardUUID = "GPU-90fde3d8-ef04-af6f-47fd-168e9af6919e"
	)
	cdiName := "k8s.gpu.nvidia.com/claim=" + uid + "-gpu-0-mig-1g18gb-19-0"

	spec := `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: ` + uid + `-gpu-0-mig-1g18gb-19-0
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia0
      permissions: rw
    - path: /dev/nvidia-caps/nvidia-cap102
      minor: 102
    - path: /dev/nvidia-caps/nvidia-cap103
      minor: 103
`
	migMinors := "gpu0/gi11/access 102\ngpu0/gi11/ci0/access 103\n"

	card := func(children ...*ddnvml.MIGDevice) *fakeDeviceCache {
		return &fakeDeviceCache{devices: []ddnvml.Device{
			&ddnvml.PhysicalDevice{
				DeviceInfo:  ddnvml.DeviceInfo{UUID: cardUUID, Index: 0},
				MIGChildren: children,
			},
		}}
	}

	t.Run("resolves to the instance, not to the instance plus its card", func(t *testing.T) {
		writeCDISpec(t, uid, spec)
		writeMIGMinors(t, migMinors)

		got := (&collector{}).resolveCDIToGPUs(card(migChild(migUUID, 11, 0)), cdiName)
		require.Equal(t, []string{migUUID}, got)
	})

	t.Run("resolves to nothing when NVML cannot see the instance", func(t *testing.T) {
		// The state observed on the cluster: no MIG children visible. The
		// container must come out unattributed, which is a condition the GPU
		// check reports, rather than attributed to the whole card, which looks
		// like success.
		writeCDISpec(t, uid, spec)
		writeMIGMinors(t, migMinors)

		got := (&collector{}).resolveCDIToGPUs(card(), cdiName)
		require.Empty(t, got)
	})

	t.Run("a whole-card entry still resolves by its device node", func(t *testing.T) {
		writeCDISpec(t, uid, `cdiVersion: 0.5.0
kind: k8s.gpu.nvidia.com/claim
devices:
- name: `+uid+`-gpu-0
  containerEdits:
    deviceNodes:
    - path: /dev/nvidia0
      permissions: rw
`)

		got := (&collector{}).resolveCDIToGPUs(card(), "k8s.gpu.nvidia.com/claim="+uid+"-gpu-0")
		require.Equal(t, []string{cardUUID}, got)
	})
}
