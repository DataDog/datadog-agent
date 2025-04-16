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

	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

const cudaVisibleDevicesEnvVar = "CUDA_VISIBLE_DEVICES"

// GetVisibleDevicesForProcess modifies the list of GPU devices according to the
// value of the CUDA_VISIBLE_DEVICES environment variable for the specified
// process. Reference:
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
func GetVisibleDevicesForProcess(systemDevices []*ddnvml.Device, pid int, procfs string) ([]*ddnvml.Device, error) {
	cudaVisibleDevices, err := kernel.GetProcessEnvVariable(pid, procfs, cudaVisibleDevicesEnvVar)
	if err != nil {
		return nil, fmt.Errorf("cannot get env var %s for process %d: %w", cudaVisibleDevicesEnvVar, pid, err)
	}

	return getVisibleDevices(systemDevices, cudaVisibleDevices)
}

// getVisibleDevices processes the list of GPU devices according to the value of
// the CUDA_VISIBLE_DEVICES environment variable
func getVisibleDevices(systemDevices []*ddnvml.Device, cudaVisibleDevices string) ([]*ddnvml.Device, error) {
	// First, we adjust the list of devices to take into account how CUDA presents MIG devices in order. This
	// list will not be used when searching by prefix, but it will be used when filtering by index or when
	// CUDA_VISIBLE_DEVICES is not set.
	migAdjustedDevices := adjustVisibleDevicesForMigDevices(systemDevices)

	if cudaVisibleDevices == "" {
		return keepEitherFirstMIGOrAllDevices(migAdjustedDevices), nil
	}

	var filteredDevices []*ddnvml.Device
	visibleDevicesList := strings.Split(cudaVisibleDevices, ",")

	for _, visibleDevice := range visibleDevicesList {
		var matchingDevice *ddnvml.Device
		var err error
		switch {
		case strings.HasPrefix(visibleDevice, "GPU-"):
			matchingDevice, err = getDeviceWithMatchingUUIDPrefix(systemDevices, visibleDevice)
			if err != nil {
				return filteredDevices, err
			}
		case strings.HasPrefix(visibleDevice, "MIG-"):
			matchingDevice, err = getMigDeviceWithMatchingUUIDPrefix(systemDevices, visibleDevice)
			if err != nil {
				return filteredDevices, err
			}
		default:
			matchingDevice, err = getDeviceWithIndex(migAdjustedDevices, visibleDevice)
			if err != nil {
				return filteredDevices, err
			}
		}

		filteredDevices = append(filteredDevices, matchingDevice)
	}

	return keepEitherFirstMIGOrAllDevices(filteredDevices), nil
}

// adjustVisibleDevicesForMigDevices adjusts the list of visible devices taking into account how CUDA
// presents MIG devices in order
func adjustVisibleDevicesForMigDevices(visibleDevices []*ddnvml.Device) []*ddnvml.Device {
	var adjustedList []*ddnvml.Device

	// First, we add only the non-MIG enabled devices to the adjusted list, as those are seen first
	for _, device := range visibleDevices {
		if !device.HasMIGFeatureEnabled {
			adjustedList = append(adjustedList, device)
		}
	}

	// Then, for every MIG-enabled device, we add only the first MIG child to the adjusted list.
	for _, device := range visibleDevices {
		if device.HasMIGFeatureEnabled && len(device.MIGChildren) > 0 {
			adjustedList = append(adjustedList, device.MIGChildren[0])
		}
	}

	return adjustedList
}

// keepEitherFirstMIGOrAllDevices returns the list of devices with only the first MIG child if it is present,
// or the list of all devices if it is not. This replicates the behavior of CUDA with MIG devices, where
// if any MIG device is present, only the first MIG child is visible and all other devices are hidden.
func keepEitherFirstMIGOrAllDevices(devices []*ddnvml.Device) []*ddnvml.Device {
	for _, device := range devices {
		if device.IsMIG {
			return []*ddnvml.Device{device}
		}
	}

	return devices
}

// getDeviceWithMatchingUUIDPrefix returns the first device with a UUID that
// matches the given prefix. If there are multiple devices with the same prefix
// or the device is not found, an error is returned.
func getDeviceWithMatchingUUIDPrefix(systemDevices []*ddnvml.Device, uuidPrefix string) (*ddnvml.Device, error) {
	var matchingDevice *ddnvml.Device
	var matchingDeviceUUID string

	for _, device := range systemDevices {
		if strings.HasPrefix(device.UUID, uuidPrefix) {
			if matchingDevice != nil {
				return nil, fmt.Errorf("non-unique UUID prefix %s, found UUIDs %s and %s", uuidPrefix, matchingDeviceUUID, device.UUID)
			}
			matchingDevice = device
			matchingDeviceUUID = device.UUID
		}
	}

	if matchingDevice == nil {
		return nil, fmt.Errorf("device with UUID prefix %s not found", uuidPrefix)
	}

	return matchingDevice, nil
}

func getMigDeviceWithMatchingUUIDPrefix(systemDevices []*ddnvml.Device, uuidPrefix string) (*ddnvml.Device, error) {
	var matchingDevice *ddnvml.Device
	var matchingDeviceUUID string

	for _, device := range systemDevices {
		for _, migChild := range device.MIGChildren {
			if strings.HasPrefix(migChild.UUID, uuidPrefix) {
				if matchingDevice != nil {
					return nil, fmt.Errorf("non-unique UUID prefix %s, found UUIDs %s and %s", uuidPrefix, matchingDeviceUUID, migChild.UUID)
				}
				matchingDevice = migChild
				matchingDeviceUUID = migChild.UUID
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
func getDeviceWithIndex(systemDevices []*ddnvml.Device, visibleDevice string) (*ddnvml.Device, error) {
	idx, err := strconv.Atoi(visibleDevice)
	if err != nil {
		return nil, fmt.Errorf("invalid device index %s: %w", visibleDevice, err)
	}

	if idx < 0 || idx >= len(systemDevices) {
		return nil, fmt.Errorf("device index %d is out of range [0, %d)", idx, len(systemDevices))
	}

	return systemDevices[idx], nil
}
