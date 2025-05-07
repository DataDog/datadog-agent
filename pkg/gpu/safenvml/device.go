// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// SafeDevice represents a safe wrapper around NVML device operations.
// It ensures that operations are only performed when the corresponding
// symbols are available in the loaded library.
type SafeDevice interface {
	// GetArchitecture returns the architecture of the device
	GetArchitecture() (nvml.DeviceArchitecture, error)
	// GetAttributes returns the attributes of the device
	GetAttributes() (nvml.DeviceAttributes, error)
	// GetClockInfo returns the current clock speed for the given clock type
	GetClockInfo(clockType nvml.ClockType) (uint32, error)
	// GetComputeRunningProcesses returns the list of compute processes running on the device
	GetComputeRunningProcesses() ([]nvml.ProcessInfo, error)
	// GetCudaComputeCapability returns the CUDA compute capability of the device
	GetCudaComputeCapability() (int, int, error)
	// GetCurrentClocksThrottleReasons returns the current clock throttle reasons bitmask
	GetCurrentClocksThrottleReasons() (uint64, error)
	// GetDecoderUtilization returns the decoder utilization
	GetDecoderUtilization() (uint32, uint32, error)
	// GetEncoderUtilization returns the encoder utilization
	GetEncoderUtilization() (uint32, uint32, error)
	// GetFanSpeed returns the fan speed percentage
	GetFanSpeed() (uint32, error)
	// GetFieldValues returns the values for the specified fields
	GetFieldValues(values []nvml.FieldValue) error
	// GetGpuInstanceId returns the GPU instance ID for MIG devices
	//nolint:revive // Maintaining consistency with go-nvml API naming
	GetGpuInstanceId() (int, error)
	// GetIndex returns the index of the device
	GetIndex() (int, error)
	// GetMaxClockInfo returns the maximum clock speed for the given clock type
	GetMaxClockInfo(clockType nvml.ClockType) (uint32, error)
	// GetMaxMigDeviceCount returns the maximum number of MIG devices that can be created
	GetMaxMigDeviceCount() (int, error)
	// GetMemoryBusWidth returns the memory bus width
	GetMemoryBusWidth() (uint32, error)
	// GetMemoryInfo returns memory information of the device
	GetMemoryInfo() (nvml.Memory, error)
	// GetMigDeviceHandleByIndex returns the MIG device handle at the given index
	GetMigDeviceHandleByIndex(index int) (SafeDevice, error)
	// GetMigMode returns the MIG mode of the device
	GetMigMode() (int, int, error)
	// GetName returns the name of the device
	GetName() (string, error)
	// GetNvLinkState returns the state of the specified NVLink
	GetNvLinkState(link int) (nvml.EnableState, error)
	// GetNumGpuCores returns the number of GPU cores in the device
	GetNumGpuCores() (int, error)
	// GetPcieThroughput returns the PCIe throughput in bytes/sec
	GetPcieThroughput(counter nvml.PcieUtilCounter) (uint32, error)
	// GetPerformanceState returns the current performance state
	GetPerformanceState() (nvml.Pstates, error)
	// GetPowerManagementLimit returns the power management limit in milliwatts
	GetPowerManagementLimit() (uint32, error)
	// GetPowerUsage returns the power usage in milliwatts
	GetPowerUsage() (uint32, error)
	// GetRemappedRows returns the remapped rows information
	GetRemappedRows() (int, int, bool, bool, error)
	// GetSamples returns samples for the specified counter type
	GetSamples(samplingType nvml.SamplingType, lastSeenTimestamp uint64) (nvml.ValueType, []nvml.Sample, error)
	// GetTemperature returns the current temperature
	GetTemperature(sensorType nvml.TemperatureSensors) (uint32, error)
	// GetTotalEnergyConsumption returns the total energy consumption in millijoules
	GetTotalEnergyConsumption() (uint64, error)
	// GetUUID returns the universally unique identifier of the device
	GetUUID() (string, error)
	// GetUtilizationRates returns the utilization rates for the device
	GetUtilizationRates() (nvml.Utilization, error)
}

// DeviceInfo holds common cached properties for a GPU device
type DeviceInfo struct {
	SMVersion uint32
	UUID      string
	Name      string
	CoreCount int

	// Index of the device in the host. For MIG devices, this is the index of the MIG device in the parent device.
	Index int

	// Memory size of the device in bytes
	Memory uint64
}
type Device interface {
	SafeDevice

	// GetDeviceInfo returns the common device info for a GPU device
	GetDeviceInfo() *DeviceInfo
}

// PhysicalDevice represents a physical GPU device
type PhysicalDevice struct {
	SafeDevice
	DeviceInfo
}

var _ Device = &PhysicalDevice{}

// NewPhysicalDevice creates a new Device from the nvml.Device and caches some properties.
func NewPhysicalDevice(dev nvml.Device) (*PhysicalDevice, error) {
	lib, err := GetSafeNvmlLib()
	if err != nil {
		return nil, err
	}

	// Create the safe device implementation
	safeDev := &safeDeviceImpl{
		nvmlDevice: dev,
		lib:        lib,
	}

	// Create the device with embedded safe device
	device := &PhysicalDevice{
		SafeDevice: safeDev,
	}

	if err := device.fillBasicDataFromNVML(safeDev); err != nil {
		return nil, fmt.Errorf("error filling basic data from NVML: %w", err)
	}

	major, minor, err := device.SafeDevice.GetCudaComputeCapability()
	if err != nil {
		return nil, fmt.Errorf("error getting CUDA compute capability: %w", err)
	}
	device.SMVersion = uint32(major*10 + minor)

	// Get the number of cores and memory size. This is not part of the fillBasicDataFromNVML
	// because it might not work for MIG devices.
	cores, err := device.SafeDevice.GetNumGpuCores()
	if err != nil {
		return nil, err
	}
	device.CoreCount = int(cores)

	memInfo, err := device.SafeDevice.GetMemoryInfo()
	if err != nil {
		return nil, err
	}
	device.Memory = memInfo.Total

	return device, nil
}

func (d *PhysicalDevice) GetDeviceInfo() *DeviceInfo {
	return &d.DeviceInfo
}

// fillBasicDataFromNVML fills the basic data (common for MIG and non-MIG) for a device from the nvml.Device object
func (d *DeviceInfo) fillBasicDataFromNVML(dev SafeDevice) error {
	var err error
	d.UUID, err = dev.GetUUID()
	if err != nil {
		return err
	}

	d.Name, err = dev.GetName()
	if err != nil {
		return err
	}

	d.Index, err = dev.GetIndex()
	if err != nil {
		return err
	}

	return nil
}
