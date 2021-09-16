//+build linux_bpf

package kprobe

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"golang.org/x/sys/unix"
)

const (
	defaultClosedChannelSize = 500
)

type kprobeTracer struct {
	m *manager.Manager

	conns    *ebpf.Map
	tcpStats *ebpf.Map
	config   *config.Config

	// tcp_close events
	closeConsumer *tcpCloseConsumer
	closeHandler  *ddebpf.PerfHandler

	pidCollisions int64
	removeTuple   *netebpf.ConnTuple
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
		runtime.RuntimeCompilationEnabled = true
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

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[probes.ProbeName(p.Section)]; !enabled {
			mgrOptions.ExcludedSections = append(mgrOptions.ExcludedSections, p.Section)
		}
	}
	for probeName := range enabledProbes {
		mgrOptions.ActivatedProbes = append(
			mgrOptions.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: string(probeName),
				},
			})
	}
	err = m.InitWithOptions(buf, mgrOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to init ebpf manager: %v", err)
	}

	tr := &kprobeTracer{
		m:            m,
		config:       config,
		closeHandler: perfHandlerTCP,
		removeTuple:  &netebpf.ConnTuple{},
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

	return tr, nil
}

func (t *kprobeTracer) Start(closeFilter func(*network.ConnectionStats) bool) error {
	closeConsumer, err := newTCPCloseConsumer(t.config, t.m, t.closeHandler, closeFilter)
	if err != nil {
		t.Stop()
		return fmt.Errorf("could not create tcpCloseConsumer: %s", err)
	}
	t.closeConsumer = closeConsumer

	err = initializePortBindingMaps(t.config, t.m)
	if err != nil {
		t.Stop()
		return fmt.Errorf("error initializing port binding maps: %s", err)
	}

	if err := t.m.Start(); err != nil {
		return fmt.Errorf("could not start ebpf manager: %s", err)
	}
	return nil
}

func (t *kprobeTracer) Stop() {
	t.closeConsumer.Stop()
	_ = t.m.Stop(manager.CleanAll)
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

func (t *kprobeTracer) GetConnections(buffer []network.ConnectionStats, filter func(*network.ConnectionStats) bool) ([]network.ConnectionStats, []network.ConnectionStats, error) {
	// Iterate through all key-value pairs in map
	key, stats := &netebpf.ConnTuple{}, &netebpf.ConnStats{}
	seen := make(map[netebpf.ConnTuple]struct{})
	entries := t.conns.IterateFrom(unsafe.Pointer(&netebpf.ConnTuple{}))
	for entries.Next(unsafe.Pointer(key), unsafe.Pointer(stats)) {
		conn := connStats(key, stats, t.getTCPStats(key, seen))
		if filter != nil && filter(&conn) {
			buffer = append(buffer, conn)
		}
	}

	if err := entries.Err(); err != nil {
		return nil, nil, fmt.Errorf("unable to iterate connection map: %s", err)
	}

	// Add all connections that were closed since the last `GetConnections` call
	closed := t.closeConsumer.GetClosedConnections(buffer[len(buffer):])
	return buffer, closed, nil
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
	pidCollisions := atomic.LoadInt64(&t.pidCollisions)

	return map[string]int64{
		"closed_conn_polling_lost":     closeStats["lost"],
		"closed_conn_polling_received": closeStats["rcvd"],
		"pid_collisions":               pidCollisions,

		"tcp_sent_miscounts":         int64(telemetry.Tcp_sent_miscounts),
		"missed_tcp_close":           int64(telemetry.Missed_tcp_close),
		"missed_udp_close":           int64(telemetry.Missed_udp_close),
		"udp_sends_processed":        int64(telemetry.Udp_sends_processed),
		"udp_sends_missed":           int64(telemetry.Udp_sends_missed),
		"conn_stats_max_entries_hit": int64(telemetry.Conn_stats_max_entries_hit),
	}
}

func initializePortBindingMaps(config *config.Config, m *manager.Manager) error {
	if tcpPorts, err := network.ReadInitialState(config.ProcRoot, network.TCP, config.CollectIPv6Conns); err != nil {
		return fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	} else {
		tcpPortMap, _, err := m.GetMap(string(probes.PortBindingsMap))
		if err != nil {
			return fmt.Errorf("failed to get TCP port binding map: %w", err)
		}
		for p := range tcpPorts {
			log.Debugf("adding initial TCP port binding: netns: %d port: %d", p.Ino, p.Port)
			pb := netebpf.PortBinding{Netns: p.Ino, Port: p.Port}
			state := netebpf.PortListening
			err = tcpPortMap.Update(unsafe.Pointer(&pb), unsafe.Pointer(&state), ebpf.UpdateNoExist)
			if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
				return fmt.Errorf("failed to update TCP port binding map: %w", err)
			}
		}
	}

	if udpPorts, err := network.ReadInitialState(config.ProcRoot, network.UDP, config.CollectIPv6Conns); err != nil {
		return fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	} else {
		udpPortMap, _, err := m.GetMap(string(probes.UdpPortBindingsMap))
		if err != nil {
			return fmt.Errorf("failed to get UDP port binding map: %w", err)
		}
		for p := range udpPorts {
			log.Debugf("adding initial UDP port binding: netns: %d port: %d", p.Ino, p.Port)
			// UDP port bindings currently do not have network namespace numbers
			pb := netebpf.PortBinding{Netns: 0, Port: p.Port}
			state := netebpf.PortListening
			err = udpPortMap.Update(unsafe.Pointer(&pb), unsafe.Pointer(&state), ebpf.UpdateNoExist)
			if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
				return fmt.Errorf("failed to update UDP port binding map: %w", err)
			}
		}
	}
	return nil
}

// getTCPStats reads tcp related stats for the given ConnTuple
func (t *kprobeTracer) getTCPStats(tuple *netebpf.ConnTuple, seen map[netebpf.ConnTuple]struct{}) *netebpf.TCPStats {
	stats := new(netebpf.TCPStats)
	if tuple.Type() != netebpf.TCP {
		return stats
	}

	// The PID isn't used as a key in the stats map, we will temporarily set it to 0 here and reset it when we're done
	pid := tuple.Pid
	tuple.Pid = 0

	err := t.tcpStats.Lookup(unsafe.Pointer(tuple), unsafe.Pointer(stats))
	if err == nil {
		// This is required to avoid (over)reporting retransmits for connections sharing the same socket.
		if _, reported := seen[*tuple]; reported {
			atomic.AddInt64(&t.pidCollisions, 1)
			stats.Retransmits = 0
		} else {
			seen[*tuple] = struct{}{}
		}
	}

	tuple.Pid = pid
	return stats
}

func connStats(t *netebpf.ConnTuple, s *netebpf.ConnStats, tcpStats *netebpf.TCPStats) network.ConnectionStats {
	stats := network.ConnectionStats{
		Pid:                  t.Pid,
		NetNS:                t.Netns,
		Source:               t.SourceAddress(),
		Dest:                 t.DestAddress(),
		SPort:                t.Sport,
		DPort:                t.Dport,
		SPortIsEphemeral:     network.IsPortInEphemeralRange(t.Sport),
		MonotonicSentBytes:   s.Sent_bytes,
		MonotonicRecvBytes:   s.Recv_bytes,
		MonotonicSentPackets: s.Sent_packets,
		MonotonicRecvPackets: s.Recv_packets,
		LastUpdateEpoch:      s.Timestamp,
		IsAssured:            s.IsAssured(),
	}

	if t.Type() == netebpf.TCP {
		stats.Type = network.TCP
		stats.MonotonicRetransmits = tcpStats.Retransmits
		stats.MonotonicTCPEstablished = uint32(tcpStats.State_transitions >> netebpf.Established & 1)
		stats.MonotonicTCPClosed = uint32(tcpStats.State_transitions >> netebpf.Close & 1)
		stats.RTT = tcpStats.Rtt
		stats.RTTVar = tcpStats.Rtt_var
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

	return stats
}
