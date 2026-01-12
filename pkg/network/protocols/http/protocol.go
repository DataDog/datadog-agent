// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"fmt"
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
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type protocol struct {
	cfg               *config.Config
	telemetry         *Telemetry
	statkeeper        *StatKeeper
	mapCleaner        *ddebpf.MapCleaner[netebpf.ConnTuple, EbpfTx]
	consumer          *events.KernelAdaptiveConsumer[EbpfEvent]
	mgr               *manager.Manager
	useDirectConsumer bool
}

const (
	inFlightMap            = "http_in_flight"
	filterTailCall         = "socket__http_filter"
	tlsProcessTailCall     = "uprobe__http_process"
	tlsTerminationTailCall = "uprobe__http_termination"
	eventStream            = "http"
	netifProbe             = "tracepoint__net__netif_receive_skb_http"
	netifProbe414          = "netif_receive_skb_core_http_4_14"
)

// Spec is the protocol spec for the HTTP protocol.
var Spec = &protocols.ProtocolSpec{
	Factory: newHTTPProtocol,
	Maps: []*manager.Map{
		{
			Name: inFlightMap,
		},
		{
			Name: "http_scratch_buffer",
		},
		{
			Name: "http_batch_events",
		},
		{
			Name: "http_batch_state",
		},
		{
			Name: "http_batches",
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
			Key:           uint32(protocols.ProgramHTTP),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: filterTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTP),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsProcessTailCall,
			},
		},
		{
			ProgArrayName: protocols.TLSDispatcherProgramsMap,
			Key:           uint32(protocols.ProgramHTTPTermination),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tlsTerminationTailCall,
			},
		},
	},
}

// newHTTPProtocol returns a new HTTP protocol.
func newHTTPProtocol(mgr *manager.Manager, cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnableHTTPMonitoring {
		return nil, nil
	}

	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("couldn't determine current kernel version: %w", err)
	}

	if kversion < usmconfig.MinimumKernelVersion {
		return nil, fmt.Errorf("http feature not available on pre %s kernels", usmconfig.MinimumKernelVersion.String())
	}

	telemetry := NewTelemetry("http")

	p := &protocol{
		cfg:       cfg,
		telemetry: telemetry,
		mgr:       mgr,
	}

	// Create adaptive consumer that determines kernel version and callback internally
	if err := p.createAdaptiveConsumer(); err != nil {
		return nil, err
	}

	return p, nil
}

// Modifiers implements the ModifierProvider interface
func (p *protocol) Modifiers() []ddebpf.Modifier {
	if p.consumer == nil {
		return nil
	}
	return p.consumer.Modifiers()
}

// Name return the program's name.
func (p *protocol) Name() string {
	return "HTTP"
}

// ConfigureOptions add the necessary options for the http monitoring to work,
// to be used by the manager. These are:
// - Set the `http_in_flight` map size to the value of the `max_tracked_connection` configuration variable.
//
// We also configure the http event stream with the manager and its options.
func (p *protocol) ConfigureOptions(opts *manager.Options) {
	opts.MapSpecEditors[inFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}

	// Only activate tracepoint when using BatchConsumer
	// DirectConsumer doesn't need the flush tracepoint since it uses direct event output
	if !p.useDirectConsumer {
		netifProbeID := manager.ProbeIdentificationPair{
			EBPFFuncName: netifProbe,
			UID:          eventStream,
		}
		if usmconfig.ShouldUseNetifReceiveSKBCoreKprobe() {
			netifProbeID.EBPFFuncName = netifProbe414
		}
		opts.ActivatedProbes = append(opts.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: netifProbeID})
	} else {
		// When using DirectConsumer, exclude flush probes to avoid loading/verifying them
		opts.ExcludedFunctions = append(opts.ExcludedFunctions, netifProbe, netifProbe414)
	}

	utils.EnableOption(opts, "http_monitoring_enabled")
	utils.AddBoolConst(opts, p.useDirectConsumer, "use_direct_consumer")

	// Configure event stream
	events.Configure(p.cfg, eventStream, p.mgr, opts)
}

func (p *protocol) PreStart() (err error) {
	// If using BatchConsumer, create it now (after manager initialization)
	if !p.useDirectConsumer {
		batchConsumer, err := events.NewBatchConsumer("http", p.mgr, p.processHTTP)
		if err != nil {
			return err
		}
		p.consumer = events.NewKernelAdaptiveConsumer[EbpfEvent](
			batchConsumer,
			[]ddebpf.Modifier{}, // BatchConsumer needs no modifiers
		)
	}

	p.statkeeper = NewStatkeeper(p.cfg, p.telemetry, NewIncompleteBuffer(p.cfg, p.telemetry))

	// Start the consumer (works for both DirectConsumer and BatchConsumer)
	p.consumer.Start()

	return
}

func (p *protocol) PostStart() error {
	// Setup map cleaner after manager start.
	p.setupMapCleaner(p.mgr)

	return nil
}

func (p *protocol) Stop() {
	// mapCleaner handles nil pointer receivers
	p.mapCleaner.Stop()

	if p.consumer != nil {
		p.consumer.Stop()
	}

	if p.statkeeper != nil {
		p.statkeeper.Close()
	}
}

func (p *protocol) DumpMaps(w io.Writer, mapName string, currentMap *ebpf.Map) {
	if mapName == inFlightMap { // maps/http_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value httpTX
		var key netebpf.ConnTuple
		var value EbpfTx
		protocols.WriteMapDumpHeader(w, currentMap, mapName, key, value)
		iter := currentMap.Iterate()
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	}
}

func (p *protocol) processHTTP(events []EbpfEvent) {
	for i := range events {
		tx := &events[i]
		p.telemetry.Count(tx)
		p.statkeeper.Process(tx)
	}
}

func (p *protocol) processHTTPDirect(event *EbpfEvent) {
	p.telemetry.Count(event)
	p.statkeeper.Process(event)
}

func (p *protocol) setupMapCleaner(mgr *manager.Manager) {
	httpMap, _, err := mgr.GetMap(inFlightMap)
	if err != nil {
		log.Errorf("error getting http_in_flight map: %s", err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner[netebpf.ConnTuple, EbpfTx](httpMap, protocols.DefaultMapCleanerBatchSize, inFlightMap, "usm_monitor")
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	ttl := p.cfg.HTTPIdleConnectionTTL.Nanoseconds()
	mapCleaner.Start(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, _ netebpf.ConnTuple, val EbpfTx) bool {
		if updated := int64(val.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Request_started)
		return started > 0 && (now-started) > ttl
	})

	p.mapCleaner = mapCleaner
}

// GetStats returns a map of HTTP stats and a callback to clean resources.
// The format of HTTP stats:
// [source, dest tuple, request path] -> RequestStats object
func (p *protocol) GetStats() (*protocols.ProtocolStats, func()) {
	p.consumer.Sync()
	p.telemetry.Log()
	stats := p.statkeeper.GetAndResetAllStats()
	return &protocols.ProtocolStats{
			Type:  protocols.HTTP,
			Stats: stats,
		}, func() {
			for _, elem := range stats {
				elem.Close()
				requestStatsPool.Put(elem)
			}
		}
}

// IsBuildModeSupported returns always true, as http module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

// createAdaptiveConsumer creates the appropriate consumer based on configuration and kernel version
// and determines which callback method to use internally
func (p *protocol) createAdaptiveConsumer() error {
	// Check if direct consumer is explicitly requested via configuration
	if p.cfg.HTTPUseDirectConsumer {
		if events.SupportsDirectConsumer() {
			// Use DirectConsumer for kernel ≥5.8 (supports bpf_perf_event_output in socket filters)
			directConsumer, err := events.NewDirectConsumer("http", p.processHTTPDirect, p.cfg)
			if err != nil {
				return err
			}
			p.consumer = events.NewKernelAdaptiveConsumer[EbpfEvent](
				directConsumer,
				[]ddebpf.Modifier{&directConsumer.EventHandler},
			)
			p.useDirectConsumer = true
			log.Debugf("HTTP monitoring: using direct consumer (requested via configuration)")
		} else {
			// Fall back to BatchConsumer on unsupported kernels
			kernelVersion, err := kernel.HostVersion()
			if err != nil {
				log.Warnf("HTTP monitoring: direct consumer requested but unable to determine kernel version (%v), falling back to batch consumer", err)
			} else {
				log.Warnf("HTTP monitoring: direct consumer requested but kernel version %v < 5.8.0, falling back to batch consumer", kernelVersion)
			}
			p.useDirectConsumer = false
		}
	} else {
		// Default behavior: use BatchConsumer regardless of kernel version
		log.Debugf("HTTP monitoring: using batch consumer (default behavior)")
		p.useDirectConsumer = false
	}

	return nil
}
