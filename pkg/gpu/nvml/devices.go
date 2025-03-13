// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

// package nvml contains utilities to wrap usage of the NVML library
package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Device represents a GPU device with some properties already computed
type Device struct {
	nvml.Device

	SMVersion uint32
	UUID      string
	CoreCount int
	Index     int
}

func NewDevice(dev nvml.Device) (*Device, error) {
	major, minor, ret := dev.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting SM version: %s", nvml.ErrorString(ret))
	}
	smVersion := uint32(major*10 + minor)

	uuid, ret := dev.GetUUID()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting UUID: %s", nvml.ErrorString(ret))
	}

	cores, ret := dev.GetNumGpuCores()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting core count: %s", nvml.ErrorString(ret))
	}

	index, ret := dev.GetIndex()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting index: %s", nvml.ErrorString(ret))
	}

	return &Device{
		Device:    dev,
		SMVersion: smVersion,
		UUID:      uuid,
		CoreCount: cores,
		Index:     index,
	}, nil
}

type DeviceCache interface {
	GetDeviceByUUID(uuid string) (*Device, error)
	GetDeviceByIndex(index int) (*Device, error)
	DeviceCount() int
	GetSMVersionSet() map[uint32]struct{}
	GetAllDevices() []*Device

	GetCores(uuid string) (uint64, error)
}

type deviceCache struct {
	allDevices   []*Device
	uuidToDevice map[string]*Device
	smVersionSet map[uint32]struct{}
}

func NewDeviceCache() (DeviceCache, error) {
	lib, err := GetNvmlLib()
	if err != nil {
		return nil, err
	}

	return NewDeviceCacheWithOptions(lib)
}

func NewDeviceCacheWithOptions(nvmlLib nvml.Interface) (DeviceCache, error) {
	cache := &deviceCache{
		uuidToDevice: make(map[string]*Device),
	}

	count, ret := nvmlLib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting device count: %s", nvml.ErrorString(ret))
	}

	for i := 0; i < count; i++ {
		dev, ret := nvmlLib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("error getting device by index: %s", nvml.ErrorString(ret))
		}

		device, err := NewDevice(dev)
		if err != nil {
			return nil, fmt.Errorf("error creating device index %d: %s", i, err)
		}

		cache.uuidToDevice[device.UUID] = device
		cache.allDevices = append(cache.allDevices, device)
		cache.smVersionSet[device.SMVersion] = struct{}{}
	}

	return cache, nil
}

func (c *deviceCache) GetDeviceByUUID(uuid string) (*Device, error) {
	device, ok := c.uuidToDevice[uuid]
	if !ok {
		return nil, fmt.Errorf("device with uuid %s not found", uuid)
	}
	return device, nil
}

func (c *deviceCache) GetDeviceByIndex(index int) (*Device, error) {
	if index < 0 || index >= len(c.allDevices) {
		return nil, fmt.Errorf("index %d out of range", index)
	}

	return c.allDevices[index], nil
}

func (c *deviceCache) DeviceCount() int {
	return len(c.allDevices)
}

func (c *deviceCache) GetSMVersionSet() map[uint32]struct{} {
	return c.smVersionSet
}

func (c *deviceCache) GetAllDevices() []*Device {
	return c.allDevices
}

func (c *deviceCache) GetCores(uuid string) (uint64, error) {
	device, err := c.GetDeviceByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return uint64(device.CoreCount), nil
}
