// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"strings"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

type protocol struct {
	cfg *config.Config
	// TODO: Do we need to duplicate?
	telemetry *http.Telemetry
	// TODO: Do we need to duplicate?
	statkeeper     *http.StatKeeper
	eventsConsumer *events.Consumer

	// kernelTelemetry is used to retrieve metrics from the kernel
	http2Telemetry *kernelTelemetry
}

const (
	inFlightMap               = "http2_in_flight"
	dynamicTable              = "http2_dynamic_table"
	dynamicTableCounter       = "http2_dynamic_counter_table"
	http2IterationsTable      = "http2_iterations"
	staticTable               = "http2_static_table"
	firstFrameHandlerTailCall = "socket__http2_handle_first_frame"
	filterTailCall            = "socket__http2_filter"
	parserTailCall            = "socket__http2_frames_parser"
	eventStream               = "http2"
)

var Spec = &protocols.ProtocolSpec{
	Factory: newHttpProtocol,
	Maps: []*manager.Map{
		{
			Name: inFlightMap,
		},
		{
			Name: dynamicTable,
		},
		{
			Name: staticTable,
		},
		{
			Name: dynamicTableCounter,
		},
		{
			Name: http2IterationsTable,
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
			Key:           uint32(protocols.ProgramHTTP2FrameParser),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: parserTailCall,
			},
		},
	},
}

func newHttpProtocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnableHTTP2Monitoring {
		return nil, nil
	}

	if !Supported() {
		return nil, fmt.Errorf("http2 feature not available on pre %s kernels", MinimumKernelVersion.String())
	}

	telemetry := http.NewTelemetry("http2")
	http2KernelTelemetry := newHTTP2KernelTelemetry("http2")

	return &protocol{
		cfg:            cfg,
		telemetry:      telemetry,
		http2Telemetry: http2KernelTelemetry,
	}, nil
}

func (p *protocol) Name() string {
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
func (p *protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[inFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}

	opts.MapSpecEditors[dynamicTable] = manager.MapSpecEditor{
		MaxEntries: mapSizeValue,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[dynamicTableCounter] = manager.MapSpecEditor{
		MaxEntries: mapSizeValue,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[http2IterationsTable] = manager.MapSpecEditor{
		MaxEntries: mapSizeValue,
		EditorFlag: manager.EditMaxEntries,
	}

	utils.EnableOption(opts, "http2_monitoring_enabled")
	// Configure event stream
	events.Configure(eventStream, mgr, opts)
}

func (p *protocol) PreStart(mgr *manager.Manager) (err error) {
	p.eventsConsumer, err = events.NewConsumer(
		eventStream,
		mgr,
		p.processHTTP2,
	)
	if err != nil {
		return
	}

	p.statkeeper = http.NewStatkeeper(p.cfg, p.telemetry)
	p.eventsConsumer.Start()

	if err = p.createStaticTable(mgr); err != nil {
		return fmt.Errorf("error creating a static table for http2 monitoring: %w", err)
	}

	return
}

func (p *protocol) PostStart(mgr *manager.Manager) error {
	go func() {
		p.UpdateKernelTelemetry(mgr)
	}()
	return nil
}

func (p *protocol) Stop(_ *manager.Manager) {
	if p.eventsConsumer != nil {
		p.eventsConsumer.Stop()
	}

	if p.statkeeper != nil {
		p.statkeeper.Close()
	}
}

func (p *protocol) DumpMaps(output *strings.Builder, mapName string, currentMap *ebpf.Map) {
	if mapName == inFlightMap { // maps/http2_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value httpTX
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'httpTX'\n")
		iter := currentMap.Iterate()
		var key http2StreamKey
		var value EbpfTx
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}
	} else if mapName == dynamicTable {
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'httpTX'\n")
		iter := currentMap.Iterate()
		var key http2DynamicTableIndex
		var value http2DynamicTableEntry
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}
	}
}

func (p *protocol) processHTTP2(data []byte) {
	tx := (*EbpfTx)(unsafe.Pointer(&data[0]))
	p.telemetry.Count(tx)
	p.statkeeper.Process(tx)
}

// GetStats returns a map of HTTP2 stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (p *protocol) GetStats() *protocols.ProtocolStats {
	p.eventsConsumer.Sync()
	p.telemetry.Log()
	return &protocols.ProtocolStats{
		Type:  protocols.HTTP2,
		Stats: p.statkeeper.GetAndResetAllStats(),
	}
}

// UpdateKernelTelemetry should be moved to the HTTP/2 part as well
func (p *protocol) UpdateKernelTelemetry(mgr *manager.Manager) {
	var zero uint64

	for {
		mp, _, err := mgr.GetMap(probes.HTTP2TelemetryMap)
		if err != nil {
			log.Warnf("error retrieving http2 telemetry map: %s", err)
			return
		}

		http2Telemetry := &HTTP2Telemetry{}
		if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
			// This can happen if we haven't initialized the telemetry object yet
			// so let's just use a trace log
			if log.ShouldLog(seelog.TraceLvl) {
				log.Tracef("error retrieving the http2 telemetry struct: %s", err)
			}
			return
		}

		p.http2Telemetry.http2requests.Set(int64(http2Telemetry.Request_seen))
		p.http2Telemetry.http2responses.Set(int64(http2Telemetry.Response_seen))
		p.http2Telemetry.endOfStreamEOS.Set(int64(http2Telemetry.End_of_stream_eos))
		p.http2Telemetry.endOfStreamRST.Set(int64(http2Telemetry.End_of_stream_rst))
		p.http2Telemetry.strLenExceedsFrame.Set(int64(http2Telemetry.Str_len_exceeds_frame))
		p.http2Telemetry.frameRemainder.Set(int64(http2Telemetry.Frame_remainder))
		p.http2Telemetry.pathSizeBucket0.Set(int64(http2Telemetry.Path_size_bucket0))
		p.http2Telemetry.pathSizeBucket1.Set(int64(http2Telemetry.Path_size_bucket1))
		p.http2Telemetry.pathSizeBucket2.Set(int64(http2Telemetry.Path_size_bucket2))
		p.http2Telemetry.pathSizeBucket3.Set(int64(http2Telemetry.Path_size_bucket3))
		p.http2Telemetry.pathSizeBucket4.Set(int64(http2Telemetry.Path_size_bucket4))
		p.http2Telemetry.pathSizeBucket5.Set(int64(http2Telemetry.Path_size_bucket5))
		p.http2Telemetry.pathSizeBucket6.Set(int64(http2Telemetry.Path_size_bucket6))
		p.http2Telemetry.maxFramesIteration.Set(int64(http2Telemetry.Max_frames_iteration))

		time.Sleep(10 * time.Second)
		p.http2Telemetry.Log()
	}
}

// The staticTableEntry represents an entry in the static table that contains an index in the table and a value.
// The value itself contains both the key and the corresponding value in the static table.
// For instance, index 2 in the static table has a value of method: GET, and index 3 has a value of method: POST.
// It is not possible to save the index by the key because we need to distinguish between the values attached to the key.
type staticTableEntry struct {
	Index uint64
	Value StaticTableEnumValue
}

var (
	staticTableEntries = []staticTableEntry{
		{
			Index: 2,
			Value: GetValue,
		},
		{
			Index: 3,
			Value: PostValue,
		},
		{
			Index: 4,
			Value: EmptyPathValue,
		},
		{
			Index: 5,
			Value: IndexPathValue,
		},
		{
			Index: 8,
			Value: K200Value,
		},
		{
			Index: 9,
			Value: K204Value,
		},
		{
			Index: 10,
			Value: K206Value,
		},
		{
			Index: 11,
			Value: K304Value,
		},
		{
			Index: 12,
			Value: K400Value,
		},
		{
			Index: 13,
			Value: K404Value,
		},
		{
			Index: 14,
			Value: K500Value,
		},
	}
)

// createStaticTable creates a static table for http2 monitor.
func (p *protocol) createStaticTable(mgr *manager.Manager) error {
	staticTable, _, _ := mgr.GetMap(probes.StaticTableMap)
	if staticTable == nil {
		return errors.New("http2 static table is null")
	}

	for _, entry := range staticTableEntries {
		err := staticTable.Put(unsafe.Pointer(&entry.Index), unsafe.Pointer(&entry.Value))

		if err != nil {
			return err
		}
	}
	return nil
}

// IsBuildModeSupported returns always true, as http2 module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}
