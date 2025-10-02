// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DeviceCache is a cache of GPU devices, with some methods to easily access devices by UUID or index
type DeviceCache interface {
	// GetByUUID returns a device by its UUID
	GetByUUID(uuid string) (Device, error)
	// GetByIndex returns a device by its index
	GetByIndex(index int) (Device, error)
	// Count returns the number of physical devices in the cache
	Count() (int, error)
	// SMVersionSet returns a set of all SM versions in the cache
	SMVersionSet() (map[uint32]struct{}, error)
	// All returns all devices in the cache
	All() ([]Device, error)
	// AllPhysicalDevices returns all root devices in the cache
	AllPhysicalDevices() ([]Device, error)
	// AllMigDevices returns all MIG children in the cache
	AllMigDevices() ([]Device, error)
	// Cores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
	Cores(uuid string) (uint64, error)
	// EnsureInit ensures that the cache is initialized, returns an error if the initialization fails
	// this function is called by the other methods of the interface so it's not necessary to call it manually, unless
	// you want to ensure the cache is initialized before calling the other methods
	EnsureInit() error
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

	return cache
}

// EnsureInit ensures that the cache is initialized, returns an error if the initialization fails
func (c *deviceCache) EnsureInit() error {
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
func (c *deviceCache) GetByUUID(uuid string) (Device, error) {
	if err := c.EnsureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	device, ok := c.uuidToDevice[uuid]
	if !ok {
		return nil, fmt.Errorf("device %s not found", uuid)
	}
	return device, nil
}

// GetByIndex returns a device by its index in the host
func (c *deviceCache) GetByIndex(index int) (Device, error) {
	if err := c.EnsureInit(); err != nil {
		return nil, err
	}

	if index < 0 || index >= len(c.allDevices) {
		return nil, fmt.Errorf("index %d out of range", index)
	}

	return c.allDevices[index], nil
}

// Count returns the number of physical devices in the cache
func (c *deviceCache) Count() (int, error) {
	if err := c.EnsureInit(); err != nil {
		return 0, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	return len(c.allPhysicalDevices), nil
}

// SMVersionSet returns a set of all SM versions in the cache
func (c *deviceCache) SMVersionSet() (map[uint32]struct{}, error) {
	if err := c.EnsureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	return c.smVersionSet, nil
}

// All returns all devices in the cache
func (c *deviceCache) All() ([]Device, error) {
	if err := c.EnsureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	return c.allDevices, nil
}

// AllPhysicalDevices returns all physical devices in the cache
func (c *deviceCache) AllPhysicalDevices() ([]Device, error) {
	if err := c.EnsureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	return c.allPhysicalDevices, nil
}

// AllMigDevices returns all MIG children in the cache
func (c *deviceCache) AllMigDevices() ([]Device, error) {
	if err := c.EnsureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	return c.allMigDevices, nil
}

// Cores returns the number of cores for a device with a given UUID. Returns an error if the device is not found.
func (c *deviceCache) Cores(uuid string) (uint64, error) {
	device, err := c.GetByUUID(uuid)
	if err != nil {
		return 0, fmt.Errorf("failed to get device %s: %w", uuid, err)
	}
	return uint64(device.GetDeviceInfo().CoreCount), nil
}
