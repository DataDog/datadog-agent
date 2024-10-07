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

type WithUUID interface {
	GetUUID() (string, nvml.Return)
}

// getVisibleDevices processes the list of GPU devices according to the value of the CUDA_VISIBLE_DEVICES environment variable
// Reference: https://docs.nvidia.com/cuda/cuda-c-programming-guide/index.html#env-vars
func getVisibleDevices[T WithUUID](systemDevices []T, cudaVisibleDevices string) ([]T, error) {
	if cudaVisibleDevices == "" {
		return systemDevices, nil
	}

	var filteredDevices []T
	visibleDevicesList := strings.Split(cudaVisibleDevices, ",")

	for _, visibleDevice := range visibleDevicesList {
		if strings.HasPrefix(visibleDevice, "GPU-") {
			for _, device := range systemDevices {
				uuid, ret := device.GetUUID()
				if ret != nvml.SUCCESS {
					return nil, fmt.Errorf("cannot get UUID for device: %w", WrapNvmlError(ret))
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

func getCudaVisibleDevicesEnvVar(pid int, procfs string) (string, error) {
	return kernel.GetProcessEnvVariable(pid, "CUDA_VISIBLE_DEVICES", procfs)
}

func GetVisibleDevicesForProcess[T WithUUID](systemDevices []T, pid int, procfs string) ([]T, error) {
	cudaVisibleDevices, err := getCudaVisibleDevicesEnvVar(pid, procfs)
	if err != nil {
		return nil, fmt.Errorf("cannot get CUDA_VISIBLE_DEVICES for process %d: %w", pid, err)
	}

	return getVisibleDevices(systemDevices, cudaVisibleDevices)
}
