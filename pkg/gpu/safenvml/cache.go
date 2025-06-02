// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DeviceCache is a cache of GPU devices, with some methods to easily access devices by UUID or index
type DeviceCache interface {
	// GetByUUID returns a device by its UUID
	GetByUUID(uuid string) (Device, bool)
	// GetByIndex returns a device by its index
	GetByIndex(index int) (Device, error)
	// Count returns the number of physical devices in the cache
	Count() int
	// SMVersionSet returns a set of all SM versions in the cache
	SMVersionSet() map[uint32]struct{}
	// All returns all devices in the cache
	All() []Device
	// AllPhysicalDevices returns all root devices in the cache
	AllPhysicalDevices() []Device
	// AllMigDevices returns all MIG children in the cache
	AllMigDevices() []Device
	// Cores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
	Cores(uuid string) (uint64, error)
}

// deviceCache is an implementation of DeviceCache
type deviceCache struct {
	allDevices         []Device
	allPhysicalDevices []Device
	allMigDevices      []Device
	uuidToDevice       map[string]Device
	smVersionSet       map[uint32]struct{}
}

// NewDeviceCache creates a new DeviceCache
func NewDeviceCache() (DeviceCache, error) {
	lib, err := GetSafeNvmlLib()
	if err != nil {
		return nil, err
	}

	return NewDeviceCacheWithOptions(lib)
}

// NewDeviceCacheWithOptions creates a new DeviceCache with an already initialized NVML library
func NewDeviceCacheWithOptions(lib SafeNVML) (DeviceCache, error) {
	cache := &deviceCache{
		uuidToDevice: make(map[string]Device),
		smVersionSet: make(map[uint32]struct{}),
	}

	count, err := lib.DeviceGetCount()
	if err != nil {
		return nil, err
	}

	for i := 0; i < count; i++ {
		nvmlDev, err := lib.DeviceGetHandleByIndex(i)
		if err != nil {
			log.Warnf("error getting device by index %d: %s", i, err)
			continue
		}

		// Convert from SafeDevice to *Device
		dev, ok := nvmlDev.(*PhysicalDevice)
		if !ok {
			// This should never happen
			log.Warnf("error converting device at index %d to *Device", i)
			continue
		}

		cache.uuidToDevice[dev.UUID] = dev
		cache.allDevices = append(cache.allDevices, dev)
		cache.allPhysicalDevices = append(cache.allPhysicalDevices, dev)
		cache.smVersionSet[dev.SMVersion] = struct{}{}

		for _, migChild := range dev.MIGChildren {
			cache.uuidToDevice[migChild.UUID] = migChild
			cache.allDevices = append(cache.allDevices, migChild)
			cache.allMigDevices = append(cache.allMigDevices, migChild)
		}
	}

	return cache, nil
}

// GetByUUID returns a device by its UUID
func (c *deviceCache) GetByUUID(uuid string) (Device, bool) {
	device, ok := c.uuidToDevice[uuid]
	return device, ok
}

// GetByIndex returns a device by its index in the host
func (c *deviceCache) GetByIndex(index int) (Device, error) {
	if index < 0 || index >= len(c.allDevices) {
		return nil, fmt.Errorf("index %d out of range", index)
	}

	return c.allDevices[index], nil
}

// Count returns the number of physical devices in the cache
func (c *deviceCache) Count() int {
	return len(c.allPhysicalDevices)
}

// SMVersionSet returns a set of all SM versions in the cache
func (c *deviceCache) SMVersionSet() map[uint32]struct{} {
	return c.smVersionSet
}

// All returns all devices in the cache
func (c *deviceCache) All() []Device {
	return c.allDevices
}

// AllPhysicalDevices returns all physical devices in the cache
func (c *deviceCache) AllPhysicalDevices() []Device {
	return c.allPhysicalDevices
}

// AllMigDevices returns all MIG children in the cache
func (c *deviceCache) AllMigDevices() []Device {
	return c.allMigDevices
}

// Cores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
func (c *deviceCache) Cores(uuid string) (uint64, error) {
	device, ok := c.GetByUUID(uuid)
	if !ok {
		return 0, fmt.Errorf("device %s not found", uuid)
	}
	return uint64(device.GetDeviceInfo().CoreCount), nil
}
