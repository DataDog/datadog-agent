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
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	httpInFlightMap  = "http_in_flight"
	http2InFlightMap = "http2_in_flight"

	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to classify protocols and dispatch the correct handlers.
	protocolDispatcherSocketFilterFunction   = "socket__protocol_dispatcher"
	protocolDispatcherProgramsMap            = "protocols_progs"
	protocolDispatcherClassificationPrograms = "dispatcher_classification_progs"
	dispatcherConnectionProtocolMap          = "dispatcher_connection_protocol"
	connectionStatesMap                      = "connection_states"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	maxActive = 128
	probeUID  = "http"

	kafkaLastTCPSeqPerConnectionMap = "kafka_last_tcp_seq_per_connection"
)

type ebpfProgram struct {
	*errtelemetry.Manager
	cfg             *config.Config
	offsets         []manager.ConstantEditor
	subprograms     []subprogram
	probesResolvers []probeResolver
	mapCleaner      *ddebpf.MapCleaner
	tailCallRouter  []manager.TailCallRoute
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

var http2TailCall = manager.TailCallRoute{
	ProgArrayName: protocolDispatcherProgramsMap,
	Key:           uint32(ProtocolHTTP2),
	ProbeIdentificationPair: manager.ProbeIdentificationPair{
		EBPFFuncName: "socket__http2_filter",
	},
}

func newEBPFProgram(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*ebpfProgram, error) {
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: httpInFlightMap},
			{Name: sslSockByCtxMap},
			{Name: protocolDispatcherProgramsMap},
			{Name: "ssl_read_args"},
			{Name: "bio_new_socket_args"},
			{Name: "fd_by_ssl_bio"},
			{Name: "ssl_ctx_by_pid_tgid"},
			{Name: connectionStatesMap},
			{Name: protocolDispatcherClassificationPrograms},
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

	tailCalls := []manager.TailCallRoute{
		{
			ProgArrayName: protocolDispatcherProgramsMap,
			Key:           uint32(ProtocolHTTP),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "socket__http_filter",
			},
		},
	}

	if c.EnableHTTP2Monitoring {
		tailCalls = append(tailCalls, http2TailCall)
	}

	// If Kafka monitoring is enabled, the kafka parsing function and the Kafka dispatching function are added to the dispatcher mechanism.
	if c.EnableKafkaMonitoring {
		tailCalls = append(tailCalls,
			manager.TailCallRoute{
				ProgArrayName: protocolDispatcherProgramsMap,
				Key:           uint32(ProtocolKafka),
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "socket__kafka_filter",
				},
			},
			manager.TailCallRoute{
				ProgArrayName: protocolDispatcherClassificationPrograms,
				Key:           uint32(DispatcherKafkaProg),
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "socket__protocol_dispatcher_kafka",
				},
			})
	}

	program := &ebpfProgram{
		Manager:         errtelemetry.NewManager(mgr, bpfTelemetry),
		cfg:             c,
		offsets:         offsets,
		subprograms:     subprograms,
		probesResolvers: subprogramProbesResolvers,
		tailCallRouter:  tailCalls,
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

	e.DumpHandler = dumpMapsHandler
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

func (e *ebpfProgram) initCORE() error {
	assetName := getAssetName("http", e.cfg.BPFDebug)
	return ddebpf.LoadCOREAsset(&e.cfg.Config, assetName, e.init)
}

func (e *ebpfProgram) initRuntimeCompiler() error {
	bc, err := getRuntimeCompiledHTTP(e.cfg)
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
	return e.init(bc, manager.Options{})
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
		httpInFlightMap: {
			Type:       ebpf.Hash,
			MaxEntries: uint32(e.cfg.MaxTrackedConnections),
			EditorFlag: manager.EditMaxEntries,
		},
		http2InFlightMap: {
			Type:       ebpf.Hash,
			MaxEntries: uint32(e.cfg.MaxTrackedConnections),
			EditorFlag: manager.EditMaxEntries,
		},
		connectionStatesMap: {
			Type:       ebpf.Hash,
			MaxEntries: uint32(e.cfg.MaxTrackedConnections),
			EditorFlag: manager.EditMaxEntries,
		},
		dispatcherConnectionProtocolMap: {
			Type:       ebpf.Hash,
			MaxEntries: uint32(e.cfg.MaxTrackedConnections),
			EditorFlag: manager.EditMaxEntries,
		},
		kafkaLastTCPSeqPerConnectionMap: {
			Type:       ebpf.Hash,
			MaxEntries: uint32(e.cfg.MaxTrackedConnections),
			EditorFlag: manager.EditMaxEntries,
		},
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
	options.ConstantEditors = e.offsets
	addBoolConst(&options, e.cfg.EnableHTTPMonitoring, "http_monitoring_enabled")
	addBoolConst(&options, e.cfg.EnableHTTP2Monitoring, "http2_monitoring_enabled")
	addBoolConst(&options, e.cfg.EnableKafkaMonitoring, "kafka_monitoring_enabled")
	options.DefaultKprobeAttachMethod = kprobeAttachMethod
	options.VerifierOptions.Programs.LogSize = 2 * 1024 * 1024

	for _, s := range e.subprograms {
		s.ConfigureOptions(&options)
	}

	// Configure event streams
	events.Configure("http", e.Manager.Manager, &options)

	if e.cfg.EnableHTTP2Monitoring {
		events.Configure("http2", e.Manager.Manager, &options)
	} else {
		options.ExcludedFunctions = append(options.ExcludedFunctions, "socket__http2_filter")
	}

	if e.cfg.EnableKafkaMonitoring {
		events.Configure("kafka", e.Manager.Manager, &options)
	} else {
		// If Kafka monitoring is not enabled, loading the program will cause a verifier issue and should be avoided.
		options.ExcludedFunctions = append(options.ExcludedFunctions, "socket__kafka_filter", "socket__protocol_dispatcher_kafka")
	}

	return e.InitWithOptions(buf, options)
}

func getAssetName(module string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", module)
	}

	return fmt.Sprintf("%s.o", module)
}
