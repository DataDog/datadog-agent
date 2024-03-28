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
	"io"
	"math"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/murmur3"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/fentry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TracerType is the type of the underlying tracer
type TracerType int

const (
	//nolint:revive // TODO(NET) Fix revive linter
	TracerTypeKProbePrebuilt TracerType = iota
	//nolint:revive // TODO(NET) Fix revive linter
	TracerTypeKProbeRuntimeCompiled
	//nolint:revive // TODO(NET) Fix revive linter
	TracerTypeKProbeCORE
	//nolint:revive // TODO(NET) Fix revive linter
	TracerTypeFentry
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the `accept` syscall).
	maxActive = 128
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
	// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
	// An individual tracer implementation may choose which maps to expose via this function.
	GetMap(string) *ebpf.Map
	// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
	DumpMaps(w io.Writer, maps ...string) error
	// Type returns the type of the underlying ebpf tracer that is currently loaded
	Type() TracerType

	Pause() error
	Resume() error

	// Describe returns all descriptions of the collector
	Describe(descs chan<- *prometheus.Desc)
	// Collect returns the current state of all metrics of the collector
	Collect(metrics chan<- prometheus.Metric)
}

const (
	defaultClosedChannelSize = 500
	connTracerModuleName     = "network_tracer__ebpf"
)

//nolint:revive // TODO(NET) Fix revive linter
var ConnTracerTelemetry = struct {
	connections       telemetry.Gauge
	tcpFailedConnects *prometheus.Desc
	//nolint:revive // TODO(NET) Fix revive linter
	TcpSentMiscounts *prometheus.Desc
	//nolint:revive // TODO(NET) Fix revive linter
	unbatchedTcpClose *prometheus.Desc
	//nolint:revive // TODO(NET) Fix revive linter
	unbatchedUdpClose *prometheus.Desc
	//nolint:revive // TODO(NET) Fix revive linter
	UdpSendsProcessed *prometheus.Desc
	//nolint:revive // TODO(NET) Fix revive linter
	UdpSendsMissed *prometheus.Desc
	//nolint:revive // TODO(NET) Fix revive linter
	UdpDroppedConns *prometheus.Desc
	PidCollisions   *telemetry.StatCounterWrapper
	iterationDups   telemetry.Counter
	iterationAborts telemetry.Counter

	//nolint:revive // TODO(NET) Fix revive linter
	lastTcpFailedConnects *atomic.Int64
	//nolint:revive // TODO(NET) Fix revive linter
	LastTcpSentMiscounts *atomic.Int64
	//nolint:revive // TODO(NET) Fix revive linter
	lastUnbatchedTcpClose *atomic.Int64
	//nolint:revive // TODO(NET) Fix revive linter
	lastUnbatchedUdpClose *atomic.Int64
	//nolint:revive // TODO(NET) Fix revive linter
	lastUdpSendsProcessed *atomic.Int64
	//nolint:revive // TODO(NET) Fix revive linter
	lastUdpSendsMissed *atomic.Int64
	//nolint:revive // TODO(NET) Fix revive linter
	lastUdpDroppedConns *atomic.Int64
}{
	telemetry.NewGauge(connTracerModuleName, "connections", []string{"ip_proto", "family"}, "Gauge measuring the number of active connections in the EBPF map"),
	prometheus.NewDesc(connTracerModuleName+"__tcp_failed_connects", "Counter measuring the number of failed TCP connections in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_sent_miscounts", "Counter measuring the number of miscounted tcp sends in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__unbatched_tcp_close", "Counter measuring the number of missed TCP close events in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__unbatched_udp_close", "Counter measuring the number of missed UDP close events in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_sends_processed", "Counter measuring the number of processed UDP sends in EBPF", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_sends_missed", "Counter measuring failures to process UDP sends in EBPF", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_dropped_conns", "Counter measuring the number of dropped UDP connections in the EBPF map", nil, nil),
	telemetry.NewStatCounterWrapper(connTracerModuleName, "pid_collisions", []string{}, "Counter measuring number of process collisions"),
	telemetry.NewCounter(connTracerModuleName, "iteration_dups", []string{}, "Counter measuring the number of connections iterated more than once"),
	telemetry.NewCounter(connTracerModuleName, "iteration_aborts", []string{}, "Counter measuring how many times ebpf iteration of connection map was aborted"),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
}

type tracer struct {
	m *manager.Manager

	conns          *maps.GenericMap[netebpf.ConnTuple, netebpf.ConnStats]
	tcpStats       *maps.GenericMap[netebpf.ConnTuple, netebpf.TCPStats]
	tcpRetransmits *maps.GenericMap[netebpf.ConnTuple, uint32]
	config         *config.Config

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
func NewTracer(config *config.Config) (Tracer, error) {
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
			probes.ConnMap:                           {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.TCPStatsMap:                       {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.TCPRetransmitsMap:                 {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.PortBindingsMap:                   {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.UDPPortBindingsMap:                {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.ConnectionProtocolMap:             {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.ConnectionTupleToSocketSKBConnMap: {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
		},
		ConstantEditors: []manager.ConstantEditor{
			boolConst("tcpv6_enabled", config.CollectTCPv6Conns),
			boolConst("udpv6_enabled", config.CollectUDPv6Conns),
		},
		VerifierOptions: ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogSize: 10 * 1024 * 1024,
			},
		},
		DefaultKProbeMaxActive: maxActive,
	}

	begin, end := network.EphemeralRange()
	mgrOptions.ConstantEditors = append(mgrOptions.ConstantEditors,
		manager.ConstantEditor{Name: "ephemeral_range_begin", Value: uint64(begin)},
		manager.ConstantEditor{Name: "ephemeral_range_end", Value: uint64(end)})

	closedChannelSize := defaultClosedChannelSize
	if config.ClosedChannelSize > 0 {
		closedChannelSize = config.ClosedChannelSize
	}
	var connCloseEventHandler ddebpf.EventHandler
	if config.RingBufferSupportedNPM() {
		connCloseEventHandler = ddebpf.NewRingBufferHandler(closedChannelSize)
	} else {
		connCloseEventHandler = ddebpf.NewPerfHandler(closedChannelSize)
	}
	var m *manager.Manager
	//nolint:revive // TODO(NET) Fix revive linter
	var tracerType TracerType = TracerTypeFentry
	var closeTracerFn func()
	m, closeTracerFn, err := fentry.LoadTracer(config, mgrOptions, connCloseEventHandler)
	if err != nil && !errors.Is(err, fentry.ErrorNotSupported) {
		// failed to load fentry tracer
		return nil, err
	}

	if err != nil {
		// load the kprobe tracer
		log.Info("fentry tracer not supported, falling back to kprobe tracer")
		var kprobeTracerType kprobe.TracerType
		m, closeTracerFn, kprobeTracerType, err = kprobe.LoadTracer(config, mgrOptions, connCloseEventHandler)
		if err != nil {
			return nil, err
		}
		tracerType = TracerType(kprobeTracerType)
	}
	m.DumpHandler = dumpMapsHandler
	ebpfcheck.AddNameMappings(m, "npm_tracer")

	batchMgr, err := newConnBatchManager(m)
	if err != nil {
		return nil, fmt.Errorf("could not create connection batch manager: %w", err)
	}

	closeConsumer := newTCPCloseConsumer(connCloseEventHandler, batchMgr)

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

	tr.conns, err = maps.GetMap[netebpf.ConnTuple, netebpf.ConnStats](m, probes.ConnMap)
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.ConnMap, err)
	}

	tr.tcpStats, err = maps.GetMap[netebpf.ConnTuple, netebpf.TCPStats](m, probes.TCPStatsMap)
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.TCPStatsMap, err)
	}

	if tr.tcpRetransmits, err = maps.GetMap[netebpf.ConnTuple, uint32](m, probes.TCPRetransmitsMap); err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.TCPRetransmitsMap, err)
	}

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
		ebpfcheck.RemoveNameMappings(t.m)
		ebpftelemetry.UnregisterTelemetry(t.m)
		_ = t.m.Stop(manager.CleanAll)
		t.closeConsumer.Stop()
		if t.closeTracer != nil {
			t.closeTracer()
		}
	})
}

func (t *tracer) GetMap(name string) *ebpf.Map {
	switch name {
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
	// connsByTuple is used to detect whether we are iterating over
	// a connection we have previously seen. This can happen when
	// ebpf maps are being iterated over and deleted at the same time.
	// The iteration can reset when that happens.
	// See https://justin.azoff.dev/blog/bpf_map_get_next_key-pitfalls/
	connsByTuple := make(map[netebpf.ConnTuple]struct{})

	// Cached objects
	conn := new(network.ConnectionStats)
	tcp := new(netebpf.TCPStats)

	var tcp4, tcp6, udp4, udp6 float64
	entries := t.conns.Iterate()
	for entries.Next(key, stats) {
		if _, exists := connsByTuple[*key]; exists {
			// already seen the connection in current batch processing,
			// due to race between the iterator and bpf_map_delete
			ConnTracerTelemetry.iterationDups.Inc()
			continue
		}

		populateConnStats(conn, key, stats, t.ch)
		connsByTuple[*key] = struct{}{}

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

		if t.getTCPStats(tcp, key) {
			updateTCPStats(conn, tcp, 0)
		}
		if retrans, ok := t.getTCPRetransmits(key, seen); ok {
			updateTCPStats(conn, nil, retrans)
		}

		*buffer.Next() = *conn
	}

	if err := entries.Err(); err != nil {
		if !errors.Is(err, ebpf.ErrIterationAborted) {
			return fmt.Errorf("unable to iterate connection map: %w", err)
		}

		log.Warn("eBPF conn_stats map iteration aborted. Some connections may not be reported")
		ConnTracerTelemetry.iterationAborts.Inc()
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

	err := t.conns.Delete(t.removeTuple)
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
		_ = t.tcpStats.Delete(t.removeTuple)
	}
	return nil
}

func (t *tracer) getEBPFTelemetry() *netebpf.Telemetry {
	var zero uint32
	mp, err := maps.GetMap[uint32, netebpf.Telemetry](t.m, probes.TelemetryMap)
	if err != nil {
		log.Warnf("error retrieving telemetry map: %s", err)
		return nil
	}

	tm := &netebpf.Telemetry{}
	if err := mp.Lookup(&zero, tm); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("error retrieving the telemetry struct: %s", err)
		}
		return nil
	}
	return tm
}

// Describe returns all descriptions of the collector
func (t *tracer) Describe(ch chan<- *prometheus.Desc) {
	ch <- ConnTracerTelemetry.tcpFailedConnects
	ch <- ConnTracerTelemetry.TcpSentMiscounts
	ch <- ConnTracerTelemetry.unbatchedTcpClose
	ch <- ConnTracerTelemetry.unbatchedUdpClose
	ch <- ConnTracerTelemetry.UdpSendsProcessed
	ch <- ConnTracerTelemetry.UdpSendsMissed
	ch <- ConnTracerTelemetry.UdpDroppedConns
}

// Collect returns the current state of all metrics of the collector
func (t *tracer) Collect(ch chan<- prometheus.Metric) {
	ebpfTelemetry := t.getEBPFTelemetry()
	if ebpfTelemetry == nil {
		return
	}
	delta := int64(ebpfTelemetry.Tcp_failed_connect) - ConnTracerTelemetry.lastTcpFailedConnects.Load()
	ConnTracerTelemetry.lastTcpFailedConnects.Store(int64(ebpfTelemetry.Tcp_failed_connect))
	ch <- prometheus.MustNewConstMetric(ConnTracerTelemetry.tcpFailedConnects, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_sent_miscounts) - ConnTracerTelemetry.LastTcpSentMiscounts.Load()
	ConnTracerTelemetry.LastTcpSentMiscounts.Store(int64(ebpfTelemetry.Tcp_sent_miscounts))
	ch <- prometheus.MustNewConstMetric(ConnTracerTelemetry.TcpSentMiscounts, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Unbatched_tcp_close) - ConnTracerTelemetry.lastUnbatchedTcpClose.Load()
	ConnTracerTelemetry.lastUnbatchedTcpClose.Store(int64(ebpfTelemetry.Unbatched_tcp_close))
	ch <- prometheus.MustNewConstMetric(ConnTracerTelemetry.unbatchedTcpClose, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Unbatched_udp_close) - ConnTracerTelemetry.lastUnbatchedUdpClose.Load()
	ConnTracerTelemetry.lastUnbatchedUdpClose.Store(int64(ebpfTelemetry.Unbatched_udp_close))
	ch <- prometheus.MustNewConstMetric(ConnTracerTelemetry.unbatchedUdpClose, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_sends_processed) - ConnTracerTelemetry.lastUdpSendsProcessed.Load()
	ConnTracerTelemetry.lastUdpSendsProcessed.Store(int64(ebpfTelemetry.Udp_sends_processed))
	ch <- prometheus.MustNewConstMetric(ConnTracerTelemetry.UdpSendsProcessed, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_sends_missed) - ConnTracerTelemetry.lastUdpSendsMissed.Load()
	ConnTracerTelemetry.lastUdpSendsMissed.Store(int64(ebpfTelemetry.Udp_sends_missed))
	ch <- prometheus.MustNewConstMetric(ConnTracerTelemetry.UdpSendsMissed, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_dropped_conns) - ConnTracerTelemetry.lastUdpDroppedConns.Load()
	ConnTracerTelemetry.lastUdpDroppedConns.Store(int64(ebpfTelemetry.Udp_dropped_conns))
	ch <- prometheus.MustNewConstMetric(ConnTracerTelemetry.UdpDroppedConns, prometheus.CounterValue, float64(delta))
}

// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
func (t *tracer) DumpMaps(w io.Writer, maps ...string) error {
	return t.m.DumpMaps(w, maps...)
}

// Type returns the type of the underlying ebpf tracer that is currently loaded
func (t *tracer) Type() TracerType {
	return t.ebpfTracerType
}

func initializePortBindingMaps(config *config.Config, m *manager.Manager) error {
	tcpPorts, err := network.ReadInitialState(config.ProcRoot, network.TCP, config.CollectTCPv6Conns)
	if err != nil {
		return fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	}

	tcpPortMap, err := maps.GetMap[netebpf.PortBinding, uint32](m, probes.PortBindingsMap)
	if err != nil {
		return fmt.Errorf("failed to get TCP port binding map: %w", err)
	}
	for p, count := range tcpPorts {
		log.Debugf("adding initial TCP port binding: netns: %d port: %d", p.Ino, p.Port)
		pb := netebpf.PortBinding{Netns: p.Ino, Port: p.Port}
		err = tcpPortMap.Update(&pb, &count, ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to update TCP port binding map: %w", err)
		}
	}

	udpPorts, err := network.ReadInitialState(config.ProcRoot, network.UDP, config.CollectUDPv6Conns)
	if err != nil {
		return fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	}

	udpPortMap, err := maps.GetMap[netebpf.PortBinding, uint32](m, probes.UDPPortBindingsMap)
	if err != nil {
		return fmt.Errorf("failed to get UDP port binding map: %w", err)
	}
	for p, count := range udpPorts {
		// ignore ephemeral port binds as they are more likely to be from
		// clients calling bind with port 0
		if network.IsPortInEphemeralRange(network.AFINET, network.UDP, p.Port) == network.EphemeralTrue {
			log.Debugf("ignoring initial ephemeral UDP port bind to %d", p)
			continue
		}

		log.Debugf("adding initial UDP port binding: netns: %d port: %d", p.Ino, p.Port)
		pb := netebpf.PortBinding{Netns: p.Ino, Port: p.Port}
		err = udpPortMap.Update(&pb, &count, ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to update UDP port binding map: %w", err)
		}
	}
	return nil
}

func (t *tracer) getTCPRetransmits(tuple *netebpf.ConnTuple, seen map[netebpf.ConnTuple]struct{}) (uint32, bool) {
	if tuple.Type() != netebpf.TCP {
		return 0, false
	}

	// The PID isn't used as a key in the stats map, we will temporarily set it to 0 here and reset it when we're done
	pid := tuple.Pid
	tuple.Pid = 0

	var retransmits uint32
	if err := t.tcpRetransmits.Lookup(tuple, &retransmits); err == nil {
		// This is required to avoid (over)reporting retransmits for connections sharing the same socket.
		if _, reported := seen[*tuple]; reported {
			ConnTracerTelemetry.PidCollisions.Inc()
			retransmits = 0
		} else {
			seen[*tuple] = struct{}{}
		}
	}

	tuple.Pid = pid
	return retransmits, true
}

// getTCPStats reads tcp related stats for the given ConnTuple
func (t *tracer) getTCPStats(stats *netebpf.TCPStats, tuple *netebpf.ConnTuple) bool {
	if tuple.Type() != netebpf.TCP {
		return false
	}

	return t.tcpStats.Lookup(tuple, stats) == nil
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
			SentPackets: uint64(s.Sent_packets),
			RecvPackets: uint64(s.Recv_packets),
		},
		LastUpdateEpoch: s.Timestamp,
		IsAssured:       s.IsAssured(),
		Cookie:          network.StatCookie(s.Cookie),
	}

	if s.Duration <= uint64(math.MaxInt64) {
		stats.Duration = time.Duration(s.Duration) * time.Nanosecond
	}

	stats.ProtocolStack = protocols.Stack{
		API:         protocols.API(s.Protocol_stack.Api),
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

	stats.SPortIsEphemeral = network.IsPortInEphemeralRange(stats.Family, stats.Type, t.Sport)

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

func updateTCPStats(conn *network.ConnectionStats, tcpStats *netebpf.TCPStats, retransmits uint32) {
	if conn.Type != network.TCP {
		return
	}

	conn.Monotonic.Retransmits = retransmits
	if tcpStats != nil {
		conn.Monotonic.TCPEstablished = uint32(tcpStats.State_transitions >> netebpf.Established & 1)
		conn.Monotonic.TCPClosed = uint32(tcpStats.State_transitions >> netebpf.Close & 1)
		conn.RTT = tcpStats.Rtt
		conn.RTTVar = tcpStats.Rtt_var
	}
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
