// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"io"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type protocol struct {
	cfg                *config.Config
	telemetry          *Telemetry
	statkeeper         *StatKeeper
	inFlightMapCleaner *ddebpf.MapCleaner[KafkaTransactionKey, KafkaTransaction]
	eventsConsumer     *events.Consumer[EbpfTx]

	kernelTelemetry            *kernelTelemetry
	kernelTelemetryStopChannel chan struct{}
}

const (
	eventStreamName = "kafka"
	filterTailCall  = "socket__kafka_filter"

	fetchResponsePartitionParserV0TailCall    = "socket__kafka_fetch_response_partition_parser_v0"
	fetchResponsePartitionParserV12TailCall   = "socket__kafka_fetch_response_partition_parser_v12"
	fetchResponseRecordBatchParserV0TailCall  = "socket__kafka_fetch_response_record_batch_parser_v0"
	fetchResponseRecordBatchParserV12TailCall = "socket__kafka_fetch_response_record_batch_parser_v12"
	produceResponsePartitionParserV0TailCall  = "socket__kafka_produce_response_partition_parser_v0"
	produceResponsePartitionParserV9TailCall  = "socket__kafka_produce_response_partition_parser_v9"

	dispatcherTailCall = "socket__protocol_dispatcher_kafka"
	kafkaHeapMap       = "kafka_heap"
	inFlightMap        = "kafka_in_flight"
	responseMap        = "kafka_response"
	telemetryMap       = "kafka_telemetry"

	tlsFilterTailCall = "uprobe__kafka_tls_filter"

	tlsFetchResponsePartitionParserV0TailCall    = "uprobe__kafka_tls_fetch_response_partition_parser_v0"
	tlsFetchResponsePartitionParserV12TailCall   = "uprobe__kafka_tls_fetch_response_partition_parser_v12"
	tlsFetchResponseRecordBatchParserV0TailCall  = "uprobe__kafka_tls_fetch_response_record_batch_parser_v0"
	tlsFetchResponseRecordBatchParserV12TailCall = "uprobe__kafka_tls_fetch_response_record_batch_parser_v12"
	tlsProduceResponsePartitionParserV0TailCall  = "uprobe__kafka_tls_produce_response_partition_parser_v0"
	tlsProduceResponsePartitionParserV9TailCall  = "uprobe__kafka_tls_produce_response_partition_parser_v9"

	tlsTerminationTailCall = "uprobe__kafka_tls_termination"
	tlsDispatcherTailCall  = "uprobe__tls_protocol_dispatcher_kafka"
	// eBPFTelemetryMap is the name of the eBPF map used to retrieve metrics from the kernel
	eBPFTelemetryMap = "kafka_telemetry"
)

// Spec is the protocol spec for the kafka protocol.
var Spec = &protocols.ProtocolSpec{
	Factory: newKafkaProtocol,
	Maps: []*manager.Map{
		{
			Name: kafkaHeapMap,
		},
		{
			Name: inFlightMap,
		},
		{
			Name: responseMap,
		},
		{
			Name: "kafka_client_id",
		},
		{
			Name: "kafka_topic_name",
		},
		{
			Name: telemetryMap,
		},
		{
			Name: "kafka_batch_events",
		},
		{
			Name: "kafka_batch_state",
		},
		{
			Name: "kafka_batches",
		},
	},
	TailCalls: []manager.TailCallRoute{
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafka),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: filterTailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponsePartitionParserV0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: fetchResponsePartitionParserV0TailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponsePartitionParserV12),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: fetchResponsePartitionParserV12TailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponseRecordBatchParserV0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: fetchResponseRecordBatchParserV0TailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponseRecordBatchParserV12),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: fetchResponseRecordBatchParserV12TailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaProduceResponsePartitionParserV0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: produceResponsePartitionParserV0TailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaProduceResponsePartitionParserV9),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: produceResponsePartitionParserV9TailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherClassificationPrograms,
			Key:           uint32(protocols.DispatcherKafkaProg),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: dispatcherTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafka),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFilterTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponsePartitionParserV0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFetchResponsePartitionParserV0TailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponsePartitionParserV12),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFetchResponsePartitionParserV12TailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponseRecordBatchParserV0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFetchResponseRecordBatchParserV0TailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaFetchResponseRecordBatchParserV12),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFetchResponseRecordBatchParserV12TailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaProduceResponsePartitionParserV0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsProduceResponsePartitionParserV0TailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaProduceResponsePartitionParserV9),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsProduceResponsePartitionParserV9TailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramKafkaTermination),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsTerminationTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSProtocolDispatcherClassificationPrograms,
			Key:           uint32(protocols.DispatcherKafkaProg),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsDispatcherTailCall,
			},
		},
	},
}

func newKafkaProtocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnableKafkaMonitoring {
		return nil, nil
	}

	return &protocol{
		cfg:                        cfg,
		telemetry:                  NewTelemetry(),
		kernelTelemetry:            newKernelTelemetry(),
		kernelTelemetryStopChannel: make(chan struct{}),
	}, nil
}

// Name returns the name of the protocol.
func (p *protocol) Name() string {
	return "Kafka"
}

// ConfigureOptions add the necessary options for the kafka monitoring to work, to be used by the manager.
// Configuring the kafka event stream with the manager and its options, and enabling the kafka_monitoring_enabled eBPF
// option.
func (p *protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[inFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[responseMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	events.Configure(p.cfg, eventStreamName, mgr, opts)
	utils.EnableOption(opts, "kafka_monitoring_enabled")
}

// PreStart creates the kafka events consumer and starts it.
func (p *protocol) PreStart(mgr *manager.Manager) error {
	var err error
	p.eventsConsumer, err = events.NewConsumer(
		eventStreamName,
		mgr,
		p.processKafka,
	)
	if err != nil {
		return err
	}

	p.statkeeper = NewStatkeeper(p.cfg, p.telemetry)
	p.eventsConsumer.Start()

	return nil
}

// PostStart starts the map cleaner.
func (p *protocol) PostStart(mgr *manager.Manager) error {
	p.setUpKernelTelemetryCollection(mgr)
	return p.setupInFlightMapCleaner(mgr)
}

// Stop stops the kafka events consumer and the map cleaner.
func (p *protocol) Stop(*manager.Manager) {
	// inFlightMapCleaner handles nil receiver pointers.
	p.inFlightMapCleaner.Stop()
	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}
	if p.kernelTelemetryStopChannel != nil {
		close(p.kernelTelemetryStopChannel)
	}
}

// DumpMaps dumps map contents for debugging.
func (p *protocol) DumpMaps(w io.Writer, mapName string, currentMap *ebpf.Map) {
	switch mapName {
	case inFlightMap:
		var key KafkaTransactionKey
		var value KafkaTransaction
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	case responseMap:
		var key ConnTuple
		var value KafkaResponseContext
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	case telemetryMap:
		var zeroKey uint32

		var value RawKernelTelemetry
		protocols.WriteMapDumpHeader(w, currentMap, mapName, zeroKey, value)
		if err := currentMap.Lookup(unsafe.Pointer(&zeroKey), unsafe.Pointer(&value)); err == nil {
			spew.Fdump(w, zeroKey, value)
		}
	}
}

func (p *protocol) processKafka(events []EbpfTx) {
	for i := range events {
		tx := &events[i]
		p.telemetry.Count(&tx.Transaction)
		p.statkeeper.Process(tx)
	}
}

func (p *protocol) setupInFlightMapCleaner(mgr *manager.Manager) error {
	inFlightMap, _, err := mgr.GetMap(inFlightMap)
	if err != nil {
		return err
	}
	mapCleaner, err := ddebpf.NewMapCleaner[KafkaTransactionKey, KafkaTransaction](inFlightMap, 1024)
	if err != nil {
		return err
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, _ KafkaTransactionKey, val KafkaTransaction) bool {
		started := int64(val.Request_started)
		return started > 0 && (now-started) > ttl
	})

	p.inFlightMapCleaner = mapCleaner
	return nil
}

// GetStats returns a map of Kafka stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (p *protocol) GetStats() *protocols.ProtocolStats {
	p.eventsConsumer.Sync()
	p.telemetry.Log()
	return &protocols.ProtocolStats{
		Type:  protocols.Kafka,
		Stats: p.statkeeper.GetAndResetAllStats(),
	}
}

// IsBuildModeSupported returns always true, as kafka module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

func (p *protocol) setUpKernelTelemetryCollection(mgr *manager.Manager) {
	mp, err := protocols.GetMap(mgr, eBPFTelemetryMap)
	if err != nil {
		log.Warn(err)
		return
	}

	zero := 0
	rawTelemetry := &RawKernelTelemetry{}
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(rawTelemetry)); err != nil {
					log.Errorf("unable to lookup %q map: %s", eBPFTelemetryMap, err)
					return
				}
				p.kernelTelemetry.update(rawTelemetry)
			case <-p.kernelTelemetryStopChannel:
				return
			}
		}
	}()
}

// GetKernelTelemetryMap retrieves Kafka kernel telemetry map from the provided manager
func GetKernelTelemetryMap(mgr *manager.Manager) (*RawKernelTelemetry, error) {
	mp, err := protocols.GetMap(mgr, eBPFTelemetryMap)
	if err != nil {
		return nil, err
	}

	zero := 0
	rawTelemetry := &RawKernelTelemetry{}
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(rawTelemetry)); err != nil {
		return nil, err
	}
	return rawTelemetry, nil
}
