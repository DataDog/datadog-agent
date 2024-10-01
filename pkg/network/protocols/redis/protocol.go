// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package redis

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
	inFlightMap            = "redis_in_flight"
	processTailCall        = "socket__redis_process"
	tlsProcessTailCall     = "uprobe__redis_tls_process"
	tlsTerminationTailCall = "uprobe__redis_tls_termination"
	eventStream            = "redis"
)

type protocol struct {
	cfg            *config.Config
	eventsConsumer *events.Consumer[EbpfEvent]
	mapCleaner     *ddebpf.MapCleaner[netebpf.ConnTuple, EbpfTx]
	statskeeper    *StatsKeeper
}

// Spec is the protocol spec for the redis protocol.
var Spec = &protocols.ProtocolSpec{
	Factory: newRedisProtocol,
	Maps: []*manager.Map{
		{Name: inFlightMap},
	},
	TailCalls: []manager.TailCallRoute{
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramRedis),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: processTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramRedis),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsProcessTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramRedisTermination),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsTerminationTailCall,
			},
		},
	},
}

// newRedisProtocol is the factory for the Redis protocol object
func newRedisProtocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnableRedisMonitoring {
		return nil, nil
	}

	return &protocol{
		cfg:         cfg,
		statskeeper: NewStatsKeeper(cfg),
	}, nil
}

// Name returns the name of the protocol.
func (p *protocol) Name() string {
	return "redis"
}

// ConfigureOptions add the necessary options for the redis monitoring
// to work, to be used by the manager.
func (p *protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[inFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	utils.EnableOption(opts, "redis_monitoring_enabled")
	events.Configure(p.cfg, eventStream, mgr, opts)
}

func (p *protocol) PreStart(mgr *manager.Manager) (err error) {
	p.eventsConsumer, err = events.NewConsumer(
		eventStream,
		mgr,
		p.processRedis,
	)

	if err != nil {
		return
	}

	p.eventsConsumer.Start()
	return
}

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
	if mapName == inFlightMap { // maps/redis_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value EbpfTx
		var key netebpf.ConnTuple
		var value EbpfTx
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	}
}

// GetStats returns a map of Redis stats.
func (p *protocol) GetStats() *protocols.ProtocolStats {
	p.eventsConsumer.Sync()

	return &protocols.ProtocolStats{
		Type:  protocols.Redis,
		Stats: p.statskeeper.GetAndResetAllStats(),
	}
}

// IsBuildModeSupported returns always true, as Redis module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

func (p *protocol) processRedis(events []EbpfEvent) {
	for i := range events {
		tx := &events[i]
		p.statskeeper.Process(tx)
	}
}

func (p *protocol) setupMapCleaner(mgr *manager.Manager) {
	redisInFlight, _, err := mgr.GetMap(inFlightMap)
	if err != nil {
		log.Errorf("error getting %s map: %s", inFlightMap, err)
		return
	}

	mapCleaner, err := ddebpf.NewMapCleaner[netebpf.ConnTuple, EbpfTx](redisInFlight, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	// Clean up idle connections. We currently use the same TTL as HTTP, but we plan to rename this variable to be more generic.
	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, _ netebpf.ConnTuple, val EbpfTx) bool {
		if updated := int64(val.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Request_started)
		return started > 0 && (now-started) > ttl
	})

	p.mapCleaner = mapCleaner
}
