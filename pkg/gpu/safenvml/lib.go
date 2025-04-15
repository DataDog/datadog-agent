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
	"fmt"
	"strings"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// ErrSymbolNotFound represents an error when a required NVML symbol is not found in the library
type ErrSymbolNotFound struct {
	Symbol string
}

func (e *ErrSymbolNotFound) Error() string {
	return fmt.Sprintf("%s symbol not found in NVML library", e.Symbol)
}

// NewErrSymbolNotFound creates a new ErrSymbolNotFound error
func NewErrSymbolNotFound(symbol string) error {
	return &ErrSymbolNotFound{Symbol: symbol}
}

// ErrNotSupported represents an error when an NVML function returns ERROR_NOT_SUPPORTED
type ErrNotSupported struct {
	APIName string
}

func (e *ErrNotSupported) Error() string {
	return fmt.Sprintf("%s is not supported by the GPU or driver", e.APIName)
}

// NewErrNotSupported creates a new ErrNotSupported error
func NewErrNotSupported(apiName string) error {
	return &ErrNotSupported{APIName: apiName}
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
		return NewErrSymbolNotFound(symbol)
	}

	return nil
}

// SystemGetDriverVersion returns the Nvidia driver version
func (s *safeNvml) SystemGetDriverVersion() (string, error) {
	if err := s.lookup("nvmlSystemGetDriverVersion"); err != nil {
		return "", err
	}
	driverVersion, ret := s.lib.SystemGetDriverVersion()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return "", NewErrNotSupported("SystemGetDriverVersion")
	} else if ret != nvml.SUCCESS {
		return "", fmt.Errorf("error getting driver version: %s", nvml.ErrorString(ret))
	}
	return driverVersion, nil
}

// Shutdown shuts down the NVML library
func (s *safeNvml) Shutdown() error {
	if err := s.lookup("nvmlShutdown"); err != nil {
		return err
	}
	ret := s.lib.Shutdown()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return NewErrNotSupported("Shutdown")
	} else if ret != nvml.SUCCESS {
		return fmt.Errorf("error shutting down NVML: %s", nvml.ErrorString(ret))
	}
	return nil
}

// DeviceGetCount returns the number of NVIDIA devices in the system
func (s *safeNvml) DeviceGetCount() (int, error) {
	if err := s.lookup(toNativeName("GetCount")); err != nil {
		return 0, err
	}
	count, ret := s.lib.DeviceGetCount()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return 0, NewErrNotSupported("GetDeviceCount")
	} else if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting device count: %s", nvml.ErrorString(ret))
	}
	return count, nil
}

// DeviceGetHandleByIndex returns a SafeDevice for the device at the given index
func (s *safeNvml) DeviceGetHandleByIndex(idx int) (SafeDevice, error) {
	if err := s.lookup(toNativeName("GetHandleByIndex")); err != nil {
		return nil, err
	}
	dev, ret := s.lib.DeviceGetHandleByIndex(idx)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return nil, NewErrNotSupported("DeviceGetHandleByIndex")
	} else if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting device handle by index %d: %s", idx, nvml.ErrorString(ret))
	}
	return NewDevice(dev)
}

// populateCapabilities verifies nvml API symbols exist in the native library (libnvidia-ml.so).
// It returns an error only if a critical symbol is missing (to properly initialize device list and create a new safe device wrapper)
func (s *safeNvml) populateCapabilities() error {
	s.capabilities = make(map[string]struct{})

	// Critical API from libnvidia-ml.so that are required for basic functionality
	criticalAPI := []string{
		toNativeName("GetCount"),
		toNativeName("GetCudaComputeCapability"),
		toNativeName("GetHandleByIndex"),
		toNativeName("GetIndex"),
		toNativeName("GetMemoryInfo"),
		toNativeName("GetName"),
		toNativeName("GetNumGpuCores"),
		toNativeName("GetUUID"),
	}

	// All other capabilities that are nice to have but not critical
	// These are methods from SafeNvml and SafeDevice interfaces that are not in criticalAPI
	allOtherAPI := []string{
		"nvmlShutdown",
		"nvmlSystemGetDriverVersion",
		toNativeName("GetArchitecture"),
		toNativeName("GetAttributes"),
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
		toNativeName("GetMigDeviceHandleByIndex"),
		toNativeName("GetMigMode"),
		toNativeName("GetNvLinkState"),
		toNativeName("GetPcieThroughput"),
		toNativeName("GetPerformanceState"),
		toNativeName("GetPowerManagementLimit"),
		toNativeName("GetPowerUsage"),
		toNativeName("GetRemappedRows"),
		toNativeName("GetSamples"),
		toNativeName("GetTemperature"),
		toNativeName("GetTotalEnergyConsumption"),
		toNativeName("GetUtilizationRates"),
	}

	// Check each critical API symbol and fail if any are missing
	for _, api := range criticalAPI {
		err := s.lib.Extensions().LookupSymbol(api)
		if err != nil {
			// fail the safe nvml wrapper initialization
			return fmt.Errorf("critical symbol %s not found in NVML library: %w", api, err)
		}
		s.capabilities[api] = struct{}{}
	}

	// Check each capability
	for _, api := range allOtherAPI {
		if err := s.lib.Extensions().LookupSymbol(api); err != nil {
			// don't add it to the capabilities map, but continue and don't fail
			// TODO: log a warning if the symbol is not found
			continue
		}
		s.capabilities[api] = struct{}{}
	}

	return nil
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
		libpath = cfg.GetString("nvml_lib_path")
	}

	s.lib = nvmlNewFunc(nvml.WithLibraryPath(libpath))
	if s.lib == nil {
		return fmt.Errorf("failed to create NVML library")
	}

	ret := s.lib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("error initializing NVML library: %s", nvml.ErrorString(ret))
	}

	// Populate and verify critical capabilities
	if err := s.populateCapabilities(); err != nil {
		return fmt.Errorf("failed to verify NVML capabilities: %w", err)
	}

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
