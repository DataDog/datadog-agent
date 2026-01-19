// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package containers

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestMatchContainerDevices(t *testing.T) {
	// Setup mock NVML with basic devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Get test devices
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2)

	t.Run("ContainerWithNvidiaGPU", func(t *testing.T) {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[1], // Use device index 1
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 1)
		assert.Equal(t, devices[1], filteredDevices[0])
	})

	t.Run("ContainerWithMultipleNvidiaGPUs", func(t *testing.T) {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-multi",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[0],
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   "nvidia2",
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 2)
		assert.Equal(t, devices[0], filteredDevices[0])
		assert.Equal(t, devices[2], filteredDevices[1])
	})

	t.Run("ContainerWithNonNvidiaResource", func(t *testing.T) {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-non-nvidia",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: "cpu",
					ID:   "cpu-0",
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 0)
	})

	t.Run("ContainerWithNoResources", func(t *testing.T) {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-empty",
			},
			ResolvedAllocatedResources: nil,
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 0)
	})

	t.Run("ContainerWithInvalidGPU", func(t *testing.T) {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-invalid",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   "invalid-uuid",
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.Error(t, err)
		require.Len(t, filteredDevices, 0)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
	})

	t.Run("DockerContainerWithVisibleDevices", func(t *testing.T) {
		useFakeProcfsWithNvidiaVisibleDevices(t, 1, "1")

		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-docker",
			},
			Runtime: workloadmeta.ContainerRuntimeDocker,
			PID:     1,
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 1)
		assert.Equal(t, devices[1], filteredDevices[0])
	})

	t.Run("DockerContainerWithInvalidVisibleDevices", func(t *testing.T) {
		useFakeProcfsWithNvidiaVisibleDevices(t, 1, "invalid")

		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-docker-invalid",
			},
			Runtime: workloadmeta.ContainerRuntimeDocker,
			PID:     1,
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.Error(t, err)
		require.Len(t, filteredDevices, 0)
	})

	t.Run("DockerContainerWithAllVisibleDevices", func(t *testing.T) {
		useFakeProcfsWithNvidiaVisibleDevices(t, 1, "all")

		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-docker-all",
			},
			Runtime: workloadmeta.ContainerRuntimeDocker,
			PID:     1,
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, len(devices))
		for i, device := range devices {
			assert.Equal(t, device, filteredDevices[i])
		}
	})

	t.Run("KubernetesDevicesOrderIsCorrect", func(t *testing.T) {
		// Get test devices with different indices
		devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2, 3, 4)

		// Test with resources in reverse order (highest index first)
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-sorting",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[4], // Device index 4
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[1], // Device index 1
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[3], // Device index 3
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[0], // Device index 0
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[2], // Device index 2
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 5)

		// Verify devices are sorted by index (0, 1, 2, 3, 4)
		for i, device := range filteredDevices {
			expectedIndex := i
			actualIndex := device.GetDeviceInfo().Index
			assert.Equal(t, expectedIndex, actualIndex, "Device at position %d should have index %d, got %d", i, expectedIndex, actualIndex)
		}
	})

	t.Run("KubernetesDevicesSortedByIndexWithMixedResourceTypes", func(t *testing.T) {
		// Get test devices with different indices
		devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2, 3, 4)

		// Test with mixed resource types (GKE and NVIDIA device plugin formats)
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-mixed-sorting",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   "nvidia3", // GKE format - device index 3
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[0], // NVIDIA device plugin format - device index 0
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   "nvidia1", // GKE format - device index 1
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[4], // NVIDIA device plugin format - device index 4
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   "nvidia2", // GKE format - device index 2
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 5)

		// Verify devices are sorted by index (0, 1, 2, 3, 4)
		for i, device := range filteredDevices {
			expectedIndex := i
			actualIndex := device.GetDeviceInfo().Index
			assert.Equal(t, expectedIndex, actualIndex, "Device at position %d should have index %d, got %d", i, expectedIndex, actualIndex)
		}
	})

	t.Run("KubernetesContainerWithMIGDevices", func(t *testing.T) {
		// Get test devices with MIG enabled
		devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, testutil.DevicesWithMIGChildren...)

		// Test with MIG devices
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-mig",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.MIGChildrenUUIDs[5][0],
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.MIGChildrenUUIDs[5][1],
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.MIGChildrenUUIDs[6][0],
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.MIGChildrenUUIDs[6][1],
				},
			},
		}

		physicalDevice1, ok := devices[0].(*ddnvml.PhysicalDevice)
		require.True(t, ok)
		physicalDevice2, ok := devices[1].(*ddnvml.PhysicalDevice)
		require.True(t, ok)
		require.GreaterOrEqual(t, len(physicalDevice1.MIGChildren), 2)
		require.GreaterOrEqual(t, len(physicalDevice2.MIGChildren), 2)
		mig1 := physicalDevice1.MIGChildren[0]
		mig2 := physicalDevice1.MIGChildren[1]
		mig3 := physicalDevice2.MIGChildren[0]
		mig4 := physicalDevice2.MIGChildren[1]
		expectedDevices := []ddnvml.Device{mig1, mig2, mig3, mig4}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 4)
		assert.ElementsMatch(t, filteredDevices, expectedDevices)
	})
}

func useFakeProcfsWithNvidiaVisibleDevices(t *testing.T, pid int, visibleDevices string) {
	procfs := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{
			Pid: uint32(pid),
			Env: map[string]string{
				"NVIDIA_VISIBLE_DEVICES": visibleDevices,
			},
		},
	})

	kernel.WithFakeProcFS(t, procfs)
}

func TestFindDeviceForResourceName(t *testing.T) {
	// Setup mock NVML with basic devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Get test devices
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2)

	t.Run("NvidiaDevicePluginUUID", func(t *testing.T) {
		// Test with NVIDIA device plugin format (UUID)
		device, err := findDeviceForResourceName(devices, testutil.GPUUUIDs[1])
		require.NoError(t, err)
		assert.Equal(t, devices[1], device)
	})

	t.Run("GKEDevicePluginIndex", func(t *testing.T) {
		// Test with GKE device plugin format (nvidiaX)
		device, err := findDeviceForResourceName(devices, "nvidia1")
		require.NoError(t, err)
		assert.Equal(t, devices[1], device)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		// Test with invalid UUID
		_, err := findDeviceForResourceName(devices, "invalid-uuid")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
	})

	t.Run("EmptyResourceID", func(t *testing.T) {
		// Test with empty resource ID
		_, err := findDeviceForResourceName(devices, "")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
	})

	t.Run("UUIDBasedMIGDevice", func(t *testing.T) {
		// Test with MIG device
		devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, testutil.DevicesWithMIGChildren...)
		device, err := findDeviceForResourceName(devices, testutil.MIGChildrenUUIDs[5][0])
		require.NoError(t, err)
		require.Equal(t, device.GetDeviceInfo().UUID, testutil.MIGChildrenUUIDs[5][0])
	})

	t.Run("GKEWithMIGDevice", func(t *testing.T) {
		// Test with MIG device
		devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, testutil.DevicesWithMIGChildren...)
		_, err := findDeviceForResourceName(devices, "nvidia3")
		require.Error(t, err)
	})
}

func TestFindDeviceByUUID(t *testing.T) {
	// Setup mock NVML with basic devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Get test devices
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2)

	t.Run("ValidUUID", func(t *testing.T) {
		device, err := findDeviceByUUID(devices, testutil.GPUUUIDs[1])
		require.NoError(t, err)
		assert.Equal(t, devices[1], device)
	})

	t.Run("ValidUUIDFirstDevice", func(t *testing.T) {
		device, err := findDeviceByUUID(devices, testutil.GPUUUIDs[0])
		require.NoError(t, err)
		assert.Equal(t, devices[0], device)
	})

	t.Run("ValidUUIDLastDevice", func(t *testing.T) {
		device, err := findDeviceByUUID(devices, testutil.GPUUUIDs[2])
		require.NoError(t, err)
		assert.Equal(t, devices[2], device)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		_, err := findDeviceByUUID(devices, "invalid-uuid")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
		assert.Contains(t, err.Error(), "invalid-uuid")
	})

	t.Run("EmptyUUID", func(t *testing.T) {
		_, err := findDeviceByUUID(devices, "")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
		assert.Contains(t, err.Error(), "")
	})

	t.Run("EmptyDeviceList", func(t *testing.T) {
		_, err := findDeviceByUUID([]ddnvml.Device{}, testutil.GPUUUIDs[0])
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
	})
}

func TestFindDeviceByUUIDWithMIG(t *testing.T) {
	// Setup mock NVML with MIG enabled for some devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMock())

	// Get test devices including MIG children
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2, 3, 4, 5, 6)

	t.Run("PhysicalDeviceUUID", func(t *testing.T) {
		device, err := findDeviceByUUID(devices, testutil.GPUUUIDs[0])
		require.NoError(t, err)
		assert.Equal(t, devices[0], device)
	})

	t.Run("MIGChildUUID", func(t *testing.T) {
		// Test finding a MIG child device by UUID
		migUUID := testutil.MIGChildrenUUIDs[5][0] // First MIG child of device 5
		device, err := findDeviceByUUID(devices, migUUID)
		require.NoError(t, err)

		// Verify it's a MIG device
		require.IsType(t, &ddnvml.MIGDevice{}, device)
		deviceInfo := device.GetDeviceInfo()
		assert.Equal(t, migUUID, deviceInfo.UUID)
	})

	t.Run("MIGChildUUIDSecondDevice", func(t *testing.T) {
		// Test finding a MIG child device by UUID from second device
		migUUID := testutil.MIGChildrenUUIDs[6][1] // Second MIG child of device 6
		device, err := findDeviceByUUID(devices, migUUID)
		require.NoError(t, err)

		// Verify it's a MIG device
		deviceInfo := device.GetDeviceInfo()
		assert.Equal(t, migUUID, deviceInfo.UUID)
	})
}

func TestFindDeviceByIndex(t *testing.T) {
	// Setup mock NVML with basic devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Get test devices
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2)

	t.Run("ValidIndex", func(t *testing.T) {
		device, err := findDeviceByIndex(devices, "1")
		require.NoError(t, err)
		assert.Equal(t, devices[1], device)
	})

	t.Run("ValidIndexZero", func(t *testing.T) {
		device, err := findDeviceByIndex(devices, "0")
		require.NoError(t, err)
		assert.Equal(t, devices[0], device)
	})

	t.Run("ValidIndexLast", func(t *testing.T) {
		device, err := findDeviceByIndex(devices, "2")
		require.NoError(t, err)
		assert.Equal(t, devices[2], device)
	})

	t.Run("InvalidIndex", func(t *testing.T) {
		_, err := findDeviceByIndex(devices, "999")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
		assert.Contains(t, err.Error(), "999")
	})

	t.Run("NegativeIndex", func(t *testing.T) {
		_, err := findDeviceByIndex(devices, "-1")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
		assert.Contains(t, err.Error(), "-1")
	})

	t.Run("EmptyDeviceList", func(t *testing.T) {
		_, err := findDeviceByIndex([]ddnvml.Device{}, "0")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
	})

	t.Run("InvalidIndexString", func(t *testing.T) {
		_, err := findDeviceByIndex(devices, "invalid-index")
		require.Error(t, err)
	})

}

func TestMatchByGPUDeviceIDs(t *testing.T) {
	// Setup mock NVML with basic devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Get test devices
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2)

	t.Run("SingleUUID", func(t *testing.T) {
		gpuDeviceIDs := []string{testutil.GPUUUIDs[1]}
		filteredDevices, err := matchByGPUDeviceIDs(gpuDeviceIDs, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 1)
		assert.Equal(t, devices[1], filteredDevices[0])
	})

	t.Run("MultipleUUIDs", func(t *testing.T) {
		gpuDeviceIDs := []string{testutil.GPUUUIDs[2], testutil.GPUUUIDs[0]}
		filteredDevices, err := matchByGPUDeviceIDs(gpuDeviceIDs, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 2)
		// Order preserved from input (matches CUDA device selection order)
		assert.Equal(t, devices[2], filteredDevices[0])
		assert.Equal(t, devices[0], filteredDevices[1])
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		gpuDeviceIDs := []string{"GPU-invalid-uuid"}
		filteredDevices, err := matchByGPUDeviceIDs(gpuDeviceIDs, devices)
		require.Error(t, err)
		require.Len(t, filteredDevices, 0)
		require.ErrorIs(t, err, ErrCannotMatchDevice)
	})

	t.Run("MixedValidAndInvalid", func(t *testing.T) {
		gpuDeviceIDs := []string{testutil.GPUUUIDs[1], "GPU-invalid", testutil.GPUUUIDs[0]}
		filteredDevices, err := matchByGPUDeviceIDs(gpuDeviceIDs, devices)
		require.Error(t, err) // Error for invalid UUID
		require.Len(t, filteredDevices, 2)
		// Order preserved from input (matches CUDA device selection order), invalid skipped
		assert.Equal(t, devices[1], filteredDevices[0])
		assert.Equal(t, devices[0], filteredDevices[1])
	})

	t.Run("EmptyList", func(t *testing.T) {
		gpuDeviceIDs := []string{}
		filteredDevices, err := matchByGPUDeviceIDs(gpuDeviceIDs, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 0)
	})
}

func TestMatchContainerDevicesWithGPUDeviceIDs(t *testing.T) {
	// Setup mock NVML with basic devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Get test devices
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2)

	t.Run("ContainerWithGPUDeviceIDsUUID", func(t *testing.T) {
		// Simulates ECS GPU container with UUID in GPUDeviceIDs
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-ecs-container",
			},
			GPUDeviceIDs: []string{testutil.GPUUUIDs[1]},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 1)
		assert.Equal(t, devices[1], filteredDevices[0])
	})

	t.Run("GPUDeviceIDsTakesPrecedenceOverResolvedAllocatedResources", func(t *testing.T) {
		// GPUDeviceIDs should be used even if ResolvedAllocatedResources is set
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-precedence-container",
			},
			GPUDeviceIDs: []string{testutil.GPUUUIDs[0]}, // Should use this
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[2], // Should NOT use this
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.NoError(t, err)
		require.Len(t, filteredDevices, 1)
		assert.Equal(t, devices[0], filteredDevices[0]) // Should be device 0, not device 2
	})
}

func TestMatchContainerDevicesWithErrors(t *testing.T) {
	// Setup mock NVML with basic devices
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Get test devices
	devices := nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1, 2)

	t.Run("ContainerWithValidAndInvalidGPUs", func(t *testing.T) {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container-mixed-validity",
			},
			ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[1], // Valid GPU
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   "invalid-uuid", // Invalid GPU
				},
				{
					Name: string(gpuutil.GpuNvidiaGeneric),
					ID:   testutil.GPUUUIDs[2], // Valid GPU
				},
			},
		}

		filteredDevices, err := MatchContainerDevices(container, devices)
		require.Error(t, err)              // Should have error due to invalid UUID
		require.Len(t, filteredDevices, 2) // Should still return valid devices
		assert.Equal(t, devices[1], filteredDevices[0])
		assert.Equal(t, devices[2], filteredDevices[1])
		require.ErrorIs(t, err, ErrCannotMatchDevice)
	})
}

func TestIsDatadogAgentContainer(t *testing.T) {
	currentPID := os.Getpid()
	currentPIDStr := strconv.Itoa(currentPID)

	// Helper function to create a container
	makeContainer := func(id string, podID string) *workloadmeta.Container {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   id,
			},
		}
		if podID != "" {
			container.Owner = &workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   podID,
			}
		}
		return container
	}

	// Helper function to set up a process
	setupProcess := func(wmetaMock workloadmetamock.Mock, pidStr string, pid int32, containerOwnerID string) {
		var owner *workloadmeta.EntityID
		if containerOwnerID != "" {
			owner = &workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   containerOwnerID,
			}
		}
		wmetaMock.Set(&workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   pidStr,
			},
			Pid:   pid,
			Owner: owner,
		})
	}

	tests := []struct {
		name             string
		setup            func(wmetaMock workloadmetamock.Mock)
		container        *workloadmeta.Container
		expectedResult   bool
		expectedErrorMsg string
	}{
		{
			name: "ProcessNotFound",
			setup: func(_ workloadmetamock.Mock) {
				// No process set up
			},
			container:      makeContainer("test-container", ""),
			expectedResult: false,
		},
		{
			name: "ProcessHasNoOwner",
			setup: func(wmetaMock workloadmetamock.Mock) {
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), "")
			},
			container:      makeContainer("test-container", ""),
			expectedResult: false,
		},
		{
			name: "ContainerEntityIDMatches",
			setup: func(wmetaMock workloadmetamock.Mock) {
				containerID := "test-container"
				wmetaMock.Set(makeContainer(containerID, ""))
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), containerID)
			},
			container:      makeContainer("test-container", ""),
			expectedResult: true,
		},
		{
			name: "SamePodDifferentContainers",
			setup: func(wmetaMock workloadmetamock.Mock) {
				podID := "test-pod"
				runningContainerID := "running-container"
				otherContainerID := "other-container"
				wmetaMock.Set(makeContainer(runningContainerID, podID))
				wmetaMock.Set(makeContainer(otherContainerID, podID))
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), runningContainerID)
			},
			container:      makeContainer("other-container", "test-pod"),
			expectedResult: true,
		},
		{
			name: "DifferentPods",
			setup: func(wmetaMock workloadmetamock.Mock) {
				runningPodID := "running-pod"
				otherPodID := "other-pod"
				runningContainerID := "running-container"
				otherContainerID := "other-container"
				wmetaMock.Set(makeContainer(runningContainerID, runningPodID))
				wmetaMock.Set(makeContainer(otherContainerID, otherPodID))
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), runningContainerID)
			},
			container:      makeContainer("other-container", "other-pod"),
			expectedResult: false,
		},
		{
			name: "RunningContainerOwnerNil",
			setup: func(wmetaMock workloadmetamock.Mock) {
				runningContainerID := "running-container"
				otherContainerID := "other-container"
				podID := "test-pod"
				wmetaMock.Set(makeContainer(runningContainerID, ""))
				wmetaMock.Set(makeContainer(otherContainerID, podID))
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), runningContainerID)
			},
			container:      makeContainer("other-container", "test-pod"),
			expectedResult: false,
		},
		{
			name: "ContainerOwnerNil",
			setup: func(wmetaMock workloadmetamock.Mock) {
				podID := "test-pod"
				runningContainerID := "running-container"
				otherContainerID := "other-container"
				wmetaMock.Set(makeContainer(runningContainerID, podID))
				wmetaMock.Set(makeContainer(otherContainerID, ""))
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), runningContainerID)
			},
			container:      makeContainer("other-container", ""),
			expectedResult: false,
		},
		{
			name: "BothOwnersNil",
			setup: func(wmetaMock workloadmetamock.Mock) {
				runningContainerID := "running-container"
				otherContainerID := "other-container"
				wmetaMock.Set(makeContainer(runningContainerID, ""))
				wmetaMock.Set(makeContainer(otherContainerID, ""))
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), runningContainerID)
			},
			container:      makeContainer("other-container", ""),
			expectedResult: false,
		},
		{
			name: "ContainerEntityIDDifferentButSamePod",
			setup: func(wmetaMock workloadmetamock.Mock) {
				podID := "test-pod"
				runningContainerID := "agent-container"
				otherContainerID := "system-probe-container"
				wmetaMock.Set(makeContainer(runningContainerID, podID))
				wmetaMock.Set(makeContainer(otherContainerID, podID))
				setupProcess(wmetaMock, currentPIDStr, int32(currentPID), runningContainerID)
			},
			container:      makeContainer("system-probe-container", "test-pod"),
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmetaMock := testutil.GetWorkloadMetaMock(t)
			tt.setup(wmetaMock)
			result := IsDatadogAgentContainer(wmetaMock, tt.container)
			assert.Equal(t, tt.expectedResult, result, tt.expectedErrorMsg)
		})
	}
}
