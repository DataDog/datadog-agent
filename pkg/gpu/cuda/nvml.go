// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package cuda

import (
	"errors"
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var initOnce sync.Once

// GpuDevice wraps the nvml.Device struct to provide additional functionality.
type GpuDevice struct {
	nvml.Device
}

// WrapNvmlError wraps an nvml.Return value in a Go error
func WrapNvmlError(ret nvml.Return) error {
	if ret == nvml.SUCCESS {
		return nil
	}

	return errors.New(nvml.ErrorString(ret))
}

func ensureNvmlInit() error {
	var err error
	initOnce.Do(func() {
		err = WrapNvmlError(nvml.Init())
	})

	return err
}

// GetGPUDevices returns a list of GPU devices on the system.
func GetGPUDevices() ([]GpuDevice, error) {
	err := ensureNvmlInit()
	if err != nil {
		return nil, err
	}

	count, ret := nvml.DeviceGetCount()
	if err := WrapNvmlError(ret); err != nil {
		return nil, fmt.Errorf("cannot get number of GPU devices: %w", err)
	}

	var devices []GpuDevice

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if err := WrapNvmlError(ret); err != nil {
			return nil, fmt.Errorf("cannot get handle for GPU device %d: %w", i, err)
		}

		devices = append(devices, GpuDevice{device})
	}

	return devices, nil
}

// GetNumMultiprocessors returns the number of multiprocessors on the GPU.
func (d *GpuDevice) GetNumMultiprocessors() (int, error) {
	devProps, ret := d.GetAttributes()
	if err := WrapNvmlError(ret); err != nil {
		return 0, fmt.Errorf("cannot get device attributes: %w", err)
	}

	return int(devProps.MultiprocessorCount), nil
}

// GetMaxThreads returns the maximum number of threads that can run concurrently on the GPU.
func (d *GpuDevice) GetMaxThreads() (int, error) {
	cores, ret := d.GetNumGpuCores()
	if err := WrapNvmlError(ret); err != nil {
		return 0, fmt.Errorf("cannot get number of GPU cores: %w", err)
	}

	return int(cores), nil
}
