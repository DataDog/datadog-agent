// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"io"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// InFlightMap is the name of the in-flight map.
	InFlightMap             = "postgres_in_flight"
	scratchBufferMap        = "postgres_scratch_buffer"
	processTailCall         = "socket__postgres_process"
	parseMessageTailCall    = "socket__postgres_process_parse_message"
	tlsProcessTailCall      = "uprobe__postgres_tls_process"
	tlsParseMessageTailCall = "uprobe__postgres_tls_process_parse_message"
	tlsTerminationTailCall  = "uprobe__postgres_tls_termination"
	eventStream             = "postgres"
)

// protocol holds the state of the postgres protocol monitoring.
type protocol struct {
	cfg            *config.Config
	telemetry      *Telemetry
	eventsConsumer *events.Consumer[EbpfEvent]
	mapCleaner     *ddebpf.MapCleaner[netebpf.ConnTuple, EbpfTx]
	statskeeper    *StatKeeper
}

// Spec is the protocol spec for the postgres protocol.
var Spec = &protocols.ProtocolSpec{
	Factory: newPostgresProtocol,
	Maps: []*manager.Map{
		{
			Name: InFlightMap,
		},
		{
			Name: scratchBufferMap,
		},
		{
			Name: "postgres_batch_events",
		},
		{
			Name: "postgres_batch_state",
		},
		{
			Name: "postgres_batches",
		},
	},
	TailCalls: []manager.TailCallRoute{
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgres),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: processTailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgresParseMessage),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: parseMessageTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgres),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsProcessTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgresParseMessage),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsParseMessageTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgresTermination),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsTerminationTailCall,
			},
		},
	},
}

func newPostgresProtocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnablePostgresMonitoring {
		return nil, nil
	}

	return &protocol{
		cfg:         cfg,
		telemetry:   NewTelemetry(),
		statskeeper: NewStatkeeper(cfg),
	}, nil
}

// Name returns the name of the protocol.
func (p *protocol) Name() string {
	return "postgres"
}

// ConfigureOptions add the necessary options for the postgres monitoring to work, to be used by the manager.
func (p *protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[InFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	utils.EnableOption(opts, "postgres_monitoring_enabled")
	// Configure event stream
	events.Configure(p.cfg, eventStream, mgr, opts)
}

// PreStart runs setup required before starting the protocol.
func (p *protocol) PreStart(mgr *manager.Manager) (err error) {
	p.eventsConsumer, err = events.NewConsumer(
		eventStream,
		mgr,
		p.processPostgres,
	)
	if err != nil {
		return
	}

	p.eventsConsumer.Start()

	return
}

// PostStart starts the map cleaner.
func (p *protocol) PostStart(mgr *manager.Manager) error {
	// Setup map cleaner after manager start.
	p.setupMapCleaner(mgr)
	return nil
}

// Stop stops all resources associated with the protocol.
func (p *protocol) Stop(*manager.Manager) {
	// mapCleaner handles nil pointer receivers
	p.mapCleaner.Stop()

	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}
}

// DumpMaps dumps map contents for debugging.
func (p *protocol) DumpMaps(w io.Writer, mapName string, currentMap *ebpf.Map) {
	if mapName == InFlightMap { // maps/postgres_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value EbpfTx
		var key netebpf.ConnTuple
		var value EbpfTx
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	}
}

// GetStats returns a map of Postgres stats.
func (p *protocol) GetStats() *protocols.ProtocolStats {
	p.eventsConsumer.Sync()

	return &protocols.ProtocolStats{
		Type:  protocols.Postgres,
		Stats: p.statskeeper.GetAndResetAllStats(),
	}
}

// IsBuildModeSupported returns always true, as postgres module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

func (p *protocol) processPostgres(events []EbpfEvent) {
	for i := range events {
		tx := &events[i]
		eventWrapper := NewEventWrapper(tx)
		p.statskeeper.Process(eventWrapper)
		p.telemetry.Count(&tx.Tx, eventWrapper)
	}
}

func (p *protocol) setupMapCleaner(mgr *manager.Manager) {
	postgresInflight, _, err := mgr.GetMap(InFlightMap)
	if err != nil {
		log.Errorf("error getting %s map: %s", InFlightMap, err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner[netebpf.ConnTuple, EbpfTx](postgresInflight, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	// Clean up idle connections. We currently use the same TTL as HTTP, but we plan to rename this variable to be more generic.
	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, key netebpf.ConnTuple, val EbpfTx) bool {
		if updated := int64(val.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Request_started)
		return started > 0 && (now-started) > ttl
	})

	p.mapCleaner = mapCleaner
}
