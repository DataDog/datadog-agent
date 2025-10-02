// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"fmt"
	"sync"

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
	// GetLastInitError returns the last error that occurred during initialization, or nil if the cache is initialized
	GetLastInitError() error
}

// deviceCache is an implementation of DeviceCache
type deviceCache struct {
	allDevices         []Device
	allPhysicalDevices []Device
	allMigDevices      []Device
	uuidToDevice       map[string]Device
	smVersionSet       map[uint32]struct{}
	lib                SafeNVML
	initMutex          sync.Mutex
	initialized        bool
	lastInitError      error
}

// NewDeviceCache creates a new DeviceCache
func NewDeviceCache() DeviceCache {
	return NewDeviceCacheWithOptions(nil)
}

// NewDeviceCacheWithOptions creates a new DeviceCache with an already initialized NVML library
func NewDeviceCacheWithOptions(lib SafeNVML) DeviceCache {
	cache := &deviceCache{
		uuidToDevice: make(map[string]Device),
		smVersionSet: make(map[uint32]struct{}),
		lib:          lib,
	}

	// Try to initialize the cache if possible, if not we will try to initialize it later
	_ = cache.ensureInit()

	return cache
}

// GetLastInitError returns the error that occurred during initialization, or nil if the cache is initialized
func (c *deviceCache) GetLastInitError() error {
	if !c.initialized {
		return errors.New("cache not initialized")
	}

	return c.lastInitError
}

func (c *deviceCache) ensureInit() error {
	if c.initialized {
		return nil
	}

	c.initMutex.Lock()
	defer c.initMutex.Unlock()

	// Check again after locking to ensure no race condition
	if c.initialized {
		return nil
	}

	if c.lib == nil {
		lib, err := GetSafeNvmlLib()
		if err != nil {
			log.Warnf("error getting NVML library: %v", err)
			c.lastInitError = err
			return err
		}
		c.lib = lib
	}

	count, err := c.lib.DeviceGetCount()
	if err != nil {
		return err
	}

	for i := 0; i < count; i++ {
		nvmlDev, err := c.lib.DeviceGetHandleByIndex(i)
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

		c.uuidToDevice[dev.UUID] = dev
		c.allDevices = append(c.allDevices, dev)
		c.allPhysicalDevices = append(c.allPhysicalDevices, dev)
		c.smVersionSet[dev.SMVersion] = struct{}{}

		for _, migChild := range dev.MIGChildren {
			c.uuidToDevice[migChild.UUID] = migChild
			c.allDevices = append(c.allDevices, migChild)
			c.allMigDevices = append(c.allMigDevices, migChild)
		}
	}

	c.initialized = true
	c.lastInitError = nil

	return nil
}

// GetByUUID returns a device by its UUID
func (c *deviceCache) GetByUUID(uuid string) (Device, bool) {
	if err := c.ensureInit(); err != nil {
		return nil, false
	}

	device, ok := c.uuidToDevice[uuid]
	return device, ok
}

// GetByIndex returns a device by its index in the host
func (c *deviceCache) GetByIndex(index int) (Device, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}

	if index < 0 || index >= len(c.allDevices) {
		return nil, fmt.Errorf("index %d out of range", index)
	}

	return c.allDevices[index], nil
}

// Count returns the number of physical devices in the cache
func (c *deviceCache) Count() int {
	if err := c.ensureInit(); err != nil {
		return 0
	}

	return len(c.allPhysicalDevices)
}

// SMVersionSet returns a set of all SM versions in the cache
func (c *deviceCache) SMVersionSet() map[uint32]struct{} {
	_ = c.ensureInit() // Ignore error, as we will return the default set if the cache is not initialized anyways

	return c.smVersionSet
}

// All returns all devices in the cache
func (c *deviceCache) All() []Device {
	_ = c.ensureInit() // Ignore error, as we will return the default set if the cache is not initialized anyways

	return c.allDevices
}

// AllPhysicalDevices returns all physical devices in the cache
func (c *deviceCache) AllPhysicalDevices() []Device {
	_ = c.ensureInit() // Ignore error, as we will return the default set if the cache is not initialized anyways

	return c.allPhysicalDevices
}

// AllMigDevices returns all MIG children in the cache
func (c *deviceCache) AllMigDevices() []Device {
	_ = c.ensureInit() // Ignore error, as we will return the default set if the cache is not initialized anyways

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
