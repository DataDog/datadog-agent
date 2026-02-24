// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package testutil

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// DefaultGpuCores is the default number of cores for a GPU device in the mock.
const DefaultGpuCores = 1024

// GPUUUIDs is a list of UUIDs for the devices returned by the mock
var GPUUUIDs = []string{
	"GPU-00000000-1234-1234-1234-123456789012",
	"GPU-11111111-1234-1234-1234-123456789013",
	"GPU-22222222-1234-1234-1234-123456789014",
	"GPU-33333333-1234-1234-1234-123456789015",
	"GPU-44444444-1234-1234-1234-123456789016",
	"GPU-55555555-1234-1234-1234-123456789017",
	"GPU-66666666-1234-1234-1234-123456789018",
}

// GPUCores is a list of number of cores for the devices returned by the mock,
// should be the same length as GPUUUIDs. If not, GetBasicNvmlMock will panic.
// Note: it is important to keep the cores count divisible by 4, to allow proper calculations for MIG children cores
var GPUCores = []int{DefaultGpuCores, 2048, 4096, 6144, 8192, 10240, 12288}

// DefaultGpuUUID is the UUID for the default device returned by the mock
var DefaultGpuUUID = GPUUUIDs[0]

// DefaultGPUName is the name for the default device returned by the mock
var DefaultGPUName = "Tesla T4"

// DefaultNvidiaDriverVersion is the default nvidia driver version
var DefaultNvidiaDriverVersion = "470.57.02"

// DefaultMemoryBusWidth is the memory bus width for the default device returned by the mock
var DefaultMemoryBusWidth = uint32(256)

// DefaultGPUComputeCapMajor is the major number for the compute capabilities for the default device returned by the mock
var DefaultGPUComputeCapMajor = 7

// DefaultGPUComputeCapMinor is the minor number for the compute capabilities for the default device returned by the mock
var DefaultGPUComputeCapMinor = 5

// DefaultSMVersion is the SM version for the default device returned by the mock
var DefaultSMVersion = uint32(DefaultGPUComputeCapMajor*10 + DefaultGPUComputeCapMinor)

// DefaultGPUArch is the architecture for the default device returned by the mock
var DefaultGPUArch = nvml.DeviceArchitecture(nvml.DEVICE_ARCH_HOPPER)

// DefaultGPUAttributes is the attributes for the default device returned by the mock
var DefaultGPUAttributes = nvml.DeviceAttributes{
	MultiprocessorCount: 10,
}

// DefaultProcessInfo is the list of processes running on the default device returned by the mock
var DefaultProcessInfo = []nvml.ProcessInfo{
	{Pid: 1, UsedGpuMemory: 100},
	{Pid: 5678, UsedGpuMemory: 200},
}

// DefaultTotalMemory is the total memory for the default device returned by the mock
var DefaultTotalMemory = uint64(1024 * 1024 * 1024)

// DefaultMaxClockRates is an array of Max clock rates for the default device
var DefaultMaxClockRates = map[nvml.ClockType]uint32{
	nvml.CLOCK_SM:       1000,
	nvml.CLOCK_MEM:      2000,
	nvml.CLOCK_GRAPHICS: 3000,
	nvml.CLOCK_VIDEO:    4000,
}

// DevicesWithMIGChildren is a list of device indexes that have MIG children.
var DevicesWithMIGChildren = []int{5, 6}

// MIGChildrenPerDevice is a map of device index to the number of MIG children for that device.
var MIGChildrenPerDevice = map[int]int{
	5: 2,
	6: 4,
}

// MIGChildrenUUIDs is a map of device index to the UUIDs of the MIG children for that device.
var MIGChildrenUUIDs = map[int][]string{
	5: {"MIG-00000000-1234-1234-1234-123456789012", "MIG-11111111-1234-1234-1234-123456789013"},
	6: {"MIG-22222222-1234-1234-1234-123456789014", "MIG-33333333-1234-1234-1234-123456789015", "MIG-44444444-1234-1234-1234-123456789016", "MIG-55555555-1234-1234-1234-123456789017"},
}

// GetDeviceMock returns a mock of the nvml.Device with the given UUID.
func GetDeviceMock(deviceIdx int, opts ...func(*nvmlmock.Device)) *nvmlmock.Device {
	fieldValuesCounter := uint64(0)
	mock := &nvmlmock.Device{
		GetNumGpuCoresFunc: func() (int, nvml.Return) {
			return GPUCores[deviceIdx], nvml.SUCCESS
		},
		GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
			return 7, 5, nvml.SUCCESS
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return GPUUUIDs[deviceIdx], nvml.SUCCESS
		},
		GetNameFunc: func() (string, nvml.Return) {
			return DefaultGPUName, nvml.SUCCESS
		},
		GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
			return DefaultGPUArch, nvml.SUCCESS
		},
		GetAttributesFunc: func() (nvml.DeviceAttributes, nvml.Return) {
			return DefaultGPUAttributes, nvml.SUCCESS
		},
		GetMigModeFunc: func() (int, int, nvml.Return) {
			if children, ok := MIGChildrenPerDevice[deviceIdx]; ok && children > 0 {
				return nvml.DEVICE_MIG_ENABLE, 0, nvml.SUCCESS
			}
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
		},
		GetMaxMigDeviceCountFunc: func() (int, nvml.Return) {
			return MIGChildrenPerDevice[deviceIdx], nvml.SUCCESS
		},
		GetMigDeviceHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			if index >= MIGChildrenPerDevice[deviceIdx] {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}

			return GetMIGDeviceMock(deviceIdx, index, opts...), nvml.SUCCESS
		},
		GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
			return DefaultProcessInfo, nvml.SUCCESS
		},
		GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
			return nvml.Memory{Total: DefaultTotalMemory, Free: 500}, nvml.SUCCESS
		},
		GetMemoryBusWidthFunc: func() (uint32, nvml.Return) {
			return DefaultMemoryBusWidth, nvml.SUCCESS
		},
		GetMaxClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
			rate, ok := DefaultMaxClockRates[clockType]
			if !ok {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return rate, nvml.SUCCESS
		},
		GetIndexFunc: func() (int, nvml.Return) {
			return deviceIdx, nvml.SUCCESS
		},
		IsMigDeviceHandleFunc: func() (bool, nvml.Return) {
			return false, nvml.SUCCESS
		},
		GetProcessUtilizationFunc: func(lastSeenTimestamp uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
			// Return one process sample newer than lastSeenTimestamp so process.* metrics
			// are emitted by sampling collectors in spec tests.
			return []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: lastSeenTimestamp + 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			}, nvml.SUCCESS
		},
		GetSamplesFunc: func(_ nvml.SamplingType, lastSeenTimestamp uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
			// Keep sample timestamps newer than lastSeenTimestamp so sample-based metrics
			// (dram_active, gr_engine_active, etc.) are emitted on collection runs.
			samples := []nvml.Sample{
				{TimeStamp: lastSeenTimestamp + 1000, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 1}},
				{TimeStamp: lastSeenTimestamp + 2000, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 2}},
			}
			return nvml.VALUE_TYPE_UNSIGNED_INT, samples, nvml.SUCCESS
		},
		GetFieldValuesFunc: func(values []nvml.FieldValue) nvml.Return {
			// Emulate monotonically increasing counters for field-based throughput metrics.
			// Fields collector computes rates from consecutive values, so counters must increase
			// between runs to emit nvlink.throughput.* metrics.
			fieldValuesCounter += 1000
			for i := range values {
				values[i].Timestamp = int64(time.Now().UnixMilli())
				values[i].NvmlReturn = uint32(nvml.SUCCESS)
				values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_LONG_LONG)

				var encoded [8]byte
				binary.LittleEndian.PutUint64(encoded[:], fieldValuesCounter+uint64(i))
				values[i].Value = encoded
			}
			return nvml.SUCCESS
		},
		GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
			return nvml.GpmSupport{IsSupportedDevice: 1}, nvml.SUCCESS
		},
		GetVirtualizationModeFunc: func() (nvml.GpuVirtualizationMode, nvml.Return) {
			return nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS
		},
		GetSupportedEventTypesFunc: func() (uint64, nvml.Return) {
			return nvml.EventTypeAll, nvml.SUCCESS
		},
		GetGpuInstanceProfileInfoFunc: func(profile int) (nvml.GpuInstanceProfileInfo, nvml.Return) {
			if _, isMig := MIGChildrenPerDevice[deviceIdx]; !isMig || profile != 0 {
				return nvml.GpuInstanceProfileInfo{}, nvml.ERROR_INVALID_ARGUMENT
			}
			return getGpuInstanceProfileInfo(deviceIdx), nvml.SUCCESS
		},
	}

	for _, opt := range opts {
		opt(mock)
	}

	return mock
}

func getGpuInstanceProfileInfo(deviceIdx int) nvml.GpuInstanceProfileInfo {
	// build a profile info consistent with the number of cores per multiprocessor
	// and the mig children count for this device
	// Hopper has 128 cores per multiprocessor, and that's the default arch we have.
	// If this is wrong, unit tests will fail as they ensure the core count is correct.
	parentMultiprocessorCount := uint32(GPUCores[deviceIdx]) / 128
	parentMemorySizeMB := DefaultTotalMemory / 1024 / 1024
	instanceCount := MIGChildrenPerDevice[deviceIdx]

	return nvml.GpuInstanceProfileInfo{
		MemorySizeMB:        parentMemorySizeMB / uint64(instanceCount),
		InstanceCount:       uint32(instanceCount),
		MultiprocessorCount: parentMultiprocessorCount / uint32(instanceCount),
	}
}

func applyUnsupportedAPIsForMIGMode(d *nvmlmock.Device) {
	// Model MIG production gaps: keep identity APIs working but mark
	// physical-device-level APIs as unsupported/not-found.
	d.GetMaxClockInfoFunc = func(_ nvml.ClockType) (uint32, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetClockInfoFunc = func(_ nvml.ClockType) (uint32, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetCurrentClocksThrottleReasonsFunc = func() (uint64, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetFanSpeedFunc = func() (uint32, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetPowerManagementLimitFunc = func() (uint32, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetPowerUsageFunc = func() (uint32, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetTotalEnergyConsumptionFunc = func() (uint64, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetTemperatureFunc = func(_ nvml.TemperatureSensors) (uint32, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetPerformanceStateFunc = func() (nvml.Pstates, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetRemappedRowsFunc = func() (int, int, bool, bool, nvml.Return) {
		return 0, 0, false, false, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetPcieReplayCounterFunc = func() (int, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetPcieThroughputFunc = func(_ nvml.PcieUtilCounter) (uint32, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetNvLinkStateFunc = func(_ int) (nvml.EnableState, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetNvLinkUtilizationCounterFunc = func(_, _ int) (uint64, uint64, nvml.Return) {
		return 0, 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetNvLinkErrorCounterFunc = func(_ int, _ nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetFieldValuesFunc = func(_ []nvml.FieldValue) nvml.Return {
		return nvml.ERROR_NOT_SUPPORTED
	}
	d.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
		return nil, nvml.ERROR_NOT_FOUND
	}
	d.GetSamplesFunc = func(_ nvml.SamplingType, _ uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
		return nvml.VALUE_TYPE_UNSIGNED_INT, nil, nvml.ERROR_NOT_FOUND
	}
	d.GetBAR1MemoryInfoFunc = func() (nvml.BAR1Memory, nvml.Return) {
		return nvml.BAR1Memory{}, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetMemoryInfo_v2Func = func() (nvml.Memory_v2, nvml.Return) {
		return nvml.Memory_v2{}, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetMemoryInfoFunc = func() (nvml.Memory, nvml.Return) {
		return nvml.Memory{}, nvml.ERROR_NOT_SUPPORTED
	}
	d.GetMemoryErrorCounterFunc = func(_ nvml.MemoryErrorType, _ nvml.EccCounterType, _ nvml.MemoryLocation) (uint64, nvml.Return) {
		return 0, nvml.ERROR_NOT_SUPPORTED
	}
}

// GetMIGDeviceMock returns a mock of the MIG Device.
func GetMIGDeviceMock(deviceIdx int, migDeviceIdx int, opts ...func(*nvmlmock.Device)) *nvmlmock.Device {
	// Take the original mock and override only the relevant functions to make it a MIG device.
	prepareMigDevice := func(d *nvmlmock.Device) {
		// No MIG children
		d.GetMigModeFunc = func() (int, int, nvml.Return) {
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
		}

		// Change the UUID
		d.GetUUIDFunc = func() (string, nvml.Return) {
			return MIGChildrenUUIDs[deviceIdx][migDeviceIdx], nvml.SUCCESS
		}

		// Change the name
		d.GetNameFunc = func() (string, nvml.Return) {
			return "MIG " + DefaultGPUName, nvml.SUCCESS
		}

		// MIG-Specific functions
		d.IsMigDeviceHandleFunc = func() (bool, nvml.Return) {
			return true, nvml.SUCCESS
		}
		d.GetGpuInstanceIdFunc = func() (int, nvml.Return) {
			return migDeviceIdx, nvml.SUCCESS
		}

		// Override GetAttributesFunc for this specific MIG child to correctly distribute parent's resources.
		d.GetAttributesFunc = func() (nvml.DeviceAttributes, nvml.Return) {
			numMigChildrenForParent, ok := MIGChildrenPerDevice[deviceIdx]
			if !ok || numMigChildrenForParent == 0 {
				// Should not happen if MIGChildrenPerDevice is consistent for a MIG-enabled parent
				// Return error
				return nvml.DeviceAttributes{}, nvml.ERROR_NOT_SUPPORTED
			}

			// use the common profile information to ensure consistent
			// attributes for all MIG devices and their parents
			profileInfo := getGpuInstanceProfileInfo(deviceIdx)

			migSpecificAttributes := nvml.DeviceAttributes{
				MultiprocessorCount: profileInfo.MultiprocessorCount,
				MemorySizeMB:        profileInfo.MemorySizeMB,
			}

			return migSpecificAttributes, nvml.SUCCESS
		}

		// Override functions that return errors for MIG devices
		d.GetArchitectureFunc = func() (nvml.DeviceArchitecture, nvml.Return) {
			return nvml.DEVICE_ARCH_UNKNOWN, nvml.ERROR_INVALID_ARGUMENT
		}
		d.GetCudaComputeCapabilityFunc = func() (int, int, nvml.Return) {
			return 0, 0, nvml.ERROR_INVALID_ARGUMENT
		}
		applyUnsupportedAPIsForMIGMode(d)
	}

	opts = append(opts, prepareMigDevice)

	return GetDeviceMock(deviceIdx, opts...)
}

type nvmlMockOptions struct {
	deviceOptions  []func(*nvmlmock.Device)
	libOptions     []func(*nvmlmock.Interface)
	extensionsFunc func() nvml.ExtendedInterface
}

// NvmlMockOption is a functional option for configuring the nvml mock.
type NvmlMockOption func(*nvmlMockOptions)

// WithMIGDisabled disables MIG support for the nvml mock.
func WithMIGDisabled() NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions = append(o.deviceOptions, func(d *nvmlmock.Device) {
			d.GetMigModeFunc = func() (int, int, nvml.Return) {
				return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
			}
		})
	}
}

// WithDeviceCount influences the return value of DeviceGetCount for the nvml mock
func WithDeviceCount(count int) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.libOptions = append(o.libOptions, func(lib *nvmlmock.Interface) {
			lib.DeviceGetCountFunc = func() (int, nvml.Return) {
				return count, nvml.SUCCESS
			}
			lib.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
				if index >= count {
					return nil, nvml.ERROR_INVALID_ARGUMENT
				}
				return GetDeviceMock(index, o.deviceOptions...), nvml.SUCCESS
			}
		})
	}
}

// WithEventSetCreate influences the definition of EventSetCreateFunc
func WithEventSetCreate(eventSetCreate func() (nvml.EventSet, nvml.Return)) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.libOptions = append(o.libOptions, func(lib *nvmlmock.Interface) {
			lib.EventSetCreateFunc = eventSetCreate
		})
	}
}

// WithProcessInfoCallback influences the return value of GetComputeRunningProcessesFunc for the nvml mock
func WithProcessInfoCallback(callback func(uuid string) ([]nvml.ProcessInfo, nvml.Return)) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions = append(o.deviceOptions, func(d *nvmlmock.Device) {
			uuid, _ := d.GetUUIDFunc()
			d.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
				return callback(uuid)
			}
		})
	}
}

// ArchNameToNVML converts a spec architecture name (e.g. "fermi", "kepler", "hopper") to
// NVML device architecture and compute capability (major, minor). It panics on unknown names.
func ArchNameToNVML(archName string) (arch nvml.DeviceArchitecture, major, minor int) {
	info, ok := archNameToNVML[archName]
	if !ok {
		panic("unknown architecture: " + archName)
	}
	return info.arch, info.major, info.minor
}

var archNameToNVML = map[string]struct {
	arch  nvml.DeviceArchitecture
	major int
	minor int
}{
	"fermi":   {nvml.DEVICE_ARCH_KEPLER - 1, 2, 0},
	"kepler":  {nvml.DEVICE_ARCH_KEPLER, 3, 0},
	"maxwell": {nvml.DEVICE_ARCH_MAXWELL, 5, 0},
	"pascal":  {nvml.DEVICE_ARCH_PASCAL, 6, 0},
	"volta":   {nvml.DEVICE_ARCH_VOLTA, 7, 0},
	"turing":  {nvml.DEVICE_ARCH_TURING, 7, 5},
	"ampere":  {nvml.DEVICE_ARCH_AMPERE, 8, 0},
	"hopper":  {nvml.DEVICE_ARCH_HOPPER, 9, 0},
	"ada":     {nvml.DEVICE_ARCH_ADA, 8, 9},
	"blackwell": {
		nvml.DeviceArchitecture(10), // nvml.DEVICE_ARCH_BLACKWELL in newer go-nvml
		10,
		0,
	},
}

// WithArchitecture sets device architecture and compute capability from a spec architecture name
// (e.g. "fermi", "kepler", "hopper"). Panics on unknown architecture name.
func WithArchitecture(archName string) NvmlMockOption {
	arch, major, minor := ArchNameToNVML(archName)
	return func(o *nvmlMockOptions) {
		o.deviceOptions = append(o.deviceOptions, func(d *nvmlmock.Device) {
			d.GetArchitectureFunc = func() (nvml.DeviceArchitecture, nvml.Return) {
				return arch, nvml.SUCCESS
			}
			d.GetCudaComputeCapabilityFunc = func() (int, int, nvml.Return) {
				return major, minor, nvml.SUCCESS
			}
			// Row remapping is only supported on Ampere+.
			if arch < nvml.DEVICE_ARCH_AMPERE {
				d.GetRemappedRowsFunc = func() (int, int, bool, bool, nvml.Return) {
					return 0, 0, false, false, nvml.ERROR_NOT_SUPPORTED
				}
			}
			// Total energy consumption is only supported on Volta+.
			if arch < nvml.DEVICE_ARCH_VOLTA {
				d.GetTotalEnergyConsumptionFunc = func() (uint64, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
			}
		})
	}
}

// DeviceFeatureMode is the device mode for capability-driven mocks.
type DeviceFeatureMode string

const (
	DeviceFeaturePhysical DeviceFeatureMode = "physical"
	DeviceFeatureMIG      DeviceFeatureMode = "mig"
	DeviceFeatureVGPU     DeviceFeatureMode = "vgpu"
)

// WithDeviceFeatureMode configures the mock for physical, mig, or vgpu behavior.
// - physical: default; MIG disabled, virtualization none.
// - mig: one physical device with MIG enabled; DeviceGetHandleByIndex(0) returns a physical device that has MIG children (from DevicesWithMIGChildren). The cache will enumerate MIG children via GetMigDeviceHandleByIndex.
// - vgpu: GetVirtualizationMode returns HOST_VGPU; sampling APIs can return ERROR_NOT_FOUND.
func WithDeviceFeatureMode(mode DeviceFeatureMode) NvmlMockOption {
	switch mode {
	case DeviceFeaturePhysical:
		return WithMIGDisabled()
	case DeviceFeatureMIG:
		parentIdx := DevicesWithMIGChildren[0]
		return func(o *nvmlMockOptions) {
			o.deviceOptions = append(o.deviceOptions, func(d *nvmlmock.Device) {
				applyUnsupportedAPIsForMIGMode(d)
			})
			o.libOptions = append(o.libOptions, func(lib *nvmlmock.Interface) {
				lib.DeviceGetCountFunc = func() (int, nvml.Return) {
					return 1, nvml.SUCCESS
				}
				lib.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
					if index != 0 {
						return nil, nvml.ERROR_INVALID_ARGUMENT
					}
					return GetDeviceMock(parentIdx, o.deviceOptions...), nvml.SUCCESS
				}
			})
		}
	case DeviceFeatureVGPU:
		return func(o *nvmlMockOptions) {
			o.deviceOptions = append(o.deviceOptions, func(d *nvmlmock.Device) {
				d.GetVirtualizationModeFunc = func() (nvml.GpuVirtualizationMode, nvml.Return) {
					return nvml.GPU_VIRTUALIZATION_MODE_HOST_VGPU, nvml.SUCCESS
				}
				// Model vGPU production gaps: keep base identity APIs working but mark
				// most performance/clock/power/link APIs as unsupported.
				d.GetMaxClockInfoFunc = func(_ nvml.ClockType) (uint32, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetClockInfoFunc = func(_ nvml.ClockType) (uint32, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetCurrentClocksThrottleReasonsFunc = func() (uint64, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetFanSpeedFunc = func() (uint32, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetPowerManagementLimitFunc = func() (uint32, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetPowerUsageFunc = func() (uint32, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetTotalEnergyConsumptionFunc = func() (uint64, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetTemperatureFunc = func(_ nvml.TemperatureSensors) (uint32, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetRemappedRowsFunc = func() (int, int, bool, bool, nvml.Return) {
					return 0, 0, false, false, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetPcieReplayCounterFunc = func() (int, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetPcieThroughputFunc = func(_ nvml.PcieUtilCounter) (uint32, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetNvLinkStateFunc = func(_ int) (nvml.EnableState, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetNvLinkUtilizationCounterFunc = func(_, _ int) (uint64, uint64, nvml.Return) {
					return 0, 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetNvLinkErrorCounterFunc = func(_ int, _ nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				d.GetFieldValuesFunc = func(_ []nvml.FieldValue) nvml.Return {
					return nvml.ERROR_NOT_SUPPORTED
				}
				d.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
					return nil, nvml.ERROR_NOT_FOUND
				}
				d.GetSamplesFunc = func(_ nvml.SamplingType, _ uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
					return nvml.VALUE_TYPE_UNSIGNED_INT, nil, nvml.ERROR_NOT_FOUND
				}
			})
		}
	default:
		return func(*nvmlMockOptions) {}
	}
}

// Capabilities drives architecture-gated API support in the mock
// (e.g. from spec/architectures.yaml capabilities).
// process_detail_list is derived from architecture (Hopper+ only) and is not a capability.
type Capabilities struct {
	GPM               bool
	UnsupportedFields []uint32
}

// WithCapabilities configures the mock so that architecture-gated APIs return
// NOT_SUPPORTED or equivalent when a capability is false.
func WithCapabilities(caps Capabilities) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions = append(o.deviceOptions, func(d *nvmlmock.Device) {
			if !caps.GPM {
				d.GpmQueryDeviceSupportFunc = func() (nvml.GpmSupport, nvml.Return) {
					return nvml.GpmSupport{IsSupportedDevice: 0}, nvml.SUCCESS
				}
			}
			if len(caps.UnsupportedFields) > 0 {
				unsupported := make(map[uint32]struct{}, len(caps.UnsupportedFields))
				for _, id := range caps.UnsupportedFields {
					unsupported[id] = struct{}{}
				}
				nvlinkAPIsUnsupported := false
				if _, found := unsupported[nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON]; found {
					nvlinkAPIsUnsupported = true
				}

				prevGetFieldValues := d.GetFieldValuesFunc
				d.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
					if prevGetFieldValues == nil {
						return nvml.ERROR_NOT_SUPPORTED
					}
					ret := prevGetFieldValues(values)
					if ret != nvml.SUCCESS {
						return ret
					}
					for i := range values {
						if _, found := unsupported[values[i].FieldId]; found {
							values[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
						}
					}
					return nvml.SUCCESS
				}

				if nvlinkAPIsUnsupported {
					d.GetNvLinkStateFunc = func(_ int) (nvml.EnableState, nvml.Return) {
						return 0, nvml.ERROR_NOT_SUPPORTED
					}
				}
			}
		})
	}
}

// GetBasicNvmlMock returns a mock of the nvml.Interface with the default devices and options.
func GetBasicNvmlMock() *nvmlmock.Interface {
	return GetBasicNvmlMockWithOptions()
}

// GetBasicNvmlMockWithOptions returns a mock of the nvml.Interface with the default devices and options,
// allowing additional configuration through functional options.
// It's ideal for tests that need custom NVML behavior beyond the defaults.
func GetBasicNvmlMockWithOptions(options ...NvmlMockOption) *nvmlmock.Interface {
	if len(GPUUUIDs) != len(GPUCores) {
		// Make it really easy to spot errors if we change any of the arrays.
		panic("GPUUUIDs and GPUCores must have the same length, please fix it")
	}

	opts := &nvmlMockOptions{}
	for _, opt := range options {
		opt(opts)
	}

	mockNvml := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return len(GPUUUIDs), nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			return GetDeviceMock(index, opts.deviceOptions...), nvml.SUCCESS
		},
		SystemGetDriverVersionFunc: func() (string, nvml.Return) {
			return DefaultNvidiaDriverVersion, nvml.SUCCESS
		},
		EventSetCreateFunc: func() (nvml.EventSet, nvml.Return) {
			return &nvmlmock.EventSet{
				FreeFunc: func() nvml.Return {
					return nvml.SUCCESS
				},
				WaitFunc: func(v uint32) (nvml.EventData, nvml.Return) {
					time.Sleep(time.Duration(v) * time.Millisecond)
					return nvml.EventData{}, nvml.ERROR_TIMEOUT
				},
			}, nvml.SUCCESS
		},
		EventSetFreeFunc: func(eventSet nvml.EventSet) nvml.Return {
			return eventSet.Free()
		},
		EventSetWaitFunc: func(eventSet nvml.EventSet, v uint32) (nvml.EventData, nvml.Return) {
			return eventSet.Wait(v)
		},
		ExtensionsFunc: opts.extensionsFunc,
	}

	for _, opt := range opts.libOptions {
		opt(mockNvml)
	}

	return mockNvml
}

// WithSymbolsMock returns an option that configures the mock NVML interface with the given symbols available.
// It takes a map of symbols that should be considered available in the mock.
// Any symbol not in the map will return an error when looked up.
func WithSymbolsMock(availableSymbols map[string]struct{}) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.extensionsFunc = func() nvml.ExtendedInterface {
			return &nvmlmock.ExtendedInterface{
				LookupSymbolFunc: func(symbol string) error {
					if _, ok := availableSymbols[symbol]; ok {
						return nil
					}
					return nvml.ERROR_NOT_FOUND
				},
			}
		}
	}
}

// WithMockAllFunctions returns an option that creates basic functions for all nvmlmock.Device.*Func attributes
// that return nil/zero values. This is useful for ensuring all functions are mocked even if not explicitly set.
// This is not the default behavior of the mock, as we want explicit errors if we use a function that is not mocked
// so that we implement the mocked method explicitly, controlling the inputs and outputs. However, in some cases
// (e.g., testing the collectors) we want to ensure that all functions are mocked without caring too much about the inputs and outputs.
func WithMockAllFunctions() NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions = append(o.deviceOptions, WithMockAllDeviceFunctions())
		o.libOptions = append(o.libOptions, func(i *nvmlmock.Interface) {
			fillAllMockFunctions(i)
		})
	}
}

// WithMockAllDeviceFunctions returns a device option that creates basic functions for all nvmlmock.Device.*Func attributes
// that return nil/zero values. This is useful for ensuring all functions are mocked even if not explicitly set.
func WithMockAllDeviceFunctions() func(*nvmlmock.Device) {
	return func(d *nvmlmock.Device) {
		fillAllMockFunctions(d)
	}
}

func fillAllMockFunctions[T any](obj T) {
	// Use reflection to find all *Func fields and set them to basic implementations
	val := reflect.ValueOf(obj).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Check if field name ends with "Func", is a function type, and is not already set
		if strings.HasSuffix(fieldType.Name, "Func") && field.Kind() == reflect.Func && field.IsZero() {
			// Create a basic function that returns zero values
			funcType := field.Type()
			funcValue := reflect.MakeFunc(funcType, func(_ []reflect.Value) []reflect.Value {
				// Return zero values for all return types
				results := make([]reflect.Value, funcType.NumOut())
				for j := 0; j < funcType.NumOut(); j++ {
					results[j] = reflect.Zero(funcType.Out(j))
				}
				return results
			})
			field.Set(funcValue)
		}
	}
}

// GetWorkloadMetaMock returns a mock of the workloadmeta.Component.
func GetWorkloadMetaMock(t testing.TB) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

// GetWorkloadMetaMockWithDefaultGPUs is the same as GetWorkloadMetaMock, but adds the GPUs of testutil.GPUUUIDs
func GetWorkloadMetaMockWithDefaultGPUs(t testing.TB) workloadmetamock.Mock {
	wmeta := GetWorkloadMetaMock(t)
	for _, uuid := range GPUUUIDs {
		wmeta.Set(&workloadmeta.GPU{
			EntityID: workloadmeta.EntityID{
				ID:   uuid,
				Kind: workloadmeta.KindGPU,
			},
		})
	}
	return wmeta
}

// GetTelemetryMock returns a mock of the telemetry.Component.
func GetTelemetryMock(t testing.TB) telemetry.Mock {
	return fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
}

// GetTotalExpectedDevices calculates the total number of devices (physical + MIG)
// based on the mock data defined in this package.
func GetTotalExpectedDevices() int {
	numPhysical := len(GPUUUIDs)
	numMIG := 0
	for _, count := range MIGChildrenPerDevice {
		numMIG += count
	}
	return numPhysical + numMIG
}
