// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

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
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	http2InFlightMap = "http2_in_flight"

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

	enabledProtocols  map[protocols.ProtocolType]protocols.Protocol
	disabledProtocols []*protocols.ProtocolSpec

	buildMode buildMode
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
	Prebuilt        buildMode = "prebuilt"
	RuntimeCompiled buildMode = "runtime-compilation"
	CORE            buildMode = "CO-RE"
)

type subprogram interface {
	Name() string
	IsBuildModeSupported(buildMode) bool
	ConfigureManager(*errtelemetry.Manager)
	ConfigureOptions(*manager.Options)
	Start()
	Stop()
}

var http2TailCall = manager.TailCallRoute{
	ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
	Key:           uint32(protocols.ProgramHTTP2),
	ProbeIdentificationPair: manager.ProbeIdentificationPair{
		EBPFFuncName: "socket__http2_filter",
	},
}

func newEBPFProgram(c *config.Config, connectionProtocolMap, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*ebpfProgram, error) {
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: sslSockByCtxMap},
			{Name: protocols.ProtocolDispatcherProgramsMap},
			{Name: "ssl_read_args"},
			{Name: "bio_new_socket_args"},
			{Name: "fd_by_ssl_bio"},
			{Name: "ssl_ctx_by_pid_tgid"},
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

	if c.EnableHTTP2Monitoring {
		mgr.Maps = append(mgr.Maps, &manager.Map{Name: "http2_dynamic_table"}, &manager.Map{Name: "http2_static_table"})
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

	var tailCalls []manager.TailCallRoute

	if c.EnableHTTP2Monitoring {
		tailCalls = append(tailCalls, http2TailCall)
	}

	if IsJavaSubprogramEnabled(c) {
		tailCalls = append(tailCalls, GetJavaTlsTailCallRoutes()...)
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
	err := e.Manager.Start()
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
		http2InFlightMap: {
			Type:       ebpf.Hash,
			MaxEntries: e.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		},
		connectionStatesMap: {
			Type:       ebpf.Hash,
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
			Type:       ebpf.Hash,
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

	// Set some eBPF constants to tell the protocol dispatcher which ones are
	// enabled. These needs to be set here, even if some protocols are not
	// enabled, to make sure they exist. Without this, the dispatcher would try
	// to check non-existing constants, which is not possible and an error.
	addBoolConst(&options, e.cfg.EnableHTTPMonitoring, "http_monitoring_enabled")
	addBoolConst(&options, e.cfg.EnableHTTP2Monitoring, "http2_monitoring_enabled")
	addBoolConst(&options, e.cfg.EnableKafkaMonitoring, "kafka_monitoring_enabled")
	// Some parts of USM (https capturing, and part of the classification) use `read_conn_tuple`, and has some if
	// clauses that handled IPV6, for USM we care (ATM) only from TCP connections, so adding the sole config about tcpv6.
	addBoolConst(&options, e.cfg.CollectTCPv6Conns, "tcpv6_enabled")

	options.DefaultKprobeAttachMethod = kprobeAttachMethod
	options.VerifierOptions.Programs.LogSize = 2 * 1024 * 1024

	for _, s := range e.subprograms {
		s.ConfigureOptions(&options)
	}

	for _, p := range e.enabledProtocols {
		p.ConfigureOptions(e.Manager.Manager, &options)
	}

	// Add excluded functions from disabled protocols
	for _, p := range e.disabledProtocols {
		for _, m := range p.Maps {
			// Unused maps still needs to have a non-zero size
			options.MapSpecEditors[m.Name] = manager.MapSpecEditor{
				Type:       ebpf.Hash,
				MaxEntries: uint32(1),
				EditorFlag: manager.EditMaxEntries,
			}

			log.Debugf("disabled map: %v", m.Name)
		}

		for _, tc := range p.TailCalls {
			options.ExcludedFunctions = append(options.ExcludedFunctions, tc.ProbeIdentificationPair.EBPFFuncName)
		}
	}

	// Configure event streams
	if e.cfg.EnableHTTP2Monitoring {
		events.Configure("http2", e.Manager.Manager, &options)
	} else {
		options.ExcludedFunctions = append(options.ExcludedFunctions, "socket__http2_filter")
	}

	return e.InitWithOptions(buf, options)
}

func getAssetName(module string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", module)
	}

	return fmt.Sprintf("%s.o", module)
}

func addBoolConst(options *manager.Options, flag bool, name string) {
	val := uint64(1)
	if !flag {
		val = uint64(0)
	}

	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{
			Name:  name,
			Value: val,
		},
	)
}
