// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

//import (
//	"fmt"
//	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
//	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/Telemetry"
//	"github.com/cilium/ebpf"
//	"golang.org/x/sys/unix"
//	"math"
//
//	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
//	"github.com/DataDog/datadog-agent/pkg/network/config"
//	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
//	"github.com/DataDog/datadog-agent/pkg/util/log"
//	manager "github.com/DataDog/ebpf-manager"
//)
//
//const (
//	kafkaLastTCPSeqPerConnectionMap = "kafka_last_tcp_seq_per_connection"
//
//	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
//	// to inspect plain kafka traffic
//	kafkaSocketFilterStub = "socket/kafka_filter_entry"
//	kafkaSocketFilter     = "socket/kafka_filter"
//	kafkaProgsMap         = "kafka_progs"
//
//	probeUID = "kafka"
//)
//
//type ebpfProgram struct {
//	*errtelemetry.Manager
//	cfg      *config.Config
//	bytecode bytecode.AssetReader
//}
//
//var tailCalls = []manager.TailCallRoute{
//	{
//		ProgArrayName: kafkaProgsMap,
//		Key:           kafkaProg,
//		ProbeIdentificationPair: manager.ProbeIdentificationPair{
//			EBPFSection:  kafkaSocketFilter,
//			EBPFFuncName: "socket__kafka_filter",
//		},
//	},
//}
//
//func newEBPFProgram(c *config.Config, bpfTelemetry *errtelemetry.EBPFTelemetry) (*ebpfProgram, error) {
//	bc, err := getBytecode(c)
//	if err != nil {
//		return nil, err
//	}
//
//	mgr := &manager.Manager{
//		Maps: []*manager.Map{},
//		Probes: []*manager.Probe{
//			{
//				ProbeIdentificationPair: manager.ProbeIdentificationPair{
//					EBPFSection:  "tracepoint/net/netif_receive_skb",
//					EBPFFuncName: "tracepoint__net__netif_receive_skb",
//					UID:          probeUID,
//				},
//			},
//			//{
//			//	ProbeIdentificationPair: manager.ProbeIdentificationPair{
//			//		EBPFSection:  kafkaSocketFilterStub,
//			//		EBPFFuncName: "socket__kafka_filter_entry",
//			//		UID:          probeUID,
//			//	},
//			//},
//		},
//	}
//
//	program := &ebpfProgram{
//		Manager:  errtelemetry.NewManager(mgr, bpfTelemetry),
//		bytecode: bc,
//		cfg:      c,
//	}
//
//	return program, nil
//}
//
//func (e *ebpfProgram) Init() error {
//	defer e.bytecode.Close()
//
//	var undefinedProbes []manager.ProbeIdentificationPair
//	for _, tc := range tailCalls {
//		undefinedProbes = append(undefinedProbes, tc.ProbeIdentificationPair)
//	}
//	e.InstructionPatcher = func(m *manager.Manager) error {
//		return errtelemetry.PatchEBPFTelemetry(m, true, undefinedProbes)
//	}
//
//	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
//	if e.cfg.AttachKprobesWithKprobeEventsABI {
//		kprobeAttachMethod = manager.AttachKprobeWithPerfEventOpen
//	}
//
//	options := manager.Options{
//		RLimit: &unix.Rlimit{
//			Cur: math.MaxUint64,
//			Max: math.MaxUint64,
//		},
//		MapSpecEditors: map[string]manager.MapSpecEditor{
//			kafkaLastTCPSeqPerConnectionMap: {
//				Type:       ebpf.Hash,
//				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
//				EditorFlag: manager.EditMaxEntries,
//			},
//		},
//		TailCallRouter: tailCalls,
//		ActivatedProbes: []manager.ProbesSelector{
//			//&manager.ProbeSelector{
//			//	ProbeIdentificationPair: manager.ProbeIdentificationPair{
//			//		EBPFSection:  kafkaSocketFilterStub,
//			//		EBPFFuncName: "socket__kafka_filter_entry",
//			//		UID:          probeUID,
//			//	},
//			//},
//			&manager.ProbeSelector{
//				ProbeIdentificationPair: manager.ProbeIdentificationPair{
//					EBPFSection:  "tracepoint/net/netif_receive_skb",
//					EBPFFuncName: "tracepoint__net__netif_receive_skb",
//					UID:          probeUID,
//				},
//			},
//		},
//		DefaultKprobeAttachMethod: kprobeAttachMethod,
//		VerifierOptions: ebpf.CollectionOptions{
//			Programs: ebpf.ProgramOptions{
//				// LogSize is the size of the log buffer given to the verifier. Give it a big enough value so that all our programs fit.
//				// If the verifier ever outputs a `no space left on device` error,  we'll need to increase this value.
//				LogSize: 10 * 10 * 1024 * 1024,
//			},
//		},
//	}
//
//	// configure event stream
//	events.Configure("kafka", e.Manager.Manager, &options)
//
//	return e.InitWithOptions(e.bytecode, options)
//}
//
//func (e *ebpfProgram) Start() error {
//	return e.Manager.Start()
//}
//
//func (e *ebpfProgram) Close() error {
//	err := e.Stop(manager.CleanAll)
//	return err
//}
//
//func getBytecode(c *config.Config) (bc bytecode.AssetReader, err error) {
//	if c.EnableRuntimeCompiler {
//		bc, err = getRuntimeCompiledKafka(c)
//		if err != nil {
//			if !c.AllowPrecompiledFallback {
//				return nil, fmt.Errorf("error compiling network kafka tracer: %w", err)
//			}
//			log.Warnf("error compiling network kafka tracer, falling back to pre-compiled: %s", err)
//		}
//	}
//
//	if bc == nil {
//		bc, err = netebpf.ReadKafkaModule(c.BPFDir, c.BPFDebug)
//		if err != nil {
//			return nil, fmt.Errorf("could not read bpf module: %s", err)
//		}
//	}
//
//	return
//}
