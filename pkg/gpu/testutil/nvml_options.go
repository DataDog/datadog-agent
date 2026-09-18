// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package testutil

import (
	"maps"
	"reflect"
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

type nvmlMockOptions struct {
	defaultDeviceOptions deviceOptions
	libOptions           []func(*nvmlmock.Interface)
	deviceCountFunc      func() int
	deviceHandleFunc     func(int, nvml.Device) (nvml.Device, nvml.Return)
	extensionsFunc       func() nvml.ExtendedInterface
	gpmState             *mockGpmState
	gpmAllocFailureCall  int
	gpmMetricsGetFunc    func(*nvml.GpmMetricsGetType) nvml.Return
	gpmMetricValues      map[nvml.GpmMetricId]MockGpmMetricValue
	physicalDeviceUUIDs  []string
	deviceOptionsByIndex map[int][]NvmlDeviceOption
}

// NvmlMockOption is a functional option for configuring the nvml mock.
type NvmlMockOption interface {
	apply(*nvmlMockOptions)
}

// NvmlDeviceOption configures a mock device. When passed directly to NewMockNVML,
// it configures the defaults for every device.
type NvmlDeviceOption interface {
	NvmlMockOption
	applyDevice(*deviceOptions)
}

type libraryOption func(*nvmlMockOptions)

func (option libraryOption) apply(options *nvmlMockOptions) {
	option(options)
}

type deviceOption func(*deviceOptions)

func (option deviceOption) apply(options *nvmlMockOptions) {
	option(&options.defaultDeviceOptions)
}

func (option deviceOption) applyDevice(options *deviceOptions) {
	option(options)
}

// WithDeviceOptions applies device-related options to one device.
func WithDeviceOptions(deviceIdx int, options ...NvmlDeviceOption) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		if deviceIdx < 0 {
			panic("device index must be non-negative")
		}
		if o.deviceOptionsByIndex == nil {
			o.deviceOptionsByIndex = make(map[int][]NvmlDeviceOption)
		}
		o.deviceOptionsByIndex[deviceIdx] = append(o.deviceOptionsByIndex[deviceIdx], options...)
	})
}

// WithMIGEnabled enables MIG support for devices in this option's scope.
func WithMIGEnabled() NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.migEnabled = true
	})
}

// WithDefaultMIGDevices enables the default MIG child topology.
func WithDefaultMIGDevices() NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		for deviceIdx, children := range MIGChildrenUUIDs {
			WithDeviceOptions(
				deviceIdx,
				WithMIGEnabled(),
				WithMIGChildUUIDs(children),
			).apply(o)
		}
	})
}

// WithDeviceCount influences the return value of DeviceGetCount for the nvml mock
func WithDeviceCount(count int) NvmlMockOption {
	return WithDeviceCountFunc(func() int {
		return count
	})
}

// WithDeviceCountFunc allows setting a dynamic device count function for the nvml mock.
func WithDeviceCountFunc(fn func() int) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.deviceCountFunc = fn
	})
}

// WithDeviceHandleByIndexCallback customizes handle lookup while preserving
// access to the canonical device for valid indexes.
func WithDeviceHandleByIndexCallback(callback func(int, nvml.Device) (nvml.Device, nvml.Return)) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.deviceHandleFunc = callback
	})
}

// WithInitCallback customizes NVML initialization.
func WithInitCallback(callback func() nvml.Return) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.libOptions = append(o.libOptions, func(lib *nvmlmock.Interface) {
			lib.InitFunc = callback
		})
	})
}

// WithInitReturn configures NVML initialization to return ret.
func WithInitReturn(ret nvml.Return) NvmlMockOption {
	return WithInitCallback(func() nvml.Return { return ret })
}

// WithGpmSampleAllocFailure makes the one-based allocation call fail.
// Passing zero disables scripted allocation failure.
func WithGpmSampleAllocFailure(call int) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		if call < 0 {
			panic("GPM sample allocation failure call must be non-negative")
		}
		o.gpmAllocFailureCall = call
	})
}

// WithGpmMetricsGetCallback customizes GPM metric queries.
func WithGpmMetricsGetCallback(callback func(*nvml.GpmMetricsGetType) nvml.Return) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.gpmMetricsGetFunc = callback
	})
}

// WithGpmMetricValues sets responses for requested GPM metric IDs.
// Unconfigured metrics return success with a zero value.
func WithGpmMetricValues(values map[nvml.GpmMetricId]MockGpmMetricValue) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.gpmMetricValues = maps.Clone(values)
	})
}

// WithGpmSupport configures GPM support for devices in this option's scope.
func WithGpmSupport(supported bool) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.gpmSupported = &supported
	})
}

// WithGpmSampleGetCallback customizes GPM sample collection for devices in
// this option's scope.
func WithGpmSampleGetCallback(callback func(*MockGpmSample) nvml.Return) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.gpmSampleGetFunc = callback
	})
}

// WithMIGChildUUIDs sets the MIG children returned by devices in this option's scope.
func WithMIGChildUUIDs(uuids map[int]string) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.migChildUUIDs = uuids
	})
}

// WithMIGDeviceCountCallback dynamically controls how many configured MIG
// children are visible for each physical device.
func WithMIGDeviceCountCallback(callback func(deviceIdx int) int) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.migDeviceCountFunc = callback
	})
}

// WithFieldValuesFullOverride sets field values returned by GetFieldValues for all mock devices. Overrides the entire default set of field values.
func WithFieldValuesFullOverride(values map[uint32]MockFieldValue) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.fieldValues = values
	})
}

// WithFieldValuesPartialOverride sets field values returned by GetFieldValues for all mock devices. Only updates the provided field values, leaving the rest unchanged.
func WithFieldValuesPartialOverride(values map[uint32]MockFieldValue) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		for fieldID, value := range values {
			o.fieldValues[fieldID] = value
		}
	})
}

// WithUnsupportedFields marks fields as unsupported in GetFieldValues responses.
func WithUnsupportedFields(fields ...uint32) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		for _, fieldID := range fields {
			o.fieldValues[fieldID] = FieldError(nvml.ERROR_NOT_SUPPORTED)
		}
	})
}

// WithInvalidArgumentFields marks fields as invalid arguments in GetFieldValues responses.
func WithInvalidArgumentFields(fields ...uint32) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		for _, fieldID := range fields {
			o.fieldValues[fieldID] = FieldError(nvml.ERROR_INVALID_ARGUMENT)
		}
	})
}

// WithScopedFieldValues sets per-scope field values returned by GetFieldValues for all mock devices.
func WithScopedFieldValues(values map[uint32]map[uint32]MockFieldValue) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		if o.scopedFieldValues == nil {
			o.scopedFieldValues = make(map[uint32]map[uint32]MockFieldValue, len(values))
		}
		for fieldID, scopedValues := range values {
			if o.scopedFieldValues[fieldID] == nil {
				o.scopedFieldValues[fieldID] = make(map[uint32]MockFieldValue, len(scopedValues))
			}
			for scopeID, value := range scopedValues {
				o.scopedFieldValues[fieldID][scopeID] = value
			}
		}
	})
}

// WithNVLinkLinkCount configures the number of NVLink ports returned by field queries.
func WithNVLinkLinkCount(count int) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.nvlinkLinkCount = count
		o.fieldValues[nvml.FI_DEV_NVLINK_LINK_COUNT] = NewFieldValue(uint64(count))
	})
}

// WithNVLinkGeneration configures the supported NVLink generation.
func WithNVLinkGeneration(generation int) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.nvlinkGeneration = generation
	})
}

// WithNVLinkStates sets the per-link NVLink states returned by GetNvLinkState and the
// link count reported by field queries. stateErrors maps a link index to a non-success
// return code, which takes precedence over the state for that link. The number of links
// reported via FI_DEV_NVLINK_LINK_COUNT is derived from len(states).
//
// NVLink support (generation) must be configured independently (e.g. via WithCapabilities),
// otherwise GetNvLinkState returns ERROR_NOT_SUPPORTED.
func WithNVLinkStates(states []nvml.EnableState, stateErrors map[int]nvml.Return) NvmlDeviceOption {
	return WithCombinedDeviceOptions(
		WithNVLinkLinkCount(len(states)),
		deviceOption(func(o *deviceOptions) {
			o.nvlinkStates = states
			o.nvlinkStateErrors = stateErrors
		}),
	)
}

// WithC2CLinkCount configures the number of C2C links returned by field queries.
func WithC2CLinkCount(count int) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.fieldValues[nvml.FI_DEV_C2C_LINK_COUNT] = NewFieldValue(uint64(count))
	})
}

// WithEventSetCreate influences the definition of EventSetCreateFunc
func WithEventSetCreate(eventSetCreate func() (nvml.EventSet, nvml.Return)) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.libOptions = append(o.libOptions, func(lib *nvmlmock.Interface) {
			lib.EventSetCreateFunc = eventSetCreate
		})
	})
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
func WithProcessData(processData []MockProcessData, returnCode nvml.Return) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.processDataCallback = func(_ string) (MockProcessInfoList, nvml.Return) {
			return MockProcessInfoList(processData), returnCode
		}
	})
}

// WithProcessDataCallback influences the return value of GetComputeRunningProcessesFunc for the nvml mock
func WithProcessDataCallback(callback func(uuid string) (MockProcessInfoList, nvml.Return)) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.processDataCallback = callback
	})
}

// WithFieldValuesReturn forces GetFieldValues to return the given code for every call,
// without populating any field values. Use it to exercise the path where the whole
// field API fails (distinct from WithUnsupportedFields, which marks individual fields).
func WithFieldValuesReturn(ret nvml.Return) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.fieldValuesReturn = &ret
	})
}

// WithSamplesUnsupported makes GetSamples return ERROR_NOT_SUPPORTED for all sampling types.
func WithSamplesUnsupported() NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.samplesUnsupported = true
	})
}

// WithProcessDetailList configures GetRunningProcessDetailList to return the given
// processes, or the given error code when ret is not nvml.SUCCESS.
func WithProcessDetailList(processes []nvml.ProcessDetail_v1, ret nvml.Return) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.processDetailList = &processDetailListResponse{processes: processes, ret: ret}
	})
}

func WithCustomHook(hook func(*MockDevice)) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.compatibilityHooks = append(o.compatibilityHooks, hook)
	})
}

func WithCustomLibHook(hook func(*nvmlmock.Interface)) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.libOptions = append(o.libOptions, hook)
	})
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
func WithArchitecture(archName string) NvmlDeviceOption {
	arch, major, minor := ArchNameToNVML(archName)
	return deviceOption(func(o *deviceOptions) {
		o.archSet = true
		o.architecture = arch
		o.computeMajor = major
		o.computeMinor = minor
	})
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
// - mig: MIG enabled; children are configured independently with WithMIGChildUUIDs.
// - vgpu: GetVirtualizationMode returns HOST_VGPU; sampling APIs can return ERROR_NOT_FOUND.
func WithDeviceFeatureMode(mode DeviceFeatureMode) NvmlDeviceOption {
	switch mode {
	case DeviceFeaturePhysical:
		return setMode(DeviceFeaturePhysical, false)
	case DeviceFeatureMIG:
		return setMode(DeviceFeatureMIG, true)
	case DeviceFeatureVGPU:
		return setMode(DeviceFeatureVGPU, false)
	default:
		panic("unknown device feature mode: " + string(mode))
	}
}

func setMode(mode DeviceFeatureMode, migEnabled bool) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.mode = mode
		o.migEnabled = migEnabled
	})
}

func WithCombinedOptions(options ...NvmlMockOption) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		for _, opt := range options {
			opt.apply(o)
		}
	})
}

// WithCombinedDeviceOptions combines device options while preserving device scope.
func WithCombinedDeviceOptions(options ...NvmlDeviceOption) NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		for _, opt := range options {
			opt.applyDevice(o)
		}
	})
}

// WithPhysicalDeviceUUIDs configures the canonical physical-device inventory.
func WithPhysicalDeviceUUIDs(uuids []string) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		o.physicalDeviceUUIDs = uuids
	})
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
func WithCapabilities(caps Capabilities) NvmlDeviceOption {
	options := []NvmlDeviceOption{
		WithGpmSupport(caps.GPM),
		WithNVLinkGeneration(caps.NvLinkGenerationSupported),
		WithNVLinkLinkCount(caps.NvLinkLinkCount),
	}
	if caps.C2C {
		options = append(options, WithC2CLinkCount(1))
	} else {
		options = append(options, WithC2CLinkCount(0))
	}
	if len(caps.UnsupportedFields) > 0 {
		options = append(options, WithUnsupportedFields(caps.UnsupportedFields...))
	}
	return WithCombinedDeviceOptions(options...)
}

func newNvmlMockOptions(options ...NvmlMockOption) *nvmlMockOptions {
	opts := &nvmlMockOptions{
		defaultDeviceOptions: deviceOptions{
			migEnabled:  false,
			fieldValues: maps.Clone(DefaultFieldValues),
		},
		gpmState: &mockGpmState{},
	}
	for _, opt := range options {
		opt.apply(opts)
	}

	if opts.physicalDeviceUUIDs == nil {
		opts.physicalDeviceUUIDs = GPUUUIDs
	}
	return opts
}

func configureNVMLInterface(mockNvml *nvmlmock.Interface, opts *nvmlMockOptions, devices []*MockDevice) {
	if len(GPUUUIDs) != len(GPUCores) {
		// Make it really easy to spot errors if we change any of the arrays.
		panic("GPUUUIDs and GPUCores must have the same length, please fix it")
	}

	*mockNvml = nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			if opts.deviceCountFunc != nil {
				return opts.deviceCountFunc(), nvml.SUCCESS
			}
			return len(opts.physicalDeviceUUIDs), nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			devCount := len(opts.physicalDeviceUUIDs)
			if opts.deviceCountFunc != nil {
				devCount = opts.deviceCountFunc()
			}
			if index < 0 || index >= devCount || index >= len(devices) {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}

			device := nvml.Device(devices[index])
			if opts.deviceHandleFunc != nil {
				return opts.deviceHandleFunc(index, device)
			}
			return device, nvml.SUCCESS
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
			opts.gpmState.mu.Lock()
			defer opts.gpmState.mu.Unlock()
			opts.gpmState.allocCalls++
			if opts.gpmAllocFailureCall != 0 && opts.gpmState.allocCalls == opts.gpmAllocFailureCall {
				return nil, nvml.ERROR_UNKNOWN
			}
			sample := &MockGpmSample{ID: opts.gpmState.allocCalls}
			opts.gpmState.samples = append(opts.gpmState.samples, sample)
			return sample, nvml.SUCCESS
		},
		GpmSampleFreeFunc: func(_ nvml.GpmSample) nvml.Return {
			opts.gpmState.mu.Lock()
			defer opts.gpmState.mu.Unlock()
			opts.gpmState.freeCalls++
			return nvml.SUCCESS
		},
		GpmMetricsGetFunc: func(metricsGet *nvml.GpmMetricsGetType) nvml.Return {
			if opts.gpmMetricsGetFunc != nil {
				return opts.gpmMetricsGetFunc(metricsGet)
			}
			for i := range metricsGet.Metrics[:metricsGet.NumMetrics] {
				response := MockGpmMetricValue{Return: nvml.SUCCESS}
				if configured, ok := opts.gpmMetricValues[nvml.GpmMetricId(metricsGet.Metrics[i].MetricId)]; ok {
					response = configured
				}
				metricsGet.Metrics[i].NvmlReturn = uint32(response.Return)
				metricsGet.Metrics[i].Value = response.Value
			}

			return nvml.SUCCESS
		},
	}

	for _, opt := range opts.libOptions {
		opt(mockNvml)
	}
}

// WithSymbolsMock returns an option that configures the mock NVML interface with the given symbols available.
// It takes a map of symbols that should be considered available in the mock.
// Any symbol not in the map will return an error when looked up.
func WithSymbolsMock(availableSymbols map[string]struct{}) NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
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
	})
}

// WithMockAllFunctions returns an option that creates basic functions for all nvmlmock.Device.*Func attributes
// that return nil/zero values. This is useful for ensuring all functions are mocked even if not explicitly set.
// This is not the default behavior of the mock, as we want explicit errors if we use a function that is not mocked
// so that we implement the mocked method explicitly, controlling the inputs and outputs. However, in some cases
// (e.g., testing the collectors) we want to ensure that all functions are mocked without caring too much about the inputs and outputs.
func WithMockAllFunctions() NvmlMockOption {
	return libraryOption(func(o *nvmlMockOptions) {
		WithMockAllDeviceFunctions().applyDevice(&o.defaultDeviceOptions)
		o.libOptions = append(o.libOptions, func(i *nvmlmock.Interface) {
			fillAllMockFunctions(i)
		})
	})
}

// WithMockAllDeviceFunctions returns a device option that creates basic functions for all nvmlmock.Device.*Func attributes
// that return nil/zero values. This is useful for ensuring all functions are mocked even if not explicitly set.
func WithMockAllDeviceFunctions() NvmlDeviceOption {
	return deviceOption(func(o *deviceOptions) {
		o.compatibilityHooks = append(o.compatibilityHooks, func(d *MockDevice) {
			fillAllMockFunctions(&d.Device)
		})
	})
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
