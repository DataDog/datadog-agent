// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"fmt"
	"io"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Protocol implements the interface that represents a protocol supported by USM for HTTP/2.
type Protocol struct {
	cfg *config.Config
	mgr *manager.Manager
	// TODO: Do we need to duplicate?
	telemetry *http.Telemetry
	// TODO: Do we need to duplicate?
	statkeeper              *http.StatKeeper
	http2InFlightMapCleaner *ddebpf.MapCleaner[http2StreamKey, EbpfTx]
	eventsConsumer          *events.Consumer[EbpfTx]

	// http2Telemetry is used to retrieve metrics from the kernel
	http2Telemetry             *kernelTelemetry
	kernelTelemetryStopChannel chan struct{}

	dynamicTable *DynamicTable
}

const (
	inFlightMap                 = "http2_in_flight"
	dynamicTable                = "http2_dynamic_table"
	interestingDynamicTableSet  = "http2_interesting_dynamic_table_set"
	dynamicTableCounter         = "http2_dynamic_counter_table"
	http2IterationsTable        = "http2_iterations"
	tlsHTTP2IterationsTable     = "tls_http2_iterations"
	firstFrameHandlerTailCall   = "socket__http2_handle_first_frame"
	filterTailCall              = "socket__http2_filter"
	headersParserTailCall       = "socket__http2_headers_parser"
	eosParserTailCall           = "socket__http2_eos_parser"
	eventStream                 = "http2"
	telemetryMap                = "http2_telemetry"
	http2DynamicTablePerfBuffer = "http2_dynamic_table_perf_buffer"

	tlsFirstFrameTailCall    = "uprobe__http2_tls_handle_first_frame"
	tlsFilterTailCall        = "uprobe__http2_tls_filter"
	tlsHeadersParserTailCall = "uprobe__http2_tls_headers_parser"
	tlsEOSParserTailCall     = "uprobe__http2_tls_eos_parser"
	tlsTerminationTailCall   = "uprobe__http2_tls_termination"
)

// Spec is the protocol spec for HTTP/2.
var Spec = &protocols.ProtocolSpec{
	Factory: newHTTP2Protocol,
	Maps: []*manager.Map{
		{
			Name: inFlightMap,
		},
		{
			Name: dynamicTable,
		},
		{
			Name: interestingDynamicTableSet,
		},
		{
			Name: dynamicTableCounter,
		},
		{
			Name: http2IterationsTable,
		},
		{
			Name: tlsHTTP2IterationsTable,
		},
		{
			Name: "http2_headers_to_process",
		},
		{
			Name: "http2_frames_to_process",
		},
		{
			Name: "http2_stream_heap",
		},
		{
			Name: "http2_stream_key_heap",
		},
	},
	TailCalls: []manager.TailCallRoute{
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2HandleFirstFrame),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: firstFrameHandlerTailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2FrameFilter),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: filterTailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2HeadersParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: headersParserTailCall,
			},
		},
		{
			ProgArrayName: protocols.ProtocolDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2EOSParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: eosParserTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramTLSHTTP2FirstFrame),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFirstFrameTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramTLSHTTP2Filter),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFilterTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramTLSHTTP2HeaderParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsHeadersParserTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramTLSHTTP2EOSParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsEOSParserTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramTLSHTTP2Termination),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsTerminationTailCall,
			},
		},
	},
}

func newHTTP2Protocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnableHTTP2Monitoring {
		return nil, nil
	}

	if !Supported() {
		return nil, fmt.Errorf("http2 feature not available on pre %s kernels", MinimumKernelVersion.String())
	}

	telemetry := http.NewTelemetry("http2")
	http2KernelTelemetry := newHTTP2KernelTelemetry()

	return &Protocol{
		cfg:                        cfg,
		telemetry:                  telemetry,
		http2Telemetry:             http2KernelTelemetry,
		kernelTelemetryStopChannel: make(chan struct{}),
		dynamicTable:               NewDynamicTable(int(cfg.MaxUSMConcurrentRequests)),
	}, nil
}

// Name returns the protocol name.
func (p *Protocol) Name() string {
	return "HTTP2"
}

const (
	mapSizeValue        = 1024
	dynamicMapSizeValue = 10240
)

// ConfigureOptions add the necessary options for http2 monitoring to work,
// to be used by the manager. These are:
// - Set the `http2_in_flight` map size to the value of the `max_tracked_connection` configuration variable.
//
// We also configure the http2 event stream with the manager and its options.
func (p *Protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[inFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}

	opts.MapSpecEditors[dynamicTable] = manager.MapSpecEditor{
		MaxEntries: dynamicMapSizeValue,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[interestingDynamicTableSet] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[dynamicTableCounter] = manager.MapSpecEditor{
		MaxEntries: dynamicMapSizeValue,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[http2IterationsTable] = manager.MapSpecEditor{
		MaxEntries: mapSizeValue,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[tlsHTTP2IterationsTable] = manager.MapSpecEditor{
		MaxEntries: mapSizeValue,
		EditorFlag: manager.EditMaxEntries,
	}

	utils.EnableOption(opts, "http2_monitoring_enabled")
	// Configure event stream
	events.Configure(eventStream, mgr, opts)
	p.dynamicTable.configureOptions(mgr, opts)
}

// PreStart is called before the start of the provided eBPF manager.
// Additional initialisation steps, such as starting an event consumer,
// should be performed here.
func (p *Protocol) PreStart(mgr *manager.Manager) (err error) {
	p.mgr = mgr
	p.eventsConsumer, err = events.NewConsumer(
		eventStream,
		mgr,
		p.processHTTP2,
	)
	if err != nil {
		return
	}

	if err = p.dynamicTable.preStart(mgr); err != nil {
		return
	}

	p.statkeeper = http.NewStatkeeper(p.cfg, p.telemetry, http.NewIncompleteBuffer(p.cfg, p.telemetry))
	p.eventsConsumer.Start()

	return
}

// PostStart is called after the start of the provided eBPF manager. Final
// initialisation steps, such as setting up a map cleaner, should be
// performed here.
func (p *Protocol) PostStart(mgr *manager.Manager) error {
	// Setup map cleaner after manager start.
	p.setupHTTP2InFlightMapCleaner(mgr)
	p.updateKernelTelemetry(mgr)

	return p.dynamicTable.postStart(mgr, p.cfg)
}

func (p *Protocol) updateKernelTelemetry(mgr *manager.Manager) {
	mp, _, err := mgr.GetMap(telemetryMap)
	if err != nil {
		log.Warnf("unable to get http2 telemetry map: %s", err)
		return
	}

	if mp == nil {
		log.Warn("http2 telemetry map is nil")
		return
	}
	var zero uint32
	http2Telemetry := &HTTP2Telemetry{}
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
					log.Errorf("unable to lookup http2 telemetry map: %s", err)
					return
				}

				p.http2Telemetry.update(http2Telemetry)
				p.http2Telemetry.Log()
			case <-p.kernelTelemetryStopChannel:
				return
			}
		}
	}()
}

// Stop is called before the provided eBPF manager is stopped.  Cleanup
// steps, such as stopping events consumers, should be performed here.
// Note that since this method is a cleanup method, it *should not* fail and
// tries to cleanup resources as best as it can.
func (p *Protocol) Stop(_ *manager.Manager) {
	p.dynamicTable.stop()
	// http2InFlightMapCleaner handles nil pointer receivers
	p.http2InFlightMapCleaner.Stop()

	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}

	if p.statkeeper != nil {
		p.statkeeper.Close()
	}

	close(p.kernelTelemetryStopChannel)
}

// DumpMaps dumps the content of the map represented by mapName &
// currentMap, if it used by the eBPF program, to output.
func (p *Protocol) DumpMaps(w io.Writer, mapName string, currentMap *ebpf.Map) {
	if mapName == inFlightMap { // maps/http2_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value httpTX
		io.WriteString(w, "Map: '"+mapName+"', key: 'ConnTuple', value: 'httpTX'\n")
		iter := currentMap.Iterate()
		var key http2StreamKey
		var value EbpfTx
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	} else if mapName == dynamicTable {
		io.WriteString(w, "Map: '"+mapName+"', key: 'ConnTuple', value: 'httpTX'\n")
		iter := currentMap.Iterate()
		var key http2DynamicTableIndex
		var value http2DynamicTableEntry
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	}
}

func (p *Protocol) processHTTP2(events []EbpfTx) {
	for i := range events {
		tx := &events[i]
		p.telemetry.Count(tx)
		p.statkeeper.Process(tx)
	}
}

func (p *Protocol) setupHTTP2InFlightMapCleaner(mgr *manager.Manager) {
	http2Map, _, err := mgr.GetMap(inFlightMap)
	if err != nil {
		log.Errorf("error getting %q map: %s", inFlightMap, err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner[http2StreamKey, EbpfTx](http2Map, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, key http2StreamKey, val EbpfTx) bool {
		if updated := int64(val.Stream.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Stream.Request_started)
		return started > 0 && (now-started) > ttl
	})

	p.http2InFlightMapCleaner = mapCleaner
}

// GetStats returns a map of HTTP2 stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (p *Protocol) GetStats() *protocols.ProtocolStats {
	p.eventsConsumer.Sync()
	p.telemetry.Log()
	return &protocols.ProtocolStats{
		Type:  protocols.HTTP2,
		Stats: p.statkeeper.GetAndResetAllStats(),
	}
}

// IsBuildModeSupported returns always true, as http2 module is supported by all modes.
func (*Protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

// GetHTTP2KernelTelemetry returns the HTTP2 kernel telemetry
func (p *Protocol) GetHTTP2KernelTelemetry() (*HTTP2Telemetry, error) {
	http2Telemetry := &HTTP2Telemetry{}
	var zero uint32

	mp, _, err := p.mgr.GetMap(telemetryMap)
	if err != nil {
		log.Errorf("unable to get http2 telemetry map: %s", err)
		return nil, err
	}

	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
		log.Errorf("unable to lookup http2 telemetry map: %s", err)
		return nil, err
	}
	return http2Telemetry, nil
}
