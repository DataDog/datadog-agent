// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package testutil

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

type deviceOptions struct {
	compatibilityHooks  []func(*MockDevice)
	mode                DeviceFeatureMode
	migEnabled          bool
	migChildIndex       *int
	uuid                *string
	archSet             bool
	architecture        nvml.DeviceArchitecture
	computeMajor        int
	computeMinor        int
	processDataCallback func(uuid string) (MockProcessInfoList, nvml.Return)
	gpmSupported        *bool
	gpmSampleGetFunc    func(*MockGpmSample) nvml.Return
	migDeviceCountFunc  func(deviceIdx int) int
	nvlinkGeneration    int
	nvlinkLinkCount     int
	fieldValues         map[uint32]MockFieldValue
	scopedFieldValues   map[uint32]map[uint32]MockFieldValue
	nvlinkStates        []nvml.EnableState
	nvlinkStateErrors   map[int]nvml.Return
	migChildUUIDs       map[int]string

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

func (o deviceOptions) effectiveUUID() string {
	return *o.uuid
}

func (o deviceOptions) migDeviceCount(deviceIdx int) int {
	if o.migDeviceCountFunc != nil {
		return o.migDeviceCountFunc(deviceIdx)
	}
	return len(o.migChildUUIDs)
}

func (o deviceOptions) processDataUUID() string {
	if o.isMIGChild() {
		return o.migChildUUIDs[*o.migChildIndex]
	}
	return o.effectiveUUID()
}

func withMIGChild(migDeviceIdx int, opts deviceOptions) deviceOptions {
	childOpts := opts
	childIdx := migDeviceIdx
	childOpts.migChildIndex = &childIdx
	childOpts.mode = DeviceFeatureMIG
	childOpts.migEnabled = false

	// MIG children report invalid argument for architecture APIs.
	childOpts.archSet = false

	// Keep compatibility hooks from parent options.
	if len(opts.compatibilityHooks) > 0 {
		childOpts.compatibilityHooks = slices.Clone(opts.compatibilityHooks)
	}

	// Ensure the parent has MIG children and the index is valid.
	if _, ok := opts.migChildUUIDs[migDeviceIdx]; !ok {
		childOpts.migChildIndex = nil
	}

	return childOpts
}

func configureDeviceMock(mock *MockDevice, deviceIdx int, opts deviceOptions, migDevices map[int]nvml.Device) {
	fieldValuesCounter := uint64(0)
	fieldValuesCounterMu := sync.Mutex{}
	arch, major, minor := opts.effectiveArchitecture()
	isMIGUnsupported := opts.shouldMarkMIGUnsupported()
	isMIGOrVGPUUnsupported := opts.shouldMarkMIGOrVGPUUnsupported()
	deviceUUID := opts.effectiveUUID()
	deviceMigChildren := opts.migChildUUIDs

	mock.Device = nvmlmock.Device{
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
		GetGpuFabricInfoFunc: func() (nvml.GpuFabricInfo, nvml.Return) {
			return nvml.GpuFabricInfo{}, nvml.ERROR_NOT_SUPPORTED
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
			if opts.isMIGChild() || !opts.migEnabled {
				return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
			}
			if opts.migDeviceCount(deviceIdx) > 0 {
				return nvml.DEVICE_MIG_ENABLE, 0, nvml.SUCCESS
			}
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
		},
		GetMaxMigDeviceCountFunc: func() (int, nvml.Return) {
			if opts.isMIGChild() || !opts.migEnabled {
				return 0, nvml.SUCCESS
			}
			return opts.migDeviceCount(deviceIdx), nvml.SUCCESS
		},
		GetMigDeviceHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			if opts.isMIGChild() || !opts.migEnabled {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}
			device, ok := migDevices[index]
			if !ok {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}
			return device, nvml.SUCCESS
		},
		GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
			if opts.processDataCallback != nil {
				proc, ret := opts.processDataCallback(opts.processDataUUID())
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
		GetNvLinkVersionFunc: func(link int) (uint32, nvml.Return) {
			if isMIGUnsupported || !opts.nvlinkSupported() || link >= opts.nvlinkLinkCount {
				return 0, nvml.ERROR_NOT_SUPPORTED
			}
			return uint32(opts.nvlinkGeneration), nvml.SUCCESS
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
				processes, ret := opts.processDataCallback(opts.processDataUUID())
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

				if mockFieldValue := mock.getFieldValue(values[i].FieldId, values[i].ScopeId); mockFieldValue != nil {
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
			if opts.gpmSupported == nil || !*opts.gpmSupported {
				return nvml.GpmSupport{IsSupportedDevice: 0}, nvml.SUCCESS
			}
			return nvml.GpmSupport{IsSupportedDevice: 1}, nvml.SUCCESS
		},
		GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
			if opts.isVGPU() || opts.gpmSupported == nil || !*opts.gpmSupported {
				return nvml.ERROR_NOT_SUPPORTED
			}
			if opts.gpmSampleGetFunc != nil {
				mockSample, ok := sample.(*MockGpmSample)
				if !ok {
					return nvml.ERROR_INVALID_ARGUMENT
				}
				return opts.gpmSampleGetFunc(mockSample)
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
