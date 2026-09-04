// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package testutil

import (
	"encoding/binary"
	"maps"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
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
// and should be the same length as GPUUUIDs.
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

// MockGpmMetricValue is a single NVML GPM metric response.
type MockGpmMetricValue struct {
	Value  float64
	Return nvml.Return
}

// DefaultFieldValues are deterministic values for NVML fields used by GPU tests.
// Capability/topology fields can still be overridden by mock options such as
// WithCapabilities.
var DefaultFieldValues = map[uint32]MockFieldValue{
	nvml.FI_DEV_MEMORY_TEMP:                                  NewFieldValue(42),
	nvml.FI_DEV_PCIE_REPLAY_COUNTER:                          NewFieldValue(7),
	nvml.FI_DEV_PERF_POLICY_THERMAL:                          NewFieldValue(85),
	nvml.FI_DEV_NVLINK_LINK_COUNT:                            NewFieldValue(0),
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
