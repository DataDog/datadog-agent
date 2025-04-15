// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package testutil holds different utilities and stubs for testing
package testutil

import (
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
var GPUCores = []int{DefaultGpuCores, 20, 30, 40, 50, 60, 70}

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
var DefaultTotalMemory = uint64(1000)

// DefaultMaxClockRates is an array of Max SM clock and Max Mem Clock rates for the default device
var DefaultMaxClockRates = [2]uint32{1000, 2000}

// GetDeviceMock returns a mock of the nvml.Device with the given UUID.
func GetDeviceMock(deviceIdx int) *nvmlmock.Device {
	return &nvmlmock.Device{
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
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
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
	}
}

// GetBasicNvmlMock returns a mock of the nvml.Interface with a single device with 10 cores,
// useful for basic tests that need only the basic interaction with NVML to be working.
func GetBasicNvmlMock() *nvmlmock.Interface {
	if len(GPUUUIDs) != len(GPUCores) {
		// Make it really easy to spot errors if we change any of the arrays.
		panic("GPUUUIDs and GPUCores must have the same length, please fix it")
	}

	return &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return len(GPUUUIDs), nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			return GetDeviceMock(index), nvml.SUCCESS
		},
		DeviceGetCudaComputeCapabilityFunc: func(nvml.Device) (int, int, nvml.Return) {
			return 7, 5, nvml.SUCCESS
		},
		DeviceGetIndexFunc: func(nvml.Device) (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		DeviceGetMigModeFunc: func(nvml.Device) (int, int, nvml.Return) {
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
		},
		DeviceGetComputeRunningProcessesFunc: func(nvml.Device) ([]nvml.ProcessInfo, nvml.Return) {
			return DefaultProcessInfo, nvml.SUCCESS
		},
		DeviceGetMemoryInfoFunc: func(nvml.Device) (nvml.Memory, nvml.Return) {
			return nvml.Memory{Total: DefaultTotalMemory, Free: 500}, nvml.SUCCESS
		},
		SystemGetDriverVersionFunc: func() (string, nvml.Return) {
			return DefaultNvidiaDriverVersion, nvml.SUCCESS
		},
	}
}

// GetBasicNvmlMockWithOptions returns a mock of the nvml.Interface with a single device with 10 cores,
// allowing additional configuration through functional options.
// It's ideal for tests that need custom NVML behavior beyond the defaults.
func GetBasicNvmlMockWithOptions(options ...func(*nvmlmock.Interface)) *nvmlmock.Interface {
	if len(GPUUUIDs) != len(GPUCores) {
		// Make it really easy to spot errors if we change any of the arrays.
		panic("GPUUUIDs and GPUCores must have the same length, please fix it")
	}

	mockNvml := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return len(GPUUUIDs), nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			return GetDeviceMock(index), nvml.SUCCESS
		},
		DeviceGetCudaComputeCapabilityFunc: func(nvml.Device) (int, int, nvml.Return) {
			return 7, 5, nvml.SUCCESS
		},
		DeviceGetIndexFunc: func(nvml.Device) (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		DeviceGetMigModeFunc: func(nvml.Device) (int, int, nvml.Return) {
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
		},
		DeviceGetComputeRunningProcessesFunc: func(nvml.Device) ([]nvml.ProcessInfo, nvml.Return) {
			return DefaultProcessInfo, nvml.SUCCESS
		},
		DeviceGetMemoryInfoFunc: func(nvml.Device) (nvml.Memory, nvml.Return) {
			return nvml.Memory{Total: DefaultTotalMemory, Free: 500}, nvml.SUCCESS
		},
		SystemGetDriverVersionFunc: func() (string, nvml.Return) {
			return DefaultNvidiaDriverVersion, nvml.SUCCESS
		},
	}

	// Apply all provided options
	for _, opt := range options {
		opt(mockNvml)
	}

	return mockNvml
}

// WithSymbolsMock returns an option that configures the mock NVML interface with the given symbols available.
// It takes a map of symbols that should be considered available in the mock.
// Any symbol not in the map will return an error when looked up.
func WithSymbolsMock(availableSymbols map[string]struct{}) func(*nvmlmock.Interface) {
	return func(mockNvml *nvmlmock.Interface) {
		mockNvml.ExtensionsFunc = func() nvml.ExtendedInterface {
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
