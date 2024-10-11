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
	"slices"
	"strings"
	"sync"

	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
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

// We cannot use sync.Once as we need to reset the singleton when the refcount reaches 0,
// as we need to completely de-initialize the program (including the eBPF manager) in that case.
// That way, any future calls to get the singleton will reinitialize the program completely.
var progSingletonLock sync.Mutex
var progSingleton *EbpfProgram

// LibraryCallback defines the type of the callback function that will be called when a shared library event is detected
type LibraryCallback func(LibPath)

// EbpfProgram represents the shared libraries eBPF program.
type EbpfProgram struct {
	*ddebpf.Manager

	cfg          *ddebpf.Config
	perfHandlers map[Libset]*ddebpf.PerfHandler
	refcount     atomic.Int32

	callbacksMutex sync.RWMutex
	callbacks      map[Libset]map[*LibraryCallback]struct{}
	wg             sync.WaitGroup
	done           map[Libset]chan struct{}

	// We need to protect the initialization variables with a mutex, as the program can be initialized
	// from multiple goroutines at the same time
	initMutex      sync.Mutex
	enabledLibsets map[Libset]struct{}
	isInitialized  bool
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
	progSingletonLock.Lock()
	defer progSingletonLock.Unlock()

	if progSingleton == nil {
		progSingleton = newEBPFProgram(cfg)
	}
	progSingleton.refcount.Inc()

	return progSingleton
}

func newEBPFProgram(c *ddebpf.Config) *EbpfProgram {
	return &EbpfProgram{
		cfg:            c,
		enabledLibsets: make(map[Libset]struct{}),
		callbacks:      make(map[Libset]map[*LibraryCallback]struct{}),
	}
}

func (e *EbpfProgram) setupManagerAndPerfHandlers() {
	mgr := &manager.Manager{}
	e.perfHandlers = make(map[Libset]*ddebpf.PerfHandler)

	// Tell the manager to load all possible maps
	for libset := range LibsetToLibSuffixes {
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
		e.perfHandlers[libset] = perfHandler
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

	e.Manager = ddebpf.NewManager(mgr, &ebpftelemetry.ErrorsTelemetryModifier{})
}

// areLibsetsAlreadyEnabled checks if the eBPF program is already enabled for the given libsets
// Requires the initMutex to be locked
func (e *EbpfProgram) areLibsetsAlreadyEnabled(libsets ...Libset) bool {
	for _, libset := range libsets {
		if _, ok := e.enabledLibsets[libset]; !ok {
			return false
		}
	}

	return true
}

func (e *EbpfProgram) loadProgram(libsets []Libset) error {
	var err error
	if e.cfg.EnableCORE {
		err = e.initCORE(libsets)
		if err == nil {
			return nil
		}

		if !e.cfg.AllowRuntimeCompiledFallback && !e.cfg.AllowPrecompiledFallback {
			return fmt.Errorf("co-re load failed: %w", err)
		}
		log.Warnf("co-re load failed. attempting fallback: %s", err)
	}

	if e.cfg.EnableRuntimeCompiler || (err != nil && e.cfg.AllowRuntimeCompiledFallback) {
		err = e.initRuntimeCompiler(libsets)
		if err == nil {
			return nil
		}

		if !e.cfg.AllowPrecompiledFallback {
			return fmt.Errorf("runtime compilation failed: %w", err)
		}
		log.Warnf("runtime compilation failed: attempting fallback: %s", err)
	}

	if err := e.initPrebuilt(libsets); err != nil {
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
		if !IsLibsetValid(libset) {
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

	if err := e.loadProgram(libsets); err != nil {
		return fmt.Errorf("cannot load program: %w", err)
	}

	if err := e.start(); err != nil {
		return fmt.Errorf("cannot start manager: %w", err)
	}

	// Add the libsets to the enabled libsets
	for _, libset := range libsets {
		e.enabledLibsets[libset] = struct{}{}
	}
	e.isInitialized = true
	return nil
}

func (e *EbpfProgram) start() error {
	err := e.Manager.Start()
	if err != nil {
		return fmt.Errorf("cannot start manager: %w", err)
	}

	for libset, perfHandler := range e.perfHandlers {
		e.wg.Add(1)
		go func(ls Libset, ph *ddebpf.PerfHandler) {
			defer e.wg.Done()

			dataChan := ph.DataChannel()
			lostChan := ph.LostChannel()
			for {
				select {
				case <-e.done[ls]:
					return
				case event, ok := <-dataChan:
					if !ok {
						return
					}
					e.handleEvent(&event, ls)
				case <-lostChan:
					// Nothing to do in this case
					break
				}
			}
		}(libset, perfHandler)
	}

	return nil
}

func (e *EbpfProgram) handleEvent(event *ebpf.DataEvent, libset Libset) {
	e.callbacksMutex.RLock()
	defer func() {
		event.Done()
		e.callbacksMutex.RUnlock()
	}()

	libpath := ToLibPath(event.Data)
	for callback := range e.callbacks[libset] {
		// Not using a callback runner for now, as we don't have a lot of callbacks
		(*callback)(libpath)
	}
}

// CheckLibsetsEnabled checks if the eBPF program is enabled for the given libsets, returns an error if not
// with the libsets that are not enabled
func (e *EbpfProgram) CheckLibsetsEnabled(libsets ...Libset) error {
	e.initMutex.Lock()
	defer e.initMutex.Unlock()

	var errs []error
	for _, libset := range libsets {
		if _, ok := e.enabledLibsets[libset]; !ok {
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

	e.callbacksMutex.Lock()
	defer e.callbacksMutex.Unlock()

	for _, libset := range libsets {
		if _, ok := e.callbacks[libset]; !ok {
			e.callbacks[libset] = make(map[*LibraryCallback]struct{})
		}
		e.callbacks[libset][&callback] = struct{}{}
	}

	// UnSubscribe()
	return func() {
		e.callbacksMutex.Lock()
		defer e.callbacksMutex.Unlock()
		for _, libset := range libsets {
			delete(e.callbacks[libset], &callback)
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

	log.Info("shared libraries monitor stopping due to a refcount of 0")

	e.stopImpl()

	// Reset the program
	progSingletonLock.Lock()
	defer progSingletonLock.Unlock()
	progSingleton = nil
}

func (e *EbpfProgram) stopImpl() {
	if e.Manager != nil {
		_ = e.Manager.Stop(manager.CleanAll)
		ebpftelemetry.UnregisterTelemetry(e.Manager.Manager)
	}

	for _, perfHandler := range e.perfHandlers {
		perfHandler.Stop()
	}
	for _, done := range e.done {
		close(done)
	}

	e.Manager = nil
}

func (e *EbpfProgram) init(buf bytecode.AssetReader, options manager.Options, libsets []Libset) error {
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
		if slices.Contains(libsets, libset) {
			value = uint64(1)
		}

		constEd := manager.ConstantEditor{
			Name:  fmt.Sprintf("%s_libset_enabled", string(libset)),
			Value: uint64(1),
		}

		options.ConstantEditors = append(options.ConstantEditors, constEd)
		enabledMsgs = append(enabledMsgs, fmt.Sprintf("%s=%d", libset, value))
	}

	log.Infof("loading shared libraries program with libsets enabled: %s", strings.Join(enabledMsgs, ", "))

	options.BypassEnabled = e.cfg.BypassEnabled
	return e.InitWithOptions(buf, &options)
}

func (e *EbpfProgram) initCORE(libsets []Libset) error {
	assetName := getAssetName("shared-libraries", e.cfg.BPFDebug)
	fn := func(buf bytecode.AssetReader, options manager.Options) error { return e.init(buf, options, libsets) }
	return ddebpf.LoadCOREAsset(assetName, fn)
}

func (e *EbpfProgram) initRuntimeCompiler(libsets []Libset) error {
	bc, err := getRuntimeCompiledSharedLibraries(e.cfg)
	if err != nil {
		return err
	}
	defer bc.Close()
	return e.init(bc, manager.Options{}, libsets)
}

func (e *EbpfProgram) initPrebuilt(libsets []Libset) error {
	bc, err := netebpf.ReadSharedLibrariesModule(e.cfg.BPFDir, e.cfg.BPFDebug)
	if err != nil {
		return err
	}
	defer bc.Close()

	return e.init(bc, manager.Options{}, libsets)
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
