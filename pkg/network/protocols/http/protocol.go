// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Simple global variables for path dumping
var (
	dumpTargetPath string
	dumpIsActive   bool
	dumpFile       *os.File
	dumpFileName   string
	dumpMutex      sync.RWMutex
)

type protocol struct {
	cfg            *config.Config
	telemetry      *Telemetry
	statkeeper     *StatKeeper
	mapCleaner     *ddebpf.MapCleaner[netebpf.ConnTuple, EbpfTx]
	eventsConsumer *events.Consumer[EbpfEvent]
	debuggerMap    *ebpf.Map
}

const (
	inFlightMap            = "http_in_flight"
	filterTailCall         = "socket__http_filter"
	tlsProcessTailCall     = "uprobe__http_process"
	tlsTerminationTailCall = "uprobe__http_termination"
	eventStream            = "http"
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
		{
			Name: "https_debug_pid",
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
func newHTTPProtocol(cfg *config.Config) (protocols.Protocol, error) {
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

	return &protocol{
		cfg:       cfg,
		telemetry: telemetry,
	}, nil
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
func (p *protocol) ConfigureOptions(mgr *manager.Manager, opts *manager.Options) {
	opts.MapSpecEditors[inFlightMap] = manager.MapSpecEditor{
		MaxEntries: p.cfg.MaxUSMConcurrentRequests,
		EditorFlag: manager.EditMaxEntries,
	}
	utils.EnableOption(opts, "http_monitoring_enabled")
	// Configure event stream
	events.Configure(p.cfg, eventStream, mgr, opts)
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

	p.statkeeper = NewStatkeeper(p.cfg, p.telemetry, NewIncompleteBuffer(p.cfg, p.telemetry))
	p.eventsConsumer.Start()

	p.debuggerMap, _, err = mgr.GetMap("https_debug_pid")
	if err != nil {
		return fmt.Errorf("error getting https_debug_pid map: %w", err)
	}
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

		// Check if path dumping is active and if this transaction matches
		p.checkAndDumpTraffic(tx)

		p.telemetry.Count(tx)
		p.statkeeper.Process(tx)
	}
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
	mapCleaner.Clean(p.cfg.HTTPMapCleanerInterval, nil, nil, func(now int64, _ netebpf.ConnTuple, val EbpfTx) bool {
		if updated := int64(val.Response_last_seen); updated > 0 {
			return (now - updated) > ttl
		}

		started := int64(val.Request_started)
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
		Type:  protocols.HTTP,
		Stats: p.statkeeper.GetAndResetAllStats(),
	}
}

// IsBuildModeSupported returns always true, as http module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

// AddPIDToDebugger adds a PID to the HTTP debugger map with the specified path and size.
func AddPIDToDebugger(pid uint32, path [24]byte, size uint8) {
	if Spec == nil || Spec.Instance == nil {
		log.Warnf("http protocol spec is nil, cannot add PID to debugger")
		return
	}

	httpProtocol, ok := Spec.Instance.(*protocol)
	if !ok {
		log.Warnf("http protocol spec is not of type *protocol, cannot add PID to debugger")
		return
	}

	if httpProtocol.debuggerMap == nil {
		log.Warnf("http debugger map is nil, cannot add PID to debugger")
		return
	}

	if size > 24 {
		log.Warnf("provided path length %d exceeds maximum of 24 bytes, cannot add PID %d to debugger", size, pid)
		return
	}
	value := &Debugger{
		Size: size,
	}
	for i := uint8(0); i < size; i++ {
		value.Pattern[i] = int8(path[i])
	}

	if err := httpProtocol.debuggerMap.Update(unsafe.Pointer(&pid), unsafe.Pointer(value), ebpf.UpdateAny); err != nil {
		log.Errorf("failed to add PID %d to http debugger map: %v", pid, err)
	}
}

// DumpTraffic enables userspace-only HTTP traffic dumping for a specific path
func DumpTraffic(path [24]byte, size uint8) {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()

	if size > 24 {
		log.Warnf("provided path length %d exceeds maximum of 24 bytes, cannot enable traffic dumping", size)
		return
	}

	// Convert byte array to string, stopping at null terminator
	pathStr := string(path[:size])
	if nullIndex := strings.IndexByte(pathStr, 0); nullIndex != -1 {
		pathStr = pathStr[:nullIndex]
	}

	// Close previous file if exists
	if dumpFile != nil {
		dumpFile.Close()
		dumpFile = nil
	}

	// Create new dump file in /tmp directory
	timestamp := time.Now().Format("20060102_150405")
	safePathStr := strings.ReplaceAll(strings.ReplaceAll(pathStr, "/", "_"), " ", "_")
	dumpFileName = filepath.Join("/tmp", fmt.Sprintf("http_traffic_dump_%s_%s.log", safePathStr, timestamp))

	var err error
	dumpFile, err = os.Create(dumpFileName)
	if err != nil {
		log.Errorf("failed to create dump file: %v", err)
		return
	}

	// Write header to file
	fmt.Fprintf(dumpFile, "HTTP Traffic Dump\n")
	fmt.Fprintf(dumpFile, "=================\n")
	fmt.Fprintf(dumpFile, "Target Pattern: %s\n", pathStr)
	fmt.Fprintf(dumpFile, "Started: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(dumpFile, "=================\n\n")
	dumpFile.Sync()

	dumpTargetPath = pathStr
	dumpIsActive = true

	log.Infof("Enabled HTTP traffic dumping for path: '%s' - writing to file: %s", pathStr, dumpFileName)
}

// StopTrafficDumping disables HTTP traffic dumping
func StopTrafficDumping() {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()

	if dumpFile != nil {
		// Write footer to file
		fmt.Fprintf(dumpFile, "\n=================\n")
		fmt.Fprintf(dumpFile, "Dump completed: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		fmt.Fprintf(dumpFile, "=================\n")
		dumpFile.Sync()
		dumpFile.Close()

		log.Infof("HTTP traffic dump saved to: %s", dumpFileName)
		dumpFile = nil
		dumpFileName = ""
	}

	dumpTargetPath = ""
	dumpIsActive = false

	log.Infof("Disabled HTTP traffic dumping")
}

// checkAndDumpTraffic checks if the current transaction matches the target path and dumps it
func (p *protocol) checkAndDumpTraffic(tx *EbpfEvent) {
	dumpMutex.RLock()
	isActive := dumpIsActive
	targetPattern := dumpTargetPath
	dumpMutex.RUnlock()

	if !isActive || targetPattern == "" {
		return
	}

	// Extract path from the transaction
	var pathBuffer [256]byte
	pathBytes, _ := tx.Path(pathBuffer[:])
	if pathBytes == nil {
		return
	}

	extractedPath := string(pathBytes)

	// Construct the full pattern: "METHOD /path"
	method := tx.Method().String()
	fullPattern := method + " " + extractedPath

	// Check if the full pattern matches the target
	if strings.Contains(fullPattern, targetPattern) {
		p.dumpHTTPTransaction(tx, extractedPath, fullPattern)
	}
}

// dumpHTTPTransaction dumps detailed information about an HTTP transaction
func (p *protocol) dumpHTTPTransaction(tx *EbpfEvent, extractedPath string, fullPattern string) {
	dumpMutex.RLock()
	file := dumpFile
	dumpMutex.RUnlock()

	if file == nil {
		return
	}

	connTuple := tx.ConnTuple()
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Write structured dump to file
	fmt.Fprintf(file, "[%s] HTTP Transaction Captured\n", timestamp)
	fmt.Fprintf(file, "─────────────────────────────────────\n")

	// Basic HTTP Information
	fmt.Fprintf(file, "Path: %s\n", extractedPath)
	fmt.Fprintf(file, "Full Pattern: %s (matched target)\n", fullPattern)
	fmt.Fprintf(file, "Method: %s\n", tx.Method().String())
	fmt.Fprintf(file, "Status Code: %d\n", tx.StatusCode())

	// Connection Information
	fmt.Fprintf(file, "Source IP: %s\n", util.FromLowHigh(connTuple.SrcIPLow, connTuple.SrcIPHigh))
	fmt.Fprintf(file, "Source Port: %d\n", connTuple.SrcPort)
	fmt.Fprintf(file, "Dest IP: %s\n", util.FromLowHigh(connTuple.DstIPLow, connTuple.DstIPHigh))
	fmt.Fprintf(file, "Dest Port: %d\n", connTuple.DstPort)

	// Process & System Information
	fmt.Fprintf(file, "PID: %d\n", tx.Tuple.Pid)
	fmt.Fprintf(file, "Network Namespace: %d\n", tx.Tuple.Netns)
	fmt.Fprintf(file, "Connection Metadata: 0x%x\n", tx.Tuple.Metadata)

	// Timing Information
	fmt.Fprintf(file, "Request Started: %d ns\n", tx.RequestStarted())
	fmt.Fprintf(file, "Response Last Seen: %d ns\n", tx.ResponseLastSeen())
	fmt.Fprintf(file, "Latency: %.3f ms\n", tx.RequestLatency())

	// Protocol & Security Information
	staticTags := tx.StaticTags()
	fmt.Fprintf(file, "Static Tags: 0x%x\n", staticTags)

	// Decode TLS information from static tags
	tlsInfo := decodeTLSInfo(staticTags)
	if tlsInfo != "" {
		fmt.Fprintf(file, "TLS Info: %s\n", tlsInfo)
	}

	// TCP & Network Details
	fmt.Fprintf(file, "TCP Sequence: %d\n", tx.Http.Tcp_seq)
	fmt.Fprintf(file, "Transaction Complete: %t\n", !tx.Incomplete())

	// Raw Request Data
	requestFragment := string(tx.Http.Request_fragment[:])
	if nullIndex := strings.IndexByte(requestFragment, 0); nullIndex != -1 {
		requestFragment = requestFragment[:nullIndex]
	}
	fmt.Fprintf(file, "Request Fragment: %s\n", requestFragment)

	// Raw Connection Tuple (for debugging)
	fmt.Fprintf(file, "Raw Tuple: {SaddrH: 0x%x, SaddrL: 0x%x, DaddrH: 0x%x, DaddrL: 0x%x}\n",
		tx.Tuple.Saddr_h, tx.Tuple.Saddr_l, tx.Tuple.Daddr_h, tx.Tuple.Daddr_l)

	fmt.Fprintf(file, "─────────────────────────────────────\n\n")
	file.Sync()

	// Also log a simple message to indicate capture
	log.Infof("HTTP traffic captured and dumped to file: %s %s", tx.Method().String(), extractedPath)
}

// decodeTLSInfo decodes TLS information from static tags
func decodeTLSInfo(tags uint64) string {
	var tlsLibraries []string

	// Check for TLS libraries based on the static tags
	if tags&uint64(GnuTLS) != 0 {
		tlsLibraries = append(tlsLibraries, "GnuTLS")
	}
	if tags&uint64(OpenSSL) != 0 {
		tlsLibraries = append(tlsLibraries, "OpenSSL")
	}
	if tags&uint64(Go) != 0 {
		tlsLibraries = append(tlsLibraries, "Go TLS")
	}
	if tags&uint64(NodeJS) != 0 {
		tlsLibraries = append(tlsLibraries, "Node.js")
	}
	if tags&uint64(Istio) != 0 {
		tlsLibraries = append(tlsLibraries, "Istio")
	}

	var result string
	if tags&uint64(TLS) != 0 {
		result = "Encrypted connection"
		if len(tlsLibraries) > 0 {
			result += " (Library: " + strings.Join(tlsLibraries, ", ") + ")"
		}
	} else if len(tlsLibraries) > 0 {
		result = "TLS Library: " + strings.Join(tlsLibraries, ", ")
	}

	return result
}
