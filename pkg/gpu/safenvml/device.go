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
	// GetCudaComputeCapability returns the CUDA compute capability of the device
	GetCudaComputeCapability() (int, int, error)
	// GetUUID returns the universally unique identifier of the device
	GetUUID() (string, error)
	// GetNumGpuCores returns the number of GPU cores in the device
	GetNumGpuCores() (int, error)
	// GetIndex returns the index of the device
	GetIndex() (int, error)
	// GetMemoryInfo returns memory information of the device
	GetMemoryInfo() (nvml.Memory, error)
	// GetArchitecture returns the architecture of the device
	GetArchitecture() (nvml.DeviceArchitecture, error)
	// GetMemoryBusWidth returns the memory bus width
	GetMemoryBusWidth() (uint32, error)
	// GetMaxClockInfo returns the maximum clock speed for the given clock type
	GetMaxClockInfo(clockType nvml.ClockType) (uint32, error)
	// GetPcieThroughput returns the PCIe throughput in bytes/sec
	GetPcieThroughput(counter nvml.PcieUtilCounter) (uint32, error)
	// GetDecoderUtilization returns the decoder utilization
	GetDecoderUtilization() (uint32, uint32, error)
	// GetUtilizationRates returns the utilization rates for the device
	GetUtilizationRates() (nvml.Utilization, error)
	// GetEncoderUtilization returns the encoder utilization
	GetEncoderUtilization() (uint32, uint32, error)
	// GetFanSpeed returns the fan speed percentage
	GetFanSpeed() (uint32, error)
	// GetPowerManagementLimit returns the power management limit in milliwatts
	GetPowerManagementLimit() (uint32, error)
	// GetPowerUsage returns the power usage in milliwatts
	GetPowerUsage() (uint32, error)
	// GetPerformanceState returns the current performance state
	GetPerformanceState() (nvml.Pstates, error)
	// GetClockInfo returns the current clock speed for the given clock type
	GetClockInfo(clockType nvml.ClockType) (uint32, error)
	// GetTemperature returns the current temperature
	GetTemperature(sensorType nvml.TemperatureSensors) (uint32, error)
	// GetTotalEnergyConsumption returns the total energy consumption in millijoules
	GetTotalEnergyConsumption() (uint64, error)
	// GetFieldValues returns the values for the specified fields
	GetFieldValues(values []nvml.FieldValue) error
	// GetNvLinkState returns the state of the specified NVLink
	GetNvLinkState(link int) (nvml.EnableState, error)
	// GetGpuInstanceId returns the GPU instance ID for MIG devices
	//nolint:revive // Maintaining consistency with go-nvml API naming
	GetGpuInstanceId() (int, error)
	// GetAttributes returns the attributes of the device
	GetAttributes() (nvml.DeviceAttributes, error)
	// GetRemappedRows returns the remapped rows information
	GetRemappedRows() (int, int, bool, bool, error)
	// GetCurrentClocksThrottleReasons returns the current clock throttle reasons bitmask
	GetCurrentClocksThrottleReasons() (uint64, error)
	// GetSamples returns samples for the specified counter type
	GetSamples(samplingType nvml.SamplingType, lastSeenTimestamp uint64) (nvml.ValueType, []nvml.Sample, error)
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

func (d *safeDeviceImpl) GetCudaComputeCapability() (int, int, error) {
	if err := d.lib.lookup(toNativeName("GetCudaComputeCapability")); err != nil {
		return 0, 0, err
	}
	major, minor, ret := d.nvmlDevice.GetCudaComputeCapability()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, 0, &ErrNotSupported{APIName: "GetCudaComputeCapability"}
	} else if ret != nvml.SUCCESS {
		return 0, 0, fmt.Errorf("error getting CUDA compute capability: %s", nvml.ErrorString(ret))
	}
	return major, minor, nil
}

func (d *safeDeviceImpl) GetUUID() (string, error) {
	if err := d.lib.lookup(toNativeName("GetUUID")); err != nil {
		return "", err
	}
	uuid, ret := d.nvmlDevice.GetUUID()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return "", &ErrNotSupported{APIName: "GetUUID"}
	} else if ret != nvml.SUCCESS {
		return "", fmt.Errorf("error getting UUID: %s", nvml.ErrorString(ret))
	}
	return uuid, nil
}

func (d *safeDeviceImpl) GetName() (string, error) {
	if err := d.lib.lookup(toNativeName("GetName")); err != nil {
		return "", err
	}
	name, ret := d.nvmlDevice.GetName()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return "", &ErrNotSupported{APIName: "GetName"}
	} else if ret != nvml.SUCCESS {
		return "", fmt.Errorf("error getting device name: %s", nvml.ErrorString(ret))
	}
	return name, nil
}

func (d *safeDeviceImpl) GetNumGpuCores() (int, error) {
	if err := d.lib.lookup(toNativeName("GetNumGpuCores")); err != nil {
		return 0, err
	}
	cores, ret := d.nvmlDevice.GetNumGpuCores()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetNumGpuCores"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting GPU core count: %s", nvml.ErrorString(ret))
	}
	return cores, nil
}

func (d *safeDeviceImpl) GetIndex() (int, error) {
	if err := d.lib.lookup(toNativeName("GetIndex")); err != nil {
		return 0, err
	}
	index, ret := d.nvmlDevice.GetIndex()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetIndex"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting device index: %s", nvml.ErrorString(ret))
	}
	return index, nil
}

func (d *safeDeviceImpl) GetMemoryInfo() (nvml.Memory, error) {
	if err := d.lib.lookup(toNativeName("GetMemoryInfo")); err != nil {
		return nvml.Memory{}, err
	}
	memInfo, ret := d.nvmlDevice.GetMemoryInfo()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return nvml.Memory{}, &ErrNotSupported{APIName: "GetMemoryInfo"}
	} else if ret != nvml.SUCCESS {
		return nvml.Memory{}, fmt.Errorf("error getting memory info: %s", nvml.ErrorString(ret))
	}
	return memInfo, nil
}

func (d *safeDeviceImpl) GetArchitecture() (nvml.DeviceArchitecture, error) {
	if err := d.lib.lookup(toNativeName("GetArchitecture")); err != nil {
		return 0, err
	}
	arch, ret := d.nvmlDevice.GetArchitecture()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetArchitecture"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting device architecture: %s", nvml.ErrorString(ret))
	}
	return arch, nil
}

func (d *safeDeviceImpl) GetMemoryBusWidth() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetMemoryBusWidth")); err != nil {
		return 0, err
	}
	width, ret := d.nvmlDevice.GetMemoryBusWidth()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetMemoryBusWidth"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting memory bus width: %s", nvml.ErrorString(ret))
	}
	return width, nil
}

func (d *safeDeviceImpl) GetMaxClockInfo(clockType nvml.ClockType) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetMaxClockInfo")); err != nil {
		return 0, err
	}
	clock, ret := d.nvmlDevice.GetMaxClockInfo(clockType)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetMaxClockInfo"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting max clock info for type %d: %s", clockType, nvml.ErrorString(ret))
	}
	return clock, nil
}

func (d *safeDeviceImpl) GetPcieThroughput(counter nvml.PcieUtilCounter) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetPcieThroughput")); err != nil {
		return 0, err
	}
	throughput, ret := d.nvmlDevice.GetPcieThroughput(counter)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetPcieThroughput"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting PCIe throughput for counter %d: %s", counter, nvml.ErrorString(ret))
	}
	return throughput, nil
}

func (d *safeDeviceImpl) GetDecoderUtilization() (uint32, uint32, error) {
	if err := d.lib.lookup(toNativeName("GetDecoderUtilization")); err != nil {
		return 0, 0, err
	}
	utilization, samplingPeriod, ret := d.nvmlDevice.GetDecoderUtilization()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, 0, &ErrNotSupported{APIName: "GetDecoderUtilization"}
	} else if ret != nvml.SUCCESS {
		return 0, 0, fmt.Errorf("error getting decoder utilization: %s", nvml.ErrorString(ret))
	}
	return utilization, samplingPeriod, nil
}

func (d *safeDeviceImpl) GetUtilizationRates() (nvml.Utilization, error) {
	if err := d.lib.lookup(toNativeName("GetUtilizationRates")); err != nil {
		return nvml.Utilization{}, err
	}
	utilization, ret := d.nvmlDevice.GetUtilizationRates()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return nvml.Utilization{}, &ErrNotSupported{APIName: "GetUtilizationRates"}
	} else if ret != nvml.SUCCESS {
		return nvml.Utilization{}, fmt.Errorf("error getting utilization rates: %s", nvml.ErrorString(ret))
	}
	return utilization, nil
}

func (d *safeDeviceImpl) GetEncoderUtilization() (uint32, uint32, error) {
	if err := d.lib.lookup(toNativeName("GetEncoderUtilization")); err != nil {
		return 0, 0, err
	}
	utilization, samplingPeriod, ret := d.nvmlDevice.GetEncoderUtilization()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, 0, &ErrNotSupported{APIName: "GetEncoderUtilization"}
	} else if ret != nvml.SUCCESS {
		return 0, 0, fmt.Errorf("error getting encoder utilization: %s", nvml.ErrorString(ret))
	}
	return utilization, samplingPeriod, nil
}

func (d *safeDeviceImpl) GetFanSpeed() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetFanSpeed")); err != nil {
		return 0, err
	}
	speed, ret := d.nvmlDevice.GetFanSpeed()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetFanSpeed"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting fan speed: %s", nvml.ErrorString(ret))
	}
	return speed, nil
}

func (d *safeDeviceImpl) GetPowerManagementLimit() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetPowerManagementLimit")); err != nil {
		return 0, err
	}
	limit, ret := d.nvmlDevice.GetPowerManagementLimit()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetPowerManagementLimit"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting power management limit: %s", nvml.ErrorString(ret))
	}
	return limit, nil
}

func (d *safeDeviceImpl) GetPowerUsage() (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetPowerUsage")); err != nil {
		return 0, err
	}
	usage, ret := d.nvmlDevice.GetPowerUsage()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetPowerUsage"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting power usage: %s", nvml.ErrorString(ret))
	}
	return usage, nil
}

func (d *safeDeviceImpl) GetPerformanceState() (nvml.Pstates, error) {
	if err := d.lib.lookup(toNativeName("GetPerformanceState")); err != nil {
		return 0, err
	}
	state, ret := d.nvmlDevice.GetPerformanceState()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetPerformanceState"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting performance state: %s", nvml.ErrorString(ret))
	}
	return state, nil
}

func (d *safeDeviceImpl) GetClockInfo(clockType nvml.ClockType) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetClockInfo")); err != nil {
		return 0, err
	}
	clock, ret := d.nvmlDevice.GetClockInfo(clockType)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetClockInfo"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting clock info for type %d: %s", clockType, nvml.ErrorString(ret))
	}
	return clock, nil
}

func (d *safeDeviceImpl) GetTemperature(sensorType nvml.TemperatureSensors) (uint32, error) {
	if err := d.lib.lookup(toNativeName("GetTemperature")); err != nil {
		return 0, err
	}
	temp, ret := d.nvmlDevice.GetTemperature(sensorType)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetTemperature"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting temperature for sensor %d: %s", sensorType, nvml.ErrorString(ret))
	}
	return temp, nil
}

func (d *safeDeviceImpl) GetTotalEnergyConsumption() (uint64, error) {
	if err := d.lib.lookup(toNativeName("GetTotalEnergyConsumption")); err != nil {
		return 0, err
	}
	energy, ret := d.nvmlDevice.GetTotalEnergyConsumption()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetTotalEnergyConsumption"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting total energy consumption: %s", nvml.ErrorString(ret))
	}
	return energy, nil
}

func (d *safeDeviceImpl) GetFieldValues(values []nvml.FieldValue) error {
	if err := d.lib.lookup(toNativeName("GetFieldValues")); err != nil {
		return err
	}
	ret := d.nvmlDevice.GetFieldValues(values)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return &ErrNotSupported{APIName: "GetFieldValues"}
	} else if ret != nvml.SUCCESS {
		return fmt.Errorf("error getting field values: %s", nvml.ErrorString(ret))
	}
	return nil
}

func (d *safeDeviceImpl) GetNvLinkState(link int) (nvml.EnableState, error) {
	if err := d.lib.lookup(toNativeName("GetNvLinkState")); err != nil {
		return 0, err
	}
	state, ret := d.nvmlDevice.GetNvLinkState(link)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetNvLinkState"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting NVLink state for link %d: %s", link, nvml.ErrorString(ret))
	}
	return state, nil
}

//nolint:revive // Maintaining consistency with go-nvml API naming
func (d *safeDeviceImpl) GetGpuInstanceId() (int, error) {
	if err := d.lib.lookup(toNativeName("GetGpuInstanceId")); err != nil {
		return 0, err
	}
	id, ret := d.nvmlDevice.GetGpuInstanceId()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetGpuInstanceId"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting GPU instance ID: %s", nvml.ErrorString(ret))
	}
	return id, nil
}

func (d *safeDeviceImpl) GetAttributes() (nvml.DeviceAttributes, error) {
	if err := d.lib.lookup(toNativeName("GetAttributes")); err != nil {
		return nvml.DeviceAttributes{}, err
	}
	attrs, ret := d.nvmlDevice.GetAttributes()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return nvml.DeviceAttributes{}, &ErrNotSupported{APIName: "GetAttributes"}
	} else if ret != nvml.SUCCESS {
		return nvml.DeviceAttributes{}, fmt.Errorf("error getting device attributes: %s", nvml.ErrorString(ret))
	}
	return attrs, nil
}

func (d *safeDeviceImpl) GetRemappedRows() (int, int, bool, bool, error) {
	if err := d.lib.lookup(toNativeName("GetRemappedRows")); err != nil {
		return 0, 0, false, false, err
	}
	corrRows, uncorrRows, isPending, failureOccurred, ret := d.nvmlDevice.GetRemappedRows()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, 0, false, false, &ErrNotSupported{APIName: "GetRemappedRows"}
	} else if ret != nvml.SUCCESS {
		return 0, 0, false, false, fmt.Errorf("error getting remapped rows: %s", nvml.ErrorString(ret))
	}
	return corrRows, uncorrRows, isPending, failureOccurred, nil
}

func (d *safeDeviceImpl) GetCurrentClocksThrottleReasons() (uint64, error) {
	if err := d.lib.lookup(toNativeName("GetCurrentClocksThrottleReasons")); err != nil {
		return 0, err
	}
	reasons, ret := d.nvmlDevice.GetCurrentClocksThrottleReasons()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, &ErrNotSupported{APIName: "GetCurrentClocksThrottleReasons"}
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting current clocks throttle reasons: %s", nvml.ErrorString(ret))
	}
	return reasons, nil
}

func (d *safeDeviceImpl) GetSamples(samplingType nvml.SamplingType, lastSeenTimestamp uint64) (nvml.ValueType, []nvml.Sample, error) {
	if err := d.lib.lookup(toNativeName("GetSamples")); err != nil {
		return 0, nil, err
	}
	valueType, samples, ret := d.nvmlDevice.GetSamples(samplingType, lastSeenTimestamp)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, nil, &ErrNotSupported{APIName: "GetSamples"}
	} else if ret != nvml.SUCCESS {
		return 0, nil, fmt.Errorf("error getting samples: %s", nvml.ErrorString(ret))
	}
	return valueType, samples, nil
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
