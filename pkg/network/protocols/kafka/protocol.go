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

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type protocol struct {
	cfg            *config.Config
	telemetry      *Telemetry
	statkeeper     *StatKeeper
	eventsConsumer *events.Consumer[EbpfTx]

	kernelTelemetry            *kernelTelemetry
	kernelTelemetryStopChannel chan struct{}
}

const (
	eventStreamName                          = "kafka"
	filterTailCall                           = "socket__kafka_filter"
	dispatcherTailCall                       = "socket__protocol_dispatcher_kafka"
	protocolDispatcherClassificationPrograms = "dispatcher_classification_progs"
	kafkaHeapMap                             = "kafka_heap"
	// eBPFTelemetryMap is the name of the eBPF map used to retrieve metrics from the kernel
	eBPFTelemetryMap = "kafka_telemetry"
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
	p.setUpKernelTelemetryCollection(mgr)
	return nil
}

// Stop stops the kafka events consumer.
func (p *protocol) Stop(*manager.Manager) {
	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}
	close(p.kernelTelemetryStopChannel)
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
