// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"math"

	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	httpInFlightMap = "http_in_flight"

	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to classify protocols and dispatch the correct handlers.
	protocolDispatcherSocketFilterSection  = "socket/protocol_dispatcher"
	protocolDispatcherSocketFilterFunction = "socket__protocol_dispatcher"
	protocolDispatcherProgramsMap          = "protocols_progs"

	httpSocketFilter = "socket/http_filter"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	maxActive = 128
	probeUID  = "http"
)

type ebpfProgram struct {
	*errtelemetry.Manager
	cfg             *config.Config
	bytecode        bytecode.AssetReader
	offsets         []manager.ConstantEditor
	subprograms     []subprogram
	probesResolvers []probeResolver
	mapCleaner      *ddebpf.MapCleaner
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

type subprogram interface {
	ConfigureManager(*errtelemetry.Manager)
	ConfigureOptions(*manager.Options)
	Start()
	Stop()
}

var tailCalls = []manager.TailCallRoute{
	{
		ProgArrayName: protocolDispatcherProgramsMap,
		Key:           uint32(ProtocolHTTP),
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			EBPFSection:  httpSocketFilter,
			EBPFFuncName: "socket__http_filter",
		},
	},
}

func newEBPFProgram(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*ebpfProgram, error) {
	bc, err := getBytecode(c)
	if err != nil {
		return nil, err
	}

	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: httpInFlightMap},
			{Name: sslSockByCtxMap},
			{Name: protocolDispatcherProgramsMap},
			{Name: "ssl_read_args"},
			{Name: "bio_new_socket_args"},
			{Name: "fd_by_ssl_bio"},
			{Name: "ssl_ctx_by_pid_tgid"},
		},
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.TCPSendMsg),
					EBPFFuncName: "kprobe__tcp_sendmsg",
					UID:          probeUID,
				},
				KProbeMaxActive: maxActive,
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "tracepoint/net/netif_receive_skb",
					EBPFFuncName: "tracepoint__net__netif_receive_skb",
					UID:          probeUID,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  protocolDispatcherSocketFilterSection,
					EBPFFuncName: protocolDispatcherSocketFilterFunction,
					UID:          probeUID,
				},
			},
		},
	}

	subprogramProbesResolvers := make([]probeResolver, 0, 3)
	subprograms := make([]subprogram, 0, 3)

	goTLSProg := newGoTLSProgram(c)
	subprogramProbesResolvers = append(subprogramProbesResolvers, goTLSProg)
	if goTLSProg != nil {
		subprograms = append(subprograms, goTLSProg)
	}
	javaTLSProg := newJavaTLSProgram(c)
	subprogramProbesResolvers = append(subprogramProbesResolvers, javaTLSProg)
	if javaTLSProg != nil {
		subprograms = append(subprograms, javaTLSProg)
	}
	openSSLProg := newSSLProgram(c, sockFD)
	subprogramProbesResolvers = append(subprogramProbesResolvers, openSSLProg)
	if openSSLProg != nil {
		subprograms = append(subprograms, openSSLProg)
	}
	program := &ebpfProgram{
		Manager:         errtelemetry.NewManager(mgr, bpfTelemetry),
		bytecode:        bc,
		cfg:             c,
		offsets:         offsets,
		subprograms:     subprograms,
		probesResolvers: subprogramProbesResolvers,
	}

	return program, nil
}

func (e *ebpfProgram) Init() error {
	var undefinedProbes []manager.ProbeIdentificationPair

	defer e.bytecode.Close()

	for _, tc := range tailCalls {
		undefinedProbes = append(undefinedProbes, tc.ProbeIdentificationPair)
	}

	for _, s := range e.probesResolvers {
		undefinedProbes = append(undefinedProbes, s.GetAllUndefinedProbes()...)
	}

	e.DumpHandler = dumpMapsHandler
	e.InstructionPatcher = func(m *manager.Manager) error {
		return errtelemetry.PatchEBPFTelemetry(m, true, undefinedProbes)
	}
	for _, s := range e.subprograms {
		s.ConfigureManager(e.Manager)
	}

	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if e.cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithPerfEventOpen
	}

	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			httpInFlightMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
				EditorFlag: manager.EditMaxEntries,
			},
		},
		TailCallRouter: tailCalls,
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  protocolDispatcherSocketFilterSection,
					EBPFFuncName: protocolDispatcherSocketFilterFunction,
					UID:          probeUID,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.TCPSendMsg),
					EBPFFuncName: "kprobe__tcp_sendmsg",
					UID:          probeUID,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "tracepoint/net/netif_receive_skb",
					EBPFFuncName: "tracepoint__net__netif_receive_skb",
					UID:          probeUID,
				},
			},
		},
		ConstantEditors:           e.offsets,
		DefaultKprobeAttachMethod: kprobeAttachMethod,
	}

	for _, s := range e.subprograms {
		s.ConfigureOptions(&options)
	}

	// configure event stream
	events.Configure("http", e.Manager.Manager, &options)

	err := e.InitWithOptions(e.bytecode, options)
	if err != nil {
		return err
	}

	return nil
}

func (e *ebpfProgram) Start() error {
	err := e.Manager.Start()
	if err != nil {
		return err
	}

	for _, s := range e.subprograms {
		s.Start()
	}

	e.setupMapCleaner()

	return nil
}

func (e *ebpfProgram) Close() error {
	e.mapCleaner.Stop()
	err := e.Stop(manager.CleanAll)
	for _, s := range e.subprograms {
		s.Stop()
	}
	return err
}

func (e *ebpfProgram) setupMapCleaner() {
	httpMap, _, _ := e.GetMap(httpInFlightMap)
	httpMapCleaner, err := ddebpf.NewMapCleaner(httpMap, new(netebpf.ConnTuple), new(ebpfHttpTx))
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	ttl := e.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	httpMapCleaner.Clean(e.cfg.HTTPMapCleanerInterval, func(now int64, key, val interface{}) bool {
		httpTxn, ok := val.(*ebpfHttpTx)
		if !ok {
			return false
		}

		if updated := int64(httpTxn.ResponseLastSeen()); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(httpTxn.RequestStarted())
		return started > 0 && (now-started) > ttl
	})

	e.mapCleaner = httpMapCleaner
}

func getBytecode(c *config.Config) (bc bytecode.AssetReader, err error) {
	if c.EnableRuntimeCompiler {
		bc, err = getRuntimeCompiledHTTP(c)
		if err != nil {
			if !c.AllowPrecompiledFallback {
				return nil, fmt.Errorf("error compiling network http tracer: %w", err)
			}
			log.Warnf("error compiling network http tracer, falling back to pre-compiled: %s", err)
		}
	}

	if bc == nil {
		bc, err = netebpf.ReadHTTPModule(c.BPFDir, c.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}
	}

	return
}
