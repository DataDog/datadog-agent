// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// safeDeviceImpl implements the SafeDevice interface
type safeDeviceImpl struct {
	nvmlDevice nvml.Device
	lib        symbolLookup

	// mu serializes native NVML calls made against nvmlDevice. The NVIDIA driver
	// does not guarantee that concurrent calls on the same device handle are safe:
	// issuing two nvmlGpmSampleGet calls on one handle at the same time has been
	// observed to fault inside the driver. Collectors run in parallel (see
	// gpu.parallel_collectors) and several of them share a device, so every entry
	// point below must hold this lock for the duration of the native call.
	//
	// The lock is per handle, so calls to different devices still run in parallel.
	// MIG handles obtained from GetMigDeviceHandleByIndex are distinct handles and
	// get their own lock; GPM sampling for a MIG instance goes through the parent
	// handle via GpmMigSampleGet and is therefore covered by the parent's lock.
	mu sync.Mutex
}

// nvmlAPI identifies an NVML entry point: the short name reported in errors and
// the dynamic symbol that must be present in the loaded library.
type nvmlAPI struct {
	name   string
	symbol string
}

// deviceAPI describes an NVML API exported with the nvmlDevice prefix.
func deviceAPI(name string) nvmlAPI {
	return nvmlAPI{name: name, symbol: toNativeName(name)}
}

// gpmAPI describes an NVML API exported with the nvmlGpm prefix.
func gpmAPI(name string) nvmlAPI {
	return nvmlAPI{name: name, symbol: "nvml" + name}
}

// deviceCall invokes an NVML device API that only reports a status code. The
// symbol availability check runs outside the device lock (it is a lookup in the
// capability set, not a driver call); the native call itself is serialized.
func deviceCall(d *safeDeviceImpl, api nvmlAPI, fn func() nvml.Return) error {
	if err := d.lib.lookup(api.symbol); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	return NewNvmlAPIErrorOrNil(api.name, fn())
}

// deviceCall1 invokes an NVML device API returning a single value, serialized on
// the device lock. See deviceCall.
func deviceCall1[T any](d *safeDeviceImpl, api nvmlAPI, fn func() (T, nvml.Return)) (T, error) {
	var zero T
	if err := d.lib.lookup(api.symbol); err != nil {
		return zero, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	value, ret := fn()
	return value, NewNvmlAPIErrorOrNil(api.name, ret)
}

// deviceCall2 invokes an NVML device API returning two values, serialized on the
// device lock. See deviceCall.
func deviceCall2[T1, T2 any](d *safeDeviceImpl, api nvmlAPI, fn func() (T1, T2, nvml.Return)) (T1, T2, error) {
	var zero1 T1
	var zero2 T2
	if err := d.lib.lookup(api.symbol); err != nil {
		return zero1, zero2, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	first, second, ret := fn()
	return first, second, NewNvmlAPIErrorOrNil(api.name, ret)
}

func (d *safeDeviceImpl) GetArchitecture() (nvml.DeviceArchitecture, error) {
	return deviceCall1(d, deviceAPI("GetArchitecture"), d.nvmlDevice.GetArchitecture)
}

func (d *safeDeviceImpl) GetAttributes() (nvml.DeviceAttributes, error) {
	return deviceCall1(d, deviceAPI("GetAttributes"), d.nvmlDevice.GetAttributes)
}

func (d *safeDeviceImpl) GetBAR1MemoryInfo() (nvml.BAR1Memory, error) {
	return deviceCall1(d, deviceAPI("GetBAR1MemoryInfo"), d.nvmlDevice.GetBAR1MemoryInfo)
}

func (d *safeDeviceImpl) GetClockInfo(clockType nvml.ClockType) (uint32, error) {
	return deviceCall1(d, deviceAPI("GetClockInfo"), func() (uint32, nvml.Return) {
		return d.nvmlDevice.GetClockInfo(clockType)
	})
}

// GetComputeRunningProcesses returns the list of compute processes running on the device
func (d *safeDeviceImpl) GetComputeRunningProcesses() ([]nvml.ProcessInfo, error) {
	return deviceCall1(d, deviceAPI("GetComputeRunningProcesses"), d.nvmlDevice.GetComputeRunningProcesses)
}

func (d *safeDeviceImpl) GetCudaComputeCapability() (int, int, error) {
	return deviceCall2(d, deviceAPI("GetCudaComputeCapability"), d.nvmlDevice.GetCudaComputeCapability)
}

func (d *safeDeviceImpl) GetCurrentClocksThrottleReasons() (uint64, error) {
	return deviceCall1(d, deviceAPI("GetCurrentClocksThrottleReasons"), d.nvmlDevice.GetCurrentClocksThrottleReasons)
}

func (d *safeDeviceImpl) GetDecoderUtilization() (uint32, uint32, error) {
	return deviceCall2(d, deviceAPI("GetDecoderUtilization"), d.nvmlDevice.GetDecoderUtilization)
}

func (d *safeDeviceImpl) GetEncoderUtilization() (uint32, uint32, error) {
	return deviceCall2(d, deviceAPI("GetEncoderUtilization"), d.nvmlDevice.GetEncoderUtilization)
}

func (d *safeDeviceImpl) GetFanSpeed() (uint32, error) {
	return deviceCall1(d, deviceAPI("GetFanSpeed"), d.nvmlDevice.GetFanSpeed)
}

//nolint:revive // Maintaining consistency with go-nvml API naming
func (d *safeDeviceImpl) GetFanSpeed_v2(fanIndex int) (uint32, error) {
	return deviceCall1(d, deviceAPI("GetFanSpeed_v2"), func() (uint32, nvml.Return) {
		return d.nvmlDevice.GetFanSpeed_v2(fanIndex)
	})
}

func (d *safeDeviceImpl) GetFieldValues(values []nvml.FieldValue) error {
	return deviceCall(d, deviceAPI("GetFieldValues"), func() nvml.Return {
		return d.nvmlDevice.GetFieldValues(values)
	})
}

//nolint:revive // Maintaining consistency with go-nvml API naming
func (d *safeDeviceImpl) ReadWritePRM_v1(buffer *nvml.PRMTLV_v1) error {
	return deviceCall(d, deviceAPI("ReadWritePRM_v1"), func() nvml.Return {
		return d.nvmlDevice.ReadWritePRM_v1(buffer)
	})
}

//nolint:revive // Maintaining consistency with go-nvml API naming
func (d *safeDeviceImpl) GetGpuInstanceId() (int, error) {
	return deviceCall1(d, deviceAPI("GetGpuInstanceId"), d.nvmlDevice.GetGpuInstanceId)
}

func (d *safeDeviceImpl) GetGpuInstanceProfileInfo(profile int) (nvml.GpuInstanceProfileInfo, error) {
	return deviceCall1(d, deviceAPI("GetGpuInstanceProfileInfo"), func() (nvml.GpuInstanceProfileInfo, nvml.Return) {
		return d.nvmlDevice.GetGpuInstanceProfileInfo(profile)
	})
}

func (d *safeDeviceImpl) GetIndex() (int, error) {
	return deviceCall1(d, deviceAPI("GetIndex"), d.nvmlDevice.GetIndex)
}

func (d *safeDeviceImpl) GetMaxClockInfo(clockType nvml.ClockType) (uint32, error) {
	return deviceCall1(d, deviceAPI("GetMaxClockInfo"), func() (uint32, nvml.Return) {
		return d.nvmlDevice.GetMaxClockInfo(clockType)
	})
}

// GetMaxMigDeviceCount returns the maximum number of MIG devices that can be created
func (d *safeDeviceImpl) GetMaxMigDeviceCount() (int, error) {
	return deviceCall1(d, deviceAPI("GetMaxMigDeviceCount"), d.nvmlDevice.GetMaxMigDeviceCount)
}

func (d *safeDeviceImpl) GetMemoryBusWidth() (uint32, error) {
	return deviceCall1(d, deviceAPI("GetMemoryBusWidth"), d.nvmlDevice.GetMemoryBusWidth)
}

func (d *safeDeviceImpl) GetMemoryInfo() (nvml.Memory, error) {
	return deviceCall1(d, deviceAPI("GetMemoryInfo"), d.nvmlDevice.GetMemoryInfo)
}

func (d *safeDeviceImpl) GetMemoryInfoV2() (nvml.Memory_v2, error) {
	return deviceCall1(d, deviceAPI("GetMemoryInfo_v2"), d.nvmlDevice.GetMemoryInfo_v2)
}

// GetMigDeviceHandleByIndex returns the MIG device handle at the given index
func (d *safeDeviceImpl) GetMigDeviceHandleByIndex(index int) (SafeDevice, error) {
	device, err := deviceCall1(d, deviceAPI("GetMigDeviceHandleByIndex"), func() (nvml.Device, nvml.Return) {
		return d.nvmlDevice.GetMigDeviceHandleByIndex(index)
	})
	if err != nil {
		return nil, err
	}

	return &safeDeviceImpl{
		nvmlDevice: device,
		lib:        d.lib,
	}, nil
}

// GetMigMode returns the MIG mode of the device
func (d *safeDeviceImpl) GetMigMode() (int, int, error) {
	return deviceCall2(d, deviceAPI("GetMigMode"), d.nvmlDevice.GetMigMode)
}

func (d *safeDeviceImpl) GetName() (string, error) {
	return deviceCall1(d, deviceAPI("GetName"), d.nvmlDevice.GetName)
}

func (d *safeDeviceImpl) GetNumGpuCores() (int, error) {
	return deviceCall1(d, deviceAPI("GetNumGpuCores"), d.nvmlDevice.GetNumGpuCores)
}

func (d *safeDeviceImpl) GetNumFans() (int, error) {
	return deviceCall1(d, deviceAPI("GetNumFans"), d.nvmlDevice.GetNumFans)
}

func (d *safeDeviceImpl) GetNvLinkState(link int) (nvml.EnableState, error) {
	return deviceCall1(d, deviceAPI("GetNvLinkState"), func() (nvml.EnableState, nvml.Return) {
		return d.nvmlDevice.GetNvLinkState(link)
	})
}

func (d *safeDeviceImpl) GetPciInfo() (nvml.PciInfo, error) {
	return deviceCall1(d, deviceAPI("GetPciInfo"), d.nvmlDevice.GetPciInfo)
}

func (d *safeDeviceImpl) GetPcieThroughput(counter nvml.PcieUtilCounter) (uint32, error) {
	return deviceCall1(d, deviceAPI("GetPcieThroughput"), func() (uint32, nvml.Return) {
		return d.nvmlDevice.GetPcieThroughput(counter)
	})
}

func (d *safeDeviceImpl) GetCurrPcieLinkGeneration() (int, error) {
	return deviceCall1(d, deviceAPI("GetCurrPcieLinkGeneration"), d.nvmlDevice.GetCurrPcieLinkGeneration)
}

func (d *safeDeviceImpl) GetMaxPcieLinkGeneration() (int, error) {
	return deviceCall1(d, deviceAPI("GetMaxPcieLinkGeneration"), d.nvmlDevice.GetMaxPcieLinkGeneration)
}

func (d *safeDeviceImpl) GetCurrPcieLinkWidth() (int, error) {
	return deviceCall1(d, deviceAPI("GetCurrPcieLinkWidth"), d.nvmlDevice.GetCurrPcieLinkWidth)
}

func (d *safeDeviceImpl) GetMaxPcieLinkWidth() (int, error) {
	return deviceCall1(d, deviceAPI("GetMaxPcieLinkWidth"), d.nvmlDevice.GetMaxPcieLinkWidth)
}

func (d *safeDeviceImpl) GetPerformanceState() (nvml.Pstates, error) {
	return deviceCall1(d, deviceAPI("GetPerformanceState"), d.nvmlDevice.GetPerformanceState)
}

func (d *safeDeviceImpl) GetPowerManagementLimit() (uint32, error) {
	return deviceCall1(d, deviceAPI("GetPowerManagementLimit"), d.nvmlDevice.GetPowerManagementLimit)
}

func (d *safeDeviceImpl) GetPowerUsage() (uint32, error) {
	return deviceCall1(d, deviceAPI("GetPowerUsage"), d.nvmlDevice.GetPowerUsage)
}

// GetProcessUtilization returns process utilization samples since the given timestamp
func (d *safeDeviceImpl) GetProcessUtilization(lastSeenTimestamp uint64) ([]nvml.ProcessUtilizationSample, error) {
	return deviceCall1(d, deviceAPI("GetProcessUtilization"), func() ([]nvml.ProcessUtilizationSample, nvml.Return) {
		return d.nvmlDevice.GetProcessUtilization(lastSeenTimestamp)
	})
}

func (d *safeDeviceImpl) GetRemappedRows() (int, int, bool, bool, error) {
	// Grouped into a struct so this shares the single locked-call helper rather
	// than open-coding the locking for a four-value signature.
	type remappedRows struct {
		corrRows        int
		uncorrRows      int
		isPending       bool
		failureOccurred bool
	}

	rows, err := deviceCall1(d, deviceAPI("GetRemappedRows"), func() (remappedRows, nvml.Return) {
		corrRows, uncorrRows, isPending, failureOccurred, ret := d.nvmlDevice.GetRemappedRows()
		return remappedRows{corrRows, uncorrRows, isPending, failureOccurred}, ret
	})
	return rows.corrRows, rows.uncorrRows, rows.isPending, rows.failureOccurred, err
}

func (d *safeDeviceImpl) GetRepairStatus() (nvml.RepairStatus, error) {
	return deviceCall1(d, deviceAPI("GetRepairStatus"), d.nvmlDevice.GetRepairStatus)
}

func (d *safeDeviceImpl) GetSamples(samplingType nvml.SamplingType, lastSeenTimestamp uint64) (nvml.ValueType, []nvml.Sample, error) {
	return deviceCall2(d, deviceAPI("GetSamples"), func() (nvml.ValueType, []nvml.Sample, nvml.Return) {
		return d.nvmlDevice.GetSamples(samplingType, lastSeenTimestamp)
	})
}

func (d *safeDeviceImpl) GetTemperature(sensorType nvml.TemperatureSensors) (uint32, error) {
	return deviceCall1(d, deviceAPI("GetTemperature"), func() (uint32, nvml.Return) {
		return d.nvmlDevice.GetTemperature(sensorType)
	})
}

func (d *safeDeviceImpl) GetTotalEnergyConsumption() (uint64, error) {
	return deviceCall1(d, deviceAPI("GetTotalEnergyConsumption"), d.nvmlDevice.GetTotalEnergyConsumption)
}

func (d *safeDeviceImpl) GetUUID() (string, error) {
	return deviceCall1(d, deviceAPI("GetUUID"), d.nvmlDevice.GetUUID)
}

func (d *safeDeviceImpl) GetUtilizationRates() (nvml.Utilization, error) {
	return deviceCall1(d, deviceAPI("GetUtilizationRates"), d.nvmlDevice.GetUtilizationRates)
}

func (d *safeDeviceImpl) GpmQueryDeviceSupport() (nvml.GpmSupport, error) {
	return deviceCall1(d, gpmAPI("GpmQueryDeviceSupport"), d.nvmlDevice.GpmQueryDeviceSupport)
}

func (d *safeDeviceImpl) GpmSampleGet(sample nvml.GpmSample) error {
	return deviceCall(d, gpmAPI("GpmSampleGet"), func() nvml.Return {
		return d.nvmlDevice.GpmSampleGet(sample)
	})
}

func (d *safeDeviceImpl) GpmMigSampleGet(migInstanceID int, sample nvml.GpmSample) error {
	return deviceCall(d, gpmAPI("GpmMigSampleGet"), func() nvml.Return {
		return d.nvmlDevice.GpmMigSampleGet(migInstanceID, sample)
	})
}

func (d *safeDeviceImpl) IsMigDeviceHandle() (bool, error) {
	return deviceCall1(d, deviceAPI("IsMigDeviceHandle"), d.nvmlDevice.IsMigDeviceHandle)
}

func (d *safeDeviceImpl) GetVirtualizationMode() (nvml.GpuVirtualizationMode, error) {
	return deviceCall1(d, deviceAPI("GetVirtualizationMode"), d.nvmlDevice.GetVirtualizationMode)
}

func (d *safeDeviceImpl) GetSupportedEventTypes() (uint64, error) {
	return deviceCall1(d, deviceAPI("GetSupportedEventTypes"), d.nvmlDevice.GetSupportedEventTypes)
}

func (d *safeDeviceImpl) RegisterEvents(evtTypes uint64, evtSet nvml.EventSet) error {
	return deviceCall(d, deviceAPI("RegisterEvents"), func() nvml.Return {
		return d.nvmlDevice.RegisterEvents(evtTypes, evtSet)
	})
}

func (d *safeDeviceImpl) GetMemoryErrorCounter(errorType nvml.MemoryErrorType, eccCounterType nvml.EccCounterType, memoryLocation nvml.MemoryLocation) (uint64, error) {
	return deviceCall1(d, deviceAPI("GetMemoryErrorCounter"), func() (uint64, nvml.Return) {
		return d.nvmlDevice.GetMemoryErrorCounter(errorType, eccCounterType, memoryLocation)
	})
}

func (d *safeDeviceImpl) GetSramEccErrorStatus() (nvml.EccSramErrorStatus, error) {
	return deviceCall1(d, deviceAPI("GetSramEccErrorStatus"), d.nvmlDevice.GetSramEccErrorStatus)
}

func (d *safeDeviceImpl) GetRunningProcessDetailList() (nvml.ProcessDetailList, error) {
	return deviceCall1(d, deviceAPI("GetRunningProcessDetailList"), d.nvmlDevice.GetRunningProcessDetailList)
}
