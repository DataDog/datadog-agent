// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

// Package safenvml provides a safe wrapper around NVIDIA's NVML library.
// It ensures compatibility with older drivers by checking symbol availability
// and prevents direct usage of the NVML library.
package safenvml

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// getCriticalAPIs returns the list of critical NVML APIs
// required for basic functionality
func getCriticalAPIs() []string {
	return []string{
		toNativeName("GetCount"),
		toNativeName("GetCudaComputeCapability"),
		toNativeName("GetHandleByIndex"),
		toNativeName("GetIndex"),
		toNativeName("GetMemoryInfo"),
		toNativeName("GetName"),
		toNativeName("GetNumGpuCores"),
		toNativeName("GetUUID"),
	}
}

// getNonCriticalAPIs returns the list of non-critical NVML APIs
// that are nice to have but not essential
func getNonCriticalAPIs() []string {
	return []string{
		"nvmlShutdown",
		"nvmlSystemGetDriverVersion",
		"nvmlGpmSampleAlloc",
		"nvmlGpmSampleFree",
		"nvmlGpmMetricsGet",
		"nvmlGpmQueryDeviceSupport",
		"nvmlGpmSampleGet",
		"nvmlEventSetCreate",
		"nvmlEventSetFree",
		"nvmlEventSetWait_v1",
		"nvmlEventSetWait_v2", // it can be either v1 or v2
		toNativeName("GetArchitecture"),
		toNativeName("GetAttributes"),
		toNativeName("GetBAR1MemoryInfo"),
		toNativeName("GetClockInfo"),
		toNativeName("GetComputeRunningProcesses"),
		toNativeName("GetCurrentClocksThrottleReasons"),
		toNativeName("GetDecoderUtilization"),
		toNativeName("GetEncoderUtilization"),
		toNativeName("GetFanSpeed"),
		toNativeName("GetFieldValues"),
		toNativeName("GetGpuInstanceId"),
		toNativeName("GetMaxClockInfo"),
		toNativeName("GetMaxMigDeviceCount"),
		toNativeName("GetMemoryBusWidth"),
		toNativeName("GetMemoryInfo_v2"),
		toNativeName("GetMigDeviceHandleByIndex"),
		toNativeName("GetMigMode"),
		toNativeName("GetNvLinkState"),
		toNativeName("GetPcieThroughput"),
		toNativeName("GetPerformanceState"),
		toNativeName("GetPowerManagementLimit"),
		toNativeName("GetPowerUsage"),
		toNativeName("GetProcessUtilization"),
		toNativeName("GetRemappedRows"),
		toNativeName("GetSamples"),
		toNativeName("GetTemperature"),
		toNativeName("GetTotalEnergyConsumption"),
		toNativeName("GetUtilizationRates"),
		toNativeName("IsMigDeviceHandle"),
		toNativeName("GetVirtualizationMode"),
		toNativeName("GetSupportedEventTypes"),
		toNativeName("RegisterEvents"),
	}
}

// symbolLookup is an internal interface for checking symbol availability
type symbolLookup interface {
	lookup(string) error
}

// SafeNVML represents a safe wrapper around NVML library operations.
// It ensures that operations are only performed when the corresponding
// symbols are available in the loaded library.
type SafeNVML interface {
	symbolLookup
	// Shutdown shuts down the NVML library
	Shutdown() error
	// DeviceGetCount returns the number of NVIDIA devices in the system
	DeviceGetCount() (int, error)
	// DeviceGetHandleByIndex returns a SafeDevice for the device at the given index
	DeviceGetHandleByIndex(idx int) (SafeDevice, error)
	// SystemGetDriverVersion returns the version of the system's graphics driver
	SystemGetDriverVersion() (string, error)
	// GpmSampleAlloc allocates a sample buffer for GPM
	GpmSampleAlloc() (nvml.GpmSample, error)
	// GpmSampleFree frees a sample buffer for GPM
	GpmSampleFree(sample nvml.GpmSample) error
	// GpmMetricsGet calculates the metrics from the given samples
	GpmMetricsGet(metrics *nvml.GpmMetricsGetType) error
	// EventSetCreate creates an event set object
	EventSetCreate() (nvml.EventSet, error)
	// EventSetFree frees an event set object
	EventSetFree(evtSet nvml.EventSet) error
	// EventSetWait waits (up to timeout) for an event to appear on the given set and returns it
	EventSetWait(evtSet nvml.EventSet, timeout time.Duration) (DeviceEventData, error)
}

type safeNvml struct {
	lib          nvml.Interface
	mu           sync.Mutex
	capabilities map[string]struct{}
}

func toNativeName(symbol string) string {
	return "nvmlDevice" + symbol
}

func (s *safeNvml) lookup(symbol string) error {
	if _, ok := s.capabilities[symbol]; !ok {
		return NewNvmlAPIErrorOrNil(symbol, nvml.ERROR_FUNCTION_NOT_FOUND)
	}

	return nil
}

// SystemGetDriverVersion returns the Nvidia driver version
func (s *safeNvml) SystemGetDriverVersion() (string, error) {
	if err := s.lookup("nvmlSystemGetDriverVersion"); err != nil {
		return "", err
	}
	driverVersion, ret := s.lib.SystemGetDriverVersion()
	return driverVersion, NewNvmlAPIErrorOrNil("SystemGetDriverVersion", ret)
}

// Shutdown shuts down the NVML library
func (s *safeNvml) Shutdown() error {
	if err := s.lookup("nvmlShutdown"); err != nil {
		return err
	}
	ret := s.lib.Shutdown()
	return NewNvmlAPIErrorOrNil("Shutdown", ret)
}

// DeviceGetCount returns the number of NVIDIA devices in the system
func (s *safeNvml) DeviceGetCount() (int, error) {
	if err := s.lookup(toNativeName("GetCount")); err != nil {
		return 0, err
	}
	count, ret := s.lib.DeviceGetCount()
	return count, NewNvmlAPIErrorOrNil("GetDeviceCount", ret)
}

// DeviceGetHandleByIndex returns a SafeDevice for the device at the given index
func (s *safeNvml) DeviceGetHandleByIndex(idx int) (SafeDevice, error) {
	if err := s.lookup(toNativeName("GetHandleByIndex")); err != nil {
		return nil, err
	}
	dev, ret := s.lib.DeviceGetHandleByIndex(idx)
	if err := NewNvmlAPIErrorOrNil("DeviceGetHandleByIndex", ret); err != nil {
		return nil, err
	}
	return NewPhysicalDevice(dev)
}

func (s *safeNvml) GpmSampleAlloc() (nvml.GpmSample, error) {
	if err := s.lookup("nvmlGpmSampleAlloc"); err != nil {
		return nil, err
	}
	sample, ret := s.lib.GpmSampleAlloc()
	return sample, NewNvmlAPIErrorOrNil("GpmSampleAlloc", ret)
}

func (s *safeNvml) GpmSampleFree(sample nvml.GpmSample) error {
	if err := s.lookup("nvmlGpmSampleFree"); err != nil {
		return err
	}
	ret := s.lib.GpmSampleFree(sample)
	return NewNvmlAPIErrorOrNil("GpmSampleFree", ret)
}

func (s *safeNvml) GpmMetricsGet(metrics *nvml.GpmMetricsGetType) error {
	if err := s.lookup("nvmlGpmMetricsGet"); err != nil {
		return err
	}
	ret := s.lib.GpmMetricsGet(metrics)
	return NewNvmlAPIErrorOrNil("GpmMetricsGet", ret)
}

func (s *safeNvml) EventSetCreate() (nvml.EventSet, error) {
	if err := s.lookup("nvmlEventSetCreate"); err != nil {
		return nil, err
	}
	evtSet, ret := s.lib.EventSetCreate()
	return evtSet, NewNvmlAPIErrorOrNil("nvmlEventSetCreate", ret)
}

func (s *safeNvml) EventSetFree(evtSet nvml.EventSet) error {
	if err := s.lookup("nvmlEventSetFree"); err != nil {
		return err
	}
	ret := s.lib.EventSetFree(evtSet)
	return NewNvmlAPIErrorOrNil("nvmlEventSetFree", ret)
}

func (s *safeNvml) EventSetWait(evtSet nvml.EventSet, timeout time.Duration) (DeviceEventData, error) {
	v1Err := errors.Join(s.lookup("nvmlEventSetWait_v1"))
	v2Err := errors.Join(s.lookup("nvmlEventSetWait_v2"))
	if v1Err != nil && v2Err != nil {
		return DeviceEventData{}, errors.Join(v1Err, v2Err)
	}
	if timeout < time.Millisecond {
		return DeviceEventData{}, errors.New("can't use sub-millisecond timeout in EventSetWait")
	}

	data, ret := s.lib.EventSetWait(evtSet, uint32(timeout.Milliseconds()))
	retErr := NewNvmlAPIErrorOrNil("nvmlEventSetWait", ret)
	safeData := DeviceEventData{
		EventType:         data.EventType,
		EventData:         data.EventData,
		GPUInstanceID:     data.GpuInstanceId,
		ComputeInstanceID: data.ComputeInstanceId,
	}

	// attempt safe resolution of device UUID
	if data.Device != nil {
		uuid, err := (&safeDeviceImpl{nvmlDevice: data.Device, lib: s}).GetUUID()
		if err != nil {
			err = fmt.Errorf("can't retrieve device UUID: %w", err)
			return DeviceEventData{}, errors.Join(err, retErr)
		}
		safeData.DeviceUUID = uuid
	}

	return safeData, retErr
}

// populateCapabilities verifies nvml API symbols exist in the native library (libnvidia-ml.so).
// It returns an error only if a critical symbol is missing (to properly initialize device list and create a new safe device wrapper)
func populateCapabilities(lib nvml.Interface) (map[string]struct{}, error) {
	capabilities := make(map[string]struct{})

	// Critical API from libnvidia-ml.so that are required for basic functionality
	criticalAPI := getCriticalAPIs()

	// All other capabilities that are nice to have but not critical
	allOtherAPI := getNonCriticalAPIs()

	// Check each critical API symbol and fail if any are missing
	for _, api := range criticalAPI {
		err := lib.Extensions().LookupSymbol(api)
		if err != nil {
			// fail the safe nvml wrapper initialization
			return nil, fmt.Errorf("critical symbol %s not found in NVML library: %w", api, err)
		}
		capabilities[api] = struct{}{}
	}

	// Check each capability
	for _, api := range allOtherAPI {
		if err := lib.Extensions().LookupSymbol(api); err != nil {
			// don't add it to the capabilities map, but continue and don't fail
			// TODO: log a warning if the symbol is not found
			continue
		}
		capabilities[api] = struct{}{}
	}

	return capabilities, nil
}

// ensureInitWithOpts initializes the NVML library with the given options (used for testing)
func (s *safeNvml) ensureInitWithOpts(nvmlNewFunc func(opts ...nvml.LibraryOption) nvml.Interface) error {
	// If the library is already initialized, return nil without locking
	if s.lib != nil {
		return nil
	}

	// Lock the mutex to ensure thread-safe initialization
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
	}()

	// Check again after locking to ensure no race condition
	if s.lib != nil {
		return nil
	}

	var libpath string
	if flavor.GetFlavor() == flavor.SystemProbe {
		cfg := pkgconfigsetup.SystemProbe()
		// Use the config directly here to avoid importing the entire gpu
		// config package, which includes system-probe specific imports
		libpath = cfg.GetString(strings.Join([]string{consts.GPUNS, "nvml_lib_path"}, "."))
	} else {
		cfg := pkgconfigsetup.Datadog()
		libpath = cfg.GetString("gpu.nvml_lib_path")
	}

	lib := nvmlNewFunc(nvml.WithLibraryPath(libpath))
	if lib == nil {
		return fmt.Errorf("failed to create NVML library")
	}

	ret := lib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("error initializing NVML library: %s", nvml.ErrorString(ret))
	}

	// Populate and verify critical capabilities
	var err error
	s.capabilities, err = populateCapabilities(lib)
	if err != nil {
		return fmt.Errorf("failed to verify NVML capabilities: %w", err)
	}

	// Once everything is verified, set the library so that it can be reused
	s.lib = lib

	return nil
}

// ensureInit initializes the NVML library with the default options.
func (s *safeNvml) ensureInit() error {
	return s.ensureInitWithOpts(nvml.New)
}

var singleton safeNvml

// GetSafeNvmlLib returns the safe wrapper around NVML library instance.
// This function acts as a singleton pattern and will initialize the library if it is not already initialized.
func GetSafeNvmlLib() (SafeNVML, error) {
	if err := singleton.ensureInit(); err != nil {
		return nil, err
	}

	return &singleton, nil
}
