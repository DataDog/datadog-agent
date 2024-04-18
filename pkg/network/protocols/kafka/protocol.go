// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"io"
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
)

type protocol struct {
	cfg                *config.Config
	telemetry          *Telemetry
	statkeeper         *StatKeeper
	inFlightMapCleaner *ddebpf.MapCleaner[KafkaTransactionKey, KafkaTransaction]
	eventsConsumer     *events.Consumer[EbpfTx]
}

const (
	eventStreamName                          = "kafka"
	filterTailCall                           = "socket__kafka_filter"
	responseParserTailCall                   = "socket__kafka_response_parser"
	dispatcherTailCall                       = "socket__protocol_dispatcher_kafka"
	protocolDispatcherClassificationPrograms = "dispatcher_classification_progs"
	kafkaHeapMap                             = "kafka_heap"
	inFlightMap                              = "kafka_in_flight"
	responseMap                              = "kafka_response"
)

// Spec is the protocol spec for the kafka protocol.
var Spec = &protocols.ProtocolSpec{
	Factory: newKafkaProtocol,
	Maps: []*manager.Map{
		{
			Name: protocolDispatcherClassificationPrograms,
		},
		{
			Name: kafkaHeapMap,
		},
		{
			Name: inFlightMap,
		},
		{
			Name: responseMap,
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
			Key:           uint32(protocols.ProgramKafkaResponseParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: responseParserTailCall,
			},
		},
		{
			ProgArrayName: protocolDispatcherClassificationPrograms,
			Key:           uint32(protocols.DispatcherKafkaProg),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: dispatcherTailCall,
			},
		},
	},
}

func newKafkaProtocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnableKafkaMonitoring {
		return nil, nil
	}

	return &protocol{
		cfg:       cfg,
		telemetry: NewTelemetry(),
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

// PostStart empty implementation.
func (p *protocol) PostStart(mgr *manager.Manager) error {
	return p.setupInFlightMapCleaner(mgr)
}

// Stop stops the kafka events consumer.
func (p *protocol) Stop(*manager.Manager) {
	// inFlightMapCleaner handles nil receiver pointers.
	p.inFlightMapCleaner.Stop()
	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
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
	}
}

func (p *protocol) processKafka(events []EbpfTx) {
	for i := range events {
		tx := &events[i]
		p.telemetry.Count(tx)
		p.statkeeper.Process(tx)
	}
}

func (p *protocol) setupInFlightMapCleaner(mgr *manager.Manager) error {
	inFlightMap, _, err := mgr.GetMap(inFlightMap)
	if err != nil {
		return err
	}
	// Disable batching as a temporary workaround since enabling it leads to
	// TestKafkaInFlightMapCleaner() failing due to the values read not matching
	// the values inserted into the map.
	mapCleaner, err := ddebpf.NewMapCleaner[KafkaTransactionKey, KafkaTransaction](inFlightMap, 1)
	if err != nil {
		return err
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, key KafkaTransactionKey, val KafkaTransaction) bool {
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
