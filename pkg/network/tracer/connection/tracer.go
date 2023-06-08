// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math"
	"sync"
	"time"
	"unsafe"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/twmb/murmur3"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/fentry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TracerType is the type of the underlying tracer
type TracerType int

const (
	TracerTypeKProbePrebuilt TracerType = iota
	TracerTypeKProbeRuntimeCompiled
	TracerTypeKProbeCORE
	TracerTypeFentry
)

// Tracer is the common interface implemented by all connection tracers.
type Tracer interface {
	// Start begins collecting network connection data.
	Start(func([]network.ConnectionStats)) error
	// Stop halts all network data collection.
	Stop()
	// GetConnections returns the list of currently active connections, using the buffer provided.
	// The optional filter function is used to prevent unwanted connections from being returned and consuming resources.
	GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error
	// FlushPending forces any closed connections waiting for batching to be processed immediately.
	FlushPending()
	// Remove deletes the connection from tracking state.
	// It does not prevent the connection from re-appearing later, if additional traffic occurs.
	Remove(conn *network.ConnectionStats) error
	// RefreshProbeTelemetry sets the prometheus objects to the current values of the underlying stats
	RefreshProbeTelemetry()
	// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
	// An individual tracer implementation may choose which maps to expose via this function.
	GetMap(string) *ebpf.Map
	// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
	DumpMaps(maps ...string) (string, error)
	// Type returns the type of the underlying ebpf tracer that is currently loaded
	Type() TracerType

	Pause() error
	Resume() error
}

const (
	defaultClosedChannelSize = 500
	connTracerModuleName     = "network_tracer__ebpf"
)

var ConnTracerTelemetry = struct {
	connections       telemetry.Gauge
	tcpFailedConnects *nettelemetry.StatCounterWrapper
	TcpSentMiscounts  *nettelemetry.StatCounterWrapper
	unbatchedTcpClose *nettelemetry.StatCounterWrapper
	unbatchedUdpClose *nettelemetry.StatCounterWrapper
	UdpSendsProcessed *nettelemetry.StatCounterWrapper
	UdpSendsMissed    *nettelemetry.StatCounterWrapper
	UdpDroppedConns   *nettelemetry.StatCounterWrapper
	PidCollisions     *nettelemetry.StatCounterWrapper
}{
	telemetry.NewGauge(connTracerModuleName, "connections", []string{"ip_proto", "family"}, "Gauge measuring the number of active connections in the EBPF map"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "tcp_failed_connects", []string{}, "Counter measuring the number of failed TCP connections in the EBPF map"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "tcp_sent_miscounts", []string{}, "Counter measuring the number of miscounted tcp sends in the EBPF map"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "unbatched_tcp_close", []string{}, "Counter measuring the number of missed TCP close events in the EBPF map"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "unbatched_udp_close", []string{}, "Counter measuring the number of missed UDP close events in the EBPF map"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "udp_sends_processed", []string{}, "Counter measuring the number of processed UDP sends in EBPF"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "udp_sends_missed", []string{}, "Counter measuring failures to process UDP sends in EBPF"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "udp_dropped_conns", []string{}, "Counter measuring the number of dropped UDP connections in the EBPF map"),
	nettelemetry.NewStatCounterWrapper(connTracerModuleName, "pid_collisions", []string{}, "Counter measuring number of process collisions"),
}

type tracer struct {
	m *manager.Manager

	conns    *ebpf.Map
	tcpStats *ebpf.Map
	config   *config.Config

	// tcp_close events
	closeConsumer *tcpCloseConsumer

	removeTuple *netebpf.ConnTuple

	closeTracer func()
	stopOnce    sync.Once

	ebpfTracerType TracerType

	exitTelemetry chan struct{}

	ch *cookieHasher
}

// NewTracer creates a new tracer
func NewTracer(config *config.Config, bpfTelemetry *errtelemetry.EBPFTelemetry) (Tracer, error) {
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
			probes.ConnMap:                           {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.TCPStatsMap:                       {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.PortBindingsMap:                   {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.UDPPortBindingsMap:                {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.SockByPidFDMap:                    {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.PidFDBySockMap:                    {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.ConnectionProtocolMap:             {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.ConnectionTupleToSocketSKBConnMap: {Type: ebpf.Hash, MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
		},
		ConstantEditors: []manager.ConstantEditor{
			boolConst("tcpv6_enabled", config.CollectTCPv6Conns),
			boolConst("udpv6_enabled", config.CollectUDPv6Conns),
		},
	}

	closedChannelSize := defaultClosedChannelSize
	if config.ClosedChannelSize > 0 {
		closedChannelSize = config.ClosedChannelSize
	}
	perfHandlerTCP := ddebpf.NewPerfHandler(closedChannelSize)
	m := &manager.Manager{
		DumpHandler: dumpMapsHandler,
	}

	var tracerType TracerType = TracerTypeFentry
	var closeTracerFn func()
	closeTracerFn, err := fentry.LoadTracer(config, m, mgrOptions, perfHandlerTCP)
	if err != nil && !errors.Is(err, fentry.ErrorNotSupported) {
		// failed to load fentry tracer
		return nil, err
	}

	if err != nil {
		// load the kprobe tracer
		log.Info("fentry tracer not supported, falling back to kprobe tracer")
		var kprobeTracerType kprobe.TracerType
		closeTracerFn, kprobeTracerType, err = kprobe.LoadTracer(config, m, mgrOptions, perfHandlerTCP)
		if err != nil {
			return nil, err
		}
		tracerType = TracerType(kprobeTracerType)
	}

	batchMgr, err := newConnBatchManager(m)
	if err != nil {
		return nil, fmt.Errorf("could not create connection batch maanager: %w", err)
	}

	closeConsumer := newTCPCloseConsumer(perfHandlerTCP, batchMgr)

	tr := &tracer{
		m:              m,
		config:         config,
		closeConsumer:  closeConsumer,
		removeTuple:    &netebpf.ConnTuple{},
		closeTracer:    closeTracerFn,
		ebpfTracerType: tracerType,
		exitTelemetry:  make(chan struct{}),
		ch:             newCookieHasher(),
	}

	tr.conns, _, err = m.GetMap(probes.ConnMap)
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.ConnMap, err)
	}

	tr.tcpStats, _, err = m.GetMap(probes.TCPStatsMap)
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.TCPStatsMap, err)
	}

	if bpfTelemetry != nil {
		bpfTelemetry.MapErrMap = tr.GetMap(probes.MapErrTelemetryMap)
		bpfTelemetry.HelperErrMap = tr.GetMap(probes.HelperErrTelemetryMap)
	}

	if err := bpfTelemetry.RegisterEBPFTelemetry(m); err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error registering ebpf telemetry: %v", err)
	}
	go tr.RefreshProbeTelemetry()

	return tr, nil
}

func boolConst(name string, value bool) manager.ConstantEditor {
	c := manager.ConstantEditor{
		Name:  name,
		Value: uint64(1),
	}
	if !value {
		c.Value = uint64(0)
	}

	return c
}

func (t *tracer) Start(callback func([]network.ConnectionStats)) (err error) {
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

func (t *tracer) Pause() error {
	// add small delay for socket filters to properly detach
	time.Sleep(1 * time.Millisecond)
	return t.m.Pause()
}

func (t *tracer) Resume() error {
	err := t.m.Resume()
	// add small delay for socket filters to properly attach
	time.Sleep(1 * time.Millisecond)
	return err
}

func (t *tracer) FlushPending() {
	t.closeConsumer.FlushPending()
}

func (t *tracer) Stop() {
	t.stopOnce.Do(func() {
		close(t.exitTelemetry)
		_ = t.m.Stop(manager.CleanAll)
		t.closeConsumer.Stop()
		if t.closeTracer != nil {
			t.closeTracer()
		}
	})
}

func (t *tracer) GetMap(name string) *ebpf.Map {
	switch name {
	case probes.SockByPidFDMap:
	case probes.ConnectionProtocolMap:
	case probes.MapErrTelemetryMap:
	case probes.HelperErrTelemetryMap:
	default:
		return nil
	}
	m, _, _ := t.m.GetMap(name)
	return m
}

func (t *tracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	// Iterate through all key-value pairs in map
	key, stats := &netebpf.ConnTuple{}, &netebpf.ConnStats{}
	seen := make(map[netebpf.ConnTuple]struct{})

	// Cached objects
	conn := new(network.ConnectionStats)
	tcp := new(netebpf.TCPStats)

	var tcp4, tcp6, udp4, udp6 float64
	entries := t.conns.Iterate()
	for entries.Next(unsafe.Pointer(key), unsafe.Pointer(stats)) {
		populateConnStats(conn, key, stats, t.ch)

		isTCP := conn.Type == network.TCP
		switch conn.Family {
		case network.AFINET6:
			if isTCP {
				tcp6++
			} else {
				udp6++
			}
		case network.AFINET:
			if isTCP {
				tcp4++
			} else {
				udp4++
			}
		}

		if filter != nil && !filter(conn) {
			continue
		}
		if t.getTCPStats(tcp, key, seen) {
			updateTCPStats(conn, stats.Cookie, tcp)
		}
		*buffer.Next() = *conn
	}

	if err := entries.Err(); err != nil {
		return fmt.Errorf("unable to iterate connection map: %s", err)
	}

	updateTelemetry(tcp4, tcp6, udp4, udp6)

	return nil
}

func updateTelemetry(tcp4 float64, tcp6 float64, udp4 float64, udp6 float64) {
	ConnTracerTelemetry.connections.Set(tcp4, "tcp", "v4")
	ConnTracerTelemetry.connections.Set(tcp6, "tcp", "v6")
	ConnTracerTelemetry.connections.Set(udp4, "udp", "v4")
	ConnTracerTelemetry.connections.Set(udp6, "udp", "v6")
}

func removeConnection(conn *network.ConnectionStats) {
	isTCP := conn.Type == network.TCP
	switch conn.Family {
	case network.AFINET6:
		if isTCP {
			ConnTracerTelemetry.connections.Dec("tcp", "v6")
		} else {
			ConnTracerTelemetry.connections.Dec("udp", "v6")
		}
	case network.AFINET:
		if isTCP {
			ConnTracerTelemetry.connections.Dec("tcp", "v4")
		} else {
			ConnTracerTelemetry.connections.Dec("udp", "v4")
		}
	}
}

func (t *tracer) Remove(conn *network.ConnectionStats) error {
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

	removeConnection(conn)

	// We have to remove the PID to remove the element from the TCP Map since we don't use the pid there
	t.removeTuple.Pid = 0
	if conn.Type == network.TCP {
		// We can ignore the error for this map since it will not always contain the entry
		_ = t.tcpStats.Delete(unsafe.Pointer(t.removeTuple))
	}
	return nil
}

func (t *tracer) getEBPFTelemetry() *netebpf.Telemetry {
	var zero uint64
	mp, _, err := t.m.GetMap(probes.TelemetryMap)
	if err != nil {
		log.Warnf("error retrieving telemetry map: %s", err)
		return nil
	}

	telemetry := &netebpf.Telemetry{}
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("error retrieving the telemetry struct: %s", err)
		}
		return nil
	}
	return telemetry
}

func (t *tracer) RefreshProbeTelemetry() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.refreshProbeTelemetry()
		case <-t.exitTelemetry:
			return
		}
	}
}

// Refreshes connection tracer telemetry on a loop
// TODO: Replace with prometheus collector interface
func (t *tracer) refreshProbeTelemetry() {
	ebpfTelemetry := t.getEBPFTelemetry()
	if ebpfTelemetry == nil {
		return
	}

	ConnTracerTelemetry.tcpFailedConnects.Add(int64(ebpfTelemetry.Tcp_failed_connect) - ConnTracerTelemetry.tcpFailedConnects.Load())
	ConnTracerTelemetry.TcpSentMiscounts.Add(int64(ebpfTelemetry.Tcp_sent_miscounts) - ConnTracerTelemetry.TcpSentMiscounts.Load())
	ConnTracerTelemetry.unbatchedTcpClose.Add(int64(ebpfTelemetry.Unbatched_tcp_close) - ConnTracerTelemetry.unbatchedTcpClose.Load())
	ConnTracerTelemetry.unbatchedUdpClose.Add(int64(ebpfTelemetry.Unbatched_udp_close) - ConnTracerTelemetry.unbatchedUdpClose.Load())
	ConnTracerTelemetry.UdpSendsProcessed.Add(int64(ebpfTelemetry.Udp_sends_processed) - ConnTracerTelemetry.UdpSendsProcessed.Load())
	ConnTracerTelemetry.UdpSendsMissed.Add(int64(ebpfTelemetry.Udp_sends_missed) - ConnTracerTelemetry.UdpSendsMissed.Load())
	ConnTracerTelemetry.UdpDroppedConns.Add(int64(ebpfTelemetry.Udp_dropped_conns) - ConnTracerTelemetry.UdpDroppedConns.Load())
}

// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
func (t *tracer) DumpMaps(maps ...string) (string, error) {
	return t.m.DumpMaps(maps...)
}

// Type returns the type of the underlying ebpf tracer that is currently loaded
func (t *tracer) Type() TracerType {
	return t.ebpfTracerType
}

func initializePortBindingMaps(config *config.Config, m *manager.Manager) error {
	tcpPorts, err := network.ReadInitialState(config.ProcRoot, network.TCP, config.CollectTCPv6Conns, true)
	if err != nil {
		return fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	}

	tcpPortMap, _, err := m.GetMap(probes.PortBindingsMap)
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

	udpPorts, err := network.ReadInitialState(config.ProcRoot, network.UDP, config.CollectUDPv6Conns, false)
	if err != nil {
		return fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	}

	udpPortMap, _, err := m.GetMap(probes.UDPPortBindingsMap)
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
	return nil
}

// getTCPStats reads tcp related stats for the given ConnTuple
func (t *tracer) getTCPStats(stats *netebpf.TCPStats, tuple *netebpf.ConnTuple, seen map[netebpf.ConnTuple]struct{}) bool {
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
			ConnTracerTelemetry.PidCollisions.Inc()
			stats.Retransmits = 0
			stats.State_transitions = 0
		} else {
			seen[*tuple] = struct{}{}
		}
	}

	tuple.Pid = pid
	return true
}

func populateConnStats(stats *network.ConnectionStats, t *netebpf.ConnTuple, s *netebpf.ConnStats, ch *cookieHasher) {
	*stats = network.ConnectionStats{
		Pid:    t.Pid,
		NetNS:  t.Netns,
		Source: t.SourceAddress(),
		Dest:   t.DestAddress(),
		SPort:  t.Sport,
		DPort:  t.Dport,
		Monotonic: network.StatCounters{
			SentBytes:   s.Sent_bytes,
			RecvBytes:   s.Recv_bytes,
			SentPackets: s.Sent_packets,
			RecvPackets: s.Recv_packets,
		},
		SPortIsEphemeral: network.IsPortInEphemeralRange(t.Sport),
		LastUpdateEpoch:  s.Timestamp,
		IsAssured:        s.IsAssured(),
		Cookie:           network.StatCookie(s.Cookie),
	}

	stats.ProtocolStack = protocols.Stack{
		Api:         protocols.API(s.Protocol_stack.Api),
		Application: protocols.Application(s.Protocol_stack.Application),
		Encryption:  protocols.Encryption(s.Protocol_stack.Encryption),
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

	if ch != nil {
		ch.Hash(stats)
	}
}

func updateTCPStats(conn *network.ConnectionStats, cookie uint32, tcpStats *netebpf.TCPStats) {
	if conn.Type != network.TCP {
		return
	}

	conn.Monotonic.Retransmits = tcpStats.Retransmits
	conn.Monotonic.TCPEstablished = uint32(tcpStats.State_transitions >> netebpf.Established & 1)
	conn.Monotonic.TCPClosed = uint32(tcpStats.State_transitions >> netebpf.Close & 1)
	conn.RTT = tcpStats.Rtt
	conn.RTTVar = tcpStats.Rtt_var
}

type cookieHasher struct {
	hash hash.Hash64
	buf  []byte
}

func newCookieHasher() *cookieHasher {
	return &cookieHasher{
		hash: murmur3.New64(),
		buf:  make([]byte, network.ConnectionByteKeyMaxLen),
	}
}

func (h *cookieHasher) Hash(stats *network.ConnectionStats) {
	h.hash.Reset()
	if err := binary.Write(h.hash, binary.BigEndian, stats.Cookie); err != nil {
		log.Errorf("error writing cookie to hash: %s", err)
		return
	}
	key := stats.ByteKey(h.buf)
	if _, err := h.hash.Write(key); err != nil {
		log.Errorf("error writing byte key to hash: %s", err)
		return
	}
	stats.Cookie = h.hash.Sum64()
}
