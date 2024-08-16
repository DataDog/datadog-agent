// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpu defines the agent corecheck for
// the GPU integration

package gpu

import (
	"errors"
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var initOnce sync.Once

type gpuDevice struct {
	nvml.Device
}

func wrapNvmlError(ret nvml.Return) error {
	if ret == nvml.SUCCESS {
		return nil
	}

	return errors.New(nvml.ErrorString(ret))
}

func ensureNvmlInit() error {
	var err error
	initOnce.Do(func() {
		err = wrapNvmlError(nvml.Init())
	})

	return err
}

func getGPUDevices() ([]gpuDevice, error) {
	err := ensureNvmlInit()
	if err != nil {
		return nil, err
	}

	count, ret := nvml.DeviceGetCount()
	if err := wrapNvmlError(ret); err != nil {
		return nil, fmt.Errorf("cannot get number of GPU devices: %w", err)
	}

	var devices []gpuDevice

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if err := wrapNvmlError(ret); err != nil {
			return nil, fmt.Errorf("cannot get handle for GPU device %d: %w", i, err)
		}

		devices = append(devices, gpuDevice{device})
	}

	return devices, nil
}

func getThreadsPerMultiprocessor(cudaMajor, cudaMinor int) (int, error) {
	// This information is not provided by the NVML API (or any other API) so we have to hardcode
	// https://docs.nvidia.com/cuda/cuda-c-programming-guide/index.html#features-and-technical-specifications-technical-specifications-per-compute-capability
	// https://en.wikipedia.org/wiki/CUDA#Technical_Specification for older versions
	// Look for "Maximum number of resident threads per SM"
	if cudaMajor >= 5 && cudaMajor < 7 {
		return 2048, nil
	} else if cudaMajor == 7 {
		if cudaMinor < 5 {
			return 2048, nil
		} else {
			return 1024, nil
		}
	} else if cudaMajor == 8 {
		if cudaMinor == 0 {
			return 2048, nil
		} else {
			return 1536, nil
		}
	} else if cudaMajor == 9 {
		return 2048, nil
	}

	return 0, fmt.Errorf("unsupported CUDA version %d.%d", cudaMajor, cudaMinor)
}

func (d *gpuDevice) GetMaxThreads() (int, error) {
	major, minor, ret := d.GetCudaComputeCapability()
	if err := wrapNvmlError(ret); err != nil {
		return 0, fmt.Errorf("cannot get CUDA compute capability: %w", err)
	}

	threadsPerSM, err := getThreadsPerMultiprocessor(major, minor)
	if err != nil {
		return 0, fmt.Errorf("cannot get threads per SM: %w", err)
	}

	multiprocessors, ret := d.GetNumGpuCores()
	if err := wrapNvmlError(ret); err != nil {
		return 0, fmt.Errorf("cannot get number of GPU cores: %w", err)
	}

	return threadsPerSM * multiprocessors, nil
}
