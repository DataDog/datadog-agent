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

var progSingleton *EbpfProgram
var progSingletonOnce sync.Once

type LibraryCallback func(LibPath)

// EbpfProgram represents the shared libraries eBPF program.
type EbpfProgram struct {
	cfg         *ddebpf.Config
	perfHandler *ddebpf.PerfHandler
	initOnce    sync.Once
	refcount    atomic.Int32
	*ddebpf.Manager
	callbacksMutex sync.RWMutex
	callbacks      map[*LibraryCallback]struct{}
	wg             sync.WaitGroup
	done           chan struct{}
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
	perfHandler := ddebpf.NewPerfHandler(100)
	pm := &manager.PerfMap{
		Map: manager.Map{
			Name: sharedLibrariesPerfMap,
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
	mgr := &manager.Manager{
		PerfMaps: []*manager.PerfMap{pm},
	}
	ebpftelemetry.ReportPerfMapTelemetry(pm)

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
		cfg:         c,
		Manager:     ddebpf.NewManager(mgr, &ebpftelemetry.ErrorsTelemetryModifier{}),
		perfHandler: perfHandler,
	}
}

// Init initializes the eBPF program.
func (e *EbpfProgram) Init() error {
	var initErr error
	e.initOnce.Do(func() {
		var err error
		if e.cfg.EnableCORE {
			err = e.initCORE()
			if err == nil {
				return
			}

			if !e.cfg.AllowRuntimeCompiledFallback && !e.cfg.AllowPrecompiledFallback {
				initErr = fmt.Errorf("co-re load failed: %w", err)
				return
			}
			log.Warnf("co-re load failed. attempting fallback: %s", err)
		}

		if e.cfg.EnableRuntimeCompiler || (err != nil && e.cfg.AllowRuntimeCompiledFallback) {
			err = e.initRuntimeCompiler()
			if err == nil {
				return
			}

			if !e.cfg.AllowPrecompiledFallback {
				initErr = fmt.Errorf("runtime compilation failed: %w", err)
				return
			}
			log.Warnf("runtime compilation failed: attempting fallback: %s", err)
		}

		if err := e.initPrebuilt(); err != nil {
			initErr = fmt.Errorf("prebuilt load failed: %w", err)
			return
		}

		initErr = e.start()
		return
	})

	return initErr
}

func (e *EbpfProgram) start() error {
	err := e.Manager.Start()
	if err != nil {
		return fmt.Errorf("cannot init manager: %w", err)
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		dataChan := e.getPerfHandler().DataChannel()
		lostChan := e.getPerfHandler().LostChannel()
		for {
			select {
			case <-e.done:
				return
			case event, ok := <-dataChan:
				if !ok {
					return
				}
				e.handleEvent(&event)
			case <-lostChan:
				// Nothing to do in this case
				break
			}
		}
	}()

	return nil
}

func (e *EbpfProgram) handleEvent(event *ebpf.DataEvent) {
	e.callbacksMutex.RLock()
	defer func() {
		event.Done()
		e.callbacksMutex.RUnlock()
	}()

	libpath := ToLibPath(event.Data)
	for callback := range e.callbacks {
		// Not using a callback runner for now, as we don't have a lot of callbacks
		(*callback)(libpath)
	}

}

func (e *EbpfProgram) Subscribe(callback LibraryCallback) func() {
	e.callbacksMutex.Lock()
	e.callbacks[&callback] = struct{}{}
	e.callbacksMutex.Unlock()

	// UnSubscribe()
	return func() {
		e.callbacksMutex.Lock()
		delete(e.callbacks, &callback)
		e.callbacksMutex.Unlock()
	}
}

// GetPerfHandler returns the perf handler
func (e *EbpfProgram) getPerfHandler() *ddebpf.PerfHandler {
	return e.perfHandler
}

// Stop stops the eBPF program
func (e *EbpfProgram) Stop() {
	if e.refcount.Dec() != 0 {
		if e.refcount.Load() < 0 {
			e.refcount.Swap(0)
		}
		return
	}

	log.Info("shared libraries monitor stopping due to a refcount of 0")

	ebpftelemetry.UnregisterTelemetry(e.Manager.Manager)
	e.Manager.Stop(manager.CleanAll) //nolint:errcheck
	e.perfHandler.Stop()
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
