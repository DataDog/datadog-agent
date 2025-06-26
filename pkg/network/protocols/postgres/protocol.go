// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"io"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	postgresebpf "github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// KernelTelemetryMap is the map for getting kernel metrics
	KernelTelemetryMap = "postgres_telemetry"
	// InFlightMap is the name of the in-flight map.
	InFlightMap               = "postgres_in_flight"
	scratchBufferMap          = "postgres_scratch_buffer"
	iterationsMap             = "postgres_iterations"
	handleTailCall            = "socket__postgres_handle"
	handleResponseTailCall    = "socket__postgres_handle_response"
	parseMessageTailCall      = "socket__postgres_process_parse_message"
	tlsHandleTailCall         = "uprobe__postgres_tls_handle"
	tlsParseMessageTailCall   = "uprobe__postgres_tls_process_parse_message"
	tlsTerminationTailCall    = "uprobe__postgres_tls_termination"
	tlsHandleResponseTailCall = "uprobe__postgres_tls_handle_response"
	eventStream               = "postgres"
	netifProbe                = "tracepoint__net__netif_receive_skb_postgres"
	netifProbe414             = "netif_receive_skb_core_postgres_4_14"
)

// protocol holds the state of the postgres protocol monitoring.
type protocol struct {
	cfg                   *config.Config
	telemetry             *Telemetry
	eventsConsumer        *events.Consumer[postgresebpf.EbpfEvent]
	mapCleaner            *ddebpf.MapCleaner[netebpf.ConnTuple, postgresebpf.EbpfTx]
	statskeeper           *StatKeeper
	kernelTelemetry       *kernelTelemetry // retrieves Postgres metrics from kernel
	kernelTelemetryStopCh chan struct{}
	mgr                   *manager.Manager
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
			Name: iterationsMap,
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
	Probes: []*manager.Probe{
		{
			KprobeAttachMethod: manager.AttachKprobeWithPerfEventOpen,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: netifProbe414,
				UID:          eventStream,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: netifProbe,
				UID:          eventStream,
			},
		},
	},
	TailCalls: []manager.TailCallRoute{
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgres),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: handleTailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgresHandleResponse),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: handleResponseTailCall,
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
				EBPFFuncName: tlsHandleTailCall,
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
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramPostgresHandleResponse),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsHandleResponseTailCall,
			},
		},
	},
}

func newPostgresProtocol(mgr *manager.Manager, cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnablePostgresMonitoring {
		return nil, nil
	}

	return &protocol{
		cfg:                   cfg,
		telemetry:             NewTelemetry(cfg),
		statskeeper:           NewStatkeeper(cfg),
		kernelTelemetry:       newKernelTelemetry(),
		kernelTelemetryStopCh: make(chan struct{}),
		mgr:                   mgr,
	}, nil
}

// Name returns the name of the protocol.
func (p *protocol) Name() string {
	return "postgres"
}

// ConfigureOptions add the necessary options for the postgres monitoring to work, to be used by the manager.
func (p *protocol) ConfigureOptions(opts *manager.Options) {
	opts.MapSpecEditors[InFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	netifProbeID := manager.ProbeIdentificationPair{
		EBPFFuncName: netifProbe,
		UID:          eventStream,
	}
	if usmconfig.ShouldUseNetifReceiveSKBCoreKprobe() {
		netifProbeID.EBPFFuncName = netifProbe414
	}
	opts.ActivatedProbes = append(opts.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: netifProbeID})
	utils.EnableOption(opts, "postgres_monitoring_enabled")
	// Configure event stream
	events.Configure(p.cfg, eventStream, p.mgr, opts)
}

// PreStart runs setup required before starting the protocol.
func (p *protocol) PreStart() (err error) {
	p.eventsConsumer, err = events.NewConsumer(
		eventStream,
		p.mgr,
		p.processPostgres,
	)
	if err != nil {
		return
	}

	p.eventsConsumer.Start()

	return
}

// PostStart starts the map cleaner.
func (p *protocol) PostStart() error {
	// Setup map cleaner after manager start.
	p.setupMapCleaner()
	p.startKernelTelemetry()
	return nil
}

// Stop stops all resources associated with the protocol.
func (p *protocol) Stop() {
	// mapCleaner handles nil pointer receivers
	p.mapCleaner.Stop()

	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}
	if p.kernelTelemetryStopCh != nil {
		close(p.kernelTelemetryStopCh)
	}
}

// DumpMaps dumps map contents for debugging.
func (p *protocol) DumpMaps(w io.Writer, mapName string, currentMap *ebpf.Map) {
	switch mapName {
	case InFlightMap:
		// maps/postgres_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value EbpfTx
		var key netebpf.ConnTuple
		var value postgresebpf.EbpfTx
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	case KernelTelemetryMap:
		// postgres_msg_count (BPF_ARRAY_MAP), key 0 and 1, value PostgresKernelMsgCount
		plainKey := uint32(0)
		tlsKey := uint32(1)

		var value postgresebpf.PostgresKernelMsgCount
		protocols.WriteMapDumpHeader(w, currentMap, mapName, plainKey, value)
		if err := currentMap.Lookup(unsafe.Pointer(&plainKey), unsafe.Pointer(&value)); err == nil {
			spew.Fdump(w, plainKey, value)
		}
		protocols.WriteMapDumpHeader(w, currentMap, mapName, tlsKey, value)
		if err := currentMap.Lookup(unsafe.Pointer(&tlsKey), unsafe.Pointer(&value)); err == nil {
			spew.Fdump(w, tlsKey, value)
		}
	}
}

// GetStats returns a map of Postgres stats and a callback to clean resources.
func (p *protocol) GetStats() (*protocols.ProtocolStats, func()) {
	p.eventsConsumer.Sync()
	p.kernelTelemetry.Log()

	stats := p.statskeeper.GetAndResetAllStats()
	return &protocols.ProtocolStats{
			Type:  protocols.Postgres,
			Stats: stats,
		}, func() {
			for _, stat := range stats {
				stat.Close()
			}
		}
}

// IsBuildModeSupported returns always true, as postgres module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

func (p *protocol) processPostgres(events []postgresebpf.EbpfEvent) {
	for i := range events {
		tx := &events[i]
		eventWrapper := NewEventWrapper(tx)
		p.statskeeper.Process(eventWrapper)
		p.telemetry.Count(tx, eventWrapper)
	}
}

func (p *protocol) setupMapCleaner() {
	postgresInflight, _, err := p.mgr.GetMap(InFlightMap)
	if err != nil {
		log.Errorf("error getting %s map: %s", InFlightMap, err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner[netebpf.ConnTuple, postgresebpf.EbpfTx](postgresInflight, protocols.DefaultMapCleanerBatchSize, InFlightMap, "usm_monitor")
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	// Clean up idle connections. We currently use the same TTL as HTTP, but we plan to rename this variable to be more generic.
	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Start(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, _ netebpf.ConnTuple, val postgresebpf.EbpfTx) bool {
		if updated := int64(val.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Request_started)
		return started > 0 && (now-started) > ttl
	})

	p.mapCleaner = mapCleaner
}

func (p *protocol) startKernelTelemetry() {
	telemetryMap, err := protocols.GetMap(p.mgr, KernelTelemetryMap)
	if err != nil {
		log.Errorf("couldnt find kernel telemetry map: %s, error: %v", telemetryMap, err)
		return
	}

	plainKey := uint32(0)
	tlsKey := uint32(1)
	pgKernelMsgCount := &postgresebpf.PostgresKernelMsgCount{}
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := telemetryMap.Lookup(unsafe.Pointer(&plainKey), unsafe.Pointer(pgKernelMsgCount)); err != nil {
					log.Errorf("unable to lookup %q map: %s", KernelTelemetryMap, err)
					return
				}
				p.kernelTelemetry.update(pgKernelMsgCount, false)

				if err := telemetryMap.Lookup(unsafe.Pointer(&tlsKey), unsafe.Pointer(pgKernelMsgCount)); err != nil {
					log.Errorf("unable to lookup %q map: %s", KernelTelemetryMap, err)
					return
				}
				p.kernelTelemetry.update(pgKernelMsgCount, true)

			case <-p.kernelTelemetryStopCh:
				return
			}
		}
	}()
}
