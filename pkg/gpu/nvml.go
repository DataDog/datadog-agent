// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

// Package gpu defines the agent corecheck for
// the GPU integration
package gpu

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func wrapNvmlError(ret nvml.Return) error {
	if ret == nvml.SUCCESS {
		return nil
	}

	return errors.New(nvml.ErrorString(ret))
}

func getGPUDevices(lib nvml.Interface) ([]nvml.Device, error) {
	count, ret := lib.DeviceGetCount()
	if err := wrapNvmlError(ret); err != nil {
		return nil, fmt.Errorf("cannot get number of GPU devices: %w", err)
	}

	var devices []nvml.Device

	for i := 0; i < count; i++ {
		device, ret := lib.DeviceGetHandleByIndex(i)
		if err := wrapNvmlError(ret); err != nil {
			return nil, fmt.Errorf("cannot get handle for GPU device %d: %w", i, err)
		}

		devices = append(devices, device)
	}

	return devices, nil
}

// GetMaxThreads returns the maximum number of threads that can be run on the
// GPU. Each GPU core runs a thread, so this is the number of cores. Do not
// confuse the number of cores with the number of streaming multiprocessors
// (SM): the number of cores is equal to the number of SMs multiplied by the
// number of cores per SM.
func getMaxThreadsForDevice(device nvml.Device) (int, error) {
	cores, ret := device.GetNumGpuCores()
	if err := wrapNvmlError(ret); err != nil {
		return 0, fmt.Errorf("cannot get number of GPU cores: %w", err)
	}

	return int(cores), nil
}
