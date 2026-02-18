// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var logLimiter = log.NewLogLimit(20, 10*time.Minute)

// DeviceCache is a cache of GPU devices, with some methods to easily access devices by UUID or index
type DeviceCache interface {
	// Refresh updates the cache with the most up to date device info queried from the system.
	// It is invoked automatically by the other methods if the cache is not initialized at the first invocation.
	// In case of error, the cache remains consistent and will keep having the same data as before the invocation.
	Refresh() error
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
}

// DeviceCacheOption customizes DeviceCache
type DeviceCacheOption func(*deviceCache)

// deviceCache is an implementation of DeviceCache
type deviceCache struct {
	mu                 sync.RWMutex
	allDevices         []Device
	allPhysicalDevices []Device
	allMigDevices      []Device
	uuidToDevice       map[string]Device
	smVersionSet       map[uint32]struct{}
	lib                SafeNVML
	initialized        bool
}

// WithDeviceCacheLib uses a specific NVML library for the device cache
func WithDeviceCacheLib(lib SafeNVML) DeviceCacheOption {
	return func(d *deviceCache) {
		d.lib = lib
	}
}

// NewDeviceCache creates a new DeviceCache
func NewDeviceCache(opts ...DeviceCacheOption) DeviceCache {
	res := &deviceCache{}
	for _, o := range opts {
		o(res)
	}
	return res
}

// ensureInit ensures that the cache is initialized, returns an error if the initialization fails
func (c *deviceCache) ensureInit() error {
	c.mu.RLock()
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized {
		return c.Refresh()
	}
	return nil
}

func (c *deviceCache) Refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// automatically acquire the library singleton if one is not provided
	lib := c.lib
	if lib == nil {
		var err error
		if lib, err = GetSafeNvmlLib(); err != nil {
			if logLimiter.ShouldLog() {
				log.Warnf("error getting NVML library: %v", err)
			}
			return err
		}
	}

	count, err := lib.DeviceGetCount()
	if err != nil {
		return fmt.Errorf("failed getting device count while refreshing cache: %w", err)
	}

	allDevices := []Device{}
	allPhysicalDevices := []Device{}
	allMigDevices := []Device{}
	uuidToDevice := make(map[string]Device)
	smVersionSet := make(map[uint32]struct{})

	for i := range count {
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

		uuidToDevice[dev.UUID] = dev
		allDevices = append(allDevices, dev)
		allPhysicalDevices = append(allPhysicalDevices, dev)
		smVersionSet[dev.SMVersion] = struct{}{}

		for _, migChild := range dev.MIGChildren {
			uuidToDevice[migChild.UUID] = migChild
			allDevices = append(allDevices, migChild)
			allMigDevices = append(allMigDevices, migChild)
		}
	}

	// on success, set the new data in the cache
	c.allDevices = allDevices
	c.allPhysicalDevices = allPhysicalDevices
	c.allMigDevices = allMigDevices
	c.uuidToDevice = uuidToDevice
	c.smVersionSet = smVersionSet
	c.initialized = true
	c.lib = lib
	return nil
}

// GetByUUID returns a device by its UUID
func (c *deviceCache) GetByUUID(uuid string) (Device, error) {
	if err := c.ensureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	device, ok := c.uuidToDevice[uuid]
	if !ok {
		return nil, fmt.Errorf("device %s not found", uuid)
	}
	return device, nil
}

// GetByIndex returns a device by its index in the host
func (c *deviceCache) GetByIndex(index int) (Device, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if index < 0 || index >= len(c.allDevices) {
		return nil, fmt.Errorf("index %d out of range", index)
	}

	return c.allDevices[index], nil
}

// Count returns the number of physical devices in the cache
func (c *deviceCache) Count() (int, error) {
	if err := c.ensureInit(); err != nil {
		return 0, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.allPhysicalDevices), nil
}

// SMVersionSet returns a set of all SM versions in the cache
func (c *deviceCache) SMVersionSet() (map[uint32]struct{}, error) {
	if err := c.ensureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.smVersionSet, nil
}

// All returns all devices in the cache
func (c *deviceCache) All() ([]Device, error) {
	if err := c.ensureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.allDevices, nil
}

// AllPhysicalDevices returns all physical devices in the cache
func (c *deviceCache) AllPhysicalDevices() ([]Device, error) {
	if err := c.ensureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.allPhysicalDevices, nil
}

// AllMigDevices returns all MIG children in the cache
func (c *deviceCache) AllMigDevices() ([]Device, error) {
	if err := c.ensureInit(); err != nil {
		return nil, fmt.Errorf("failed to initialize device cache: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

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
