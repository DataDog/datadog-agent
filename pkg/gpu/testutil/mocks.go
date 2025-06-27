// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package testutil holds different utilities and stubs for testing
package testutil

import (
	"reflect"
	"strings"
	"testing"

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
const DefaultGpuCores = 10

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
var GPUCores = []int{DefaultGpuCores, 20, 40, 60, 80, 100, 120}

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

// DefaultMaxClockRates is an array of Max SM clock and Max Mem Clock rates for the default device
var DefaultMaxClockRates = [2]uint32{1000, 2000}

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
			switch clockType {
			case nvml.CLOCK_SM:
				return DefaultMaxClockRates[0], nvml.SUCCESS
			case nvml.CLOCK_MEM:
				return DefaultMaxClockRates[1], nvml.SUCCESS
			default:
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
		},
		GetIndexFunc: func() (int, nvml.Return) {
			return deviceIdx, nvml.SUCCESS
		},
		IsMigDeviceHandleFunc: func() (bool, nvml.Return) {
			return false, nvml.SUCCESS
		},
		GetSamplesFunc: func(_ nvml.SamplingType, _ uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
			samples := []nvml.Sample{
				{TimeStamp: 1000, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 1}},
				{TimeStamp: 2000, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 2}},
			}
			return nvml.VALUE_TYPE_UNSIGNED_INT, samples, nvml.SUCCESS
		},
		GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
			return nvml.GpmSupport{IsSupportedDevice: 1}, nvml.SUCCESS
		},
	}

	for _, opt := range opts {
		opt(mock)
	}

	return mock
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
		d.IsMigDeviceHandleFunc = func() (bool, nvml.Return) {
			return true, nvml.SUCCESS
		}

		// Override GetAttributesFunc for this specific MIG child to correctly distribute parent's resources.
		d.GetAttributesFunc = func() (nvml.DeviceAttributes, nvml.Return) {
			numMigChildrenForParent, ok := MIGChildrenPerDevice[deviceIdx]
			if !ok || numMigChildrenForParent == 0 {
				// Should not happen if MIGChildrenPerDevice is consistent for a MIG-enabled parent
				// Return error
				return nvml.DeviceAttributes{}, nvml.ERROR_NOT_SUPPORTED
			}

			// core count and total memory - equally distribute between all mig devices
			// in the future, we might want to make this more sophisticated, to support more complex scenarios in our UTs
			parentTotalCores := GPUCores[deviceIdx]
			coresPerMigDevice := parentTotalCores / numMigChildrenForParent
			memoryPerMigDevice := DefaultTotalMemory / uint64(numMigChildrenForParent)

			migSpecificAttributes := nvml.DeviceAttributes{
				MultiprocessorCount: uint32(coresPerMigDevice),
				MemorySizeMB:        memoryPerMigDevice / (1024 * 1024),
			}

			return migSpecificAttributes, nvml.SUCCESS
		}
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

// GetBasicNvmlMock returns a mock of the nvml.Interface with a single device with 10 cores,
// useful for basic tests that need only the basic interaction with NVML to be working.
func GetBasicNvmlMock() *nvmlmock.Interface {
	return GetBasicNvmlMockWithOptions()
}

// GetBasicNvmlMockWithOptions returns a mock of the nvml.Interface with a single device with 10 cores,
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
