// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"

	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxActive              = 1024
	sharedLibrariesPerfMap = "shared_libraries"
	probeUID               = "so"

	// probe used for streaming shared library events
	openSysCall    = "open"
	openatSysCall  = "openat"
	openat2SysCall = "openat2"
)

var traceTypes = []string{"enter", "exit"}

var progSingletonOnce sync.Once
var progSingleton *EbpfProgram

// LibraryCallback defines the type of the callback function that will be called when a shared library event is detected
type LibraryCallback func(LibPath)

// libsetHandler contains all the structures and state required to handle a single libset.
type libsetHandler struct {
	// callbacksMutex protects the callbacks map for this specific libset
	callbacksMutex sync.RWMutex

	// callbacks is a map of the callbacks that are subscribed to this libset
	callbacks map[*LibraryCallback]struct{}

	// done is a channel that is closed when the handler stops, to signal the goroutine to end
	done chan struct{}

	// perfHandler is the perf handler for this libset, that will get the events from the perf buffer
	perfHandler *ddebpf.PerfHandler

	// enabled is true if the eBPF program has been enabled for this libset,
	// which means that the perf buffer is being read, the eBPF program has been
	// edited to enable this libset, and the handler that redirects events to
	// callbacks is running.
	enabled bool

	// requested means that the libset has been requested to be enabled, but it
	// might not be enabled yet. We need to have this distinction as the init flow
	// is different depending on whether a libset is enabled/requested or none.
	// For example, an enabled program will need to stop the handlers and re-start them
	// as the underlying eBPF probe is updated. Meanwhile, a requested program will not
	// need to stop the handlers, as they are not running yet.
	requested bool
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
	refcount atomic.Int32

	// initMutex is a mutex to protect the initialization variables and the libset map
	wg sync.WaitGroup

	// We need to protect the initialization variables and libset map with a
	// mutex, as the program can be initialized from multiple goroutines at the
	// same time.
	initMutex sync.Mutex

	// isInitialized is true if the program has been initialized, false
	// otherwise used to check if the program needs to be stopped and re-started
	// when adding new libsets
	isInitialized bool
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
	progSingletonOnce.Do(func() {
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
	})
	progSingleton.refcount.Inc()

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

// isLibsetEnabled checks if the libset has been requested to be enabled. Assumes initMutex is locked
func (e *EbpfProgram) isLibsetRequested(libset Libset) bool {
	data, ok := e.libsets[libset]
	return ok && data.requested
}

// setupManagerAndPerfHandlers sets up the manager and perf handlers for the eBPF program, creating the perf handlers
// Assumes initMutex is locked
func (e *EbpfProgram) setupManagerAndPerfHandlers() {
	mgr := &manager.Manager{}

	// Tell the manager to load all possible maps
	for libset, handler := range e.libsets {
		perfHandler := ddebpf.NewPerfHandler(100)
		pm := &manager.PerfMap{
			Map: manager.Map{
				Name: fmt.Sprintf("%s_%s", string(libset), sharedLibrariesPerfMap),
			},
			PerfMapOptions: manager.PerfMapOptions{
				PerfRingBufferSize: 8 * os.Getpagesize(),
				Watermark:          1,
				RecordHandler:      perfHandler.RecordHandler,
				LostHandler:        perfHandler.LostHandler,
				RecordGetter:       perfHandler.RecordGetter,
				TelemetryEnabled:   e.cfg.InternalTelemetryEnabled,
			},
		}
		mgr.PerfMaps = append(mgr.PerfMaps, pm)
		ebpftelemetry.ReportPerfMapTelemetry(pm)
		handler.perfHandler = perfHandler
	}

	probeIDs := getSysOpenHooksIdentifiers()
	for _, identifier := range probeIDs {
		mgr.Probes = append(mgr.Probes,
			&manager.Probe{
				ProbeIdentificationPair: identifier,
				KProbeMaxActive:         maxActive,
			},
		)
	}

	e.Manager = ddebpf.NewManager(mgr, "shared-libraries", &ebpftelemetry.ErrorsTelemetryModifier{})
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

	e.setupManagerAndPerfHandlers()

	// Mark the libsets as requested so they can be started
	// Note that other libsets might be requested from previous executions
	for _, libset := range libsets {
		e.libsets[libset].requested = true
	}

	if err := e.loadProgram(); err != nil {
		return fmt.Errorf("cannot load program: %w", err)
	}

	if err := e.start(); err != nil {
		return fmt.Errorf("cannot start manager: %w", err)
	}

	e.isInitialized = true
	return nil
}

// start starts the eBPF program and the perf handlers, assumes the initMutex is locked
func (e *EbpfProgram) start() error {
	err := e.Manager.Start()
	if err != nil {
		return fmt.Errorf("cannot start manager: %w", err)
	}

	for _, handler := range e.libsets {
		if !handler.requested {
			continue
		}

		// Init the "done" channel for the handler, it will be closed when the handler stops
		handler.done = make(chan struct{})
		e.wg.Add(1)
		go handler.eventLoop(&e.wg)

		handler.enabled = true
	}

	return nil
}

// eventLoop is the main loop for a single libset. Should be called with all perfHandlers initialized.
func (l *libsetHandler) eventLoop(wg *sync.WaitGroup) {
	defer wg.Done()

	dataChan := l.perfHandler.DataChannel()
	lostChan := l.perfHandler.LostChannel()
	for {
		select {
		case <-l.done:
			return
		case event, ok := <-dataChan:
			if !ok {
				return
			}
			l.handleEvent(&event)
		case <-lostChan:
			// Nothing to do in this case
			break
		}
	}
}

func (l *libsetHandler) handleEvent(event *ddebpf.DataEvent) {
	defer event.Done()

	libpath := ToLibPath(event.Data)

	l.callbacksMutex.RLock()
	defer l.callbacksMutex.RUnlock()
	for callback := range l.callbacks {
		// Not using a callback runner for now, as we don't have a lot of callbacks
		(*callback)(libpath)
	}
}

// stop stops the libset handler. Assumes the initMutex for the main ebpfProgram is locked. To be called
func (l *libsetHandler) stop() {
	// The done channel might not be initialized if the program is stopped before it starts (e.g., two sequential calls to InitWithLibsets()).
	if l.done != nil {
		close(l.done)
	}

	// stop the perf handler after the event loop is done
	if l.perfHandler != nil {
		l.perfHandler.Stop()
	}

	l.enabled = false
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
	if e.refcount.Dec() != 0 {
		if e.refcount.Load() < 0 {
			e.refcount.Swap(0)
		}
		return
	}

	// At this point any operations are thread safe, as we're using atomics
	// so it's guaranteed only one thread can reach this point with refcount == 0
	log.Info("shared libraries monitor stopping due to a refcount of 0")

	e.stopImpl()

	// Reset the program singleton in case it's used again (e.g. in tests)
	progSingletonOnce = sync.Once{}
	progSingleton = nil
}

func (e *EbpfProgram) stopImpl() {
	if e.Manager != nil {
		_ = e.Manager.Stop(manager.CleanAll)
		ebpftelemetry.UnregisterTelemetry(e.Manager.Manager)
	}

	for _, handler := range e.libsets {
		if handler.enabled {
			handler.stop()
		}
	}

	e.wg.Wait()

	e.Manager = nil
}

func (e *EbpfProgram) init(buf bytecode.AssetReader, options manager.Options) error {
	options.RLimit = &unix.Rlimit{
		Cur: math.MaxUint64,
		Max: math.MaxUint64,
	}

	for _, probe := range e.Probes {
		options.ActivatedProbes = append(options.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: probe.ProbeIdentificationPair,
			},
		)
	}

	var enabledMsgs []string
	for libset := range LibsetToLibSuffixes {
		value := uint64(0)
		if e.isLibsetRequested(libset) {
			value = uint64(1)
		}

		constEd := manager.ConstantEditor{
			Name:  fmt.Sprintf("%s_libset_enabled", string(libset)),
			Value: value,
		}

		options.ConstantEditors = append(options.ConstantEditors, constEd)
		enabledMsgs = append(enabledMsgs, fmt.Sprintf("%s=%d", libset, value))
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

	if err == nil && len(missing) == 0 {
		return true
	}

	kversion, err := kernel.HostVersion()

	if err != nil {
		log.Error("could not determine the current kernel version. fallback to do_sys_open")
		return false
	}

	return kversion >= kernel.VersionCode(5, 6, 0)
}

// getSysOpenHooksIdentifiers returns the enter and exit tracepoints for supported open*
// system calls.
func getSysOpenHooksIdentifiers() []manager.ProbeIdentificationPair {
	openatProbes := []string{openatSysCall}
	if sysOpenAt2Supported() {
		openatProbes = append(openatProbes, openat2SysCall)
	}
	// amd64 has open(2), arm64 doesn't
	if runtime.GOARCH == "amd64" {
		openatProbes = append(openatProbes, openSysCall)
	}

	res := make([]manager.ProbeIdentificationPair, 0, len(traceTypes)*len(openatProbes))
	for _, probe := range openatProbes {
		for _, traceType := range traceTypes {
			res = append(res, manager.ProbeIdentificationPair{
				EBPFFuncName: fmt.Sprintf("tracepoint__syscalls__sys_%s_%s", traceType, probe),
				UID:          probeUID,
			})
		}
	}

	return res
}

func getAssetName(module string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", module)
	}

	return fmt.Sprintf("%s.o", module)
}
