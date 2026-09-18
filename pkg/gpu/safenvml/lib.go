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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
		"nvmlGpmMigSampleGet",
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
		toNativeName("GetFanSpeed_v2"),
		toNativeName("GetFieldValues"),
		"nvmlDeviceReadWritePRM_v1",
		toNativeName("GetGpuFabricInfoV"),
		toNativeName("GetGpuInstanceId"),
		toNativeName("GetGpuInstanceProfileInfo"),
		toNativeName("GetMaxClockInfo"),
		toNativeName("GetMaxMigDeviceCount"),
		toNativeName("GetMemoryBusWidth"),
		toNativeName("GetMemoryInfo_v2"),
		toNativeName("GetMigDeviceHandleByIndex"),
		toNativeName("GetMigMode"),
		toNativeName("GetNvLinkState"),
		toNativeName("GetNvLinkVersion"),
		toNativeName("GetNumFans"),
		toNativeName("GetPciInfo"),
		toNativeName("GetPcieThroughput"),
		toNativeName("GetCurrPcieLinkGeneration"),
		toNativeName("GetMaxPcieLinkGeneration"),
		toNativeName("GetCurrPcieLinkWidth"),
		toNativeName("GetMaxPcieLinkWidth"),
		toNativeName("GetPerformanceState"),
		toNativeName("GetPowerManagementLimit"),
		toNativeName("GetPowerUsage"),
		toNativeName("GetProcessUtilization"),
		toNativeName("GetRepairStatus"),
		toNativeName("GetRemappedRows"),
		toNativeName("GetSamples"),
		toNativeName("GetTemperature"),
		toNativeName("GetTotalEnergyConsumption"),
		toNativeName("GetUtilizationRates"),
		toNativeName("IsMigDeviceHandle"),
		toNativeName("GetVirtualizationMode"),
		toNativeName("GetSupportedEventTypes"),
		toNativeName("RegisterEvents"),
		toNativeName("GetMemoryErrorCounter"),
		toNativeName("GetSramEccErrorStatus"),
		toNativeName("GetRunningProcessDetailList"),
	}
}

// nvmlSafety is an internal interface with methods to ensure safe operations
// with NVML
type nvmlSafety interface {
	// lookup checks if the given symbol is available in the NVML library
	lookup(string) error
	// gpmLock locks the GPM mutex. Despite NVIDIA documentation, the GPM API is not thread safe.
	// We need to lock the mutex to ensure that only one thread can access the GPM API at a time, specifically GpmSampleGet
	gpmLock()
	// gpmUnlock unlocks the GPM mutex. Despite NVIDIA documentation, the GPM API is not thread safe.
	// We need to unlock the mutex to allow other threads to access the GPM API.
	gpmUnlock()

	// fieldValuesLock locks the field values mutex. Similarly to GPM, the field values API is not thread safe
	// despite docs saying that NVML is thread safe.
	fieldValuesLock()
	// fieldValuesUnlock unlocks the field values mutex. Similarly to GPM, the field values API is not thread safe
	// despite docs saying that NVML is thread safe.
	fieldValuesUnlock()
}

// SafeNVML represents a safe wrapper around NVML library operations.
// It ensures that operations are only performed when the corresponding
// symbols are available in the loaded library.
type SafeNVML interface {
	nvmlSafety
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
	lib              nvml.Interface
	mu               sync.Mutex
	gpmMutex         sync.Mutex
	fieldValuesMutex sync.Mutex
	capabilities     map[string]struct{}
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

func (s *safeNvml) gpmLock() {
	s.gpmMutex.Lock()
}

func (s *safeNvml) gpmUnlock() {
	s.gpmMutex.Unlock()
}

func (s *safeNvml) fieldValuesLock() {
	s.fieldValuesMutex.Lock()
}

func (s *safeNvml) fieldValuesUnlock() {
	s.fieldValuesMutex.Unlock()
}

// SystemGetDriverVersion returns the Nvidia driver version
func (s *safeNvml) SystemGetDriverVersion() (string, error) {
	if err := s.lookup("nvmlSystemGetDriverVersion"); err != nil {
		return "", err
	}
	driverVersion, ret := s.lib.SystemGetDriverVersion()
	return driverVersion, NewNvmlAPIErrorOrNil("SystemGetDriverVersion", ret)
}

// nvmlMu guards the released flag and the in-flight user count transitions.
// nvmlCount tracks in-flight NVML users: Begin increments, End decrements, and
// the release waits for the count to drain before shutting the library down.
// The design is counting-based (not a held read lock) so nested users — a
// workloadmeta pull that triggers a device-cache refresh — cannot deadlock
// against a pending release writer.
var (
	nvmlMu       sync.Mutex
	nvmlCount    int
	nvmlDraining bool // a release drain/shutdown is in progress
	nvmlReleased atomic.Bool
)

// nvmlReleaseDrainTimeout bounds how long a release waits for in-flight NVML
// users before giving up (the caller retries on its next cycle). A
// workloadmeta pull or a cache refresh drains in well under this bound.
const nvmlReleaseDrainTimeout = 30 * time.Second

// BeginNVMLUse registers an NVML user: it takes the gate's read side (blocking
// while a release is in progress), refuses while the library is deliberately
// released, and initializes the library if needed. The caller MUST call
// EndNVMLUse when done with NVML.
func BeginNVMLUse() error {
	nvmlMu.Lock()
	defer nvmlMu.Unlock()

	if nvmlReleased.Load() {
		return ErrNVMLReleased
	}
	nvmlCount++
	if err := singleton.ensureInit(); err != nil {
		nvmlCount--
		return err
	}
	return nil
}

// EndNVMLUse marks the user done. Pairs with BeginNVMLUse; every Begin must
// be matched by exactly one End.
func EndNVMLUse() {
	nvmlMu.Lock()
	defer nvmlMu.Unlock()
	if nvmlCount == 0 {
		// A negative count never reaches the drain's zero, so every later
		// release would spin to its timeout forever.
		log.Errorf("EndNVMLUse called without a matching BeginNVMLUse; ignoring")
		return
	}
	nvmlCount--
}

// IsDraining reports whether a release drain/shutdown is currently in
// progress. Used by the reacquire path to avoid clearing the released flag
// mid-drain.
func IsDraining() bool {
	// ReleaseNVML writes nvmlDraining under nvmlMu; an unlocked read races.
	nvmlMu.Lock()
	defer nvmlMu.Unlock()
	return nvmlDraining
}

// WithNVML runs fn with the shared library wrapper while holding the NVML
// gate (BeginNVMLUse/EndNVMLUse), so a concurrent deliberate release waits
// for fn to finish instead of racing it. The wrapper never escapes this
// function.
func WithNVML(fn func(SafeNVML) error) error {
	if err := BeginNVMLUse(); err != nil {
		return err
	}
	defer EndNVMLUse()
	return fn(&singleton)
}

// ReleaseNVML arms the deliberate-release state (rejecting all new
// acquisitions), waits (bounded) for in-flight users to finish, and shuts the
// library down. On a shutdown error the flag is un-armed — callers stay in the
// not-released state and simply retry on their next cycle, instead of latching
// a released state over an initialized (still reset-blocking) library.
func ReleaseNVML() error {
	nvmlMu.Lock()
	if nvmlDraining || nvmlReleased.Load() {
		nvmlMu.Unlock()
		return nil
	}
	nvmlDraining = true
	nvmlReleased.Store(true)
	nvmlMu.Unlock()

	// Bounded poll for the in-flight users to drain. Deliberately NOT a
	// WaitGroup.Wait in a helper goroutine: on timeout that waiter would be
	// abandoned, and re-arming the flag would let new Begin calls Add against
	// the outstanding Wait — a documented WaitGroup misuse that can panic.
	deadline := time.Now().Add(nvmlReleaseDrainTimeout)
	for {
		nvmlMu.Lock()
		count := nvmlCount
		nvmlMu.Unlock()
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			// Don't latch: un-arm so the caller's next cycle retries the
			// release instead of leaving an initialized (still reset-blocking)
			// library behind a released flag.
			nvmlMu.Lock()
			nvmlDraining = false
			nvmlReleased.Store(false)
			nvmlMu.Unlock()
			return errors.New("timed out waiting for in-flight NVML users; the release was not applied and will be retried")
		}
		time.Sleep(50 * time.Millisecond)
	}

	singleton.mu.Lock()
	defer singleton.mu.Unlock()
	if singleton.lib == nil {
		nvmlMu.Lock()
		nvmlDraining = false
		nvmlMu.Unlock()
		return nil
	}
	if err := singleton.shutdownLocked(); err != nil {
		// Don't latch: un-arm so the caller's next cycle retries the release
		// instead of leaving an initialized (still reset-blocking) library
		// behind a released flag.
		nvmlMu.Lock()
		nvmlDraining = false
		nvmlReleased.Store(false)
		nvmlMu.Unlock()
		return err
	}
	nvmlMu.Lock()
	nvmlDraining = false
	nvmlMu.Unlock()
	return nil
}

// Shutdown shuts down the NVML library.
func (s *safeNvml) Shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdownLocked()
}

// shutdownLocked shuts the library down; the caller must hold s.mu so the
// inited-check and the shutdown are atomic against a concurrent ensureInit.
func (s *safeNvml) shutdownLocked() error {
	if err := s.lookup("nvmlShutdown"); err != nil {
		return err
	}
	ret := s.lib.Shutdown()
	if err := NewNvmlAPIErrorOrNil("Shutdown", ret); err != nil {
		return err
	}

	// After shutdown the wrapper must reinitialize NVML before reuse.
	s.lib = nil
	s.capabilities = nil
	return nil
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

// tryCandidateNvmlPaths tries to load the NVML library from the given paths, using the given function to create a new NVML library instance.
// We use nvmlNewWithPath to wrap the nvmlNewFunc, so that we can more easily test this code (we can't inspect the NVML library options).
func tryCandidateNvmlPaths(paths []string, nvmlNewWithPath func(path string) nvml.Interface) (nvml.Interface, error) {
	for _, path := range paths {
		log.Debugf("Trying to load NVML library from path '%s'", path)

		lib := nvmlNewWithPath(path)
		if lib == nil {
			return nil, errors.New("failed to create NVML library")
		}
		ret := lib.Init()
		if ret == nvml.SUCCESS || ret == nvml.ERROR_ALREADY_INITIALIZED {
			return lib, nil
		} else if ret != nvml.ERROR_LIBRARY_NOT_FOUND {
			return nil, NewNvmlAPIErrorOrNil("Init", ret)
		}
	}

	return nil, fmt.Errorf("failed to find NVML library in any of the candidate paths, searched: %v", paths)
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

	// Note that if the default "libpath" is empty, NVML will just invoke
	// `dlopen` with no specified path, and the linker will try to open the
	// library from the default library search paths. This is the default we
	// want. The alternative paths are only used if the default path is not
	// found, to make it more convenient and robust, without users having to
	// specify some common paths that might not be in the library search paths,
	// specially in containerized environments.
	libPaths := []string{libpath}
	libPaths = append(libPaths, generateDefaultNvmlPaths()...)

	nvmlNewWithPath := func(path string) nvml.Interface {
		return nvmlNewFunc(nvml.WithLibraryPath(path))
	}
	lib, err := tryCandidateNvmlPaths(libPaths, nvmlNewWithPath)
	if err != nil {
		return err
	}

	// Populate and verify critical capabilities
	s.capabilities, err = populateCapabilities(lib)
	if err != nil {
		return fmt.Errorf("failed to verify NVML capabilities: %w", err)
	}

	// Once everything is verified, set the library so that it can be reused
	s.lib = lib

	return nil
}

// nvmlNewFunc is the function to create a new NVML library instance. It can be overridden for testing purposes.
var nvmlNewFunc = nvml.New

// ensureInit initializes the NVML library with the default options.
func (s *safeNvml) ensureInit() error {
	return s.ensureInitWithOpts(nvmlNewFunc)
}

var singleton safeNvml

// ErrNVMLReleased is returned by initialization paths while NVML has been
// deliberately released (a GPU reset window): callers should skip quietly
// and retry on their next cycle.
var ErrNVMLReleased = errors.New("NVML deliberately released (GPU reset window)")

// ReacquireNVML ends the deliberate-release state so NVML can be initialized
// again on the next use. The caller must only clear the state once the release
// window is really over; use IsDraining to defer until an in-flight release
// drain has completed.
func ReacquireNVML() {
	nvmlReleased.Store(false)
}

// IsNVMLReleased reports whether NVML was deliberately released. Unlike a
// combined inited-and-not-released check, this does not conflate "not yet
// initialized" with "deliberately released", so a collector that runs before
// the first initialization proceeds normally.
func IsNVMLReleased() bool {
	return nvmlReleased.Load()
}

// GetSafeNvmlLib returns the safe wrapper around NVML library instance.
// This function acts as a singleton pattern and will initialize the library if it is not already initialized.
func GetSafeNvmlLib() (SafeNVML, error) {
	if nvmlReleased.Load() {
		return nil, ErrNVMLReleased
	}
	if err := singleton.ensureInit(); err != nil {
		return nil, err
	}

	return &singleton, nil
}

// generateDefaultNvmlPaths generates the default paths for the NVML library,
// taking into account containerized environments and the HOST_ROOT environment variable.
// NOTE: This logic is intentionally duplicated in pkg/config/env/environment_containers.go
// (getDefaultNvmlPaths) to avoid adding pkg/gpu as a dependency of pkg/config/env, which
// is imported by nearly every binary in the repo.
func generateDefaultNvmlPaths() []string {
	systemPaths := []string{
		"/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1",                    // default system install
		"/run/nvidia/driver/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1",  // nvidia-gpu-operator install
		"/usr/lib/aarch64-linux-gnu/libnvidia-ml.so.1",                   // default system install on ARM64
		"/run/nvidia/driver/usr/lib/aarch64-linux-gnu/libnvidia-ml.so.1", // nvidia-gpu-operator install on ARM64
	}

	hostRoot := os.Getenv("HOST_ROOT")
	if hostRoot == "" {
		if env.IsContainerized() {
			hostRoot = "/host"
		} else {
			return systemPaths
		}
	}

	paths := make([]string, 0, len(systemPaths))
	for _, p := range systemPaths {
		paths = append(paths, filepath.Join(hostRoot, p))
	}
	return paths
}
