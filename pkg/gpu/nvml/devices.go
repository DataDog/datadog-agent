// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

// Package nvml contains utilities to wrap usage of the NVML library
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

// NewDevice creates a new Device from an nvml.Device and caches some properties
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

// DeviceCache is a cache of GPU devices, with some methods to easily access devices by UUID or index
type DeviceCache interface {
	// GetDeviceByUUID returns a device by its UUID
	GetDeviceByUUID(uuid string) (*Device, error)
	// GetDeviceByIndex returns a device by its index
	GetDeviceByIndex(index int) (*Device, error)
	// DeviceCount returns the number of devices in the cache
	DeviceCount() int
	// GetSMVersionSet returns a set of all SM versions in the cache
	GetSMVersionSet() map[uint32]struct{}
	// GetAllDevices returns all devices in the cache
	GetAllDevices() []*Device
	// GetCores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
	GetCores(uuid string) (uint64, error)
}

// deviceCache is an implementation of DeviceCache
type deviceCache struct {
	allDevices   []*Device
	uuidToDevice map[string]*Device
	smVersionSet map[uint32]struct{}
}

// NewDeviceCache creates a new DeviceCache
func NewDeviceCache() (DeviceCache, error) {
	lib, err := GetNvmlLib()
	if err != nil {
		return nil, err
	}

	return NewDeviceCacheWithOptions(lib)
}

// NewDeviceCacheWithOptions creates a new DeviceCache with an already initialized NVML library
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

// GetDeviceByUUID returns a device by its UUID
func (c *deviceCache) GetDeviceByUUID(uuid string) (*Device, error) {
	device, ok := c.uuidToDevice[uuid]
	if !ok {
		return nil, fmt.Errorf("device with uuid %s not found", uuid)
	}
	return device, nil
}

// GetDeviceByIndex returns a device by its index in the host
func (c *deviceCache) GetDeviceByIndex(index int) (*Device, error) {
	if index < 0 || index >= len(c.allDevices) {
		return nil, fmt.Errorf("index %d out of range", index)
	}

	return c.allDevices[index], nil
}

// DeviceCount returns the number of devices in the cache
func (c *deviceCache) DeviceCount() int {
	return len(c.allDevices)
}

// GetSMVersionSet returns a set of all SM versions in the cache
func (c *deviceCache) GetSMVersionSet() map[uint32]struct{} {
	return c.smVersionSet
}

// GetAllDevices returns all devices in the cache
func (c *deviceCache) GetAllDevices() []*Device {
	return c.allDevices
}

// GetCores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
func (c *deviceCache) GetCores(uuid string) (uint64, error) {
	device, err := c.GetDeviceByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return uint64(device.CoreCount), nil
}
