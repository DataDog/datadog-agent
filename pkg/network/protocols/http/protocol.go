// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type protocol struct {
	cfg            *config.Config
	telemetry      *Telemetry
	statkeeper     *StatKeeper
	mapCleaner     *ddebpf.MapCleaner
	eventsConsumer *events.Consumer
}

const (
	inFlightMap    = "http_in_flight"
	filterTailCall = "socket__http_filter"
	eventStream    = "http"
)

var Spec = protocols.ProtocolSpec{
	Factory: newHttpProtocol,
	Maps: []*manager.Map{
		{Name: inFlightMap},
	},
	TailCalls: []manager.TailCallRoute{
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(ProtocolHTTP),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: filterTailCall,
			},
		},
	},
}

func newHttpProtocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnableHTTPMonitoring {
		return nil, nil
	}

	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("couldn't determine current kernel version: %w", err)
	}

	if kversion < MinimumKernelVersion {
		return nil, fmt.Errorf("http feature not available on pre %s kernels", MinimumKernelVersion.String())
	}

	telemetry := NewTelemetry()

	return &protocol{
		cfg:       cfg,
		telemetry: telemetry,
	}, nil
}

// ConfigureOptions add the necessary options for the http monitoring to work,
// to be used by the manager. These are:
// - Set the `http_in_flight` map size to the value of the `max_tracked_connection` configuration variable.
//
// We also configure the http event stream with the manager and its options.
func (p *protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[inFlightMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: p.cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}

	// Configure event stream
	events.Configure(eventStream, mgr, opts)
}

func (p *protocol) PreStart(mgr *manager.Manager) (err error) {
	p.eventsConsumer, err = events.NewConsumer(
		"http",
		mgr,
		p.processHTTP,
	)
	if err != nil {
		return
	}

	p.statkeeper = NewStatkeeper(p.cfg, p.telemetry)
	p.eventsConsumer.Start()

	return
}

func (p *protocol) PostStart(mgr *manager.Manager) error {
	// Setup map cleaner after manager start.
	p.setupMapCleaner(mgr)

	return nil
}

func (p *protocol) Stop(_ *manager.Manager) {
	// mapCleaner handles nil pointer receivers
	p.mapCleaner.Stop()

	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}

	if p.statkeeper != nil {
		p.statkeeper.Close()
	}
}

func (p *protocol) DumpMaps(output *strings.Builder, mapName string, currentMap *ebpf.Map) {
	if mapName == inFlightMap { // maps/http_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value httpTX
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'httpTX'\n")
		iter := currentMap.Iterate()
		var key netebpf.ConnTuple
		var value Transaction
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}
	}
}

func (p *protocol) processHTTP(data []byte) {
	tx := (*EbpfTx)(unsafe.Pointer(&data[0]))
	p.telemetry.Count(tx)
	p.statkeeper.Process(tx)
}

func (p *protocol) setupMapCleaner(mgr *manager.Manager) {
	httpMap, _, err := mgr.GetMap(inFlightMap)
	if err != nil {
		log.Errorf("error getting http_in_flight map: %s", err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner(httpMap, new(netebpf.ConnTuple), new(EbpfTx))
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, func(now int64, key, val interface{}) bool {
		httpTxn, ok := val.(*EbpfTx)
		if !ok {
			return false
		}

		if updated := int64(httpTxn.ResponseLastSeen()); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(httpTxn.RequestStarted())
		return started > 0 && (now-started) > ttl
	})

	p.mapCleaner = mapCleaner
}

// GetStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (p *protocol) GetStats() *protocols.ProtocolStats {
	p.eventsConsumer.Sync()
	p.telemetry.Log()
	return &protocols.ProtocolStats{
		Type:  protocols.ProtocolHTTP,
		Stats: p.statkeeper.GetAndResetAllStats(),
	}
}
