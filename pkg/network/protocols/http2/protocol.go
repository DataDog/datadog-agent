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
	cfg                     *config.Config
	mgr                     *manager.Manager
	telemetry               *http.Telemetry
	statkeeper              *http.StatKeeper
	http2InFlightMapCleaner *ddebpf.MapCleaner[HTTP2StreamKey, HTTP2Stream]
	eventsConsumer          *events.Consumer[EbpfTx]

	// http2Telemetry is used to retrieve metrics from the kernel
	http2Telemetry             *kernelTelemetry
	kernelTelemetryStopChannel chan struct{}

	dynamicTable *DynamicTable
}

const (
	// InFlightMap is the name of the map used to store in-flight HTTP/2 streams
	InFlightMap               = "http2_in_flight"
	remainderTable            = "http2_remainder"
	dynamicTable              = "http2_dynamic_table"
	dynamicTableCounter       = "http2_dynamic_counter_table"
	http2IterationsTable      = "http2_iterations"
	tlsHTTP2IterationsTable   = "tls_http2_iterations"
	firstFrameHandlerTailCall = "socket__http2_handle_first_frame"
	filterTailCall            = "socket__http2_filter"
	headersParserTailCall     = "socket__http2_headers_parser"
	dynamicTableCleaner       = "socket__http2_dynamic_table_cleaner"
	eosParserTailCall         = "socket__http2_eos_parser"
	eventStream               = "http2"

	// TelemetryMap is the name of the map used to retrieve plaintext metrics from the kernel
	TelemetryMap = "http2_telemetry"
	// TLSTelemetryMap is the name of the map used to retrieve metrics from the eBPF probes for TLS
	TLSTelemetryMap = "tls_http2_telemetry"

	tlsFirstFrameTailCall    = "uprobe__http2_tls_handle_first_frame"
	tlsFilterTailCall        = "uprobe__http2_tls_filter"
	tlsHeadersParserTailCall = "uprobe__http2_tls_headers_parser"
	tlsDynamicTableCleaner   = "uprobe__http2_dynamic_table_cleaner"
	tlsEOSParserTailCall     = "uprobe__http2_tls_eos_parser"
	tlsTerminationTailCall   = "uprobe__http2_tls_termination"
)

// Spec is the protocol spec for HTTP/2.
var Spec = &protocols.ProtocolSpec{
	Factory: newHTTP2Protocol,
	Maps: []*manager.Map{
		{
			Name: InFlightMap,
		},
		{
			Name: dynamicTable,
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
			Name: remainderTable,
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
			Name: "http2_ctx_heap",
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
			Key:           uint32(protocols.ProgramHTTP2DynamicTableCleaner),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: dynamicTableCleaner,
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
			Key:           uint32(protocols.ProgramTLSHTTP2DynamicTableCleaner),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsDynamicTableCleaner,
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
		dynamicTable:               NewDynamicTable(cfg),
	}, nil
}

// Name returns the protocol name.
func (p *Protocol) Name() string {
	return "HTTP2"
}

const (
	mapSizeValue = 1024
)

// ConfigureOptions add the necessary options for http2 monitoring to work,
// to be used by the manager. These are:
// - Set the `http2_in_flight` map size to the value of the `max_tracked_connection` configuration variable.
//
// We also configure the http2 event stream with the manager and its options.
func (p *Protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[InFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[remainderTable] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[dynamicTable] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[dynamicTableCounter] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
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
	utils.EnableOption(opts, "terminated_http2_monitoring_enabled")
	// Configure event stream
	events.Configure(p.cfg, eventStream, mgr, opts)
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

	p.statkeeper = http.NewStatkeeper(p.cfg, p.telemetry, NewIncompleteBuffer(p.cfg))
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
	mp, err := protocols.GetMap(mgr, TelemetryMap)
	if err != nil {
		log.Warn(err)
		return
	}

	tlsMap, err := protocols.GetMap(mgr, TLSTelemetryMap)
	if err != nil {
		log.Warn(err)
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
					log.Errorf("unable to lookup %q map: %s", TelemetryMap, err)
					return
				}
				p.http2Telemetry.update(http2Telemetry, false)

				if err := tlsMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
					log.Errorf("unable to lookup %q map: %s", TLSTelemetryMap, err)
					return
				}
				p.http2Telemetry.update(http2Telemetry, true)

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
	if mapName == InFlightMap {
		var key HTTP2StreamKey
		var value HTTP2Stream
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	} else if mapName == dynamicTable {
		var key HTTP2DynamicTableIndex
		var value HTTP2DynamicTableEntry
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
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
	http2Map, _, err := mgr.GetMap(InFlightMap)
	if err != nil {
		log.Errorf("error getting %q map: %s", InFlightMap, err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner[HTTP2StreamKey, HTTP2Stream](http2Map, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Clean(p.cfg.HTTP2DynamicTableMapCleanerInterval, nil, nil, func(now int64, key HTTP2StreamKey, val HTTP2Stream) bool {
		if updated := int64(val.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Request_started)
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
