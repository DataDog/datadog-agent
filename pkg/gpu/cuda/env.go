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

// getVisibleDevices processes the list of GPU devices according to the value of the CUDA_VISIBLE_DEVICES environment variable
// Reference: https://docs.nvidia.com/cuda/cuda-c-programming-guide/index.html#env-vars
func getVisibleDevices(systemDevices []nvml.Device, cudaVisibleDevices string) ([]nvml.Device, error) {
	if cudaVisibleDevices == "" {
		return systemDevices, nil
	}

	var filteredDevices []nvml.Device
	visibleDevicesList := strings.Split(cudaVisibleDevices, ",")

	for _, visibleDevice := range visibleDevicesList {
		if strings.HasPrefix(visibleDevice, "GPU-") {
			for _, device := range systemDevices {
				uuid, ret := device.GetUUID()
				if ret != nvml.SUCCESS {
					return nil, fmt.Errorf("cannot get UUID for device: %s", nvml.ErrorString(ret))
				}

				if strings.HasPrefix(uuid, visibleDevice) {
					filteredDevices = append(filteredDevices, device)
					break
				}
			}
		} else if strings.HasPrefix(visibleDevice, "MIG-GPU") {
			return nil, fmt.Errorf("MIG devices are not supported")
		} else {
			idx, err := strconv.Atoi(visibleDevice)
			if err != nil {
				return nil, fmt.Errorf("invalid device index %s: %w", visibleDevice, err)
			}

			if idx < 0 || idx >= len(systemDevices) {
				return nil, fmt.Errorf("device index %d is out of range", idx)
			}

			filteredDevices = append(filteredDevices, systemDevices[idx])
		}
	}

	return filteredDevices, nil
}

// GetVisibleDevicesForProcess modifies the list of GPU devices according to the value of the CUDA_VISIBLE_DEVICES environment variable for the specified process
func GetVisibleDevicesForProcess(systemDevices []nvml.Device, pid int, procfs string) ([]nvml.Device, error) {
	cudaVisibleDevices, err := kernel.GetProcessEnvVariable(pid, cudaVisibleDevicesEnvVar, procfs)
	if err != nil {
		return nil, fmt.Errorf("cannot get CUDA_VISIBLE_DEVICES for process %d: %w", pid, err)
	}

	return getVisibleDevices(systemDevices, cudaVisibleDevices)
}
