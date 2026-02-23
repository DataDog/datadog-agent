// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	bugs "github.com/DataDog/datadog-agent/pkg/ebpf/kernelbugs"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxActive              = 1024
	sharedLibrariesPerfMap = "shared_libraries"
	probeUID               = "so"
	perCPUBufferPages      = 8
	dataChannelSize        = 100

	// probe used for streaming shared library events
	openSysCall    = "open"
	openatSysCall  = "openat"
	openat2SysCall = "openat2"
)

var (
	singletonMutex = sync.Mutex{}
	progSingleton  *EbpfProgram

	traceTypes = []string{"enter", "exit"}
)

// LibraryCallback defines the type of the callback function that will be called when a shared library event is detected
type LibraryCallback func(LibPath)

// libsetHandler contains all the structures and state required to handle a single libset.
type libsetHandler struct {
	// callbacksMutex protects the callbacks map for this specific libset
	callbacksMutex sync.RWMutex

	// callbacks is a map of the callbacks that are subscribed to this libset
	callbacks map[*LibraryCallback]struct{}

	// enabled is true if the eBPF program has been enabled for this libset,
	// which means that the perf buffer is being read and the eBPF program has been
	// edited to enable this libset.
	enabled bool
}

// EbpfProgram represents the shared libraries eBPF program.
type EbpfProgram struct {
	*ddebpf.Manager

	// libsets is a map of all defined libsets to their respective handlers. This map
	// is filled with all libsets from LibsetToLibSuffixes when the program is initialized
	// in GetEBPFProgram. The fact that all libsets are initialized at the same time
	// allows us to avoid locking the map when accessing it, as Golang maps are thread-safe
	// for reads.
	libsets map[Libset]*libsetHandler

	// cfg is the configuration for the eBPF program
	cfg *ddebpf.Config

	// refcount is the number of times the program has been initialized. It is used to
	// stop the program only when the refcount reaches 0.
	refcount uint16

	// We need to protect the initialization variables and libset map with a
	// mutex, as the program can be initialized from multiple goroutines at the
	// same time.
	initMutex sync.Mutex

	// isInitialized is true if the program has been initialized, false
	// otherwise used to check if the program needs to be stopped and re-started
	// when adding new libsets
	isInitialized bool

	// enabledProbes is a list of the probes that are enabled for the current system.
	enabledProbes []manager.ProbeIdentificationPair
	// disabledProbes is a list of the probes that are disabled for the current system.
	disabledProbes []manager.ProbeIdentificationPair
}

// IsSupported returns true if the shared libraries monitoring is supported on the current system.
func IsSupported(cfg *ddebpf.Config) bool {
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. shared libraries monitoring disabled.")
		return false
	}

	if strings.HasPrefix(runtime.GOARCH, "arm") {
		return kversion >= kernel.VersionCode(5, 5, 0) && (cfg.EnableRuntimeCompiler || cfg.EnableCORE)
	}

	// Minimum version for shared libraries monitoring is 4.14
	return kversion >= kernel.VersionCode(4, 14, 0)
}

// GetEBPFProgram returns an instance of the shared libraries eBPF program singleton
func GetEBPFProgram(cfg *ddebpf.Config) *EbpfProgram {
	singletonMutex.Lock()
	defer singletonMutex.Unlock()

	if progSingleton == nil {
		progSingleton = &EbpfProgram{
			cfg:     cfg,
			libsets: make(map[Libset]*libsetHandler),
		}

		// Initialize the libsets to avoid requiring a mutex on the map. Golang maps are thread safe
		// for reads.
		for libset := range LibsetToLibSuffixes {
			progSingleton.libsets[libset] = &libsetHandler{
				callbacks: make(map[*LibraryCallback]struct{}),
			}
		}
	}

	progSingleton.refcount++

	return progSingleton
}

// isLibsetValid checks if the given libset is valid (i.e., it's in the libsets map). Note that
// this function assumes that a libset is valid if it's in the map, as the map is initialized with
// all valid libsets when the program is initialized. We could also call IsLibsetValid, but doing
// it this way centralizes the "validity" check in the program: if in the future we have a different
// way to check if a libset is valid, we only need to change how the e.libsets map is filled.
func (e *EbpfProgram) isLibsetValid(libset Libset) bool {
	_, ok := e.libsets[libset]
	return ok
}

// isLibsetEnabled checks if the libset is enabled. Assumes initMutex is locked
func (e *EbpfProgram) isLibsetEnabled(libset Libset) bool {
	data, ok := e.libsets[libset]
	return ok && data.enabled
}

// setupManagerAndPerfHandlers sets up the manager and perf handlers for the eBPF program, creating the perf handlers
// Assumes initMutex is locked
func (e *EbpfProgram) setupManagerAndPerfHandlers() error {
	mgr := &manager.Manager{}
	numCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		numCPUs = 1
	}

	perfBufferSize := perCPUBufferPages * os.Getpagesize()
	ringBufferSize := common.ToPowerOf2(perfBufferSize * numCPUs)

	managerMods := []ddebpf.Modifier{
		&ebpftelemetry.ErrorsTelemetryModifier{},
	}

	// Load perf handlers for all enabled libsets
	for libset, handler := range e.libsets {
		if !handler.enabled {
			continue
		}

		mapName := string(libset) + "_" + sharedLibrariesPerfMap
		mode := perf.UpgradePerfBuffers(perfBufferSize, dataChannelSize, perf.Watermark(1), ringBufferSize)

		perfHandler, err := perf.NewEventHandler(mapName, handler.handleEvent, mode,
			perf.SendTelemetry(e.cfg.InternalTelemetryEnabled),
			perf.RingBufferEnabledConstantName("ringbuffers_enabled"))
		if err != nil {
			return fmt.Errorf("failed to create perf handler for map %s: %w", mapName, err)
		}

		managerMods = append(managerMods, perfHandler)
	}

	e.initializeProbes()
	for _, identifier := range e.enabledProbes {
		probe := &manager.Probe{
			ProbeIdentificationPair: identifier,
			KProbeMaxActive:         maxActive,
		}
		mgr.Probes = append(mgr.Probes, probe)
	}

	e.Manager = ddebpf.NewManager(mgr, "shared-libraries", managerMods...)

	return nil
}

// areLibsetsAlreadyEnabled checks if the eBPF program is already enabled for the given libsets
// Requires the initMutex to be locked
func (e *EbpfProgram) areLibsetsAlreadyEnabled(libsets ...Libset) bool {
	for _, libset := range libsets {
		if !e.isLibsetEnabled(libset) {
			return false
		}
	}

	return true
}

func (e *EbpfProgram) loadProgram() error {
	var err error
	if e.cfg.EnableCORE {
		err = e.initCORE()
		if err == nil {
			return nil
		}

		if !e.cfg.AllowRuntimeCompiledFallback && !e.cfg.AllowPrebuiltFallback {
			return fmt.Errorf("co-re load failed: %w", err)
		}
		log.Warnf("co-re load failed. attempting fallback: %s", err)
	}

	if e.cfg.EnableRuntimeCompiler || (err != nil && e.cfg.AllowRuntimeCompiledFallback) {
		err = e.initRuntimeCompiler()
		if err == nil {
			return nil
		}

		if !e.cfg.AllowPrebuiltFallback {
			return fmt.Errorf("runtime compilation failed: %w", err)
		}
		log.Warnf("runtime compilation failed: attempting fallback: %s", err)
	}

	if err := e.initPrebuilt(); err != nil {
		return fmt.Errorf("prebuilt load failed: %w", err)
	}

	return nil
}

// InitWithLibsets initializes the eBPF program and prepares it to listen to the
// given libsets. It is guaranteed to perform the initialization only if needed
// For example, if the program is already initialized to listen for a certain
// libset, it will not reinitialize the program to listen for the same libset.
// However, if the libsets are changed, it will reinitialize the program.
func (e *EbpfProgram) InitWithLibsets(libsets ...Libset) error {
	// Ensure we have all valid libsets, we don't want cryptic errors later
	for _, libset := range libsets {
		if !e.isLibsetValid(libset) {
			return fmt.Errorf("libset %s is not valid, ensure it is in the LibsetToLibSuffixes map", libset)
		}
	}

	// Lock for the initialization variables
	e.initMutex.Lock()
	defer e.initMutex.Unlock()

	// If the program is initialized, check if the libsets are already enabled
	if e.isInitialized {
		// If the libsets are already enabled, return, we have nothing to do
		if e.areLibsetsAlreadyEnabled(libsets...) {
			return nil
		}

		// If not, we need to reinitialize the eBPF program to re-edit the
		// constants (that step is done in Manager.InitWithOptions), so we stop
		// it We use stopImpl to avoid changing the refcount. This will stop
		// perf handlers, but will retain callbacks and other state. This way,
		// listeners will not actually notice any change (other than the lost
		// events in the meantime).
		e.stopImpl()
	}

	// Mark the libsets as enabled, so that we will edit the corresponding constants and
	// enable the corresponding perf handlers.
	// Note that other libsets might be enabled from previous executions
	for _, libset := range libsets {
		e.libsets[libset].enabled = true
	}

	if err := e.setupManagerAndPerfHandlers(); err != nil {
		return fmt.Errorf("cannot setup manager and perf handlers: %w", err)
	}

	if err := e.loadProgram(); err != nil {
		return fmt.Errorf("cannot load program: %w", err)
	}

	if err := e.start(); err != nil {
		return fmt.Errorf("cannot start manager: %w", err)
	}

	ddebpf.AddNameMappings(e.Manager.Manager, "shared-libraries")
	e.isInitialized = true
	return nil
}

// start starts the eBPF program and the perf handlers, assumes the initMutex is locked
func (e *EbpfProgram) start() error {
	// Manager.Start will also start all the perf handlers that were added to the manager
	err := e.Manager.Start()
	if err != nil {
		return err
	}

	ddebpf.AddProbeFDMappings(e.Manager.Manager)

	return nil
}

// toLibPath casts the perf event data to the LibPath structure
func toLibPath(data []byte) LibPath {
	return *(*LibPath)(unsafe.Pointer(&data[0]))
}

func (l *libsetHandler) handleEvent(data []byte) {
	libpath := toLibPath(data)

	l.callbacksMutex.RLock()
	defer l.callbacksMutex.RUnlock()
	for callback := range l.callbacks {
		// Not using a callback runner for now, as we don't have a lot of callbacks
		(*callback)(libpath)
	}
}

// subscribe subscribes to the shared libraries events for this libset, returns the function
// to call to unsubscribe
func (l *libsetHandler) subscribe(callback LibraryCallback) func() {
	l.callbacksMutex.Lock()
	defer l.callbacksMutex.Unlock()

	l.callbacks[&callback] = struct{}{}

	return func() {
		l.callbacksMutex.Lock()
		defer l.callbacksMutex.Unlock()

		delete(l.callbacks, &callback)
	}
}

// CheckLibsetsEnabled checks if the eBPF program is enabled for the given libsets, returns an error if not
// with the libsets that are not enabled
func (e *EbpfProgram) CheckLibsetsEnabled(libsets ...Libset) error {
	e.initMutex.Lock()
	defer e.initMutex.Unlock()

	var errs []error
	for _, libset := range libsets {
		if !e.isLibsetEnabled(libset) {
			errs = append(errs, fmt.Errorf("libset %s is not enabled", libset))
		}
	}

	return errors.Join(errs...)
}

// Subscribe subscribes to the shared libraries events for the given libsets, returns the function
// to call to unsubscribe and an error if the libsets are not enabled
func (e *EbpfProgram) Subscribe(callback LibraryCallback, libsets ...Libset) (func(), error) {
	if err := e.CheckLibsetsEnabled(libsets...); err != nil {
		return nil, err
	}

	var unsubscribers []func()
	for _, libset := range libsets {
		// e.libsets is only modified when creating the EbpfProgram struct,
		// which is a singleton with a mutex in the GetEBPFProgram function. As
		// Golang maps are thread-safe for reads, we don't need here a mutex to
		// access it. subscribe() will the libset-specific callback, to avoid
		// locking on all callbacks for all libsets.
		unsub := e.libsets[libset].subscribe(callback)
		unsubscribers = append(unsubscribers, unsub)
	}

	// UnSubscribe()
	return func() {
		for _, unsub := range unsubscribers {
			unsub()
		}
	}, nil
}

// Stop stops the eBPF program if the refcount reaches 0
func (e *EbpfProgram) Stop() {
	singletonMutex.Lock()
	defer singletonMutex.Unlock()

	if e.refcount == 0 {
		log.Warn("shared libraries monitor stopping with a refcount of 0")
		return
	}
	e.refcount--
	if e.refcount > 0 {
		// Still in use
		return
	}

	log.Info("shared libraries monitor stopping due to a refcount of 0")

	e.stopImpl()

	progSingleton = nil
}

func (e *EbpfProgram) stopImpl() {
	if e.Manager != nil {
		err := e.Manager.Stop(manager.CleanAll)
		if err != nil {
			log.Errorf("error stopping manager: %s", err)
		}
	}

	e.Manager = nil
}

func (e *EbpfProgram) init(buf bytecode.AssetReader, options manager.Options) error {
	options.RemoveRlimit = true

	for _, probe := range e.enabledProbes {
		options.ActivatedProbes = append(options.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: probe,
			},
		)
	}
	for _, probe := range e.disabledProbes {
		options.ExcludedFunctions = append(options.ExcludedFunctions, probe.EBPFFuncName)
	}

	var enabledMsgs []string
	for libset := range LibsetToLibSuffixes {
		value := uint64(0)
		if e.isLibsetEnabled(libset) {
			value = uint64(1)
		}

		constEd := manager.ConstantEditor{
			Name:  string(libset) + "_libset_enabled",
			Value: value,
		}

		options.ConstantEditors = append(options.ConstantEditors, constEd)
		enabledMsgs = append(enabledMsgs, string(libset)+"="+strconv.FormatUint(value, 10))
	}

	log.Infof("loading shared libraries program with libsets enabled: %s", strings.Join(enabledMsgs, ", "))

	options.BypassEnabled = e.cfg.BypassEnabled
	return e.InitWithOptions(buf, &options)
}

func (e *EbpfProgram) initCORE() error {
	assetName := getAssetName("shared-libraries", e.cfg.BPFDebug)
	return ddebpf.LoadCOREAsset(assetName, e.init)
}

func (e *EbpfProgram) initRuntimeCompiler() error {
	bc, err := getRuntimeCompiledSharedLibraries(e.cfg)
	if err != nil {
		return err
	}
	defer bc.Close()
	return e.init(bc, manager.Options{})
}

func (e *EbpfProgram) initPrebuilt() error {
	bc, err := netebpf.ReadSharedLibrariesModule(e.cfg.BPFDir, e.cfg.BPFDebug)
	if err != nil {
		return err
	}
	defer bc.Close()

	return e.init(bc, manager.Options{})
}

func sysOpenAt2Supported() bool {
	missing, err := ddebpf.VerifyKernelFuncs("do_sys_openat2")

	return err == nil && len(missing) == 0
}

// fexitSupported checks if fexit type of probe is supported on the current host.
// It does this by creating a dummy program that attaches to the given function name, and returns true if it succeeds.
// Method was adapted from the CWS code.
func fexitSupported(funcName string) bool {
	if features.HaveProgramType(ebpf.Tracing) != nil {
		return false
	}

	spec := &ebpf.ProgramSpec{
		Type:       ebpf.Tracing,
		AttachType: ebpf.AttachTraceFExit,
		AttachTo:   funcName,
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
	}
	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogDisabled: true,
	})
	if err != nil {
		return false
	}
	defer prog.Close()

	l, err := link.AttachTracing(link.TracingOptions{
		Program: prog,
	})
	if err != nil {
		return false
	}
	defer l.Close()

	hasPotentialFentryDeadlock, err := bugs.HasTasksRCUExitLockSymbol()
	if hasPotentialFentryDeadlock || (err != nil) {
		// incase of error, let's be safe and assume the bug is present
		return false
	}

	return true
}

// initializedProbes initializes the probes that are enabled for the current system
func (e *EbpfProgram) initializeProbes() {
	openat2Supported := sysOpenAt2Supported()
	isFexitSupported := fexitSupported("do_sys_openat2")

	// Tracing represents fentry/fexit probes.
	tracingProbes := []manager.ProbeIdentificationPair{
		{
			EBPFFuncName: "do_sys_" + openat2SysCall + "_exit",
			UID:          probeUID,
		},
	}

	openatProbes := []string{openatSysCall}
	if openat2Supported {
		openatProbes = append(openatProbes, openat2SysCall)
	}
	// amd64 has open(2), arm64 doesn't
	if runtime.GOARCH == "amd64" {
		openatProbes = append(openatProbes, openSysCall)
	}

	// tp stands for tracepoints, which is the older format of the probes.
	tpProbes := make([]manager.ProbeIdentificationPair, 0, len(traceTypes)*len(openatProbes))
	for _, probe := range openatProbes {
		for _, traceType := range traceTypes {
			tpProbes = append(tpProbes, manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint__syscalls__sys_" + traceType + "_" + probe,
				UID:          probeUID,
			})
		}
	}

	// Kprobe fallback for kernels < 4.15 that don't support multiple tracepoint attachments
	// Both open() and openat() syscalls call the same kernel function do_sys_open(),
	// so we only need one kprobe/kretprobe pair for both.
	// Note: We don't include openat2 here because it was introduced in kernel 5.6,
	// which is much newer than our 4.15 cutoff.
	kprobeProbes := []manager.ProbeIdentificationPair{
		{
			EBPFFuncName: "kprobe__do_sys_open",
			UID:          probeUID,
		},
		{
			EBPFFuncName: "kretprobe__do_sys_open",
			UID:          probeUID,
		},
	}

	// Default to kprobe fallback (works on all kernel versions)
	e.enabledProbes = kprobeProbes
	e.disabledProbes = append(tracingProbes, tpProbes...)

	kv, err := kernel.HostVersion()
	if err != nil {
		log.Warnf("Failed to get kernel version for shared library probes, using kprobe fallback: %v", err)
	} else {
		kv415 := kernel.VersionCode(4, 15, 0)

		if isFexitSupported && openat2Supported {
			// Kernel >= 5.6 with fexit support - use fexit probes (most efficient)
			e.enabledProbes = tracingProbes
			e.disabledProbes = append(tpProbes, kprobeProbes...)
			log.Infof("Using fexit probes for shared library monitoring (kernel %s)", kv)
		} else if kv >= kv415 {
			// Kernel >= 4.15 - use tracepoints (multiple attachment supported)
			e.enabledProbes = tpProbes
			e.disabledProbes = append(tracingProbes, kprobeProbes...)
			log.Infof("Using tracepoint probes for shared library monitoring (kernel %s >= 4.15)", kv)
		} else {
			// Kernel < 4.15 - keep kprobe fallback (no multiple tracepoint attachment)
			log.Infof("Using kprobe fallback for shared library monitoring (kernel %s < 4.15)", kv)
		}
	}
}

func getAssetName(module string, debug bool) string {
	if debug {
		return module + "-debug.o"
	}

	return module + ".o"
}

// ToBytes converts the libpath to a byte array containing the path
func ToBytes(l *LibPath) []byte {
	return l.Buf[:l.Len]
}

func (l *LibPath) String() string {
	return string(ToBytes(l))
}
