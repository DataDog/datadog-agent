// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

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

type httpProtocol struct {
	cfg            *config.Config
	telemetry      *Telemetry
	statkeeper     *HttpStatKeeper
	mapCleaner     *ddebpf.MapCleaner
	eventsConsumer *events.Consumer
}

const (
	httpInFlightMap = "http_in_flight"
)

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
	statkeeper := NewHTTPStatkeeper(cfg, telemetry)

	return &httpProtocol{
		cfg:        cfg,
		telemetry:  telemetry,
		statkeeper: statkeeper,
	}, nil
}

func (p *httpProtocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	if p == nil {
		return
	}

	opts.MapSpecEditors[httpInFlightMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: uint32(p.cfg.MaxTrackedConnections),
		EditorFlag: manager.EditMaxEntries,
	}

	// Configure event stream
	events.Configure("http", mgr, opts)
}

func (p *httpProtocol) PreStart(mgr *manager.Manager) (err error) {
	if p == nil {
		return
	}

	p.eventsConsumer, err = events.NewConsumer(
		"http",
		mgr,
		p.processHTTP,
	)
	if err != nil {
		return
	}

	p.eventsConsumer.Start()

	return
}

func (p *httpProtocol) PostStart(mgr *manager.Manager) {
	if p == nil {
		return
	}

	p.setupMapCleaner(mgr)

	log.Info("http monitoring enabled")
}

func (p *httpProtocol) PreStop(mgr *manager.Manager) {
	if p == nil {
		return
	}

	p.mapCleaner.Stop()
	p.eventsConsumer.Stop()
	p.statkeeper.Close()
}

func (p *httpProtocol) PostStop(mgr *manager.Manager) {
}

func (p *httpProtocol) DumpMaps(output *strings.Builder, mapName string, currentMap *ebpf.Map) {
	if p == nil {
		return
	}

	switch mapName {
	case httpInFlightMap: // maps/http_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value httpTX
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'httpTX'\n")
		iter := currentMap.Iterate()
		var key netebpf.ConnTuple
		var value HttpTX
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	}
}

func (p *httpProtocol) processHTTP(data []byte) {
	if p == nil {
		return
	}

	tx := (*EbpfHttpTx)(unsafe.Pointer(&data[0]))
	p.telemetry.Count(tx)
	p.statkeeper.Process(tx)
}

func (p *httpProtocol) setupMapCleaner(mgr *manager.Manager) {
	if p == nil {
		return
	}

	httpMap, _, _ := mgr.GetMap(httpInFlightMap)
	mapCleaner, err := ddebpf.NewMapCleaner(httpMap, new(netebpf.ConnTuple), new(EbpfHttpTx))
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, func(now int64, key, val interface{}) bool {
		httpTxn, ok := val.(*EbpfHttpTx)
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
func (p *httpProtocol) GetStats() *protocols.ProtocolStats {
	if p == nil {
		return nil
	}

	p.eventsConsumer.Sync()
	p.telemetry.Log()
	return &protocols.ProtocolStats{
		Type:  protocols.HTTP,
		Stats: p.statkeeper.GetAndResetAllStats(),
	}
}

func init() {
	protocols.RegisterProtocol(protocols.HTTP, protocols.ProtocolSpec{
		Factory: newHttpProtocol,
		Maps: []*manager.Map{
			{Name: httpInFlightMap},
		},
		TailCalls: []manager.TailCallRoute{
			{
				ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
				Key:           uint32(protocols.ProgramHTTP),
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "socket__http_filter",
				},
			},
		},
	})

	log.Debug("[USM] Registered HTTP protocol factory")
}
