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

func (d *gpuDevice) GetNumMultiprocessors() (int, error) {
	devProps, ret := d.GetAttributes()
	if err := wrapNvmlError(ret); err != nil {
		return 0, fmt.Errorf("cannot get device attributes: %w", err)
	}

	return int(devProps.MultiprocessorCount), nil
}

func (d *gpuDevice) GetMaxThreads() (int, error) {
	cores, ret := d.GetNumGpuCores()
	if err := wrapNvmlError(ret); err != nil {
		return 0, fmt.Errorf("cannot get number of GPU cores: %w", err)
	}

	return int(cores), nil
}
