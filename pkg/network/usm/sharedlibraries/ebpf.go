// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"

	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/hashicorp/go-multierror"
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

var progSingleton *EbpfProgram
var progSingletonOnce sync.Once

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
	// from multiple goroutines
	initMutex      sync.Mutex
	enabledLibsets []Libset
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
	progSingletonOnce.Do(func() {
		progSingleton = newEBPFProgram(cfg)
	})
	progSingleton.refcount.Inc()
	return progSingleton
}

func newEBPFProgram(c *ddebpf.Config) *EbpfProgram {
	mgr := &manager.Manager{}
	perfHandlers := make(map[Libset]*ddebpf.PerfHandler)

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
				TelemetryEnabled:   c.InternalTelemetryEnabled,
			},
		}
		mgr.PerfMaps = append(mgr.PerfMaps, pm)
		ebpftelemetry.ReportPerfMapTelemetry(pm)
		perfHandlers[libset] = perfHandler
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

	return &EbpfProgram{
		cfg:          c,
		Manager:      ddebpf.NewManager(mgr, &ebpftelemetry.ErrorsTelemetryModifier{}),
		perfHandlers: perfHandlers,
	}
}

func (e *EbpfProgram) areLibsetsAlreadyEnabled(libsets ...Libset) bool {
	for _, libset := range libsets {
		if !slices.Contains(e.enabledLibsets, libset) {
			return false
		}
	}

	return true
}

// Init initializes the eBPF program. It is guaranteed to perform the initialization only if needed
// For example, if the program is already initialized to listen for a certain libset, it will not reinitialize
// the program to listen for the same libset. However, if the libsets are changed, it will reinitialize the program.
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

		// If not, we need to reinitialize the eBPF program, so we stop it
		// We use stopImpl to avoid changing the refcount
		e.stopImpl()
	}

	var err error
	if e.cfg.EnableCORE {
		err = e.initCORE()
		if err == nil {
			return nil
		}

		if !e.cfg.AllowRuntimeCompiledFallback && !e.cfg.AllowPrecompiledFallback {
			return fmt.Errorf("co-re load failed: %w", err)
		}
		log.Warnf("co-re load failed. attempting fallback: %s", err)
	}

	if e.cfg.EnableRuntimeCompiler || (err != nil && e.cfg.AllowRuntimeCompiledFallback) {
		err = e.initRuntimeCompiler()
		if err == nil {
			return nil
		}

		if !e.cfg.AllowPrecompiledFallback {
			return fmt.Errorf("runtime compilation failed: %w", err)
		}
		log.Warnf("runtime compilation failed: attempting fallback: %s", err)
	}

	if err := e.initPrebuilt(); err != nil {
		return fmt.Errorf("prebuilt load failed: %w", err)
	}

	return e.start()
}

func (e *EbpfProgram) start() error {
	err := e.Manager.Start()
	if err != nil {
		return fmt.Errorf("cannot init manager: %w", err)
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

	var err error
	for _, libset := range libsets {
		if !slices.Contains(e.enabledLibsets, libset) {
			err = multierror.Append(err, fmt.Errorf("libset %s is not enabled", libset))
		}
	}

	return err
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

	// Only stop telemetry reporting if we are the last instance
	ebpftelemetry.UnregisterTelemetry(e.Manager.Manager)
}

func (e *EbpfProgram) stopImpl() {
	_ = e.Manager.Stop(manager.CleanAll)
	for _, perfHandler := range e.perfHandlers {
		perfHandler.Stop()
	}
	for _, done := range e.done {
		close(done)
	}
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

	for _, lib := range e.enabledLibsets {
		constEd := manager.ConstantEditor{
			Name:  fmt.Sprintf("%s_libset_enabled", string(lib)),
			Value: 1,
		}

		options.ConstantEditors = append(options.ConstantEditors, constEd)
	}

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
