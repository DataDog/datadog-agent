// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package cuda

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
func GetVisibleDevicesForProcess(systemDevices []nvml.Device, pid int, procfs string) ([]nvml.Device, error) {
	cudaVisibleDevices, err := kernel.GetProcessEnvVariable(pid, procfs, cudaVisibleDevicesEnvVar)
	if err != nil {
		return nil, fmt.Errorf("cannot get env var %s for process %d: %w", cudaVisibleDevicesEnvVar, pid, err)
	}

	return getVisibleDevices(systemDevices, cudaVisibleDevices)
}

// getVisibleDevices processes the list of GPU devices according to the value of
// the CUDA_VISIBLE_DEVICES environment variable
func getVisibleDevices(systemDevices []nvml.Device, cudaVisibleDevices string) ([]nvml.Device, error) {
	if cudaVisibleDevices == "" {
		return systemDevices, nil
	}

	var filteredDevices []nvml.Device
	visibleDevicesList := strings.Split(cudaVisibleDevices, ",")

	for _, visibleDevice := range visibleDevicesList {
		var matchingDevice nvml.Device
		var err error
		switch {
		case strings.HasPrefix(visibleDevice, "GPU-"):
			matchingDevice, err = getDeviceWithMatchingUUIDPrefix(systemDevices, visibleDevice)
			if err != nil {
				return filteredDevices, err
			}
		case strings.HasPrefix(visibleDevice, "MIG-GPU"):
			// MIG (Multi Instance GPUs) devices require extra parsing and data
			// about the MIG instance assignment, which is not supported yet.
			return filteredDevices, fmt.Errorf("MIG devices are not supported")
		default:
			matchingDevice, err = getDeviceWithIndex(systemDevices, visibleDevice)
			if err != nil {
				return filteredDevices, err
			}
		}

		filteredDevices = append(filteredDevices, matchingDevice)
	}

	return filteredDevices, nil
}

// getDeviceWithMatchingUUIDPrefix returns the first device with a UUID that
// matches the given prefix. If there are multiple devices with the same prefix
// or the device is not found, an error is returned.
func getDeviceWithMatchingUUIDPrefix(systemDevices []nvml.Device, uuidPrefix string) (nvml.Device, error) {
	var matchingDevice nvml.Device
	var matchingDeviceUUID string

	for _, device := range systemDevices {
		uuid, ret := device.GetUUID()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("cannot get UUID for device: %s", nvml.ErrorString(ret))
		}

		if strings.HasPrefix(uuid, uuidPrefix) {
			if matchingDevice != nil {
				return nil, fmt.Errorf("non-unique UUID prefix %s, found UUIDs %s and %s", uuidPrefix, matchingDeviceUUID, uuid)
			}
			matchingDevice = device
			matchingDeviceUUID = uuid
		}
	}

	if matchingDevice == nil {
		return nil, fmt.Errorf("device with UUID prefix %s not found", uuidPrefix)
	}

	return matchingDevice, nil
}

// getDeviceWithIndex returns the device with the given index. If the index is
// out of range or the index is not a number, an error is returned.
func getDeviceWithIndex(systemDevices []nvml.Device, visibleDevice string) (nvml.Device, error) {
	idx, err := strconv.Atoi(visibleDevice)
	if err != nil {
		return nil, fmt.Errorf("invalid device index %s: %w", visibleDevice, err)
	}

	if idx < 0 || idx >= len(systemDevices) {
		return nil, fmt.Errorf("device index %d is out of range [0, %d]", idx, len(systemDevices)-1)
	}

	return systemDevices[idx], nil
}
