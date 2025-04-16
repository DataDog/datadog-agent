// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
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

// Device represents a GPU device with some properties already computed.
// It embeds SafeDevice for safe API access and contains cached properties.
type Device struct {
	SafeDevice

	// Cached fields for quick access
	SMVersion uint32
	UUID      string
	Name      string
	CoreCount int
	Index     int
	Memory    uint64
}

// safeDeviceImpl implements the SafeDevice interface
type safeDeviceImpl struct {
	nvmlDevice nvml.Device
	lib        symbolLookup
}

func (d *safeDeviceImpl) GetArchitecture() (nvml.DeviceArchitecture, error) {
	if err := d.lib.lookup(toNativeName("GetArchitecture")); err != nil {
		return 0, err
	}
	arch, ret := d.nvmlDevice.GetArchitecture()
	return arch, NewNvmlAPIErrorOrNil("GetArchitecture", ret)
}

func (d *safeDeviceImpl) GetAttributes() (nvml.DeviceAttributes, error) {
	if err := d.lib.lookup(toNativeName("GetAttributes")); err != nil {
		return nvml.DeviceAttributes{}, err
	}
	attrs, ret := d.nvmlDevice.GetAttributes()
	return attrs, NewNvmlAPIErrorOrNil("GetAttributes", ret)
}

func (d *safeDeviceImpl) GetClockInfo(clockType nvml.ClockType) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetClockInfo")); err != nil {
		return 0, err
	}
	clock, ret := d.nvmlDevice.GetClockInfo(clockType)
	return clock, NewNvmlAPIErrorOrNil("GetClockInfo", ret)
}

// GetComputeRunningProcesses returns the list of compute processes running on the device
func (d *safeDeviceImpl) GetComputeRunningProcesses() ([]nvml.ProcessInfo, error) {
	if err := d.lib.lookup(toNativeName("GetComputeRunningProcesses")); err != nil {
		return nil, err
	}
	processes, ret := d.nvmlDevice.GetComputeRunningProcesses()
	return processes, NewNvmlAPIErrorOrNil("GetComputeRunningProcesses", ret)
}

func (d *safeDeviceImpl) GetCudaComputeCapability() (int, int, error) {
	if err := d.lib.lookup(toNativeName("GetCudaComputeCapability")); err != nil {
		return 0, 0, err
	}
	major, minor, ret := d.nvmlDevice.GetCudaComputeCapability()
	return major, minor, NewNvmlAPIErrorOrNil("GetCudaComputeCapability", ret)
}

func (d *safeDeviceImpl) GetCurrentClocksThrottleReasons() (uint64, error) {
	if err := d.lib.lookup(toNativeName("GetCurrentClocksThrottleReasons")); err != nil {
		return 0, err
	}
	reasons, ret := d.nvmlDevice.GetCurrentClocksThrottleReasons()
	return reasons, NewNvmlAPIErrorOrNil("GetCurrentClocksThrottleReasons", ret)
}

func (d *safeDeviceImpl) GetDecoderUtilization() (uint32, uint32, error) {
	if err := d.lib.lookup(toNativeName("GetDecoderUtilization")); err != nil {
		return 0, 0, err
	}
	utilization, samplingPeriod, ret := d.nvmlDevice.GetDecoderUtilization()
	return utilization, samplingPeriod, NewNvmlAPIErrorOrNil("GetDecoderUtilization", ret)
}

func (d *safeDeviceImpl) GetEncoderUtilization() (uint32, uint32, error) {
	if err := d.lib.lookup(toNativeName("GetEncoderUtilization")); err != nil {
		return 0, 0, err
	}
	utilization, samplingPeriod, ret := d.nvmlDevice.GetEncoderUtilization()
	return utilization, samplingPeriod, NewNvmlAPIErrorOrNil("GetEncoderUtilization", ret)
}

func (d *safeDeviceImpl) GetFanSpeed() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetFanSpeed")); err != nil {
		return 0, err
	}
	speed, ret := d.nvmlDevice.GetFanSpeed()
	return speed, NewNvmlAPIErrorOrNil("GetFanSpeed", ret)
}

func (d *safeDeviceImpl) GetFieldValues(values []nvml.FieldValue) error {
	if err := d.lib.lookup(toNativeName("GetFieldValues")); err != nil {
		return err
	}
	ret := d.nvmlDevice.GetFieldValues(values)
	return NewNvmlAPIErrorOrNil("GetFieldValues", ret)
}

//nolint:revive // Maintaining consistency with go-nvml API naming
func (d *safeDeviceImpl) GetGpuInstanceId() (int, error) {
	if err := d.lib.lookup(toNativeName("GetGpuInstanceId")); err != nil {
		return 0, err
	}
	id, ret := d.nvmlDevice.GetGpuInstanceId()
	return id, NewNvmlAPIErrorOrNil("GetGpuInstanceId", ret)
}

func (d *safeDeviceImpl) GetIndex() (int, error) {
	if err := d.lib.lookup(toNativeName("GetIndex")); err != nil {
		return 0, err
	}
	index, ret := d.nvmlDevice.GetIndex()
	return index, NewNvmlAPIErrorOrNil("GetIndex", ret)
}

func (d *safeDeviceImpl) GetMaxClockInfo(clockType nvml.ClockType) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetMaxClockInfo")); err != nil {
		return 0, err
	}
	clock, ret := d.nvmlDevice.GetMaxClockInfo(clockType)
	return clock, NewNvmlAPIErrorOrNil("GetMaxClockInfo", ret)
}

// GetMaxMigDeviceCount returns the maximum number of MIG devices that can be created
func (d *safeDeviceImpl) GetMaxMigDeviceCount() (int, error) {
	if err := d.lib.lookup(toNativeName("GetMaxMigDeviceCount")); err != nil {
		return 0, err
	}
	count, ret := d.nvmlDevice.GetMaxMigDeviceCount()
	return count, NewNvmlAPIErrorOrNil("GetMaxMigDeviceCount", ret)
}

func (d *safeDeviceImpl) GetMemoryBusWidth() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetMemoryBusWidth")); err != nil {
		return 0, err
	}
	width, ret := d.nvmlDevice.GetMemoryBusWidth()
	return width, NewNvmlAPIErrorOrNil("GetMemoryBusWidth", ret)
}

func (d *safeDeviceImpl) GetMemoryInfo() (nvml.Memory, error) {
	if err := d.lib.lookup(toNativeName("GetMemoryInfo")); err != nil {
		return nvml.Memory{}, err
	}
	memInfo, ret := d.nvmlDevice.GetMemoryInfo()
	return memInfo, NewNvmlAPIErrorOrNil("GetMemoryInfo", ret)
}

// GetMigDeviceHandleByIndex returns the MIG device handle at the given index
func (d *safeDeviceImpl) GetMigDeviceHandleByIndex(index int) (SafeDevice, error) {
	if err := d.lib.lookup(toNativeName("GetMigDeviceHandleByIndex")); err != nil {
		return nil, err
	}
	device, ret := d.nvmlDevice.GetMigDeviceHandleByIndex(index)
	if err := NewNvmlAPIErrorOrNil("GetMigDeviceHandleByIndex", ret); err != nil {
		return nil, err
	}
	return NewDevice(device)
}

// GetMigMode returns the MIG mode of the device
func (d *safeDeviceImpl) GetMigMode() (int, int, error) {
	if err := d.lib.lookup(toNativeName("GetMigMode")); err != nil {
		return 0, 0, err
	}
	mode, pendingMode, ret := d.nvmlDevice.GetMigMode()
	return mode, pendingMode, NewNvmlAPIErrorOrNil("GetMigMode", ret)
}

func (d *safeDeviceImpl) GetName() (string, error) {
	if err := d.lib.lookup(toNativeName("GetName")); err != nil {
		return "", err
	}
	name, ret := d.nvmlDevice.GetName()
	return name, NewNvmlAPIErrorOrNil("GetName", ret)
}

func (d *safeDeviceImpl) GetNumGpuCores() (int, error) {
	if err := d.lib.lookup(toNativeName("GetNumGpuCores")); err != nil {
		return 0, err
	}
	cores, ret := d.nvmlDevice.GetNumGpuCores()
	return cores, NewNvmlAPIErrorOrNil("GetNumGpuCores", ret)
}

func (d *safeDeviceImpl) GetNvLinkState(link int) (nvml.EnableState, error) {
	if err := d.lib.lookup(toNativeName("GetNvLinkState")); err != nil {
		return 0, err
	}
	state, ret := d.nvmlDevice.GetNvLinkState(link)
	return state, NewNvmlAPIErrorOrNil("GetNvLinkState", ret)
}

func (d *safeDeviceImpl) GetPcieThroughput(counter nvml.PcieUtilCounter) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetPcieThroughput")); err != nil {
		return 0, err
	}
	throughput, ret := d.nvmlDevice.GetPcieThroughput(counter)
	return throughput, NewNvmlAPIErrorOrNil("GetPcieThroughput", ret)
}

func (d *safeDeviceImpl) GetPerformanceState() (nvml.Pstates, error) {
	if err := d.lib.lookup(toNativeName("GetPerformanceState")); err != nil {
		return 0, err
	}
	state, ret := d.nvmlDevice.GetPerformanceState()
	return state, NewNvmlAPIErrorOrNil("GetPerformanceState", ret)
}

func (d *safeDeviceImpl) GetPowerManagementLimit() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetPowerManagementLimit")); err != nil {
		return 0, err
	}
	limit, ret := d.nvmlDevice.GetPowerManagementLimit()
	return limit, NewNvmlAPIErrorOrNil("GetPowerManagementLimit", ret)
}

func (d *safeDeviceImpl) GetPowerUsage() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetPowerUsage")); err != nil {
		return 0, err
	}
	usage, ret := d.nvmlDevice.GetPowerUsage()
	return usage, NewNvmlAPIErrorOrNil("GetPowerUsage", ret)
}

func (d *safeDeviceImpl) GetRemappedRows() (int, int, bool, bool, error) {
	if err := d.lib.lookup(toNativeName("GetRemappedRows")); err != nil {
		return 0, 0, false, false, err
	}
	corrRows, uncorrRows, isPending, failureOccurred, ret := d.nvmlDevice.GetRemappedRows()
	return corrRows, uncorrRows, isPending, failureOccurred, NewNvmlAPIErrorOrNil("GetRemappedRows", ret)
}

func (d *safeDeviceImpl) GetSamples(samplingType nvml.SamplingType, lastSeenTimestamp uint64) (nvml.ValueType, []nvml.Sample, error) {
	if err := d.lib.lookup(toNativeName("GetSamples")); err != nil {
		return 0, nil, err
	}
	valueType, samples, ret := d.nvmlDevice.GetSamples(samplingType, lastSeenTimestamp)
	return valueType, samples, NewNvmlAPIErrorOrNil("GetSamples", ret)
}

func (d *safeDeviceImpl) GetTemperature(sensorType nvml.TemperatureSensors) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetTemperature")); err != nil {
		return 0, err
	}
	temp, ret := d.nvmlDevice.GetTemperature(sensorType)
	return temp, NewNvmlAPIErrorOrNil("GetTemperature", ret)
}

func (d *safeDeviceImpl) GetTotalEnergyConsumption() (uint64, error) {
	if err := d.lib.lookup(toNativeName("GetTotalEnergyConsumption")); err != nil {
		return 0, err
	}
	energy, ret := d.nvmlDevice.GetTotalEnergyConsumption()
	return energy, NewNvmlAPIErrorOrNil("GetTotalEnergyConsumption", ret)
}

func (d *safeDeviceImpl) GetUUID() (string, error) {
	if err := d.lib.lookup(toNativeName("GetUUID")); err != nil {
		return "", err
	}
	uuid, ret := d.nvmlDevice.GetUUID()
	return uuid, NewNvmlAPIErrorOrNil("GetUUID", ret)
}

func (d *safeDeviceImpl) GetUtilizationRates() (nvml.Utilization, error) {
	if err := d.lib.lookup(toNativeName("GetUtilizationRates")); err != nil {
		return nvml.Utilization{}, err
	}
	utilization, ret := d.nvmlDevice.GetUtilizationRates()
	return utilization, NewNvmlAPIErrorOrNil("GetUtilizationRates", ret)
}

// NewDevice creates a new Device from the nvml.Device and caches some properties
func NewDevice(dev nvml.Device) (*Device, error) {
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
	device := &Device{
		SafeDevice: safeDev,
	}

	// Now use safe methods to populate the cached fields
	major, minor, err := safeDev.GetCudaComputeCapability()
	if err != nil {
		return nil, err
	}
	device.SMVersion = uint32(major*10 + minor)

	uuid, err := safeDev.GetUUID()
	if err != nil {
		return nil, err
	}
	device.UUID = uuid

	cores, err := safeDev.GetNumGpuCores()
	if err != nil {
		return nil, err
	}
	device.CoreCount = cores

	name, err := safeDev.GetName()
	if err != nil {
		return nil, err
	}
	device.Name = name

	index, err := safeDev.GetIndex()
	if err != nil {
		return nil, err
	}
	device.Index = index

	memInfo, err := safeDev.GetMemoryInfo()
	if err != nil {
		return nil, err
	}
	device.Memory = memInfo.Total

	return device, nil
}
