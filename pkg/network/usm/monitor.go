// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type buildMode string
type monitorState = string

const (
	// Prebuilt mode
	Prebuilt buildMode = "prebuilt"
	// RuntimeCompiled mode
	RuntimeCompiled buildMode = "runtime-compilation"
	// CORE mode
	CORE buildMode = "CO-RE"

	Disabled   monitorState = "Disabled"
	Running    monitorState = "Running"
	NotRunning monitorState = "Not Running"

	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to classify protocols and dispatch the correct handlers.
	protocolDispatcherSocketFilterFunction = "socket__protocol_dispatcher"
	connectionStatesMap                    = "connection_states"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	maxActive = 128
	probeUID  = "http"

	connProtoTTL              = 3 * time.Minute
	connProtoCleaningInterval = 5 * time.Minute
)

var (
	state        = Disabled
	startupError error

	// knownProtocols holds all known protocols supported by USM to initialize.
	knownProtocols = []*protocols.ProtocolSpec{
		http.Spec,
		http2.Spec,
		kafka.Spec,
		javaTLSSpec,
		// opensslSpec is unique, as we're modifying its factory during runtime to allow getting more parameters in the
		// factory.
		opensslSpec,
	}

	usmMaps = []*manager.Map{
		{Name: protocols.ProtocolDispatcherProgramsMap},
		{Name: connectionStatesMap},
	}

	usmProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe__tcp_sendmsg",
				UID:          probeUID,
			},
			KProbeMaxActive: maxActive,
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint__net__netif_receive_skb",
				UID:          probeUID,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolDispatcherSocketFilterFunction,
				UID:          probeUID,
			},
		},
	}

	errNoProtocols = errors.New("no protocol monitors were initialised")
)

type subprogram interface {
	Name() string
	IsBuildModeSupported(buildMode) bool
	ConfigureManager(*errtelemetry.Manager)
	ConfigureOptions(*manager.Options)
	Start()
	Stop()
}

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Consuming HTTP transaction "events" that are sent from Kernel space;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	cfg            *config.Config
	processMonitor *monitor.ProcessMonitor
	lastUpdateTime *atomic.Int64

	mgr *errtelemetry.Manager
	// Socket filter cleaner callback.
	closeFilterFn func()
	// List of all tail calls from the eBPF program.
	tailCallRouter []manager.TailCallRoute
	// A shared map between USM and NPM of connection protocol.
	connectionProtocolMap *ebpf.Map

	// A list of enabled protocols.
	enabledProtocols []protocols.Protocol
	// A list of disabled and not running protocols.
	disabledProtocols []*protocols.ProtocolSpec

	// The build mode (CO-RE, runtime, prebuilt) USM is using.
	buildMode buildMode

	// Used for connection_protocol data expiration
	mapCleaner *ddebpf.MapCleaner
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, connectionProtocolMap, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*Monitor, error) {
	var err error
	defer func() {
		// capture error and wrap it
		if err != nil {
			state = NotRunning
			err = fmt.Errorf("could not initialize USM: %w", err)
			startupError = err
		}
	}()

	mon := &Monitor{
		cfg: c,
		mgr: errtelemetry.NewManager(&manager.Manager{
			Maps:   usmMaps,
			Probes: usmProbes,
		}, bpfTelemetry),
		tailCallRouter:        make([]manager.TailCallRoute, 0),
		connectionProtocolMap: connectionProtocolMap,
		enabledProtocols:      make([]protocols.Protocol, 0),
		disabledProtocols:     make([]*protocols.ProtocolSpec, 0),
	}

	// Dynamically modifying openssl spec. Required as we need sockFD and bpfTelemetry, which are created only in
	// runtime.
	opensslSpec.Factory = newSSLProgramProtocolFactory(mon.mgr.Manager, sockFD, bpfTelemetry)

	if err := mon.initProtocols(c); err != nil {
		return nil, err
	}
	if len(mon.enabledProtocols) == 0 {
		state = Disabled
		log.Debug("not enabling USM as no protocols monitoring were enabled.")
		return nil, nil
	}

	if err := mon.initEBPFProgram(); err != nil {
		return nil, fmt.Errorf("error initializing ebpf program: %w", err)
	}

	ebpfcheck.AddNameMappings(mon.mgr.Manager, "usm_monitor")

	mon.closeFilterFn, err = mon.initializeSocketFilter()
	if err != nil {
		return nil, fmt.Errorf("error enabling traffic inspection: %s", err)
	}

	mon.processMonitor = monitor.GetProcessMonitor()
	mon.lastUpdateTime = atomic.NewInt64(time.Now().Unix())
	state = Running

	return mon, nil
}

// initializeSocketFilter creates a socket filter for USM's entrypoint.
func (m *Monitor) initializeSocketFilter() (func(), error) {
	filter, _ := m.mgr.GetProbe(manager.ProbeIdentificationPair{EBPFFuncName: protocolDispatcherSocketFilterFunction, UID: probeUID})
	if filter == nil {
		return nil, errors.New("error retrieving socket filter")
	}

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(m.cfg, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling traffic inspection: %s", err)
	}

	return closeFilterFn, err
}

// Start USM monitor.
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	var err error

	defer func() {
		if err != nil {
			if errors.Is(err, syscall.ENOMEM) {
				err = fmt.Errorf("could not enable usm monitoring: not enough memory to attach http ebpf socket filter. please consider raising the limit via sysctl -w net.core.optmem_max=<LIMIT>")
			} else {
				err = fmt.Errorf("could not enable USM: %s", err)
			}

			m.Stop()

			startupError = err
		}
	}()

	// Deleting from an array while iterating it is not a simple task. Instead, every successfully enabled protocol,
	// we'll keep it in a temporary copy, and in case of a mismatch (a.k.a., we have a failed protocols) between
	// enabledProtocolsTmp to m.enabledProtocols, we'll use the enabledProtocolsTmp.
	enabledProtocolsTmp := m.enabledProtocols[:0]
	for _, protocol := range m.enabledProtocols {
		startErr := protocol.PreStart(m.mgr.Manager)
		if startErr != nil {
			log.Errorf("could not complete pre-start phase of %s monitoring: %s", protocol.Name(), startErr)
			continue
		}
		enabledProtocolsTmp = append(enabledProtocolsTmp, protocol)
	}
	m.enabledProtocols = enabledProtocolsTmp

	// No protocols could be enabled, abort.
	if len(m.enabledProtocols) == 0 {
		return errNoProtocols
	}

	mapCleaner, err := m.setupMapCleaner()
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
	} else {
		m.mapCleaner = mapCleaner
	}

	if err := m.mgr.Manager.Start(); err != nil {
		return err
	}

	enabledProtocolsTmp = m.enabledProtocols[:0]
	for _, protocol := range m.enabledProtocols {
		startErr := protocol.PostStart(m.mgr.Manager)
		if startErr != nil {
			// Cleanup the protocol. Note that at this point we can't unload the
			// ebpf programs of a specific protocol without shutting down the
			// entire manager.
			protocol.Stop(m.mgr.Manager)

			// Log and reset the error value
			log.Errorf("could not complete post-start phase of %s monitoring: %s", protocol.Name(), startErr)
			continue
		}
		enabledProtocolsTmp = append(enabledProtocolsTmp, protocol)
	}
	m.enabledProtocols = enabledProtocolsTmp

	// We check again if there are protocols that could be enabled, and abort if
	// it is not the case.
	if len(m.enabledProtocols) == 0 {
		return errNoProtocols
	}

	// Need to explicitly save the error in `err` so the defer function could save the startup error.
	if m.cfg.EnableNativeTLSMonitoring || m.cfg.EnableGoTLSSupport || m.cfg.EnableJavaTLSSupport || m.cfg.EnableIstioMonitoring {
		err = m.processMonitor.Initialize()
	}

	for _, protocolName := range m.enabledProtocols {
		log.Infof("enabled USM protocol: %s", protocolName.Name())
	}

	return err
}

func (m *Monitor) GetUSMStats() map[string]interface{} {
	response := map[string]interface{}{
		"state": state,
	}

	if startupError != nil {
		response["error"] = startupError.Error()
	}

	if m != nil {
		response["last_check"] = m.lastUpdateTime
	}
	return response
}

func (m *Monitor) GetProtocolStats() map[protocols.ProtocolType]interface{} {
	if m == nil {
		return nil
	}

	defer func() {
		// Update update time
		now := time.Now().Unix()
		m.lastUpdateTime.Swap(now)
		telemetry.ReportPrometheus()
	}()

	ret := make(map[protocols.ProtocolType]interface{})

	for _, protocol := range m.enabledProtocols {
		ps := protocol.GetStats()
		if ps != nil {
			ret[ps.Type] = ps.Stats
		}
	}

	return ret
}

// Stop HTTP monitoring
func (m *Monitor) Stop() {
	if m == nil {
		return
	}

	m.processMonitor.Stop()

	ebpfcheck.RemoveNameMappings(m.mgr.Manager)
	for _, protocol := range m.enabledProtocols {
		protocol.Stop(m.mgr.Manager)
	}

	m.mapCleaner.Stop()
	_ = m.mgr.Stop(manager.CleanAll)
	m.closeFilterFn()
}

// DumpMaps dumps the maps associated with the monitor
func (m *Monitor) DumpMaps(maps ...string) (string, error) {
	return m.mgr.DumpMaps(maps...)
}

// initProtocols takes the network configuration `c` and uses it to initialise
// the enabled protocols' monitoring, and configures the ebpf-manager `mgr`
// accordingly.
//
// For each enabled protocols, a protocol-specific instance of the Protocol
// interface is initialised, and the required maps and tail calls routers are setup
// in the manager.
//
// If a protocol is not enabled, its tail calls are instead added to the list of
// excluded functions for them to be patched out by ebpf-manager on startup.
//
// It returns:
// - a slice containing instances of the Protocol interface for each enabled protocol support
// - a slice containing pointers to the protocol specs of disabled protocols.
// - an error value, which is non-nil if an error occurred while initialising a protocol
func (m *Monitor) initProtocols(c *config.Config) error {
	for _, spec := range knownProtocols {
		protocol, err := spec.Factory(c)
		if err != nil {
			return &errNotSupported{err}
		}

		if protocol != nil {
			// Configure the manager
			m.mgr.Manager.Maps = append(m.mgr.Manager.Maps, spec.Maps...)
			m.mgr.Manager.Probes = append(m.mgr.Manager.Probes, spec.Probes...)
			m.tailCallRouter = append(m.tailCallRouter, spec.TailCalls...)

			m.enabledProtocols = append(m.enabledProtocols, protocol)

			log.Infof("%v monitoring enabled", protocol.Name())
		} else {
			m.disabledProtocols = append(m.disabledProtocols, spec)
		}
	}

	return nil
}

func (m *Monitor) initCORE() error {
	assetName := netebpf.ModuleFileName("usm", m.cfg.BPFDebug)
	return ddebpf.LoadCOREAsset(&m.cfg.Config, assetName, m.initEBPFModuleCommon)
}

func (m *Monitor) initRuntimeCompiler() error {
	bc, err := getRuntimeCompiledUSM(m.cfg)
	if err != nil {
		return err
	}
	defer bc.Close()
	return m.initEBPFModuleCommon(bc, manager.Options{})
}

func (m *Monitor) initPrebuilt() error {
	bc, err := netebpf.ReadHTTPModule(m.cfg.BPFDir, m.cfg.BPFDebug)
	if err != nil {
		return err
	}
	defer bc.Close()

	var offsets []manager.ConstantEditor
	if offsets, err = offsetguess.TracerOffsets.Offsets(m.cfg); err != nil {
		return err
	}

	return m.initEBPFModuleCommon(bc, manager.Options{ConstantEditors: offsets})
}

func (m *Monitor) initEBPFModuleCommon(buf bytecode.AssetReader, options manager.Options) error {
	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if m.cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}

	options.RLimit = &unix.Rlimit{
		Cur: math.MaxUint64,
		Max: math.MaxUint64,
	}

	options.MapSpecEditors = map[string]manager.MapSpecEditor{
		connectionStatesMap: {
			Type:       ebpf.Hash,
			MaxEntries: m.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		},
	}
	if m.connectionProtocolMap != nil {
		if options.MapEditors == nil {
			options.MapEditors = make(map[string]*ebpf.Map)
		}
		options.MapEditors[probes.ConnectionProtocolMap] = m.connectionProtocolMap
	} else {
		options.MapSpecEditors[probes.ConnectionProtocolMap] = manager.MapSpecEditor{
			Type:       ebpf.Hash,
			MaxEntries: m.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		}
	}

	options.TailCallRouter = m.tailCallRouter
	options.ActivatedProbes = []manager.ProbesSelector{
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolDispatcherSocketFilterFunction,
				UID:          probeUID,
			},
		},
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe__tcp_sendmsg",
				UID:          probeUID,
			},
		},
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint__net__netif_receive_skb",
				UID:          probeUID,
			},
		},
	}

	// Some parts of USM (https capturing, and part of the classification) use `read_conn_tuple`, and has some if
	// clauses that handled IPV6, for USM we care (ATM) only from TCP connections, so adding the sole config about tcpv6.
	utils.AddBoolConst(&options, m.cfg.CollectTCPv6Conns, "tcpv6_enabled")

	options.DefaultKprobeAttachMethod = kprobeAttachMethod
	options.VerifierOptions.Programs.LogSize = 10 * 1024 * 1024

	for _, p := range m.enabledProtocols {
		p.ConfigureOptions(m.mgr.Manager, &options)
	}

	// Add excluded functions from disabled protocols
	for _, p := range m.disabledProtocols {
		for _, m := range p.Maps {
			// Unused maps still needs to have a non-zero size
			options.MapSpecEditors[m.Name] = manager.MapSpecEditor{
				Type:       ebpf.Hash,
				MaxEntries: uint32(1),
				EditorFlag: manager.EditMaxEntries,
			}

			log.Debugf("disabled map: %v", m.Name)
		}

		for _, probe := range p.Probes {
			options.ExcludedFunctions = append(options.ExcludedFunctions, probe.ProbeIdentificationPair.EBPFFuncName)
		}

		for _, tc := range p.TailCalls {
			options.ExcludedFunctions = append(options.ExcludedFunctions, tc.ProbeIdentificationPair.EBPFFuncName)
		}
	}

	return m.mgr.InitWithOptions(buf, options)
}

func (m *Monitor) initEBPFProgram() error {
	undefinedProbes := make([]manager.ProbeIdentificationPair, 0, len(m.tailCallRouter))
	for _, tc := range m.tailCallRouter {
		undefinedProbes = append(undefinedProbes, tc.ProbeIdentificationPair)
	}

	m.mgr.DumpHandler = m.dumpMapsHandler
	m.mgr.InstructionPatcher = func(m *manager.Manager) error {
		return errtelemetry.PatchEBPFTelemetry(m, true, undefinedProbes)
	}

	var err error
	if m.cfg.EnableCORE {
		err = m.initCORE()
		if err == nil {
			m.buildMode = CORE
			return nil
		}

		if !m.cfg.AllowRuntimeCompiledFallback && !m.cfg.AllowPrecompiledFallback {
			return fmt.Errorf("co-re load failed: %w", err)
		}
		log.Warnf("co-re load failed. attempting fallback: %s", err)
	}

	if m.cfg.EnableRuntimeCompiler || (err != nil && m.cfg.AllowRuntimeCompiledFallback) {
		err = m.initRuntimeCompiler()
		if err == nil {
			m.buildMode = RuntimeCompiled
			return nil
		}

		if !m.cfg.AllowPrecompiledFallback {
			return fmt.Errorf("runtime compilation failed: %w", err)
		}
		log.Warnf("runtime compilation failed: attempting fallback: %s", err)
	}

	err = m.initPrebuilt()
	if err == nil {
		m.buildMode = Prebuilt
	}
	return err
}

func (m *Monitor) setupMapCleaner() (*ddebpf.MapCleaner, error) {
	mapCleaner, err := ddebpf.NewMapCleaner(m.connectionProtocolMap, new(netebpf.ConnTuple), new(netebpf.ProtocolStackWrapper))
	if err != nil {
		return nil, err
	}

	ttl := connProtoTTL.Nanoseconds()
	mapCleaner.Clean(connProtoCleaningInterval, func(now int64, key, val interface{}) bool {
		protoStack, ok := val.(*netebpf.ProtocolStackWrapper)
		if !ok {
			return false
		}

		updated := int64(protoStack.Updated)
		return (now - updated) > ttl
	})

	return mapCleaner, nil
}

func (m *Monitor) dumpMapsHandler(_ *manager.Manager, mapName string, currentMap *ebpf.Map) string {
	var output strings.Builder

	switch mapName {
	case sslSockByCtxMap: // maps/ssl_sock_by_ctx (BPF_MAP_TYPE_HASH), key uintptr // C.void *, value C.ssl_sock_t
		output.WriteString("Map: '" + mapName + "', key: 'uintptr // C.void *', value: 'C.ssl_sock_t'\n")
		iter := currentMap.Iterate()
		var key uintptr // C.void *
		var value http.SslSock
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "ssl_read_args": // maps/ssl_read_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_read_args_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.ssl_read_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslReadArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "bio_new_socket_args": // maps/bio_new_socket_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.__u32
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "fd_by_ssl_bio": // maps/fd_by_ssl_bio (BPF_MAP_TYPE_HASH), key C.__u32, value uintptr // C.void *
		output.WriteString("Map: '" + mapName + "', key: 'C.__u32', value: 'uintptr // C.void *'\n")
		iter := currentMap.Iterate()
		var key uint32
		var value uintptr // C.void *
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "ssl_ctx_by_pid_tgid": // maps/ssl_ctx_by_pid_tgid (BPF_MAP_TYPE_HASH), key C.__u64, value uintptr // C.void *
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'uintptr // C.void *'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uintptr // C.void *
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case connectionStatesMap: // maps/connection_states (BPF_MAP_TYPE_HASH), key C.conn_tuple_t, value C.__u32
		output.WriteString("Map: '" + mapName + "', key: 'C.conn_tuple_t', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key http.ConnTuple
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	default: // Go through enabled protocols in case one of them now how to handle the current map
		for _, p := range m.enabledProtocols {
			p.DumpMaps(&output, mapName, currentMap)
		}
	}
	return output.String()
}
