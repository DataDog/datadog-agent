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
	incompleteFramesTable     = "http2_incomplete_frames"
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
	netifProbe                = "tracepoint__net__netif_receive_skb_http2"

	// TelemetryMap is the name of the map that collects telemetry for plaintext and TLS encrypted HTTP/2 traffic.
	TelemetryMap = "http2_telemetry"

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
			Name: incompleteFramesTable,
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
		{
			Name: "http2_batch_events",
		},
		{
			Name: "http2_batch_state",
		},
		{
			Name: "http2_batches",
		},
		{
			Name: "terminated_http2_batch_events",
		},
		{
			Name: "terminated_http2_batch_state",
		},
		{
			Name: "terminated_http2_batches",
		},
	},
	Probes: []*manager.Probe{
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
			Key:           uint32(protocols.ProgramHTTP2HandleFirstFrame),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFirstFrameTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2FrameFilter),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsFilterTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2HeadersParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsHeadersParserTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2DynamicTableCleaner),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsDynamicTableCleaner,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2EOSParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsEOSParserTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP2Termination),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsTerminationTailCall,
			},
		},
	},
}

func newHTTP2Protocol(mgr *manager.Manager, cfg *config.Config) (protocols.Protocol, error) {
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
		mgr:                        mgr,
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
func (p *Protocol) ConfigureOptions(opts *manager.Options) {
	opts.MapSpecEditors[InFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[incompleteFramesTable] = manager.MapSpecEditor{
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

	opts.ActivatedProbes = append(opts.ActivatedProbes, &manager.ProbeSelector{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          eventStream,
			EBPFFuncName: netifProbe,
		},
	})
	utils.EnableOption(opts, "http2_monitoring_enabled")
	utils.EnableOption(opts, "terminated_http2_monitoring_enabled")
	// Configure event stream
	events.Configure(p.cfg, eventStream, p.mgr, opts)
	p.dynamicTable.configureOptions(p.mgr, opts)
}

// PreStart is called before the start of the provided eBPF manager.
// Additional initialisation steps, such as starting an event consumer,
// should be performed here.
func (p *Protocol) PreStart() (err error) {
	p.eventsConsumer, err = events.NewConsumer(
		eventStream,
		p.mgr,
		p.processHTTP2,
	)
	if err != nil {
		return
	}

	if err = p.dynamicTable.preStart(p.mgr); err != nil {
		return
	}

	p.statkeeper = http.NewStatkeeper(p.cfg, p.telemetry, NewIncompleteBuffer(p.cfg))
	p.eventsConsumer.Start()

	return
}

// PostStart is called after the start of the provided eBPF manager. Final
// initialisation steps, such as setting up a map cleaner, should be
// performed here.
func (p *Protocol) PostStart() error {
	// Setup map cleaner after manager start.
	p.setupHTTP2InFlightMapCleaner()
	p.updateKernelTelemetry()

	return p.dynamicTable.postStart(p.mgr, p.cfg)
}

func (p *Protocol) updateKernelTelemetry() {
	mp, err := protocols.GetMap(p.mgr, TelemetryMap)
	if err != nil {
		log.Warn(err)
		return
	}

	plaintextKey := uint32(0)
	tlsKey := uint32(1)
	http2Telemetry := &HTTP2Telemetry{}
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := mp.Lookup(unsafe.Pointer(&plaintextKey), unsafe.Pointer(http2Telemetry)); err != nil {
					log.Errorf("unable to lookup %q map: %s", TelemetryMap, err)
					return
				}
				p.http2Telemetry.update(http2Telemetry, false)

				if err := mp.Lookup(unsafe.Pointer(&tlsKey), unsafe.Pointer(http2Telemetry)); err != nil {
					log.Errorf("unable to lookup %q map: %s", TelemetryMap, err)
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
func (p *Protocol) Stop() {
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

func (p *Protocol) setupHTTP2InFlightMapCleaner() {
	http2Map, _, err := p.mgr.GetMap(InFlightMap)
	if err != nil {
		log.Errorf("error getting %q map: %s", InFlightMap, err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner[HTTP2StreamKey, HTTP2Stream](http2Map, protocols.DefaultMapCleanerBatchSize, InFlightMap, "usm_monitor")
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Start(p.cfg.HTTP2DynamicTableMapCleanerInterval, nil, nil, func(now int64, _ HTTP2StreamKey, val HTTP2Stream) bool {
		if updated := int64(val.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Request_started)
		return started > 0 && (now-started) > ttl
	})

	p.http2InFlightMapCleaner = mapCleaner
}

// GetStats returns a map of HTTP2 stats and a callback to clean resources.
// The format of HTTP2 stats:
// [source, dest tuple, request path] -> RequestStats object
func (p *Protocol) GetStats() (*protocols.ProtocolStats, func()) {
	p.eventsConsumer.Sync()
	p.telemetry.Log()
	stats := p.statkeeper.GetAndResetAllStats()
	return &protocols.ProtocolStats{
			Type:  protocols.HTTP2,
			Stats: stats,
		}, func() {
			for _, elem := range stats {
				elem.Close()
			}
		}
}

// IsBuildModeSupported returns always true, as http2 module is supported by all modes.
func (*Protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}
