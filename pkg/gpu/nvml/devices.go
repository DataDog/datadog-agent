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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Device represents a GPU device with some properties already computed
type Device struct {
	// NVMLDevice is the underlying NVML device. While it would make more sense to embed it,
	// that causes this type to include all the methods of the nvml.Device, which makes it
	// heavier than it needs to be and causes a binary size increase. As we're not using this
	// type as a drop-in replacement for the nvml.Device in too many places, it is not
	// too problematic to have it as a separate field.
	NVMLDevice nvml.Device

	SMVersion uint32
	UUID      string
	Name      string
	CoreCount int
	Index     int
	Memory    uint64
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

	name, ret := dev.GetName()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting name: %s", nvml.ErrorString(ret))
	}

	cores, ret := dev.GetNumGpuCores()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting core count: %s", nvml.ErrorString(ret))
	}

	index, ret := dev.GetIndex()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting index: %s", nvml.ErrorString(ret))
	}

	memInfo, ret := dev.GetMemoryInfo()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting memory info: %s", nvml.ErrorString(ret))
	}

	return &Device{
		NVMLDevice: dev,
		SMVersion:  smVersion,
		UUID:       uuid,
		Name:       name,
		CoreCount:  cores,
		Index:      index,
		Memory:     memInfo.Total,
	}, nil
}

// DeviceCache is a cache of GPU devices, with some methods to easily access devices by UUID or index
type DeviceCache interface {
	// GetByUUID returns a device by its UUID
	GetByUUID(uuid string) (*Device, bool)
	// GetByIndex returns a device by its index
	GetByIndex(index int) (*Device, error)
	// Count returns the number of devices in the cache
	Count() int
	// SMVersionSet returns a set of all SM versions in the cache
	SMVersionSet() map[uint32]struct{}
	// All returns all devices in the cache
	All() []*Device
	// Cores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
	Cores(uuid string) (uint64, error)
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
		smVersionSet: make(map[uint32]struct{}),
	}

	count, ret := nvmlLib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting device count: %s", nvml.ErrorString(ret))
	}

	for i := 0; i < count; i++ {
		dev, ret := nvmlLib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			log.Warnf("error getting device by index %d: %s", i, nvml.ErrorString(ret))
			continue
		}

		device, err := NewDevice(dev)
		if err != nil {
			log.Warnf("error creating device index %d: %s", i, err)
			continue
		}

		cache.uuidToDevice[device.UUID] = device
		cache.allDevices = append(cache.allDevices, device)
		cache.smVersionSet[device.SMVersion] = struct{}{}
	}

	return cache, nil
}

// GetByUUID returns a device by its UUID
func (c *deviceCache) GetByUUID(uuid string) (*Device, bool) {
	device, ok := c.uuidToDevice[uuid]
	return device, ok
}

// GetByIndex returns a device by its index in the host
func (c *deviceCache) GetByIndex(index int) (*Device, error) {
	if index < 0 || index >= len(c.allDevices) {
		return nil, fmt.Errorf("index %d out of range", index)
	}

	return c.allDevices[index], nil
}

// Count returns the number of devices in the cache
func (c *deviceCache) Count() int {
	return len(c.allDevices)
}

// SMVersionSet returns a set of all SM versions in the cache
func (c *deviceCache) SMVersionSet() map[uint32]struct{} {
	return c.smVersionSet
}

// All returns all devices in the cache
func (c *deviceCache) All() []*Device {
	return c.allDevices
}

// Cores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
func (c *deviceCache) Cores(uuid string) (uint64, error) {
	device, ok := c.GetByUUID(uuid)
	if !ok {
		return 0, fmt.Errorf("device %s not found", uuid)
	}
	return uint64(device.CoreCount), nil
}
