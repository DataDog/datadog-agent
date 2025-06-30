// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package cuda

import (
	"fmt"
	"strconv"
	"strings"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

// CudaVisibleDevicesEnvVar is the name of the environment variable that controls the visible GPUs for CUDA applications
const CudaVisibleDevicesEnvVar = "CUDA_VISIBLE_DEVICES"

// ParseVisibleDevices modifies the list of GPU devices according to the
// value of the CUDA_VISIBLE_DEVICES environment variable. Reference:
// https://docs.nvidia.com/cuda/cuda-c-programming-guide/index.html#env-vars.
//
// As a summary, the CUDA_VISIBLE_DEVICES environment variable should be a comma
// separated list of GPU identifiers. These can be either the index of the GPU
// (0, 1, 2) or the UUID of the GPU (GPU-<UUID>, or
// MIG-GPU-<UUID>/<instance-index>/<compute-index for multi-instance GPUs). UUID
// identifiers do not need to be the full UUID, it is enough with specifying the
// prefix that uniquely identifies the GPU.
//
// Invalid device indexes are ignored, and anything that comes after that is
// invisible, following the spec: "If one of the indices is invalid, only the
// devices whose index precedes the invalid index are visible to CUDA
// applications." If an invalid index is found, an error is returned together
// with the list of valid devices found up until that point.
func ParseVisibleDevices(devices []ddnvml.Device, cudaVisibleDevicesForProcess string) ([]ddnvml.Device, error) {
	// First, we adjust the list of devices to take into account how CUDA presents MIG devices in order. This
	// list will not be used when searching by prefix because prefix matching is done against *all* devices,
	// but index filtering is done against the adjusted list where devices with MIG children are replaced by
	// their first child.
	visibleDevicesWithMig := getVisibleDevicesForMig(devices)

	if cudaVisibleDevicesForProcess == "" {
		return applyMIGVisibilityRules(visibleDevicesWithMig), nil
	}

	var filteredDevices []ddnvml.Device
	visibleDevicesList := strings.Split(cudaVisibleDevicesForProcess, ",")

	for _, visibleDevice := range visibleDevicesList {
		var matchingDevice ddnvml.Device
		var err error
		switch {
		case strings.HasPrefix(visibleDevice, "GPU-"):
			matchingDevice, err = getDeviceByUUIDPrefix(devices, visibleDevice, false)
			if err != nil {
				return filteredDevices, err
			}

			// Selecting a parent device when there are MIG children just returns the first MIG child,
			physicalDevice, ok := matchingDevice.(*ddnvml.PhysicalDevice)
			if ok && physicalDevice.HasMIGFeatureEnabled && len(physicalDevice.MIGChildren) > 0 {
				matchingDevice = physicalDevice.MIGChildren[0]
			}

		case strings.HasPrefix(visibleDevice, "MIG-"):
			matchingDevice, err = getDeviceByUUIDPrefix(devices, visibleDevice, true)
			if err != nil {
				return filteredDevices, err
			}
		default:
			matchingDevice, err = getDeviceWithIndex(visibleDevicesWithMig, visibleDevice)
			if err != nil {
				return filteredDevices, err
			}
		}

		filteredDevices = append(filteredDevices, matchingDevice)
	}

	return applyMIGVisibilityRules(filteredDevices), nil
}

// getVisibleDevicesForMig returns a list of devices taking into account how CUDA
// presents MIG devices in order
func getVisibleDevicesForMig(visibleDevices []ddnvml.Device) []ddnvml.Device {
	// CUDA removes all devices with MIG feature enabled and then appends at the
	// end of the list the first MIG child of those devices. So we split the
	// list into two: one with devices without MIG feature enabled (which
	// includes physical device with MIG disabled and MIG children of devices
	// with MIG enabled) and one with MIG children of MIG-enabled physical
	// devices.
	var migDisabledDevices []ddnvml.Device
	var migChildDevices []ddnvml.Device

	for _, device := range visibleDevices {
		physicalDevice, ok := device.(*ddnvml.PhysicalDevice)
		if !ok || !physicalDevice.HasMIGFeatureEnabled {
			// either a MIG device or a physical device without MIG feature enabled
			migDisabledDevices = append(migDisabledDevices, device)
		} else if len(physicalDevice.MIGChildren) > 0 {
			// a MIG-enabled physical device, add the first MIG child
			migChildDevices = append(migChildDevices, physicalDevice.MIGChildren[0])
		}
	}

	// Now merge the two lists
	return append(migDisabledDevices, migChildDevices...)
}

// applyMIGVisibilityRules returns the list of devices with only the first MIG child if it is present,
// or the list of all devices if it is not. This replicates the behavior of CUDA with MIG devices.
// Returns:
// - If any MIG device exists: only the first MIG device found
// - If no MIG devices exist: all devices unchanged
func applyMIGVisibilityRules(devices []ddnvml.Device) []ddnvml.Device {
	for _, device := range devices {
		migDevice, ok := device.(*ddnvml.MIGDevice)
		if ok {
			return []ddnvml.Device{migDevice}
		}
	}

	return devices
}

// getDeviceByUUIDPrefix returns the first device with a UUID that
// matches the given prefix. If there are multiple devices with the same prefix
// or the device is not found, an error is returned.
func getDeviceByUUIDPrefix(systemDevices []ddnvml.Device, uuidPrefix string, searchMigChildren bool) (ddnvml.Device, error) {
	var matchingDevice ddnvml.Device
	var matchingDeviceUUID string

	for _, device := range systemDevices {
		if strings.HasPrefix(device.GetDeviceInfo().UUID, uuidPrefix) {
			if matchingDevice != nil {
				return nil, fmt.Errorf("non-unique UUID prefix %s, found UUIDs %s and %s", uuidPrefix, matchingDeviceUUID, device.GetDeviceInfo().UUID)
			}
			matchingDevice = device
			matchingDeviceUUID = device.GetDeviceInfo().UUID
		}

		if searchMigChildren {
			physicalDevice, ok := device.(*ddnvml.PhysicalDevice)
			if !ok {
				continue
			}

			for _, migChild := range physicalDevice.MIGChildren {
				if strings.HasPrefix(migChild.GetDeviceInfo().UUID, uuidPrefix) {
					if matchingDevice != nil {
						return nil, fmt.Errorf("non-unique UUID prefix %s, found UUIDs %s and %s", uuidPrefix, matchingDeviceUUID, migChild.GetDeviceInfo().UUID)
					}
					matchingDevice = migChild
					matchingDeviceUUID = migChild.GetDeviceInfo().UUID
				}
			}
		}
	}

	if matchingDevice == nil {
		return nil, fmt.Errorf("device with UUID prefix %s not found", uuidPrefix)
	}

	return matchingDevice, nil
}

// getDeviceWithIndex returns the device with the given index. If the index is
// out of range or the index is not a number, an error is returned.
func getDeviceWithIndex(systemDevices []ddnvml.Device, visibleDevice string) (ddnvml.Device, error) {
	idx, err := strconv.Atoi(visibleDevice)
	if err != nil {
		return nil, fmt.Errorf("invalid device index %s: %w", visibleDevice, err)
	}

	if idx < 0 || idx >= len(systemDevices) {
		return nil, fmt.Errorf("device index %d is out of range [0, %d)", idx, len(systemDevices))
	}

	return systemDevices[idx], nil
}
