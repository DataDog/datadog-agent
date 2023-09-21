// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to classify protocols and dispatch the correct handlers.
	protocolDispatcherSocketFilterFunction = "socket__protocol_dispatcher"
	connectionStatesMap                    = "connection_states"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	maxActive = 128
	probeUID  = "http"
)

type ebpfProgram struct {
	*errtelemetry.Manager
	cfg                   *config.Config
	subprograms           []subprogram
	probesResolvers       []probeResolver
	tailCallRouter        []manager.TailCallRoute
	connectionProtocolMap *ebpf.Map

	enabledProtocols  []protocols.Protocol
	disabledProtocols []*protocols.ProtocolSpec

	// Used for connection_protocol data expiration
	mapCleaner *ddebpf.MapCleaner
	buildMode  buildMode
}

type probeResolver interface {
	// GetAllUndefinedProbes returns all undefined probes.
	// Subprogram probes maybe defined in the same ELF file as the probes
	// of the main program. The cilium loader loads all programs defined
	// in an ELF file in to the kernel. Therefore, these programs may be
	// loaded into the kernel, whether the subprogram is activated or not.
	//
	// Before the loading can be performed we must associate a function which
	// performs some fixup in the EBPF bytecode:
	// https://github.com/DataDog/datadog-agent/blob/main/pkg/ebpf/c/bpf_telemetry.h#L58
	// If this is not correctly done, the verifier will reject the EBPF bytecode.
	//
	// The ebpf telemetry manager
	// (https://github.com/DataDog/datadog-agent/blob/main/pkg/network/telemetry/telemetry_manager.go#L19)
	// takes an instance of the Manager managing the main program, to acquire
	// the list of the probes to patch.
	// https://github.com/DataDog/datadog-agent/blob/main/pkg/network/telemetry/ebpf_telemetry.go#L256
	// This Manager may not include the probes of the subprograms. GetAllUndefinedProbes() is,
	// therefore, necessary for returning the probes of these subprograms so they can be
	// correctly patched at load-time, when the Manager is being initialized.
	//
	// To reiterate, this is necessary due to the fact that the cilium loader loads
	// all programs defined in an ELF file regardless if they are later attached or not.
	GetAllUndefinedProbes() []manager.ProbeIdentificationPair
}

type buildMode string

const (
	// Prebuilt mode
	Prebuilt buildMode = "prebuilt"
	// RuntimeCompiled mode
	RuntimeCompiled buildMode = "runtime-compilation"
	// CORE mode
	CORE buildMode = "CO-RE"
)

type subprogram interface {
	Name() string
	IsBuildModeSupported(buildMode) bool
	ConfigureManager(*errtelemetry.Manager)
	ConfigureOptions(*manager.Options)
	Start()
	Stop()
}

func newEBPFProgram(c *config.Config, sockFD, connectionProtocolMap *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*ebpfProgram, error) {
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: protocols.TLSDispatcherProgramsMap},
			{Name: protocols.ProtocolDispatcherProgramsMap},
			{Name: connectionStatesMap},
		},
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe__tcp_sendmsg",
					UID:          probeUID,
				},
				KProbeMaxActive: maxActive,
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tracepoint__net__netif_receive_skb",
					UID:          probeUID,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: protocolDispatcherSocketFilterFunction,
					UID:          probeUID,
				},
			},
		},
	}

	subprogramProbesResolvers := make([]probeResolver, 0, 1)
	subprograms := make([]subprogram, 0, 1)
	var tailCalls []manager.TailCallRoute

	goTLSProg := newGoTLSProgram(c, sockFD)
	subprogramProbesResolvers = append(subprogramProbesResolvers, goTLSProg)
	if goTLSProg != nil {
		subprograms = append(subprograms, goTLSProg)
	}

	program := &ebpfProgram{
		Manager:               errtelemetry.NewManager(mgr, bpfTelemetry),
		cfg:                   c,
		subprograms:           subprograms,
		probesResolvers:       subprogramProbesResolvers,
		tailCallRouter:        tailCalls,
		connectionProtocolMap: connectionProtocolMap,
	}

	return program, nil
}

func (e *ebpfProgram) Init() error {
	var undefinedProbes []manager.ProbeIdentificationPair
	for _, tc := range e.tailCallRouter {
		undefinedProbes = append(undefinedProbes, tc.ProbeIdentificationPair)
	}

	for _, s := range e.probesResolvers {
		undefinedProbes = append(undefinedProbes, s.GetAllUndefinedProbes()...)
	}

	e.DumpHandler = e.dumpMapsHandler
	e.InstructionPatcher = func(m *manager.Manager) error {
		return errtelemetry.PatchEBPFTelemetry(m, true, undefinedProbes)
	}
	for _, s := range e.subprograms {
		s.ConfigureManager(e.Manager)
	}

	var err error
	if e.cfg.EnableCORE {
		err = e.initCORE()
		if err == nil {
			e.buildMode = CORE
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
			e.buildMode = RuntimeCompiled
			return nil
		}

		if !e.cfg.AllowPrecompiledFallback {
			return fmt.Errorf("runtime compilation failed: %w", err)
		}
		log.Warnf("runtime compilation failed: attempting fallback: %s", err)
	}

	err = e.initPrebuilt()
	if err == nil {
		e.buildMode = Prebuilt
	}
	return err
}

func (e *ebpfProgram) Start() error {
	mapCleaner, err := e.setupMapCleaner()
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
	} else {
		e.mapCleaner = mapCleaner
	}

	err = e.Manager.Start()
	if err != nil {
		return err
	}

	for _, s := range e.subprograms {
		if s.IsBuildModeSupported(e.buildMode) {
			s.Start()
			log.Infof("launched %s subprogram", s.Name())
		} else {
			log.Infof("%s subprogram does not support %s build mode", s.Name(), e.buildMode)
		}
	}

	return nil
}

func (e *ebpfProgram) Close() error {
	e.mapCleaner.Stop()
	for _, s := range e.subprograms {
		s.Stop()
	}
	return e.Stop(manager.CleanAll)
}

func (e *ebpfProgram) initCORE() error {
	assetName := getAssetName("usm", e.cfg.BPFDebug)
	return ddebpf.LoadCOREAsset(&e.cfg.Config, assetName, e.init)
}

func (e *ebpfProgram) initRuntimeCompiler() error {
	bc, err := getRuntimeCompiledUSM(e.cfg)
	if err != nil {
		return err
	}
	defer bc.Close()
	return e.init(bc, manager.Options{})
}

func (e *ebpfProgram) initPrebuilt() error {
	bc, err := netebpf.ReadHTTPModule(e.cfg.BPFDir, e.cfg.BPFDebug)
	if err != nil {
		return err
	}
	defer bc.Close()

	var offsets []manager.ConstantEditor
	if offsets, err = offsetguess.TracerOffsets.Offsets(e.cfg); err != nil {
		return err
	}

	return e.init(bc, manager.Options{ConstantEditors: offsets})
}

func (e *ebpfProgram) init(buf bytecode.AssetReader, options manager.Options) error {
	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if e.cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}

	options.RLimit = &unix.Rlimit{
		Cur: math.MaxUint64,
		Max: math.MaxUint64,
	}

	options.MapSpecEditors = map[string]manager.MapSpecEditor{
		connectionStatesMap: {
			MaxEntries: e.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		},
	}
	if e.connectionProtocolMap != nil {
		if options.MapEditors == nil {
			options.MapEditors = make(map[string]*ebpf.Map)
		}
		options.MapEditors[probes.ConnectionProtocolMap] = e.connectionProtocolMap
	} else {
		options.MapSpecEditors[probes.ConnectionProtocolMap] = manager.MapSpecEditor{
			MaxEntries: e.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		}
	}

	options.TailCallRouter = e.tailCallRouter
	options.ActivatedProbes = []manager.ProbesSelector{
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolDispatcherSocketFilterFunction,
				UID:          probeUID,
			},
		},
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe__tcp_sendmsg",
				UID:          probeUID,
			},
		},
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint__net__netif_receive_skb",
				UID:          probeUID,
			},
		},
	}

	// Some parts of USM (https capturing, and part of the classification) use `read_conn_tuple`, and has some if
	// clauses that handled IPV6, for USM we care (ATM) only from TCP connections, so adding the sole config about tcpv6.
	utils.AddBoolConst(&options, e.cfg.CollectTCPv6Conns, "tcpv6_enabled")

	options.DefaultKprobeAttachMethod = kprobeAttachMethod
	options.VerifierOptions.Programs.LogSize = 10 * 1024 * 1024

	for _, s := range e.subprograms {
		s.ConfigureOptions(&options)
	}

	for _, p := range e.enabledProtocols {
		p.ConfigureOptions(e.Manager.Manager, &options)
	}

	// Add excluded functions from disabled protocols
	for _, p := range e.disabledProtocols {
		for _, m := range p.Maps {
			// Unused maps still need to have a non-zero size
			options.MapSpecEditors[m.Name] = manager.MapSpecEditor{
				MaxEntries: uint32(1),
				EditorFlag: manager.EditMaxEntries,
			}

			log.Debugf("disabled map: %v", m.Name)
		}

		for _, probe := range p.Probes {
			options.ExcludedFunctions = append(options.ExcludedFunctions, probe.ProbeIdentificationPair.EBPFFuncName)
		}

		for _, tc := range p.TailCalls {
			options.ExcludedFunctions = append(options.ExcludedFunctions, tc.ProbeIdentificationPair.EBPFFuncName)
		}
	}

	return e.InitWithOptions(buf, options)
}

const connProtoTTL = 3 * time.Minute
const connProtoCleaningInterval = 5 * time.Minute

func (e *ebpfProgram) setupMapCleaner() (*ddebpf.MapCleaner, error) {
	mapCleaner, err := ddebpf.NewMapCleaner(e.connectionProtocolMap, new(netebpf.ConnTuple), new(netebpf.ProtocolStackWrapper))
	if err != nil {
		return nil, err
	}

	ttl := connProtoTTL.Nanoseconds()
	mapCleaner.Clean(connProtoCleaningInterval, func(now int64, key, val interface{}) bool {
		protoStack, ok := val.(*netebpf.ProtocolStackWrapper)
		if !ok {
			return false
		}

		updated := int64(protoStack.Updated)
		return (now - updated) > ttl
	})

	return mapCleaner, nil
}

func getAssetName(module string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", module)
	}

	return fmt.Sprintf("%s.o", module)
}

func (e *ebpfProgram) dumpMapsHandler(_ *manager.Manager, mapName string, currentMap *ebpf.Map) string {
	var output strings.Builder

	switch mapName {
	case connectionStatesMap: // maps/connection_states (BPF_MAP_TYPE_HASH), key C.conn_tuple_t, value C.__u32
		output.WriteString("Map: '" + mapName + "', key: 'C.conn_tuple_t', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key http.ConnTuple
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	default: // Go through enabled protocols in case one of them now how to handle the current map
		for _, p := range e.enabledProtocols {
			p.DumpMaps(&output, mapName, currentMap)
		}
	}
	return output.String()
}
