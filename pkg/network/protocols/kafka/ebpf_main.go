// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"fmt"
	"math"
	"os"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/iovisor/gobpf/pkg/cpupossible"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	kafkaInFlightMap                = "kafka_in_flight"
	kafkaBatchesMap                 = "kafka_batches"
	kafkaBatchStateMap              = "kafka_batch_state"
	kafkaBatchEvents                = "kafka_batch_events"
	kafkaLastTCPSeqPerConnectionMap = "kafka_last_tcp_seq_per_connection"

	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to inspect plain kafka traffic
	kafkaSocketFilterStub = "socket/kafka_filter_entry"
	kafkaSocketFilter     = "socket/kafka_filter"
	kafkaProgsMap         = "kafka_progs"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	//maxActive = 128

	// size of the channel containing completed kafka_notification_objects
	batchNotificationsChanSize = 100

	probeUID = "kafka"
)

type ebpfProgram struct {
	*errtelemetry.Manager
	cfg         *config.Config
	bytecode    bytecode.AssetReader
	offsets     []manager.ConstantEditor
	subprograms []subprogram
	mapCleaner  *ddebpf.MapCleaner

	batchCompletionHandler *ddebpf.PerfHandler
}

type subprogram interface {
	ConfigureManager(*errtelemetry.Manager)
	ConfigureOptions(*manager.Options)
	GetAllUndefinedProbes() []manager.ProbeIdentificationPair
	Start()
	Stop()
}

var tailCalls = []manager.TailCallRoute{
	{
		ProgArrayName: kafkaProgsMap,
		Key:           kafkaProg,
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			EBPFSection:  kafkaSocketFilter,
			EBPFFuncName: "socket__kafka_filter",
		},
	},
}

func newEBPFProgram(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*ebpfProgram, error) {
	bc, err := getBytecode(c)
	if err != nil {
		return nil, err
	}

	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if c.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithPerfEventOpen
	}

	batchCompletionHandler := ddebpf.NewPerfHandler(batchNotificationsChanSize)
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: kafkaInFlightMap},
			{Name: kafkaLastTCPSeqPerConnectionMap},
			{Name: kafkaBatchesMap},
			{Name: kafkaBatchStateMap},
			//	{Name: sslSockByCtxMap},
			//	{Name: httpProgsMap},
			//	{Name: "ssl_read_args"},
			//	{Name: "bio_new_socket_args"},
			//	{Name: "fd_by_ssl_bio"},
			//	{Name: "ssl_ctx_by_pid_tgid"},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: kafkaBatchEvents},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 256 * os.Getpagesize(),
					Watermark:          1,
					RecordHandler:      batchCompletionHandler.RecordHandler,
					LostHandler:        batchCompletionHandler.LostHandler,
					RecordGetter:       batchCompletionHandler.RecordGetter,
				},
			},
		},
		Probes: []*manager.Probe{
			//{
			//	ProbeIdentificationPair: manager.ProbeIdentificationPair{
			//		EBPFSection:  string(probes.TCPSendMsg),
			//		EBPFFuncName: "kprobe__tcp_sendmsg",
			//		UID:          probeUID,
			//	},
			//	KProbeMaxActive:    maxActive,
			//	KprobeAttachMethod: kprobeAttachMethod,
			//},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "tracepoint/net/netif_receive_skb",
					EBPFFuncName: "tracepoint__net__netif_receive_skb",
					UID:          probeUID,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  kafkaSocketFilterStub,
					EBPFFuncName: "socket__kafka_filter_entry",
					UID:          probeUID,
				},
				KprobeAttachMethod: kprobeAttachMethod,
			},
		},
	}

	program := &ebpfProgram{
		Manager:                errtelemetry.NewManager(mgr, bpfTelemetry),
		bytecode:               bc,
		cfg:                    c,
		offsets:                offsets,
		batchCompletionHandler: batchCompletionHandler,
		subprograms:            []subprogram{},
	}

	return program, nil
}

func (e *ebpfProgram) Init() error {
	var undefinedProbes []manager.ProbeIdentificationPair

	defer e.bytecode.Close()

	for _, tc := range tailCalls {
		undefinedProbes = append(undefinedProbes, tc.ProbeIdentificationPair)
	}
	for _, s := range e.subprograms {
		undefinedProbes = append(undefinedProbes, s.GetAllUndefinedProbes()...)
	}

	//e.DumpHandler = dumpMapsHandler
	e.InstructionPatcher = func(m *manager.Manager) error {
		return errtelemetry.PatchEBPFTelemetry(m, true, undefinedProbes)
	}
	for _, s := range e.subprograms {
		s.ConfigureManager(e.Manager)
	}

	onlineCPUs, err := cpupossible.Get()
	if err != nil {
		return fmt.Errorf("couldn't determine number of CPUs: %w", err)
	}

	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			kafkaInFlightMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
				EditorFlag: manager.EditMaxEntries,
			},
			kafkaLastTCPSeqPerConnectionMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
				EditorFlag: manager.EditMaxEntries,
			},
			kafkaBatchesMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(len(onlineCPUs) * KAFKABatchPages),
				EditorFlag: manager.EditMaxEntries,
			},
		},
		TailCallRouter: tailCalls,
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  kafkaSocketFilterStub,
					EBPFFuncName: "socket__kafka_filter_entry",
					UID:          probeUID,
				},
			},
			//&manager.ProbeSelector{
			//	ProbeIdentificationPair: manager.ProbeIdentificationPair{
			//		EBPFSection:  string(probes.TCPSendMsg),
			//		EBPFFuncName: "kprobe__tcp_sendmsg",
			//		UID:          probeUID,
			//	},
			//},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "tracepoint/net/netif_receive_skb",
					EBPFFuncName: "tracepoint__net__netif_receive_skb",
					UID:          probeUID,
				},
			},
		},
		ConstantEditors: e.offsets,
		VerifierOptions: ebpf.CollectionOptions{
			Maps: ebpf.MapOptions{
				PinPath: "",
				LoadPinOptions: ebpf.LoadPinOptions{
					ReadOnly:  false,
					WriteOnly: false,
					Flags:     0,
				},
			},
			Programs: ebpf.ProgramOptions{
				LogSize:     1024 * 1024 * 20,
				LogDisabled: false,
				KernelTypes: &btf.Spec{},
			},
			MapReplacements: nil,
		},
	}

	for _, s := range e.subprograms {
		s.ConfigureOptions(&options)
	}

	err = e.InitWithOptions(e.bytecode, options)
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

	//e.setupMapCleaner()

	return nil
}

func (e *ebpfProgram) Close() error {
	e.mapCleaner.Stop()
	err := e.Stop(manager.CleanAll)
	e.batchCompletionHandler.Stop()
	for _, s := range e.subprograms {
		s.Stop()
	}
	return err
}

func getBytecode(c *config.Config) (bc bytecode.AssetReader, err error) {
	if c.EnableRuntimeCompiler {
		bc, err = getRuntimeCompiledKafka(c)
		if err != nil {
			if !c.AllowPrecompiledFallback {
				return nil, fmt.Errorf("error compiling network kafka tracer: %w", err)
			}
			log.Warnf("error compiling network kafka tracer, falling back to pre-compiled: %s", err)
		}
	}

	if bc == nil {
		bc, err = netebpf.ReadKafkaModule(c.BPFDir, c.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}
	}

	return
}
