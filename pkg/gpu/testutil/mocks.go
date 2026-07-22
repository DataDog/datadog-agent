// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package testutil

import (
	"encoding/binary"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	logslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
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

// DefaultPCIBusIDFields are the PCI bus ID fields for the default device returned by the mock.
var DefaultPCIBusIDFields = nvml.PciInfo{
	Domain: 0,
	Bus:    0,
	Device: 0x1e,
}

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
var DefaultProcessInfo = MockProcessInfoList{
	{Pid: 1, UsedGpuMemory: 100},
	{Pid: 5678, UsedGpuMemory: 200},
}

// DefaultActivePIDs returns the PIDs of DefaultProcessInfo, matching the active
// PIDs the mock reports for a default device.
func DefaultActivePIDs() []int {
	pids := make([]int, len(DefaultProcessInfo))
	for i, proc := range DefaultProcessInfo {
		pids[i] = int(proc.Pid)
	}
	return pids
}

// DefaultTotalMemory is the total memory for the default device returned by the mock.
// The MiB count (3072) is divisible by the MIG child counts used in tests (2 and 3) so
// that the parent memory derived from GPU instance profiles round-trips exactly.
var DefaultTotalMemory = uint64(3 * 1024 * 1024 * 1024)

// DefaultMaxClockRates is an array of Max clock rates for the default device
var DefaultMaxClockRates = map[nvml.ClockType]uint32{
	nvml.CLOCK_SM:       1000,
	nvml.CLOCK_MEM:      2000,
	nvml.CLOCK_GRAPHICS: 3000,
	nvml.CLOCK_VIDEO:    4000,
}

// MockFieldValue is a single NVML field value response.
type MockFieldValue struct {
	Value     uint64
	ValueType nvml.ValueType
	Return    nvml.Return
}

// DefaultFieldValues are deterministic values for NVML fields used by GPU tests.
// Capability/topology fields can still be overridden by mock options such as
// WithCapabilities.
var DefaultFieldValues = map[uint32]MockFieldValue{
	nvml.FI_DEV_MEMORY_TEMP:                                  NewFieldValue(42),
	nvml.FI_DEV_PCIE_REPLAY_COUNTER:                          NewFieldValue(7),
	nvml.FI_DEV_PERF_POLICY_THERMAL:                          NewFieldValue(85),
	nvml.FI_DEV_NVLINK_LINK_COUNT:                            NewFieldValue(2),
	nvml.FI_DEV_C2C_LINK_COUNT:                               NewFieldValue(0),
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX:                    NewFieldValue(1000),
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX:                    NewFieldValue(2000),
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX:                     NewFieldValue(3000),
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX:                     NewFieldValue(4000),
	nvml.FI_DEV_NVLINK_COUNT_RCV_BYTES:                       NewFieldValue(5000),
	nvml.FI_DEV_NVLINK_COUNT_XMIT_BYTES:                      NewFieldValue(6000),
	nvml.FI_DEV_NVLINK_GET_SPEED:                             NewFieldValue(25000),
	nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON:                     NewFieldValue(24000),
	nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT:                NewFieldValue(16),
	nvml.FI_DEV_GET_GPU_RECOVERY_ACTION:                      NewFieldValue(uint64(nvml.GPU_RECOVERY_ACTION_NONE)),
	nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL:            NewFieldValue(1),
	nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL:            NewFieldValue(2),
	nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL:            NewFieldValue(3),
	nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL:            NewFieldValue(4),
	nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL:              NewFieldValue(5),
	nvml.FI_DEV_NVLINK_COUNT_XMIT_PACKETS:                    NewFieldValue(6),
	nvml.FI_DEV_NVLINK_COUNT_RCV_PACKETS:                     NewFieldValue(7),
	nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS:                   NewFieldValue(8),
	nvml.FI_DEV_NVLINK_COUNT_MALFORMED_PACKET_ERRORS:         NewFieldValue(9),
	nvml.FI_DEV_NVLINK_COUNT_BUFFER_OVERRUN_ERRORS:           NewFieldValue(10),
	nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS:                      NewFieldValue(11),
	nvml.FI_DEV_NVLINK_COUNT_RCV_REMOTE_ERRORS:               NewFieldValue(12),
	nvml.FI_DEV_NVLINK_COUNT_RCV_GENERAL_ERRORS:              NewFieldValue(13),
	nvml.FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS:     NewFieldValue(14),
	nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS: NewFieldValue(15),
	nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS:     NewFieldValue(16),
	nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS:                NewFieldValue(17),
	nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_BER:                   NewFieldValue(18),
	nvml.FI_DEV_NVLINK_COUNT_SYMBOL_ERRORS:                   NewFieldValue(19),
	nvml.FI_DEV_NVLINK_COUNT_SYMBOL_BER:                      NewFieldValue(20),
	nvml.FI_DEV_C2C_LINK_ERROR_INTR:                          NewFieldValue(37),
	nvml.FI_DEV_C2C_LINK_ERROR_REPLAY:                        NewFieldValue(38),
	nvml.FI_DEV_C2C_LINK_ERROR_REPLAY_B2B:                    NewFieldValue(39),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_0:                   NewFieldValue(100),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_1:                   NewFieldValue(101),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_2:                   NewFieldValue(102),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_3:                   NewFieldValue(103),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_4:                   NewFieldValue(104),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_5:                   NewFieldValue(105),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_6:                   NewFieldValue(106),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_7:                   NewFieldValue(107),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_8:                   NewFieldValue(108),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_9:                   NewFieldValue(109),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_10:                  NewFieldValue(110),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_11:                  NewFieldValue(111),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_12:                  NewFieldValue(112),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_13:                  NewFieldValue(113),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_14:                  NewFieldValue(114),
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_15:                  NewFieldValue(115),
}

// NewFieldValue returns a successful unsigned-long-long NVML field value.
func NewFieldValue(value uint64) MockFieldValue {
	return MockFieldValue{Value: value, ValueType: nvml.VALUE_TYPE_UNSIGNED_LONG_LONG, Return: nvml.SUCCESS}
}

// FieldError returns an NVML field value with the given field-level return.
func FieldError(ret nvml.Return) MockFieldValue {
	return MockFieldValue{Return: ret}
}

// ApplyMockFieldValue writes a mock value into an NVML FieldValue.
func ApplyMockFieldValue(fv *nvml.FieldValue, value MockFieldValue) {
	fv.NvmlReturn = uint32(value.Return)
	fv.ValueType = uint32(value.ValueType)
	binary.LittleEndian.PutUint64(fv.Value[:], value.Value)
}

const (
	mockPpcntGroupPLR         = 0x22
	mockPpcntSizeBytes        = 256
	mockRegTLVHeaderLenDwords = 1
	mockDwordSizeBytes        = 4
)

var MIGUUIDs = map[int]string{
	0: "MIG-00000000-1234-1234-1234-123456789012",
	1: "MIG-11111111-1234-1234-1234-123456789013",
	2: "MIG-22222222-1234-1234-1234-123456789014",
	3: "MIG-33333333-1234-1234-1234-123456789015",
	4: "MIG-44444444-1234-1234-1234-123456789016",
}

const DefaultMIGParentDeviceIdx = 5

// MIGChildrenUUIDs is a map of device index to the UUIDs of the MIG children for that device.
var MIGChildrenUUIDs = map[int]map[int]string{
	DefaultMIGParentDeviceIdx: {0: MIGUUIDs[0], 1: MIGUUIDs[1]},
	6:                         {0: MIGUUIDs[2], 1: MIGUUIDs[3], 2: MIGUUIDs[4]},
}

func DefaultDevicesWithMIGChildren() []int {
	return slices.Collect(maps.Keys(MIGChildrenUUIDs))
}

type deviceOptions struct {
	compatibilityHooks  []func(*nvmlmock.Device)
	mode                DeviceFeatureMode
	migDisabled         bool
	migChildIndex       *int
	archSet             bool
	architecture        nvml.DeviceArchitecture
	computeMajor        int
	computeMinor        int
	processDataCallback func(uuid string) (MockProcessInfoList, nvml.Return)
	gpmSupported        *bool
	nvlinkGeneration    int
	nvlinkLinkCount     int
	fieldValues         map[uint32]MockFieldValue
	scopedFieldValues   map[uint32]map[uint32]MockFieldValue
	nvlinkStates        []nvml.EnableState
	nvlinkStateErrors   map[int]nvml.Return
	migChildUUIDs       map[int]map[int]string
	parentUUIDs         []string

	fieldValuesReturn  *nvml.Return
	samplesUnsupported bool
	processDetailList  *processDetailListResponse
}

type processDetailListResponse struct {
	processes []nvml.ProcessDetail_v1
	ret       nvml.Return
}

func (o deviceOptions) isMIGChild() bool {
	return o.migChildIndex != nil
}

func (o deviceOptions) isMIGMode() bool {
	return o.mode == DeviceFeatureMIG
}

func (o deviceOptions) isVGPU() bool {
	return o.mode == DeviceFeatureVGPU
}

func (o deviceOptions) shouldMarkMIGUnsupported() bool {
	return o.isMIGMode() || o.isMIGChild()
}

func (o deviceOptions) shouldMarkMIGOrVGPUUnsupported() bool {
	return o.shouldMarkMIGUnsupported() || o.isVGPU()
}

func (o deviceOptions) nvlinkSupported() bool {
	return o.nvlinkGeneration > 0
}

func (o deviceOptions) effectiveArchitecture() (nvml.DeviceArchitecture, int, int) {
	if o.archSet {
		return o.architecture, o.computeMajor, o.computeMinor
	}
	return DefaultGPUArch, DefaultGPUComputeCapMajor, DefaultGPUComputeCapMinor
}

// getFieldValue returns the MockFieldValue for the given field ID and scope ID, or nil if not found on any of the maps
func (o deviceOptions) getFieldValue(fieldID uint32, scopeID uint32) *MockFieldValue {
	if o.scopedFieldValues != nil {
		if scopedValues, ok := o.scopedFieldValues[fieldID]; ok {
			if value, ok := scopedValues[scopeID]; ok {
				return &value
			}
		}
	}

	if o.fieldValues != nil {
		if value, ok := o.fieldValues[fieldID]; ok {
			return &value
		}
	}

	return nil
}

func withMIGChild(deviceIdx int, migDeviceIdx int, opts deviceOptions) deviceOptions {
	childOpts := opts
	childIdx := migDeviceIdx
	childOpts.migChildIndex = &childIdx
	childOpts.mode = DeviceFeatureMIG
	childOpts.migDisabled = true

	// MIG children report invalid argument for architecture APIs.
	childOpts.archSet = false

	// Keep compatibility hooks from parent options.
	if len(opts.compatibilityHooks) > 0 {
		childOpts.compatibilityHooks = append([]func(*nvmlmock.Device){}, opts.compatibilityHooks...)
	}

	// Ensure the parent has MIG children and the index is valid.
	if _, ok := opts.migChildUUIDs[deviceIdx][migDeviceIdx]; !ok {
		childOpts.migChildIndex = nil
	}

	return childOpts
}

// GetDeviceMock returns a mock of the nvml.Device with the given UUID.
func GetDeviceMock(deviceIdx int, opts ...func(*nvmlmock.Device)) *nvmlmock.Device {
	var mockOpts []NvmlMockOption
	for _, opt := range opts {
		mockOpts = append(mockOpts, WithCustomHook(opt))
	}

	libOpts := newNvmlMockOptions(mockOpts...)
	libOpts.deviceOptions.mode = DeviceFeaturePhysical

	return getDeviceMockWithOptions(deviceIdx, libOpts.deviceOptions)
}

func getMIGDeviceMockWithOptions(deviceIdx int, migDeviceIdx int, opts deviceOptions) *nvmlmock.Device {
	return getDeviceMockWithOptions(deviceIdx, withMIGChild(deviceIdx, migDeviceIdx, opts))
}

func getDeviceMockWithOptions(deviceIdx int, opts deviceOptions) *nvmlmock.Device {
	fieldValuesCounter := uint64(0)
	fieldValuesCounterMu := sync.Mutex{}
	arch, major, minor := opts.effectiveArchitecture()
	isMIGUnsupported := opts.shouldMarkMIGUnsupported()
	isMIGOrVGPUUnsupported := opts.shouldMarkMIGOrVGPUUnsupported()
	deviceUUID := opts.parentUUIDs[deviceIdx]
	deviceMigChildren := opts.migChildUUIDs[deviceIdx]
	processDataUUID := func() string {
		if opts.isMIGChild() {
			return deviceMigChildren[*opts.migChildIndex]
		}
		return deviceUUID
	}

	mock := &nvmlmock.Device{
		GetNumGpuCoresFunc: func() (int, nvml.Return) {
			return GPUCores[deviceIdx], nvml.SUCCESS
		},
		GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
			if opts.isMIGChild() {
				return 0, 0, nvml.ERROR_INVALID_ARGUMENT
			}
			return major, minor, nvml.SUCCESS
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			if opts.isMIGChild() && deviceMigChildren != nil {
				return deviceMigChildren[*opts.migChildIndex], nvml.SUCCESS
			}
			return deviceUUID, nvml.SUCCESS
		},
		GetNameFunc: func() (string, nvml.Return) {
			if opts.isMIGChild() {
				return DefaultGPUName + " MIG 3g.40gb", nvml.SUCCESS
			}
			return DefaultGPUName, nvml.SUCCESS
		},
		GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
			if opts.isMIGChild() {
				return nvml.DEVICE_ARCH_UNKNOWN, nvml.ERROR_INVALID_ARGUMENT
			}
			return arch, nvml.SUCCESS
		},
		GetAttributesFunc: func() (nvml.DeviceAttributes, nvml.Return) {
			if opts.isMIGChild() {
				if len(deviceMigChildren) == 0 {
					return nvml.DeviceAttributes{}, nvml.ERROR_NOT_SUPPORTED
				}

				profileInfo := getGpuInstanceProfileInfo(deviceIdx, len(deviceMigChildren))
				return nvml.DeviceAttributes{
					MultiprocessorCount: profileInfo.MultiprocessorCount,
					MemorySizeMB:        profileInfo.MemorySizeMB,
				}, nvml.SUCCESS
			}
			return DefaultGPUAttributes, nvml.SUCCESS
		},
		GetMigModeFunc: func() (int, int, nvml.Return) {
			if opts.isMIGChild() || opts.migDisabled {
				return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
			}
			if len(deviceMigChildren) > 0 {
				return nvml.DEVICE_MIG_ENABLE, 0, nvml.SUCCESS
			}
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
		},
		GetMaxMigDeviceCountFunc: func() (int, nvml.Return) {
			if opts.isMIGChild() || opts.migDisabled {
				return 0, nvml.SUCCESS
			}
			return len(deviceMigChildren), nvml.SUCCESS
		},
		GetMigDeviceHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			if opts.isMIGChild() || opts.migDisabled {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}
			if _, ok := deviceMigChildren[index]; !ok {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}
			return getMIGDeviceMockWithOptions(deviceIdx, index, opts), nvml.SUCCESS
		},
		GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
			if opts.processDataCallback != nil {
				proc, ret := opts.processDataCallback(processDataUUID())
				return proc.ProcessInfo(), ret
			}

			return DefaultProcessInfo.ProcessInfo(), nvml.SUCCESS
		},
		GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
			return nvml.Memory{Total: DefaultTotalMemory, Free: 500}, nvml.SUCCESS
		},
		GetMemoryInfo_v2Func: func() (nvml.Memory_v2, nvml.Return) {
			return nvml.Memory_v2{}, nvml.SUCCESS
		},
		GetMemoryBusWidthFunc: func() (uint32, nvml.Return) {
			return DefaultMemoryBusWidth, nvml.SUCCESS
		},
		GetMaxClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			rate, ok := DefaultMaxClockRates[clockType]
			if !ok {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return rate, nvml.SUCCESS
		},
		GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
			if isMIGUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			rate, ok := DefaultMaxClockRates[clockType]
			if !ok {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return rate / 2, nvml.SUCCESS
		},
		GetCurrentClocksThrottleReasonsFunc: func() (uint64, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 0, nvml.SUCCESS
		},
		GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 250000, nvml.SUCCESS
		},
		GetPowerUsageFunc: func() (uint32, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 150000, nvml.SUCCESS
		},
		GetTotalEnergyConsumptionFunc: func() (uint64, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			if arch < nvml.DEVICE_ARCH_VOLTA {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 12345, nvml.SUCCESS
		},
		GetTemperatureFunc: func(_ nvml.TemperatureSensors) (uint32, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 60, nvml.SUCCESS
		},
		GetFanSpeedFunc: func() (uint32, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 40, nvml.SUCCESS
		},
		GetPerformanceStateFunc: func() (nvml.Pstates, nvml.Return) {
			if isMIGUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return nvml.PSTATE_0, nvml.SUCCESS
		},
		GetPcieThroughputFunc: func(_ nvml.PcieUtilCounter) (uint32, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 42, nvml.SUCCESS
		},
		GetPcieReplayCounterFunc: func() (int, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 0, nvml.SUCCESS
		},
		GetCurrPcieLinkGenerationFunc: func() (int, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 1, nvml.SUCCESS
		},
		GetMaxPcieLinkGenerationFunc: func() (int, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 4, nvml.SUCCESS
		},
		GetCurrPcieLinkWidthFunc: func() (int, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 8, nvml.SUCCESS
		},
		GetMaxPcieLinkWidthFunc: func() (int, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 16, nvml.SUCCESS
		},
		GetPciInfoFunc: func() (nvml.PciInfo, nvml.Return) {
			return DefaultPCIBusIDFields, nvml.SUCCESS
		},
		GetRemappedRowsFunc: func() (int, int, bool, bool, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return 0, 0, false, false, nvml.ERROR_NOT_SUPPORTED
			}
			if arch < nvml.DEVICE_ARCH_AMPERE {
				return 0, 0, false, false, nvml.ERROR_NOT_SUPPORTED
			}
			return 0, 0, false, false, nvml.SUCCESS
		},
		GetRepairStatusFunc: func() (nvml.RepairStatus, nvml.Return) {
			if isMIGOrVGPUUnsupported {
				return nvml.RepairStatus{}, nvml.ERROR_NOT_SUPPORTED
			}
			if arch < nvml.DEVICE_ARCH_AMPERE {
				return nvml.RepairStatus{}, nvml.ERROR_NOT_SUPPORTED
			}
			return nvml.RepairStatus{}, nvml.SUCCESS
		},
		GetNvLinkStateFunc: func(link int) (nvml.EnableState, nvml.Return) {
			if isMIGUnsupported || !opts.nvlinkSupported() {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			if ret, ok := opts.nvlinkStateErrors[link]; ok && ret != nvml.SUCCESS {
				return 0, ret
			}
			if opts.nvlinkStates != nil {
				if link >= len(opts.nvlinkStates) {
					return 0, nvml.ERROR_INVALID_ARGUMENT
				}
				return opts.nvlinkStates[link], nvml.SUCCESS
			}
			if opts.nvlinkLinkCount == 0 {
				return nvml.FEATURE_DISABLED, nvml.SUCCESS
			}
			return nvml.FEATURE_ENABLED, nvml.SUCCESS
		},
		GetNvLinkUtilizationCounterFunc: func(_, _ int) (uint64, uint64, nvml.Return) {
			if isMIGOrVGPUUnsupported || !opts.nvlinkSupported() || opts.nvlinkLinkCount == 0 {
				return 0, 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 100, 200, nvml.SUCCESS
		},
		GetNvLinkErrorCounterFunc: func(_ int, _ nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
			if isMIGOrVGPUUnsupported || !opts.nvlinkSupported() || opts.nvlinkLinkCount == 0 {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 0, nvml.SUCCESS
		},
		GetBAR1MemoryInfoFunc: func() (nvml.BAR1Memory, nvml.Return) {
			return nvml.BAR1Memory{}, nvml.SUCCESS
		},
		GetMemoryErrorCounterFunc: func(_ nvml.MemoryErrorType, _ nvml.EccCounterType, _ nvml.MemoryLocation) (uint64, nvml.Return) {
			if isMIGUnsupported {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return 0, nvml.SUCCESS
		},
		GetSramEccErrorStatusFunc: func() (nvml.EccSramErrorStatus, nvml.Return) {
			if isMIGUnsupported || arch < nvml.DEVICE_ARCH_AMPERE {
				return nvml.EccSramErrorStatus{}, nvml.ERROR_NOT_SUPPORTED
			}
			return nvml.EccSramErrorStatus{}, nvml.SUCCESS
		},
		GetIndexFunc: func() (int, nvml.Return) {
			return deviceIdx, nvml.SUCCESS
		},
		IsMigDeviceHandleFunc: func() (bool, nvml.Return) {
			return opts.isMIGChild(), nvml.SUCCESS
		},
		GetGpuInstanceIdFunc: func() (int, nvml.Return) {
			if !opts.isMIGChild() {
				return 0, nvml.ERROR_INVALID_ARGUMENT
			}
			return *opts.migChildIndex, nvml.SUCCESS
		},
		GetProcessUtilizationFunc: func(lastSeenTimestamp uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
			if isMIGUnsupported {
				return nil, nvml.ERROR_NOT_FOUND
			}
			if opts.processDataCallback != nil {
				processes, ret := opts.processDataCallback(processDataUUID())
				return processes.ProcessUtilizationSamples(), ret
			}

			// Return one process sample newer than lastSeenTimestamp so process.* metrics
			// are emitted by sampling collectors in spec tests.
			return []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: lastSeenTimestamp + 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			}, nvml.SUCCESS
		},
		GetSamplesFunc: func(samplingType nvml.SamplingType, lastSeenTimestamp uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
			if opts.samplesUnsupported {
				return nvml.VALUE_TYPE_UNSIGNED_INT, nil, nvml.ERROR_NOT_SUPPORTED
			}
			if isMIGUnsupported {
				return nvml.VALUE_TYPE_UNSIGNED_INT, nil, nvml.ERROR_NOT_FOUND
			}
			if opts.isVGPU() && (samplingType == nvml.ENC_UTILIZATION_SAMPLES || samplingType == nvml.DEC_UTILIZATION_SAMPLES) {
				return nvml.VALUE_TYPE_UNSIGNED_INT, nil, nvml.ERROR_NOT_FOUND
			}
			// Keep sample timestamps newer than lastSeenTimestamp so sample-based metrics
			// (dram_active, gr_engine_active, etc.) are emitted on collection runs.
			samples := []nvml.Sample{
				{TimeStamp: lastSeenTimestamp + 1000, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 1}},
				{TimeStamp: lastSeenTimestamp + 2000, SampleValue: [8]byte{0, 0, 0, 0, 0, 0, 0, 2}},
			}
			return nvml.VALUE_TYPE_UNSIGNED_INT, samples, nvml.SUCCESS
		},
		GetFieldValuesFunc: func(values []nvml.FieldValue) nvml.Return {
			fieldValuesCounterMu.Lock()
			defer fieldValuesCounterMu.Unlock()

			if opts.fieldValuesReturn != nil {
				return *opts.fieldValuesReturn
			}
			// Emulate monotonically increasing counters for field-based throughput metrics.
			// Fields collector computes rates from consecutive values, so counters must increase
			// between runs to emit nvlink.throughput.* metrics.
			fieldValuesCounter += 1000
			for i := range values {
				values[i].Timestamp = int64(time.Now().UnixMilli())

				if mockFieldValue := opts.getFieldValue(values[i].FieldId, values[i].ScopeId); mockFieldValue != nil {
					ApplyMockFieldValue(&values[i], *mockFieldValue)
					continue
				}

				value := fieldValuesCounter + uint64(i)
				if values[i].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
					value = uint64(opts.nvlinkLinkCount)
				}
				values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_LONG)

				var encoded [8]byte
				binary.LittleEndian.PutUint64(encoded[:], value)
				values[i].Value = encoded
			}
			return nvml.SUCCESS
		},
		GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
			if opts.isVGPU() {
				return nvml.GpmSupport{IsSupportedDevice: 0}, nvml.SUCCESS
			}
			if opts.gpmSupported != nil && !*opts.gpmSupported {
				return nvml.GpmSupport{IsSupportedDevice: 0}, nvml.SUCCESS
			}
			return nvml.GpmSupport{IsSupportedDevice: 1}, nvml.SUCCESS
		},
		GpmSampleGetFunc: func(_ nvml.GpmSample) nvml.Return {
			if opts.isVGPU() || opts.gpmSupported == nil || !*opts.gpmSupported {
				return nvml.ERROR_NOT_SUPPORTED
			}
			return nvml.SUCCESS
		},
		GpmMigSampleGetFunc: func(_ int, _ nvml.GpmSample) nvml.Return {
			if opts.isVGPU() || opts.gpmSupported == nil || !*opts.gpmSupported {
				return nvml.ERROR_NOT_SUPPORTED
			}
			return nvml.SUCCESS
		},
		GetVirtualizationModeFunc: func() (nvml.GpuVirtualizationMode, nvml.Return) {
			if opts.isVGPU() {
				return nvml.GPU_VIRTUALIZATION_MODE_VGPU, nvml.SUCCESS
			}
			return nvml.GPU_VIRTUALIZATION_MODE_NONE, nvml.SUCCESS
		},
		GetSupportedEventTypesFunc: func() (uint64, nvml.Return) {
			return nvml.EventTypeAll, nvml.SUCCESS
		},
		GetGpuInstanceProfileInfoFunc: func(profile int) (nvml.GpuInstanceProfileInfo, nvml.Return) {
			// TODO: handle the case where there are no MIG children but the device is MIG enabled.
			// Related ticket: EBPF-1118
			if profile != 0 {
				return nvml.GpuInstanceProfileInfo{}, nvml.ERROR_INVALID_ARGUMENT
			}
			return getGpuInstanceProfileInfo(deviceIdx, max(1, len(deviceMigChildren))), nvml.SUCCESS
		},
		ReadWritePRM_v1Func: func(buffer *nvml.PRMTLV_v1) nvml.Return {
			if opts.isVGPU() || opts.isMIGMode() || arch < nvml.DEVICE_ARCH_BLACKWELL {
				return nvml.ERROR_NOT_SUPPORTED
			}
			fillMockPLRPRMResponse(buffer)
			return nvml.SUCCESS
		},
	}

	if opts.processDetailList != nil {
		resp := opts.processDetailList
		mock.GetRunningProcessDetailListFunc = func() (nvml.ProcessDetailList, nvml.Return) {
			if resp.ret != nvml.SUCCESS {
				return nvml.ProcessDetailList{}, resp.ret
			}
			list := nvml.ProcessDetailList{NumProcArrayEntries: uint32(len(resp.processes))}
			if len(resp.processes) > 0 {
				list.ProcArray = &resp.processes[0]
			}
			return list, nvml.SUCCESS
		}
	}

	for _, opt := range opts.compatibilityHooks {
		opt(mock)
	}

	return mock
}

func fillMockPLRPRMResponse(buffer *nvml.PRMTLV_v1) {
	port := uint64(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)

	regHeaderOffset := 4 * mockDwordSizeBytes
	payloadOffset := regHeaderOffset + mockDwordSizeBytes
	regLenDwords := uint32(mockPpcntSizeBytes/mockDwordSizeBytes + mockRegTLVHeaderLenDwords)
	regHeader := uint32(3<<27) | (regLenDwords << 16)
	binary.BigEndian.PutUint32(buffer.InData[regHeaderOffset:payloadOffset], regHeader)

	payload := buffer.InData[payloadOffset : payloadOffset+mockPpcntSizeBytes]
	for i := range payload {
		payload[i] = 0
	}
	binary.BigEndian.PutUint32(payload[0:4], mockPpcntGroupPLR)

	offset := 2 * mockDwordSizeBytes
	for i := 0; i < 9; i++ {
		value := port*100 + uint64(i)
		binary.BigEndian.PutUint32(payload[offset:offset+mockDwordSizeBytes], uint32(value>>32))
		offset += mockDwordSizeBytes
		binary.BigEndian.PutUint32(payload[offset:offset+mockDwordSizeBytes], uint32(value))
		offset += mockDwordSizeBytes
	}
}

func getGpuInstanceProfileInfo(deviceIdx int, migChildCount int) nvml.GpuInstanceProfileInfo {
	// build a profile info consistent with the number of cores per multiprocessor
	// and the mig children count for this device
	// Hopper has 128 cores per multiprocessor, and that's the default arch we have.
	// If this is wrong, unit tests will fail as they ensure the core count is correct.
	parentMultiprocessorCount := uint32(GPUCores[deviceIdx]) / 128
	parentMemorySizeMB := DefaultTotalMemory / 1024 / 1024

	return nvml.GpuInstanceProfileInfo{
		MemorySizeMB:        parentMemorySizeMB / uint64(migChildCount),
		InstanceCount:       uint32(migChildCount),
		MultiprocessorCount: parentMultiprocessorCount / uint32(migChildCount),
	}
}

// GetMIGDeviceMock returns a mock of the MIG Device.
func GetMIGDeviceMock(deviceIdx int, migDeviceIdx int, opts ...func(*nvmlmock.Device)) *nvmlmock.Device {
	var mockOpts []NvmlMockOption
	for _, opt := range opts {
		mockOpts = append(mockOpts, WithCustomHook(opt))
	}

	// Route through newNvmlMockOptions so defaults (parentUUIDs, migChildUUIDs,
	// fieldValues) are populated; getDeviceMockWithOptions indexes them directly.
	libOpts := newNvmlMockOptions(mockOpts...)
	libOpts.deviceOptions.mode = DeviceFeatureMIG

	return getMIGDeviceMockWithOptions(deviceIdx, migDeviceIdx, libOpts.deviceOptions)
}

type nvmlMockOptions struct {
	deviceOptions   deviceOptions
	libOptions      []func(*nvmlmock.Interface)
	deviceCountFunc func() int
	extensionsFunc  func() nvml.ExtendedInterface
}

// NvmlMockOption is a functional option for configuring the nvml mock.
type NvmlMockOption func(*nvmlMockOptions)

// WithMIGDisabled disables MIG support for the nvml mock.
func WithMIGDisabled() NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.migDisabled = true
	}
}

// WithDeviceCount influences the return value of DeviceGetCount for the nvml mock
func WithDeviceCount(count int) NvmlMockOption {
	return WithDeviceCountFunc(func() int {
		return count
	})
}

// WithDeviceCountFunc allows setting a dynamic device count function for the nvml mock.
func WithDeviceCountFunc(fn func() int) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceCountFunc = fn
	}
}

// WithMIGChildUUIDs sets the UUIDs of the MIG children for the nvml mock. Use it to customize the MIG children returned and their UUIDs
func WithMIGChildUUIDs(uuids map[int]map[int]string) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.migChildUUIDs = uuids
	}
}

// WithDeviceUUID overrides the UUID returned by device mocks built from this option.
func WithDeviceUUID(uuid string) func(*nvmlmock.Device) {
	return func(d *nvmlmock.Device) {
		d.GetUUIDFunc = func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		}
	}
}

// WithFieldValuesFullOverride sets field values returned by GetFieldValues for all mock devices. Overrides the entire default set of field values.
func WithFieldValuesFullOverride(values map[uint32]MockFieldValue) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.fieldValues = values
	}
}

// WithFieldValuesPartialOverride sets field values returned by GetFieldValues for all mock devices. Only updates the provided field values, leaving the rest unchanged.
func WithFieldValuesPartialOverride(values map[uint32]MockFieldValue) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		for fieldID, value := range values {
			o.deviceOptions.fieldValues[fieldID] = value
		}
	}
}

// WithUnsupportedFields marks fields as unsupported in GetFieldValues responses.
func WithUnsupportedFields(fields ...uint32) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		for _, fieldID := range fields {
			o.deviceOptions.fieldValues[fieldID] = FieldError(nvml.ERROR_NOT_SUPPORTED)
		}
	}
}

// WithInvalidArgumentFields marks fields as invalid arguments in GetFieldValues responses.
func WithInvalidArgumentFields(fields ...uint32) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		for _, fieldID := range fields {
			o.deviceOptions.fieldValues[fieldID] = FieldError(nvml.ERROR_INVALID_ARGUMENT)
		}
	}
}

// WithScopedFieldValues sets per-scope field values returned by GetFieldValues for all mock devices.
func WithScopedFieldValues(values map[uint32]map[uint32]MockFieldValue) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		if o.deviceOptions.scopedFieldValues == nil {
			o.deviceOptions.scopedFieldValues = make(map[uint32]map[uint32]MockFieldValue, len(values))
		}
		for fieldID, scopedValues := range values {
			if o.deviceOptions.scopedFieldValues[fieldID] == nil {
				o.deviceOptions.scopedFieldValues[fieldID] = make(map[uint32]MockFieldValue, len(scopedValues))
			}
			for scopeID, value := range scopedValues {
				o.deviceOptions.scopedFieldValues[fieldID][scopeID] = value
			}
		}
	}
}

// WithNVLinkLinkCount configures the number of NVLink ports returned by field queries.
func WithNVLinkLinkCount(count int) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.nvlinkLinkCount = count
		o.deviceOptions.fieldValues[nvml.FI_DEV_NVLINK_LINK_COUNT] = NewFieldValue(uint64(count))
	}
}

// WithNVLinkStates sets the per-link NVLink states returned by GetNvLinkState and the
// link count reported by field queries. stateErrors maps a link index to a non-success
// return code, which takes precedence over the state for that link. The number of links
// reported via FI_DEV_NVLINK_LINK_COUNT is derived from len(states).
//
// NVLink support (generation) must be configured independently (e.g. via WithCapabilities),
// otherwise GetNvLinkState returns ERROR_NOT_SUPPORTED.
func WithNVLinkStates(states []nvml.EnableState, stateErrors map[int]nvml.Return) NvmlMockOption {
	return WithCombinedOptions(
		WithNVLinkLinkCount(len(states)),
		func(o *nvmlMockOptions) {
			o.deviceOptions.nvlinkStates = states
			o.deviceOptions.nvlinkStateErrors = stateErrors
		},
	)
}

// WithC2CLinkCount configures the number of C2C links returned by field queries.
func WithC2CLinkCount(count int) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.fieldValues[nvml.FI_DEV_C2C_LINK_COUNT] = NewFieldValue(uint64(count))
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

// MockProcessData is a single process data entry for the mock, which can be
// used to have a single entry for all process-related NVML APIs
type MockProcessData struct {
	// Common fields
	Pid       uint32
	TimeStamp uint64

	// nvml.ProcessInfo fields
	UsedGpuMemory uint64

	// nvml.ProcessUtilizationSample fields
	SmUtil  uint32
	MemUtil uint32
	EncUtil uint32
	DecUtil uint32
}

// ProcessInfo returns the process info for the mock.
func (m *MockProcessData) ProcessInfo() nvml.ProcessInfo {
	return nvml.ProcessInfo{
		Pid:           m.Pid,
		UsedGpuMemory: m.UsedGpuMemory,
	}
}

// ProcessUtilizationSample returns the process utilization sample for the mock.
func (m *MockProcessData) ProcessUtilizationSample() nvml.ProcessUtilizationSample {
	return nvml.ProcessUtilizationSample{
		Pid:       m.Pid,
		TimeStamp: m.TimeStamp,
		SmUtil:    m.SmUtil,
		MemUtil:   m.MemUtil,
		EncUtil:   m.EncUtil,
		DecUtil:   m.DecUtil,
	}
}

// MockProcessInfoList is a list of process data for the mock.
type MockProcessInfoList []MockProcessData

// ProcessInfo returns the process info for the mock.
func (m MockProcessInfoList) ProcessInfo() []nvml.ProcessInfo {
	processInfo := make([]nvml.ProcessInfo, len(m))
	for i, process := range m {
		processInfo[i] = process.ProcessInfo()
	}
	return processInfo
}

// ProcessUtilizationSamples returns the process utilization samples for the mock.
func (m MockProcessInfoList) ProcessUtilizationSamples() []nvml.ProcessUtilizationSample {
	processUtilizationSamples := make([]nvml.ProcessUtilizationSample, len(m))
	for i, process := range m {
		processUtilizationSamples[i] = process.ProcessUtilizationSample()
	}
	return processUtilizationSamples
}

// WithProcessData sets the process data returned by the mock.
func WithProcessData(processData []MockProcessData, returnCode nvml.Return) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.processDataCallback = func(_ string) (MockProcessInfoList, nvml.Return) {
			return MockProcessInfoList(processData), returnCode
		}
	}
}

// WithProcessDataCallback influences the return value of GetComputeRunningProcessesFunc for the nvml mock
func WithProcessDataCallback(callback func(uuid string) (MockProcessInfoList, nvml.Return)) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.processDataCallback = callback
	}
}

// WithFieldValuesReturn forces GetFieldValues to return the given code for every call,
// without populating any field values. Use it to exercise the path where the whole
// field API fails (distinct from WithUnsupportedFields, which marks individual fields).
func WithFieldValuesReturn(ret nvml.Return) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.fieldValuesReturn = &ret
	}
}

// WithSamplesUnsupported makes GetSamples return ERROR_NOT_SUPPORTED for all sampling types.
func WithSamplesUnsupported() NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.samplesUnsupported = true
	}
}

// WithProcessDetailList configures GetRunningProcessDetailList to return the given
// processes, or the given error code when ret is not nvml.SUCCESS.
func WithProcessDetailList(processes []nvml.ProcessDetail_v1, ret nvml.Return) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.processDetailList = &processDetailListResponse{processes: processes, ret: ret}
	}
}

func WithCustomHook(hook func(*nvmlmock.Device)) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.compatibilityHooks = append(o.deviceOptions.compatibilityHooks, hook)
	}
}

func WithCustomLibHook(hook func(*nvmlmock.Interface)) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.libOptions = append(o.libOptions, hook)
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
		nvml.DEVICE_ARCH_BLACKWELL,
		10,
		0,
	},
}

// WithArchitecture sets device architecture and compute capability from a spec architecture name
// (e.g. "fermi", "kepler", "hopper"). Panics on unknown architecture name.
func WithArchitecture(archName string) NvmlMockOption {
	arch, major, minor := ArchNameToNVML(archName)
	return func(o *nvmlMockOptions) {
		o.deviceOptions.archSet = true
		o.deviceOptions.architecture = arch
		o.deviceOptions.computeMajor = major
		o.deviceOptions.computeMinor = minor
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
		return setMode(DeviceFeaturePhysical, true)
	case DeviceFeatureMIG:
		return WithCombinedOptions(
			WithDeviceCount(1),
			WithMIGChildUUIDs(map[int]map[int]string{
				0: MIGChildrenUUIDs[DefaultMIGParentDeviceIdx],
			}),
			setMode(DeviceFeatureMIG, false),
		)
	case DeviceFeatureVGPU:
		return setMode(DeviceFeatureVGPU, true)
	default:
		return func(*nvmlMockOptions) {}
	}
}

func setMode(mode DeviceFeatureMode, migDisabled bool) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.mode = mode
		o.deviceOptions.migDisabled = migDisabled
	}
}

func WithCombinedOptions(options ...NvmlMockOption) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		for _, opt := range options {
			opt(o)
		}
	}
}

// WithDeviceUUIDs configures the mock so that the device UUIDs are the given UUIDs.
func WithDeviceUUIDs(uuids []string) NvmlMockOption {
	return func(o *nvmlMockOptions) {
		o.deviceOptions.parentUUIDs = uuids
	}
}

// Capabilities drives architecture-gated API support in the mock
// (e.g. from spec/architectures.yaml capabilities).
// process_detail_list is derived from architecture (Hopper+ only) and is not a capability.
type Capabilities struct {
	GPM                       bool
	NvLinkGenerationSupported int
	NvLinkLinkCount           int
	C2C                       bool
	UnsupportedFields         []uint32
}

// WithCapabilities configures the mock so that architecture-gated APIs return
// NOT_SUPPORTED or equivalent when a capability is false.
func WithCapabilities(caps Capabilities) NvmlMockOption {
	opts := []NvmlMockOption{
		func(o *nvmlMockOptions) {
			o.deviceOptions.gpmSupported = &caps.GPM
			o.deviceOptions.nvlinkGeneration = caps.NvLinkGenerationSupported
		},
		WithNVLinkLinkCount(caps.NvLinkLinkCount),
	}

	if len(caps.UnsupportedFields) > 0 {
		opts = append(opts, WithUnsupportedFields(caps.UnsupportedFields...))
	}

	return WithCombinedOptions(opts...)
}

// GetBasicNvmlMock returns a mock of the nvml.Interface with the default devices and options.
func GetBasicNvmlMock() *nvmlmock.Interface {
	return GetBasicNvmlMockWithOptions()
}

func newNvmlMockOptions(options ...NvmlMockOption) *nvmlMockOptions {
	opts := &nvmlMockOptions{
		deviceOptions: deviceOptions{
			fieldValues: maps.Clone(DefaultFieldValues),
		},
	}
	for _, opt := range options {
		opt(opts)
	}

	if opts.deviceOptions.parentUUIDs == nil {
		opts.deviceOptions.parentUUIDs = GPUUUIDs
	}
	if opts.deviceOptions.migChildUUIDs == nil {
		opts.deviceOptions.migChildUUIDs = MIGChildrenUUIDs
	}

	return opts
}

// GetBasicNvmlMockWithOptions returns a mock of the nvml.Interface with the default devices and options,
// allowing additional configuration through functional options.
// It's ideal for tests that need custom NVML behavior beyond the defaults.
func GetBasicNvmlMockWithOptions(options ...NvmlMockOption) *nvmlmock.Interface {
	if len(GPUUUIDs) != len(GPUCores) {
		// Make it really easy to spot errors if we change any of the arrays.
		panic("GPUUUIDs and GPUCores must have the same length, please fix it")
	}

	opts := newNvmlMockOptions(options...)

	mockNvml := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			if opts.deviceCountFunc != nil {
				return opts.deviceCountFunc(), nvml.SUCCESS
			}
			return len(opts.deviceOptions.parentUUIDs), nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			devCount := len(opts.deviceOptions.parentUUIDs)
			if opts.deviceCountFunc != nil {
				devCount = opts.deviceCountFunc()
			}
			if index >= devCount {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}

			return getDeviceMockWithOptions(index, opts.deviceOptions), nvml.SUCCESS
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
		GpmSampleAllocFunc: func() (nvml.GpmSample, nvml.Return) {
			return &MockGpmSample{}, nvml.SUCCESS
		},
		GpmSampleFreeFunc: func(_ nvml.GpmSample) nvml.Return {
			return nvml.SUCCESS
		},
		GpmMetricsGetFunc: func(metricsGet *nvml.GpmMetricsGetType) nvml.Return {
			for i := range metricsGet.Metrics[:metricsGet.NumMetrics] {
				metricsGet.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
			}

			return nvml.SUCCESS
		},
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
		o.deviceOptions.compatibilityHooks = append(o.deviceOptions.compatibilityHooks, WithMockAllDeviceFunctions())
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
	opts := []fx.Option{
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	}

	// If the test is a fuzz test, the logger provided in core.MockBundle() will be created with the wrong testing.TB
	// and cause a panic.
	if _, ok := t.(*testing.F); ok {
		// fx.Decorate allows transforming a given component, in this case we replace it with a disabled logger
		opts = append(opts, fx.Decorate(func(log.Component) log.Component { return logslog.Disabled() }))
	}

	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(opts...))
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
	return fxutil.Test[telemetry.Mock](t, mocktelemetry.Module())
}

// GetTotalExpectedDevices calculates the total number of devices (physical + MIG)
// based on the mock data defined in this package.
func GetTotalExpectedDevices() int {
	numPhysical := len(GPUUUIDs)
	numMIG := 0
	for _, children := range MIGChildrenUUIDs {
		numMIG += len(children)
	}
	return numPhysical + numMIG
}

type MockGpmSample struct {
	nvml.GpmSample
}
