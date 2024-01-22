// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"io"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

type protocol struct {
	cfg            *config.Config
	telemetry      *Telemetry
	statkeeper     *StatKeeper
	eventsConsumer *events.Consumer[EbpfTx]
}

const (
	eventStreamName                          = "kafka"
	filterTailCall                           = "socket__kafka_filter"
	dispatcherTailCall                       = "socket__protocol_dispatcher_kafka"
	protocolDispatcherClassificationPrograms = "dispatcher_classification_progs"
	kafkaLastTCPSeqPerConnectionMap          = "kafka_last_tcp_seq_per_connection"
	kafkaHeapMap                             = "kafka_heap"
)

// Spec is the protocol spec for the kafka protocol.
var Spec = &protocols.ProtocolSpec{
	Factory: newKafkaProtocol,
	Maps: []*manager.Map{
		{
			Name: protocolDispatcherClassificationPrograms,
		},
		{
			Name: kafkaLastTCPSeqPerConnectionMap,
		},
		{
			Name: kafkaHeapMap,
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

// ConfigureOptions add the necessary options for the kafka monitoring to work,
// to be used by the manager. These are:
// - Set the `kafka_last_tcp_seq_per_connection` map size to the value of the `max_tracked_connection` configuration variable.
//
// We also configure the kafka event stream with the manager and its options.
func (p *protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	events.Configure(eventStreamName, mgr, opts)
	opts.MapSpecEditors[kafkaLastTCPSeqPerConnectionMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}
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
func (p *protocol) PostStart(*manager.Manager) error {
	return nil
}

// Stop stops the kafka events consumer.
func (p *protocol) Stop(*manager.Manager) {
	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}
}

// DumpMaps empty implementation.
func (p *protocol) DumpMaps(io.Writer, string, *ebpf.Map) {}

func (p *protocol) processKafka(events []EbpfTx) {
	for i := range events {
		tx := &events[i]
		p.telemetry.Count(tx)
		p.statkeeper.Process(tx)
	}
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
