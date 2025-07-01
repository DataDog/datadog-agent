// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	errNoProtocols = errors.New("no protocol monitors were initialised")

	// knownProtocols holds all known protocols supported by USM to initialize.
	knownProtocols = []*protocols.ProtocolSpec{
		http.Spec,
		http2.Spec,
		kafka.Spec,
		postgres.Spec,
		redis.Spec,
		// opensslSpec is unique, as we're modifying its factory during runtime to allow getting more parameters in the
		// factory.
		opensslSpec,
		goTLSSpec,
		istioSpec,
		nodejsSpec,
	}
)

const (
	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to classify protocols and dispatch the correct handlers.
	protocolDispatcherSocketFilterFunction = "socket__protocol_dispatcher"
	connectionStatesMap                    = "connection_states"
	sockFDLookupArgsMap                    = "sockfd_lookup_args"
	tupleByPidFDMap                        = "tuple_by_pid_fd"
	pidFDByTupleMap                        = "pid_fd_by_tuple"

	sockFDLookup    = "kprobe__sockfd_lookup_light"
	sockFDLookupRet = "kretprobe__sockfd_lookup_light"

	tcpCloseProbe = "kprobe__tcp_close"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	maxActive = 128
	probeUID  = "http"
)

type ebpfProgram struct {
	*ddebpf.Manager
	cfg                   *config.Config
	tailCallRouter        []manager.TailCallRoute
	connectionProtocolMap *ebpf.Map

	enabledProtocols  []*protocols.ProtocolSpec
	disabledProtocols []*protocols.ProtocolSpec

	buildMode buildmode.Type
}

func newEBPFProgram(c *config.Config, connectionProtocolMap *ebpf.Map) (*ebpfProgram, error) {
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: protocols.TLSDispatcherProgramsMap},
			{Name: protocols.ProtocolDispatcherProgramsMap},
			{Name: protocols.ProtocolDispatcherClassificationPrograms},
			{Name: protocols.TLSProtocolDispatcherClassificationPrograms},
			{Name: connectionStatesMap},
			{Name: sockFDLookupArgsMap},
			{Name: tupleByPidFDMap},
			{Name: pidFDByTupleMap},
		},
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: tcpCloseProbe,
					UID:          probeUID,
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: protocolDispatcherSocketFilterFunction,
					UID:          probeUID,
				},
			},
		},
	}

	if c.CollectTCPv4Conns || c.CollectTCPv6Conns {
		missing, err := ddebpf.VerifyKernelFuncs("sockfd_lookup_light")
		if err == nil && len(missing) == 0 {
			mgr.Probes = append(mgr.Probes, []*manager.Probe{
				{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: sockFDLookup,
						UID:          probeUID,
					},
				},
				{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: sockFDLookupRet,
						UID:          probeUID,
					},
				},
			}...)
		}
	}

	program := &ebpfProgram{
		Manager:               ddebpf.NewManager(mgr, "usm", &ebpftelemetry.ErrorsTelemetryModifier{}),
		cfg:                   c,
		connectionProtocolMap: connectionProtocolMap,
	}

	if err := program.initProtocols(c); err != nil {
		return nil, err
	}

	return program, nil
}

// Init initializes the ebpf program.
func (e *ebpfProgram) Init() error {
	var err error
	defer func() {
		if err != nil {
			e.buildMode = ""
		}
	}()

	e.DumpHandler = e.dumpMapsHandler

	if e.cfg.EnableCORE {
		e.buildMode = buildmode.CORE
		err = e.initCORE()
		if err == nil {
			return nil
		}

		if !e.cfg.AllowRuntimeCompiledFallback && !e.cfg.AllowPrebuiltFallback {
			return fmt.Errorf("co-re load failed: %w", err)
		}
		log.Warnf("co-re load failed. attempting fallback: %s", err)
	}

	if e.cfg.EnableRuntimeCompiler || (err != nil && e.cfg.AllowRuntimeCompiledFallback) {
		e.buildMode = buildmode.RuntimeCompiled
		err = e.initRuntimeCompiler()
		if err == nil {
			return nil
		}

		if !e.cfg.AllowPrebuiltFallback {
			return fmt.Errorf("runtime compilation failed: %w", err)
		}
		log.Warnf("runtime compilation failed: attempting fallback: %s", err)
	}

	if prebuilt.IsDeprecated() {
		log.Warn("using deprecated prebuilt USM monitor")
	}

	e.buildMode = buildmode.Prebuilt
	err = e.initPrebuilt()
	return err
}

// Start starts the ebpf program and the enabled protocols.
func (e *ebpfProgram) Start() error {
	initializeTupleMaps(e.Manager)

	// Mainly for tests, but possible for other cases as well, we might have a nil (not shared) connection protocol map
	// between NPM and USM. In such a case we just create our own instance, but we don't modify the
	// `e.connectionProtocolMap` field.
	if e.connectionProtocolMap == nil {
		m, _, err := e.GetMap(probes.ConnectionProtocolMap)
		if err != nil {
			return err
		}
		e.connectionProtocolMap = m
	}

	e.enabledProtocols = e.executePerProtocol(e.enabledProtocols, "pre-start",
		func(protocol protocols.Protocol) error { return protocol.PreStart() },
		func(protocols.Protocol) {})

	// No protocols could be enabled, abort.
	if len(e.enabledProtocols) == 0 {
		return errNoProtocols
	}

	err := e.Manager.Start()
	if err != nil {
		return err
	}

	e.enabledProtocols = e.executePerProtocol(e.enabledProtocols, "post-start",
		func(protocol protocols.Protocol) error { return protocol.PostStart() },
		func(protocol protocols.Protocol) { protocol.Stop() })

	// We check again if there are protocols that could be enabled, and abort if
	// it is not the case.
	if len(e.enabledProtocols) == 0 {
		err = e.Close()
		if err != nil {
			log.Errorf("error during USM shutdown: %s", err)
		}

		return errNoProtocols
	}

	for _, protocolName := range e.enabledProtocols {
		log.Infof("enabled USM protocol: %s", protocolName.Instance.Name())
	}

	return nil
}

// Close stops the ebpf program and cleans up all resources.
func (e *ebpfProgram) Close() error {
	ebpftelemetry.UnregisterTelemetry(e.Manager.Manager)
	var err error
	// We need to stop the perf maps and ring buffers before stopping the protocols, as we need to stop sending events
	// to them. If we don't do this, we might send events on closed channels which will panic.
	for _, pm := range e.PerfMaps {
		err = errors.Join(err, pm.Stop(manager.CleanAll))
	}
	for _, rb := range e.RingBuffers {
		err = errors.Join(err, rb.Stop(manager.CleanAll))
	}
	stopProtocolWrapper := func(protocol protocols.Protocol) error {
		protocol.Stop()
		return nil
	}
	e.executePerProtocol(e.enabledProtocols, "stop", stopProtocolWrapper, nil)
	return errors.Join(err, e.Stop(manager.CleanAll))
}

func (e *ebpfProgram) initCORE() error {
	assetName := getAssetName("usm", e.cfg.BPFDebug)
	return ddebpf.LoadCOREAsset(assetName, e.init)
}

func (e *ebpfProgram) initRuntimeCompiler() error {
	bc, err := getRuntimeCompiledUSM(e.cfg)
	if err != nil {
		return err
	}
	defer bc.Close()
	return e.init(bc, manager.Options{})
}

func (e *ebpfProgram) initPrebuilt() error {
	bc, err := netebpf.ReadHTTPModule(e.cfg.BPFDir, e.cfg.BPFDebug)
	if err != nil {
		return err
	}
	defer bc.Close()

	var offsets []manager.ConstantEditor
	if offsets, err = offsetguess.TracerOffsets.Offsets(e.cfg); err != nil {
		return err
	}

	return e.init(bc, manager.Options{ConstantEditors: offsets})
}

// getProtocolsForBuildMode returns 2 lists - supported and not-supported protocol lists.
// 1. Supported - enabled protocols which are supported by the current build mode (`e.buildMode`)
// 2. Not Supported - disabled protocols, and enabled protocols which are not supported by the current build mode.
func (e *ebpfProgram) getProtocolsForBuildMode() ([]*protocols.ProtocolSpec, []*protocols.ProtocolSpec) {
	supported := make([]*protocols.ProtocolSpec, 0)
	notSupported := make([]*protocols.ProtocolSpec, 0, len(e.disabledProtocols))
	notSupported = append(notSupported, e.disabledProtocols...)

	for _, p := range e.enabledProtocols {
		if p.Instance.IsBuildModeSupported(e.buildMode) {
			supported = append(supported, p)
		} else {
			notSupported = append(notSupported, p)
		}
	}

	return supported, notSupported
}

// configureManagerWithSupportedProtocols given a protocol list, we're adding for each protocol its Maps, Probes and
// TailCalls to the program's lists. Also, we're providing a cleanup method (the return value) which allows removal
// of the elements we added in case of a failure in the initialization.
func (e *ebpfProgram) configureManagerWithSupportedProtocols(protocols []*protocols.ProtocolSpec) func() {
	// Track already existing items
	existingMaps := make(map[string]struct{})
	existingProbes := make(map[string]struct{})
	existingTailCalls := make(map[string]struct{})

	// Populate sets with existing elements
	for _, m := range e.Maps {
		existingMaps[m.Name] = struct{}{}
	}
	for _, p := range e.Probes {
		existingProbes[p.EBPFFuncName] = struct{}{}
	}
	for _, tc := range e.tailCallRouter {
		existingTailCalls[tc.ProbeIdentificationPair.EBPFFuncName] = struct{}{}
	}

	// Track newly added elements for cleanup
	var addedMaps []*manager.Map
	var addedProbes []*manager.Probe
	var addedTailCalls []manager.TailCallRoute

	for _, spec := range protocols {
		for _, m := range spec.Maps {
			if _, exists := existingMaps[m.Name]; !exists {
				e.Maps = append(e.Maps, m)
				addedMaps = append(addedMaps, m)
				existingMaps[m.Name] = struct{}{}
			}
		}

		for _, p := range spec.Probes {
			if _, exists := existingProbes[p.EBPFFuncName]; !exists {
				e.Probes = append(e.Probes, p)
				addedProbes = append(addedProbes, p)
				existingProbes[p.EBPFFuncName] = struct{}{}
			}
		}

		for _, tc := range spec.TailCalls {
			if _, exists := existingTailCalls[tc.ProbeIdentificationPair.EBPFFuncName]; !exists {
				e.tailCallRouter = append(e.tailCallRouter, tc)
				addedTailCalls = append(addedTailCalls, tc)
				existingTailCalls[tc.ProbeIdentificationPair.EBPFFuncName] = struct{}{}
			}
		}
	}

	// Cleanup function to remove only what was added
	return func() {
		e.Maps = slices.DeleteFunc(e.Maps, func(m *manager.Map) bool {
			for _, added := range addedMaps {
				if m.Name == added.Name {
					return true
				}
			}
			return false
		})
		e.Probes = slices.DeleteFunc(e.Probes, func(p *manager.Probe) bool {
			for _, added := range addedProbes {
				if p.EBPFFuncName == added.EBPFFuncName {
					return true
				}
			}
			return false
		})
		e.tailCallRouter = slices.DeleteFunc(e.tailCallRouter, func(tc manager.TailCallRoute) bool {
			for _, added := range addedTailCalls {
				if tc.ProbeIdentificationPair.EBPFFuncName == added.ProbeIdentificationPair.EBPFFuncName {
					return true
				}
			}
			return false
		})
	}
}

func (e *ebpfProgram) init(buf bytecode.AssetReader, options manager.Options) error {
	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if e.cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}

	options.RemoveRlimit = true

	options.MapSpecEditors = map[string]manager.MapSpecEditor{
		connectionStatesMap: {
			MaxEntries: e.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		},
		probes.ConnectionProtocolMap: {
			MaxEntries: e.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		},
		tupleByPidFDMap: {
			MaxEntries: e.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		},
		pidFDByTupleMap: {
			MaxEntries: e.cfg.MaxTrackedConnections,
			EditorFlag: manager.EditMaxEntries,
		},
	}

	if e.connectionProtocolMap != nil {
		if options.MapEditors == nil {
			options.MapEditors = make(map[string]*ebpf.Map)
		}
		options.MapEditors[probes.ConnectionProtocolMap] = e.connectionProtocolMap
	}

	begin, end := network.EphemeralRange()
	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{Name: "ephemeral_range_begin", Value: uint64(begin)},
		manager.ConstantEditor{Name: "ephemeral_range_end", Value: uint64(end)})

	for _, p := range e.Manager.Probes {
		options.ActivatedProbes = append(options.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: p.ProbeIdentificationPair})
	}

	// Some parts of USM (https capturing, and part of the classification) use `read_conn_tuple`, and has some if
	// clauses that handled IPV6, for USM we care (ATM) only from TCP connections, so adding the sole config about tcpv6.
	utils.AddBoolConst(&options, e.cfg.CollectTCPv6Conns, "tcpv6_enabled")

	options.DefaultKProbeMaxActive = maxActive
	options.DefaultKprobeAttachMethod = kprobeAttachMethod
	options.BypassEnabled = e.cfg.BypassEnabled

	supported, notSupported := e.getProtocolsForBuildMode()
	cleanup := e.configureManagerWithSupportedProtocols(supported)
	options.TailCallRouter = e.tailCallRouter
	for _, p := range supported {
		p.Instance.ConfigureOptions(&options)
	}
	if e.cfg.InternalTelemetryEnabled {
		for _, pm := range e.PerfMaps {
			pm.TelemetryEnabled = true
			ebpftelemetry.ReportPerfMapTelemetry(pm)
		}
		for _, rb := range e.RingBuffers {
			rb.TelemetryEnabled = true
			ebpftelemetry.ReportRingBufferTelemetry(rb)
		}
	}

	// We might have shared maps, probes and TCs between supported and not supported protocols. We need to make sure
	// that we're not excluding a shared resource if it is used by at least one supported protocol.
	supportedMaps := make(map[string]struct{})
	supportedProbes := make(map[string]struct{})
	supportedTCs := make(map[string]struct{})
	for _, p := range supported {
		for _, m := range p.Maps {
			supportedMaps[m.Name] = struct{}{}
		}
		for _, probe := range p.Probes {
			supportedProbes[probe.ProbeIdentificationPair.EBPFFuncName] = struct{}{}
		}
		for _, tc := range p.TailCalls {
			supportedTCs[tc.ProbeIdentificationPair.EBPFFuncName] = struct{}{}
		}
	}

	// Add excluded functions from disabled protocols
	for _, p := range notSupported {
		for _, m := range p.Maps {
			if _, ok := supportedMaps[m.Name]; ok {
				log.Debugf("map %s is shared between enabled and disabled protocols", m.Name)
				continue
			}
			// Unused maps still need to have a non-zero size
			options.MapSpecEditors[m.Name] = manager.MapSpecEditor{
				MaxEntries: uint32(1),
				EditorFlag: manager.EditMaxEntries,
			}

			log.Debugf("disabled map: %v", m.Name)
		}

		for _, probe := range p.Probes {
			if _, ok := supportedProbes[probe.ProbeIdentificationPair.EBPFFuncName]; ok {
				log.Debugf("probe %s is shared between enabled and disabled protocols", probe.ProbeIdentificationPair.EBPFFuncName)
				continue
			}
			options.ExcludedFunctions = append(options.ExcludedFunctions, probe.ProbeIdentificationPair.EBPFFuncName)
			log.Debugf("disabled probe: %v", probe.ProbeIdentificationPair.EBPFFuncName)
		}

		for _, tc := range p.TailCalls {
			if _, ok := supportedTCs[tc.ProbeIdentificationPair.EBPFFuncName]; ok {
				log.Debugf("tail call %s is shared between enabled and disabled protocols", tc.ProbeIdentificationPair.EBPFFuncName)
				continue
			}
			options.ExcludedFunctions = append(options.ExcludedFunctions, tc.ProbeIdentificationPair.EBPFFuncName)
			log.Debugf("disabled tail call: %v", tc.ProbeIdentificationPair.EBPFFuncName)
		}
	}

	err := e.InitWithOptions(buf, &options)
	if err != nil {
		cleanup()
	} else {
		// Update the protocols lists to reflect the ones we actually enabled
		e.enabledProtocols = supported
		e.disabledProtocols = notSupported
	}

	return err
}

func getAssetName(module string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", module)
	}

	return fmt.Sprintf("%s.o", module)
}

func (e *ebpfProgram) dumpMapsHandler(w io.Writer, _ *manager.Manager, mapName string, currentMap *ebpf.Map) {
	switch mapName {
	case connectionStatesMap: // maps/connection_states (BPF_MAP_TYPE_HASH), key C.conn_tuple_t, value C.__u32
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.conn_tuple_t', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key http.ConnTuple
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	case sockFDLookupArgsMap: // maps/sockfd_lookup_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.__u32
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case tupleByPidFDMap: // maps/tuple_by_pid_fd (BPF_MAP_TYPE_HASH), key C.pid_fd_t, value C.conn_tuple_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.pid_fd_t', value: 'C.conn_tuple_t'\n")
		iter := currentMap.Iterate()
		var key netebpf.PIDFD
		var value http.ConnTuple
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case pidFDByTupleMap: // maps/pid_fd_by_tuple (BPF_MAP_TYPE_HASH), key C.conn_tuple_t, value C.pid_fd_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.conn_tuple_t', value: 'C.pid_fd_t'\n")
		iter := currentMap.Iterate()
		var key http.ConnTuple
		var value netebpf.PIDFD
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	default: // Go through enabled protocols in case one of them now how to handle the current map
		for _, p := range e.enabledProtocols {
			p.Instance.DumpMaps(w, mapName, currentMap)
		}
	}
}

func (e *ebpfProgram) getProtocolStats() (map[protocols.ProtocolType]interface{}, func()) {
	ret := make(map[protocols.ProtocolType]interface{})
	cleaners := make([]func(), 0)

	for _, protocol := range e.enabledProtocols {
		ps, cleaner := protocol.Instance.GetStats()
		if ps != nil {
			ret[ps.Type] = ps.Stats
		}
		if cleaner != nil {
			cleaners = append(cleaners, cleaner)
		}
	}

	return ret, func() {
		for _, c := range cleaners {
			c()
		}
	}
}

// executePerProtocol runs the given callback (`cb`) for every protocol in the given list (`protocolList`).
// If the callback failed, then we call the error callback (`errorCb`). Eventually returning a list of protocols which
// successfully executed the callback.
func (e *ebpfProgram) executePerProtocol(protocolList []*protocols.ProtocolSpec, phaseName string, cb func(protocols.Protocol) error, errorCb func(protocols.Protocol)) []*protocols.ProtocolSpec {
	// Deleting from an array while iterating it is not a simple task. Instead, every successfully enabled protocol,
	// we'll keep in a temporary copy and return it at the end.
	res := make([]*protocols.ProtocolSpec, 0)
	for _, protocol := range protocolList {
		if err := cb(protocol.Instance); err != nil {
			if errorCb != nil {
				errorCb(protocol.Instance)
			}
			log.Errorf("could not complete %q phase of %q monitoring: %s", phaseName, protocol.Instance.Name(), err)
			continue
		}
		res = append(res, protocol)
	}
	return res
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
func (e *ebpfProgram) initProtocols(c *config.Config) error {
	e.enabledProtocols = make([]*protocols.ProtocolSpec, 0)
	e.disabledProtocols = make([]*protocols.ProtocolSpec, 0)

	for _, spec := range knownProtocols {
		protocol, err := spec.Factory(e.Manager.Manager, c)
		if err != nil {
			return &errNotSupported{err}
		}

		if protocol != nil {
			spec.Instance = protocol
			e.enabledProtocols = append(e.enabledProtocols, spec)

			log.Infof("%v monitoring enabled", protocol.Name())
		} else {
			e.disabledProtocols = append(e.disabledProtocols, spec)
		}
	}

	return nil
}
