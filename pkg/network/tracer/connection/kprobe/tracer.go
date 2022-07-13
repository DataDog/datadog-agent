// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/stats"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"
)

const (
	defaultClosedChannelSize = 500

	probeUID = "net"
)

type kprobeTracer struct {
	m *manager.Manager

	conns    *ebpf.Map
	tcpStats *ebpf.Map
	config   *config.Config

	// tcp_close events
	closeConsumer *tcpCloseConsumer

	pidCollisions *atomic.Int64
	removeTuple   *netebpf.ConnTuple

	telemetry telemetry
}

type telemetry struct {
	tcpConns4, tcpConns6 *atomic.Int64 `stats:""`
	udpConns4, udpConns6 *atomic.Int64 `stats:""`

	reporter stats.Reporter
}

func newTelemetry() telemetry {
	return telemetry{
		tcpConns4: atomic.NewInt64(0),
		udpConns4: atomic.NewInt64(0),
		tcpConns6: atomic.NewInt64(0),
		udpConns6: atomic.NewInt64(0),
	}
}

func New(config *config.Config, constants []manager.ConstantEditor) (connection.Tracer, error) {
	mgrOptions := manager.Options{
		// Extend RLIMIT_MEMLOCK (8) size
		// On some systems, the default for RLIMIT_MEMLOCK may be as low as 64 bytes.
		// This will result in an EPERM (Operation not permitted) error, when trying to create an eBPF map
		// using bpf(2) with BPF_MAP_CREATE.
		//
		// We are setting the limit to infinity until we have a better handle on the true requirements.
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			string(probes.ConnMap):            {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.TcpStatsMap):        {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.PortBindingsMap):    {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.UdpPortBindingsMap): {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.SockByPidFDMap):     {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.PidFDBySockMap):     {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
		},
		ConstantEditors: constants,
	}

	runtimeTracer := false
	var buf bytecode.AssetReader
	var err error
	if config.EnableRuntimeCompiler {
		buf, err = getRuntimeCompiledTracer(config)
		if err != nil {
			if !config.AllowPrecompiledFallback {
				return nil, fmt.Errorf("error compiling network tracer: %s", err)
			}
			log.Warnf("error compiling network tracer, falling back to pre-compiled: %s", err)
		} else {
			runtimeTracer = true
			defer func() { _ = buf.Close() }()
		}
	}

	if buf == nil {
		buf, err = netebpf.ReadBPFModule(config.BPFDir, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}
		defer buf.Close()
	}

	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledProbes(config, runtimeTracer)
	if err != nil {
		return nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	closedChannelSize := defaultClosedChannelSize
	if config.ClosedChannelSize > 0 {
		closedChannelSize = config.ClosedChannelSize
	}
	perfHandlerTCP := ddebpf.NewPerfHandler(closedChannelSize)
	m := newManager(perfHandlerTCP, runtimeTracer)
	m.DumpHandler = dumpMapsHandler

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[probes.ProbeName(p.EBPFSection)]; !enabled {
			mgrOptions.ExcludedFunctions = append(mgrOptions.ExcludedFunctions, p.EBPFFuncName)
		}
	}
	for probeName, funcName := range enabledProbes {
		mgrOptions.ActivatedProbes = append(
			mgrOptions.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probeName),
					EBPFFuncName: funcName,
					UID:          probeUID,
				},
			})
	}
	err = m.InitWithOptions(buf, mgrOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to init ebpf manager: %v", err)
	}

	closeConsumer, err := newTCPCloseConsumer(m, perfHandlerTCP)
	if err != nil {
		return nil, fmt.Errorf("could not create tcpCloseConsumer: %s", err)
	}

	tr := &kprobeTracer{
		m:             m,
		config:        config,
		closeConsumer: closeConsumer,
		pidCollisions: atomic.NewInt64(0),
		removeTuple:   &netebpf.ConnTuple{},
		telemetry:     newTelemetry(),
	}

	tr.conns, _, err = m.GetMap(string(probes.ConnMap))
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.ConnMap, err)
	}

	tr.tcpStats, _, err = m.GetMap(string(probes.TcpStatsMap))
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.TcpStatsMap, err)
	}

	tr.telemetry.reporter, err = stats.NewReporter(&tr.telemetry)
	if err != nil {
		return nil, fmt.Errorf("error creating stats reporter: %w", err)
	}

	return tr, nil
}

func (t *kprobeTracer) Start(callback func([]network.ConnectionStats)) (err error) {
	defer func() {
		if err != nil {
			t.Stop()
		}
	}()

	err = initializePortBindingMaps(t.config, t.m)
	if err != nil {
		return fmt.Errorf("error initializing port binding maps: %s", err)
	}

	if err := t.m.Start(); err != nil {
		return fmt.Errorf("could not start ebpf manager: %s", err)
	}

	t.closeConsumer.Start(callback)
	return nil
}

func (t *kprobeTracer) FlushPending() {
	t.closeConsumer.FlushPending()
}

func (t *kprobeTracer) Stop() {
	_ = t.m.Stop(manager.CleanAll)
	t.closeConsumer.Stop()
}

func (t *kprobeTracer) GetMap(name string) *ebpf.Map {
	switch name {
	case string(probes.SockByPidFDMap):
		m, _, _ := t.m.GetMap(name)
		return m
	default:
		return nil
	}
}

func (t *kprobeTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	// Iterate through all key-value pairs in map
	key, stats := &netebpf.ConnTuple{}, &netebpf.ConnStats{}
	seen := make(map[netebpf.ConnTuple]struct{})

	// Cached objects
	conn := new(network.ConnectionStats)
	tcp := new(netebpf.TCPStats)

	tel := newTelemetry()
	entries := t.conns.Iterate()
	for entries.Next(unsafe.Pointer(key), unsafe.Pointer(stats)) {
		populateConnStats(conn, key, stats)

		tel.addConnection(conn)

		if filter != nil && !filter(conn) {
			continue
		}
		if t.getTCPStats(tcp, key, seen) {
			updateTCPStats(conn, tcp)
		}
		*buffer.Next() = *conn
	}

	if err := entries.Err(); err != nil {
		return fmt.Errorf("unable to iterate connection map: %s", err)
	}

	t.telemetry.assign(tel)

	return nil
}

func (t *telemetry) assign(other telemetry) {
	t.tcpConns4.Store(other.tcpConns4.Load())
	t.tcpConns6.Store(other.tcpConns6.Load())
	t.udpConns4.Store(other.udpConns4.Load())
	t.udpConns6.Store(other.udpConns6.Load())
}

func (t *telemetry) addConnection(conn *network.ConnectionStats) {
	isTcp := conn.Type == network.TCP
	switch conn.Family {
	case network.AFINET6:
		if isTcp {
			t.tcpConns6.Inc()
		} else {
			t.udpConns6.Inc()
		}
	case network.AFINET:
		if isTcp {
			t.tcpConns4.Inc()
		} else {
			t.udpConns4.Inc()
		}
	}
}

func (t *telemetry) removeConnection(conn *network.ConnectionStats) {
	isTcp := conn.Type == network.TCP
	switch conn.Family {
	case network.AFINET6:
		if isTcp {
			t.tcpConns6.Dec()
		} else {
			t.udpConns6.Dec()
		}
	case network.AFINET:
		if isTcp {
			t.tcpConns4.Dec()
		} else {
			t.udpConns4.Dec()
		}
	}
}

func (t *telemetry) get() map[string]interface{} {
	return t.reporter.Report()
}

func (t *kprobeTracer) Remove(conn *network.ConnectionStats) error {
	t.removeTuple.Sport = conn.SPort
	t.removeTuple.Dport = conn.DPort
	t.removeTuple.Netns = conn.NetNS
	t.removeTuple.Pid = conn.Pid
	t.removeTuple.Saddr_l, t.removeTuple.Saddr_h = util.ToLowHigh(conn.Source)
	t.removeTuple.Daddr_l, t.removeTuple.Daddr_h = util.ToLowHigh(conn.Dest)

	if conn.Family == network.AFINET6 {
		t.removeTuple.Metadata = uint32(netebpf.IPv6)
	} else {
		t.removeTuple.Metadata = uint32(netebpf.IPv4)
	}
	if conn.Type == network.TCP {
		t.removeTuple.Metadata |= uint32(netebpf.TCP)
	} else {
		t.removeTuple.Metadata |= uint32(netebpf.UDP)
	}

	err := t.conns.Delete(unsafe.Pointer(t.removeTuple))
	if err != nil {
		// If this entry no longer exists in the eBPF map it means `tcp_close` has executed
		// during this function call. In that case state.StoreClosedConnection() was already called for this connection,
		// and we can't delete the corresponding client state, or we'll likely over-report the metric values.
		// By skipping to the next iteration and not calling state.RemoveConnections() we'll let
		// this connection expire "naturally" when either next connection check runs or the client itself expires.
		return err
	}

	t.telemetry.removeConnection(conn)

	// We have to remove the PID to remove the element from the TCP Map since we don't use the pid there
	t.removeTuple.Pid = 0
	// We can ignore the error for this map since it will not always contain the entry
	_ = t.tcpStats.Delete(unsafe.Pointer(t.removeTuple))

	return nil
}

func (t *kprobeTracer) GetTelemetry() map[string]int64 {
	var zero uint64
	mp, _, err := t.m.GetMap(string(probes.TelemetryMap))
	if err != nil {
		log.Warnf("error retrieving telemetry map: %s", err)
		return map[string]int64{}
	}

	telemetry := &netebpf.Telemetry{}
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		log.Tracef("error retrieving the telemetry struct: %s", err)
	}

	closeStats := t.closeConsumer.GetStats()
	pidCollisions := t.pidCollisions.Load()

	stats := map[string]int64{
		"closed_conn_polling_lost":     closeStats[perfLostStat],
		"closed_conn_polling_received": closeStats[perfReceivedStat],
		"pid_collisions":               pidCollisions,

		"tcp_sent_miscounts":         int64(telemetry.Tcp_sent_miscounts),
		"missed_tcp_close":           int64(telemetry.Missed_tcp_close),
		"missed_udp_close":           int64(telemetry.Missed_udp_close),
		"udp_sends_processed":        int64(telemetry.Udp_sends_processed),
		"udp_sends_missed":           int64(telemetry.Udp_sends_missed),
		"conn_stats_max_entries_hit": int64(telemetry.Conn_stats_max_entries_hit),
	}

	for k, v := range t.telemetry.get() {
		stats[k] = v.(int64)
	}

	return stats
}

// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
func (t *kprobeTracer) DumpMaps(maps ...string) (string, error) {
	return t.m.DumpMaps(maps...)
}

func initializePortBindingMaps(config *config.Config, m *manager.Manager) error {
	if tcpPorts, err := network.ReadInitialState(config.ProcRoot, network.TCP, config.CollectIPv6Conns, true); err != nil {
		return fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	} else {
		tcpPortMap, _, err := m.GetMap(string(probes.PortBindingsMap))
		if err != nil {
			return fmt.Errorf("failed to get TCP port binding map: %w", err)
		}
		for p, count := range tcpPorts {
			log.Debugf("adding initial TCP port binding: netns: %d port: %d", p.Ino, p.Port)
			pb := netebpf.PortBinding{Netns: p.Ino, Port: p.Port}
			err = tcpPortMap.Update(unsafe.Pointer(&pb), unsafe.Pointer(&count), ebpf.UpdateNoExist)
			if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
				return fmt.Errorf("failed to update TCP port binding map: %w", err)
			}
		}
	}

	if udpPorts, err := network.ReadInitialState(config.ProcRoot, network.UDP, config.CollectIPv6Conns, false); err != nil {
		return fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	} else {
		udpPortMap, _, err := m.GetMap(string(probes.UdpPortBindingsMap))
		if err != nil {
			return fmt.Errorf("failed to get UDP port binding map: %w", err)
		}
		for p, count := range udpPorts {
			log.Debugf("adding initial UDP port binding: netns: %d port: %d", p.Ino, p.Port)
			// UDP port bindings currently do not have network namespace numbers
			pb := netebpf.PortBinding{Netns: 0, Port: p.Port}
			err = udpPortMap.Update(unsafe.Pointer(&pb), unsafe.Pointer(&count), ebpf.UpdateNoExist)
			if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
				return fmt.Errorf("failed to update UDP port binding map: %w", err)
			}
		}
	}
	return nil
}

func updateTCPStats(conn *network.ConnectionStats, tcpStats *netebpf.TCPStats) {
	if conn.Type != network.TCP {
		return
	}
	conn.Monotonic.Retransmits = tcpStats.Retransmits
	conn.Monotonic.TCPEstablished = uint32(tcpStats.State_transitions >> netebpf.Established & 1)
	conn.Monotonic.TCPClosed = uint32(tcpStats.State_transitions >> netebpf.Close & 1)
	conn.RTT = tcpStats.Rtt
	conn.RTTVar = tcpStats.Rtt_var
}

// getTCPStats reads tcp related stats for the given ConnTuple
func (t *kprobeTracer) getTCPStats(stats *netebpf.TCPStats, tuple *netebpf.ConnTuple, seen map[netebpf.ConnTuple]struct{}) bool {
	if tuple.Type() != netebpf.TCP {
		return false
	}

	// The PID isn't used as a key in the stats map, we will temporarily set it to 0 here and reset it when we're done
	pid := tuple.Pid
	tuple.Pid = 0

	*stats = netebpf.TCPStats{}
	err := t.tcpStats.Lookup(unsafe.Pointer(tuple), unsafe.Pointer(stats))
	if err == nil {
		// This is required to avoid (over)reporting retransmits for connections sharing the same socket.
		if _, reported := seen[*tuple]; reported {
			t.pidCollisions.Inc()
			stats.Retransmits = 0
		} else {
			seen[*tuple] = struct{}{}
		}
	}

	tuple.Pid = pid
	return true
}

func populateConnStats(stats *network.ConnectionStats, t *netebpf.ConnTuple, s *netebpf.ConnStats) {
	*stats = network.ConnectionStats{
		Pid:              t.Pid,
		NetNS:            t.Netns,
		Source:           t.SourceAddress(),
		Dest:             t.DestAddress(),
		SPort:            t.Sport,
		DPort:            t.Dport,
		SPortIsEphemeral: network.IsPortInEphemeralRange(t.Sport),
		Monotonic: network.StatCounters{
			SentBytes:   s.Sent_bytes,
			RecvBytes:   s.Recv_bytes,
			SentPackets: s.Sent_packets,
			RecvPackets: s.Recv_packets,
		},
		LastUpdateEpoch: s.Timestamp,
		IsAssured:       s.IsAssured(),
	}

	if t.Type() == netebpf.TCP {
		stats.Type = network.TCP
	} else {
		stats.Type = network.UDP
	}

	switch t.Family() {
	case netebpf.IPv4:
		stats.Family = network.AFINET
	case netebpf.IPv6:
		stats.Family = network.AFINET6
	}

	switch s.ConnectionDirection() {
	case netebpf.Incoming:
		stats.Direction = network.INCOMING
	case netebpf.Outgoing:
		stats.Direction = network.OUTGOING
	default:
		stats.Direction = network.OUTGOING
	}
}
