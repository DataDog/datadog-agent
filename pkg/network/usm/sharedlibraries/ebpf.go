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

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/encoding"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
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

var libPathPool = ddsync.NewDefaultTypedPool[libPath]()

type ebpfProgram struct {
	cfg          *config.Config
	eventHandler *perf.EventHandler
	eventChannel <-chan *libPath
	*ddebpf.Manager
}

func newEBPFProgram(c *config.Config) *ebpfProgram {
	callback, callbackCh := ddsync.CallbackChannel[*libPath](100)
	ehopts := perf.EventHandlerOptions{
		MapName:          sharedLibrariesPerfMap,
		TelemetryEnabled: c.InternalTelemetryEnabled,
		UseRingBuffer:    false,
		Handler: encoding.BinaryUnmarshalCallback(libPathPool.Get, func(b *libPath, err error) {
			if err != nil {
				log.Debug(err.Error())
				return
			}
			callback(b)
		}),
		PerfOptions: perf.PerfBufferOptions{
			BufferSize: 8 * os.Getpagesize(),
			Watermark:  1,
		},
	}
	eh, err := perf.NewEventHandler(ehopts)
	if err != nil {
		return nil
	}

	mgr := &manager.Manager{}
	probeIDs := getSysOpenHooksIdentifiers()
	for _, identifier := range probeIDs {
		mgr.Probes = append(mgr.Probes,
			&manager.Probe{
				ProbeIdentificationPair: identifier,
				KProbeMaxActive:         maxActive,
			},
		)
	}
	emgr := ddebpf.NewManager(mgr, &ebpftelemetry.ErrorsTelemetryModifier{})

	return &ebpfProgram{
		cfg:          c,
		Manager:      emgr,
		eventHandler: eh,
		eventChannel: callbackCh,
	}
}

func (e *ebpfProgram) Init() error {
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

	return e.initPrebuilt()
}

func (e *ebpfProgram) GetPerfHandler() <-chan *libPath {
	return e.eventChannel
}

func (e *ebpfProgram) Stop() {
	ebpftelemetry.UnregisterTelemetry(e.Manager.Manager)
	_ = e.Manager.Stop(manager.CleanAll)
}

func (e *ebpfProgram) init(buf bytecode.AssetReader, options manager.Options) error {
	if err := e.Manager.LoadELF(buf); err != nil {
		return err
	}

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

	if err := e.eventHandler.Init(e.Manager.Manager, &options); err != nil {
		return err
	}
	return e.InitWithOptions(nil, &options)
}

func (e *ebpfProgram) initCORE() error {
	assetName := getAssetName("shared-libraries", e.cfg.BPFDebug)
	return ddebpf.LoadCOREAsset(assetName, e.init)
}

func (e *ebpfProgram) initRuntimeCompiler() error {
	bc, err := getRuntimeCompiledSharedLibraries(e.cfg)
	if err != nil {
		return err
	}
	defer bc.Close()
	return e.init(bc, manager.Options{})
}

func (e *ebpfProgram) initPrebuilt() error {
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
