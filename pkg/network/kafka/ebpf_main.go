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
	"github.com/iovisor/gobpf/pkg/cpupossible"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	kafkaBatchesMap                 = "kafka_batches"
	kafkaBatchStateMap              = "kafka_batch_state"
	kafkaBatchEvents                = "kafka_batch_events"
	kafkaLastTCPSeqPerConnectionMap = "kafka_last_tcp_seq_per_connection"

	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to inspect plain kafka traffic
	kafkaSocketFilterStub = "socket/kafka_filter_entry"
	kafkaSocketFilter     = "socket/kafka_filter"
	kafkaProgsMap         = "kafka_progs"

	// size of the channel containing completed kafka_notification_objects
	batchNotificationsChanSize = 100

	probeUID = "kafka"
)

type ebpfProgram struct {
	*manager.Manager
	cfg      *config.Config
	bytecode bytecode.AssetReader

	batchCompletionHandler *ddebpf.PerfHandler
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

func newEBPFProgram(c *config.Config) (*ebpfProgram, error) {
	bc, err := getBytecode(c)
	if err != nil {
		return nil, err
	}

	batchCompletionHandler := ddebpf.NewPerfHandler(batchNotificationsChanSize)
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: kafkaLastTCPSeqPerConnectionMap},
			{Name: kafkaBatchesMap},
			{Name: kafkaBatchStateMap},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: kafkaBatchEvents},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					RecordHandler:      batchCompletionHandler.RecordHandler,
					LostHandler:        batchCompletionHandler.LostHandler,
					RecordGetter:       batchCompletionHandler.RecordGetter,
				},
			},
		},
		Probes: []*manager.Probe{
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
			},
		},
	}

	program := &ebpfProgram{
		Manager:                mgr,
		bytecode:               bc,
		cfg:                    c,
		batchCompletionHandler: batchCompletionHandler,
	}

	return program, nil
}

func (e *ebpfProgram) Init() error {
	defer e.bytecode.Close()

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
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "tracepoint/net/netif_receive_skb",
					EBPFFuncName: "tracepoint__net__netif_receive_skb",
					UID:          probeUID,
				},
			},
		},
		VerifierOptions: ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				// LogSize is the size of the log buffer given to the verifier. Give it a big enough (2 * 1024 * 1024)
				// value so that all our programs fit. If the verifier ever outputs a `no space left on device` error,
				// we'll need to increase this value.
				LogSize: 2097152,
			},
		},
	}

	return e.InitWithOptions(e.bytecode, options)
}

func (e *ebpfProgram) Start() error {
	return e.Manager.Start()
}

func (e *ebpfProgram) Close() error {
	err := e.Stop(manager.CleanAll)
	e.batchCompletionHandler.Stop()
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
