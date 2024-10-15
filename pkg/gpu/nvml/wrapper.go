// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvml

import (
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var nvmlWrapper = &nvidiaNvmlWrapper{}

// GetLibrary returns an NVML library instance
func GetLibrary() (Library, error) {
	if err := nvmlWrapper.Init(); err != nil {
		return nil, fmt.Errorf("cannot initialize NVML library: %w", err)
	}

	return nvmlWrapper, nil
}

type nvidiaNvmlWrapper struct {
	initOnce sync.Once
}

func (w *nvidiaNvmlWrapper) Init() error {
	var err error
	w.initOnce.Do(func() {
		err = wrapError(nvml.Init())
	})

	return err
}

func (w *nvidiaNvmlWrapper) Shutdown() error {
	return wrapError(nvml.Shutdown())
}

func (w *nvidiaNvmlWrapper) GetGpuDevices() ([]GpuDevice, error) {
	count, ret := nvml.DeviceGetCount()
	if err := wrapError(ret); err != nil {
		return nil, fmt.Errorf("cannot get number of GPU devices: %w", err)
	}

	var devices []GpuDevice

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if err := wrapError(ret); err != nil {
			return nil, fmt.Errorf("cannot get handle for GPU device %d: %w", i, err)
		}

		devices = append(devices, &gpuDevice{device})
	}

	return devices, nil
}

type gpuDevice struct {
	nvml.Device
}

// GetNumMultiprocessors returns the number of multiprocessors on the GPU device
func (d *gpuDevice) GetNumMultiprocessors() (int, error) {
	devProps, ret := d.GetAttributes()
	if err := wrapError(ret); err != nil {
		return 0, fmt.Errorf("cannot get device attributes: %w", err)
	}

	return int(devProps.MultiprocessorCount), nil
}

// GetMaxThreads returns the maximum number of threads per multiprocessor on the GPU device
func (d *gpuDevice) GetMaxThreads() (int, error) {
	cores, ret := d.GetNumGpuCores()
	if err := wrapError(ret); err != nil {
		return 0, fmt.Errorf("cannot get number of GPU cores: %w", err)
	}

	return int(cores), nil
}
