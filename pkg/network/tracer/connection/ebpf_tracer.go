// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/failure"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/fentry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultClosedChannelSize = 500
	defaultFailedChannelSize = 500
	connTracerModuleName     = "network_tracer__ebpf"
)

var tcpOngoingConnectMapTTL = 30 * time.Minute.Nanoseconds()

var EbpfTracerTelemetry = struct {
	connections       telemetry.Gauge
	tcpFailedConnects *prometheus.Desc
	TcpSentMiscounts  *prometheus.Desc
	unbatchedTcpClose *prometheus.Desc
	unbatchedUdpClose *prometheus.Desc
	UdpSendsProcessed *prometheus.Desc
	UdpSendsMissed    *prometheus.Desc
	UdpDroppedConns   *prometheus.Desc
	// doubleFlushAttemptsClose is a counter measuring the number of attempts to flush a closed connection twice from tcp_close
	doubleFlushAttemptsClose *prometheus.Desc
	// doubleFlushAttemptsDone is a counter measuring the number of attempts to flush a closed connection twice from tcp_done
	doubleFlushAttemptsDone *prometheus.Desc
	// unsupportedTcpFailures is a counter measuring the number of attempts to flush a TCP failure that is not supported
	unsupportedTcpFailures *prometheus.Desc
	// tcpDoneMissingPid is a counter measuring the number of TCP connections with a PID mismatch between tcp_connect and tcp_done
	tcpDoneMissingPid           *prometheus.Desc
	tcpConnectFailedTuple       *prometheus.Desc
	tcpDoneFailedTuple          *prometheus.Desc
	tcpFinishConnectFailedTuple *prometheus.Desc
	tcpCloseTargetFailures      *prometheus.Desc
	tcpDoneConnectionFlush      *prometheus.Desc
	tcpCloseConnectionFlush     *prometheus.Desc
	ongoingConnectPidCleaned    telemetry.Counter
	PidCollisions               *telemetry.StatCounterWrapper
	iterationDups               telemetry.Counter
	iterationAborts             telemetry.Counter

	lastTcpFailedConnects *atomic.Int64
	LastTcpSentMiscounts  *atomic.Int64
	lastUnbatchedTcpClose *atomic.Int64
	lastUnbatchedUdpClose *atomic.Int64
	lastUdpSendsProcessed *atomic.Int64
	lastUdpSendsMissed    *atomic.Int64
	lastUdpDroppedConns   *atomic.Int64
	// lastDoubleFlushAttemptsClose is a counter measuring the diff between the last two values of doubleFlushAttemptsClose
	lastDoubleFlushAttemptsClose *atomic.Int64
	// lastDoubleFlushAttemptsDone is a counter measuring the diff between the last two values of doubleFlushAttemptsDone
	lastDoubleFlushAttemptsDone *atomic.Int64
	// lastUnsupportedTcpFailures is a counter measuring the diff between the last two values of unsupportedTcpFailures
	lastUnsupportedTcpFailures *atomic.Int64
	// lastTcpDoneMissingPid is a counter measuring the diff between the last two values of tcpDoneMissingPid
	lastTcpDoneMissingPid           *atomic.Int64
	lastTcpConnectFailedTuple       *atomic.Int64
	lastTcpDoneFailedTuple          *atomic.Int64
	lastTcpFinishConnectFailedTuple *atomic.Int64
	lastTcpCloseTargetFailures      *atomic.Int64
	lastTcpDoneConnectionFlush      *atomic.Int64
	lastTcpCloseConnectionFlush     *atomic.Int64
}{
	telemetry.NewGauge(connTracerModuleName, "connections", []string{"ip_proto", "family"}, "Gauge measuring the number of active connections in the EBPF map"),
	prometheus.NewDesc(connTracerModuleName+"__tcp_failed_connects", "Counter measuring the number of failed TCP connections in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_sent_miscounts", "Counter measuring the number of miscounted tcp sends in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__unbatched_tcp_close", "Counter measuring the number of missed TCP close events in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__unbatched_udp_close", "Counter measuring the number of missed UDP close events in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_sends_processed", "Counter measuring the number of processed UDP sends in EBPF", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_sends_missed", "Counter measuring failures to process UDP sends in EBPF", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_dropped_conns", "Counter measuring the number of dropped UDP connections in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__double_flush_attempts_close", "Counter measuring the number of attempts to flush a closed connection twice from tcp_close", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__double_flush_attempts_done", "Counter measuring the number of attempts to flush a closed connection twice from tcp_done", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__unsupported_tcp_failures", "Counter measuring the number of attempts to flush a TCP failure that is not supported", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_done_missing_pid", "Counter measuring the number of TCP connections with a missing PID in tcp_done", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_connect_failed_tuple", "Counter measuring the number of failed TCP connections due to tuple collisions", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_done_failed_tuple", "Counter measuring the number of failed TCP connections due to tuple collisions", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_finish_connect_failed_tuple", "Counter measuring the number of failed TCP connections due to tuple collisions", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_close_target_failures", "Counter measuring the number of failed TCP connections in tcp_close", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_done_connection_flush", "Counter measuring the number of connection flushes performed in tcp_done", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_close_connection_flush", "Counter measuring the number of connection flushes performed in tcp_close", nil, nil),
	telemetry.NewCounter(connTracerModuleName, "ongoing_connect_pid_cleaned", []string{}, "Counter measuring the number of tcp_ongoing_connect_pid entries cleaned in userspace"),
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
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
	atomic.NewInt64(0),
}

type ebpfTracer struct {
	m *manager.Manager

	conns          *maps.GenericMap[netebpf.ConnTuple, netebpf.ConnStats]
	tcpStats       *maps.GenericMap[netebpf.ConnTuple, netebpf.TCPStats]
	tcpRetransmits *maps.GenericMap[netebpf.ConnTuple, uint32]
	config         *config.Config

	// tcp_close events
	closeConsumer *tcpCloseConsumer
	// tcp failure events
	failedConnConsumer *failure.TCPFailedConnConsumer

	// periodically clean the ongoing connection pid map
	ongoingConnectCleaner *ddebpf.MapCleaner[netebpf.SkpConn, netebpf.PidTs]

	removeTuple *netebpf.ConnTuple

	closeTracer func()
	stopOnce    sync.Once

	ebpfTracerType TracerType

	exitTelemetry chan struct{}

	ch *cookieHasher
}

// NewTracer creates a new tracer
func newEbpfTracer(config *config.Config, _ telemetryComponent.Component) (Tracer, error) {
	if _, err := tracefs.Root(); err != nil {
		return nil, fmt.Errorf("eBPF based network tracer unsupported: %s", err)
	}

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
			probes.TCPOngoingConnectPid:              {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
		},
		ConstantEditors: []manager.ConstantEditor{
			boolConst("tcpv6_enabled", config.CollectTCPv6Conns),
			boolConst("udpv6_enabled", config.CollectUDPv6Conns),
		},
		DefaultKProbeMaxActive: maxActive,
		BypassEnabled:          config.BypassEnabled,
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
	var failedConnsHandler ddebpf.EventHandler
	if config.RingBufferSupportedNPM() {
		connCloseEventHandler = ddebpf.NewRingBufferHandler(closedChannelSize)
		failedConnsHandler = ddebpf.NewRingBufferHandler(defaultFailedChannelSize)
	} else {
		connCloseEventHandler = ddebpf.NewPerfHandler(closedChannelSize)
		failedConnsHandler = ddebpf.NewPerfHandler(defaultFailedChannelSize)
	}

	var m *manager.Manager
	var tracerType TracerType = TracerTypeFentry
	var closeTracerFn func()
	m, closeTracerFn, err := fentry.LoadTracer(config, mgrOptions, connCloseEventHandler)
	if err != nil && !errors.Is(err, fentry.ErrorNotSupported) {
		// failed to load fentry tracer
		return nil, err
	}

	if err != nil {
		// load the kprobe tracer
		log.Info("loading kprobe-based tracer")
		var kprobeTracerType kprobe.TracerType
		m, closeTracerFn, kprobeTracerType, err = kprobe.LoadTracer(config, mgrOptions, connCloseEventHandler, failedConnsHandler)
		if err != nil {
			return nil, err
		}
		tracerType = TracerType(kprobeTracerType)
	}
	m.DumpHandler = dumpMapsHandler
	ddebpf.AddNameMappings(m, "npm_tracer")

	numCPUs, err := ebpf.PossibleCPU()
	if err != nil {
		return nil, fmt.Errorf("could not determine number of CPUs: %w", err)
	}
	extractor := newBatchExtractor(numCPUs)
	batchMgr, err := newConnBatchManager(m, extractor)
	if err != nil {
		return nil, fmt.Errorf("could not create connection batch manager: %w", err)
	}

	closeConsumer := newTCPCloseConsumer(connCloseEventHandler, batchMgr)

	var failedConnConsumer *failure.TCPFailedConnConsumer
	// Failed connections are not supported on prebuilt
	if tracerType == TracerTypeKProbePrebuilt {
		config.TCPFailedConnectionsEnabled = false
	}
	if config.FailedConnectionsSupported() {
		failedConnConsumer = failure.NewFailedConnConsumer(failedConnsHandler, m, config.MaxFailedConnectionsBuffered)
	}

	tr := &ebpfTracer{
		m:                  m,
		config:             config,
		closeConsumer:      closeConsumer,
		failedConnConsumer: failedConnConsumer,
		removeTuple:        &netebpf.ConnTuple{},
		closeTracer:        closeTracerFn,
		ebpfTracerType:     tracerType,
		exitTelemetry:      make(chan struct{}),
		ch:                 newCookieHasher(),
	}

	tr.setupMapCleaner(m)

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

func (t *ebpfTracer) Start(callback func(*network.ConnectionStats)) (err error) {
	defer func() {
		if err != nil {
			t.Stop()
		}
	}()

	err = t.initializePortBindingMaps()
	if err != nil {
		return fmt.Errorf("error initializing port binding maps: %s", err)
	}

	if err := t.m.Start(); err != nil {
		return fmt.Errorf("could not start ebpf manager: %s", err)
	}

	t.closeConsumer.Start(callback)
	t.failedConnConsumer.Start()
	return nil
}

func (t *ebpfTracer) Pause() error {
	// add small delay for socket filters to properly detach
	time.Sleep(1 * time.Millisecond)
	return t.m.Pause()
}

func (t *ebpfTracer) Resume() error {
	err := t.m.Resume()
	// add small delay for socket filters to properly attach
	time.Sleep(1 * time.Millisecond)
	return err
}

func (t *ebpfTracer) FlushPending() {
	t.closeConsumer.FlushPending()
}

func (t *ebpfTracer) GetFailedConnections() *failure.FailedConns {
	if t.failedConnConsumer == nil {
		return nil
	}
	return t.failedConnConsumer.FailedConns
}

func (t *ebpfTracer) Stop() {
	t.stopOnce.Do(func() {
		close(t.exitTelemetry)
		ddebpf.RemoveNameMappings(t.m)
		ebpftelemetry.UnregisterTelemetry(t.m)
		_ = t.m.Stop(manager.CleanAll)
		t.closeConsumer.Stop()
		t.failedConnConsumer.Stop()
		t.ongoingConnectCleaner.Stop()
		if t.closeTracer != nil {
			t.closeTracer()
		}
	})
}

func (t *ebpfTracer) GetMap(name string) (*ebpf.Map, error) {
	m, _, err := t.m.GetMap(name)
	if err != nil {
		return nil, fmt.Errorf("error getting map %s: %w", name, err)
	}
	return m, nil
}

func (t *ebpfTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	// Iterate through all key-value pairs in map
	key, stats := &netebpf.ConnTuple{}, &netebpf.ConnStats{}
	seen := make(map[netebpf.ConnTuple]struct{})
	// connsByTuple is used to detect whether we are iterating over
	// a connection we have previously seen. This can happen when
	// ebpf maps are being iterated over and deleted at the same time.
	// The iteration can reset when that happens.
	// See https://justin.azoff.dev/blog/bpf_map_get_next_key-pitfalls/
	connsByTuple := make(map[netebpf.ConnTuple]uint32)

	// Cached objects
	conn := new(network.ConnectionStats)
	tcp := new(netebpf.TCPStats)

	var tcp4, tcp6, udp4, udp6 float64
	entries := t.conns.Iterate()
	for entries.Next(key, stats) {
		if cookie, exists := connsByTuple[*key]; exists && cookie == stats.Cookie {
			// already seen the connection in current batch processing,
			// due to race between the iterator and bpf_map_delete
			EbpfTracerTelemetry.iterationDups.Inc()
			continue
		}

		populateConnStats(conn, key, stats, t.ch)
		connsByTuple[*key] = stats.Cookie

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
		EbpfTracerTelemetry.iterationAborts.Inc()
	}

	updateTelemetry(tcp4, tcp6, udp4, udp6)

	return nil
}

func updateTelemetry(tcp4 float64, tcp6 float64, udp4 float64, udp6 float64) {
	EbpfTracerTelemetry.connections.Set(tcp4, "tcp", "v4")
	EbpfTracerTelemetry.connections.Set(tcp6, "tcp", "v6")
	EbpfTracerTelemetry.connections.Set(udp4, "udp", "v4")
	EbpfTracerTelemetry.connections.Set(udp6, "udp", "v6")
}

func removeConnectionFromTelemetry(conn *network.ConnectionStats) {
	isTCP := conn.Type == network.TCP
	switch conn.Family {
	case network.AFINET6:
		if isTCP {
			EbpfTracerTelemetry.connections.Dec("tcp", "v6")
		} else {
			EbpfTracerTelemetry.connections.Dec("udp", "v6")
		}
	case network.AFINET:
		if isTCP {
			EbpfTracerTelemetry.connections.Dec("tcp", "v4")
		} else {
			EbpfTracerTelemetry.connections.Dec("udp", "v4")
		}
	}
}

func (t *ebpfTracer) Remove(conn *network.ConnectionStats) error {
	util.ConnTupleToEBPFTuple(&conn.ConnectionTuple, t.removeTuple)

	err := t.conns.Delete(t.removeTuple)
	if err != nil {
		// If this entry no longer exists in the eBPF map it means `tcp_close` has executed
		// during this function call. In that case state.StoreClosedConnection() was already called for this connection,
		// and we can't delete the corresponding client state, or we'll likely over-report the metric values.
		// By skipping to the next iteration and not calling state.RemoveConnections() we'll let
		// this connection expire "naturally" when either next connection check runs or the client itself expires.
		return err
	}

	removeConnectionFromTelemetry(conn)

	if conn.Type == network.TCP {
		// We can ignore the error for this map since it will not always contain the entry
		_ = t.tcpStats.Delete(t.removeTuple)
		// We remove the PID from the tuple as it is not used in the retransmits map
		pid := t.removeTuple.Pid
		t.removeTuple.Pid = 0
		_ = t.tcpRetransmits.Delete(t.removeTuple)
		t.removeTuple.Pid = pid
	}
	return nil
}

func (t *ebpfTracer) getEBPFTelemetry() *netebpf.Telemetry {
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
func (t *ebpfTracer) Describe(ch chan<- *prometheus.Desc) {
	ch <- EbpfTracerTelemetry.tcpFailedConnects
	ch <- EbpfTracerTelemetry.TcpSentMiscounts
	ch <- EbpfTracerTelemetry.unbatchedTcpClose
	ch <- EbpfTracerTelemetry.unbatchedUdpClose
	ch <- EbpfTracerTelemetry.UdpSendsProcessed
	ch <- EbpfTracerTelemetry.UdpSendsMissed
	ch <- EbpfTracerTelemetry.UdpDroppedConns
	ch <- EbpfTracerTelemetry.doubleFlushAttemptsClose
	ch <- EbpfTracerTelemetry.doubleFlushAttemptsDone
	ch <- EbpfTracerTelemetry.unsupportedTcpFailures
	ch <- EbpfTracerTelemetry.tcpDoneMissingPid
	ch <- EbpfTracerTelemetry.tcpConnectFailedTuple
	ch <- EbpfTracerTelemetry.tcpDoneFailedTuple
	ch <- EbpfTracerTelemetry.tcpFinishConnectFailedTuple
	ch <- EbpfTracerTelemetry.tcpCloseTargetFailures
	ch <- EbpfTracerTelemetry.tcpDoneConnectionFlush
	ch <- EbpfTracerTelemetry.tcpCloseConnectionFlush
}

// Collect returns the current state of all metrics of the collector
func (t *ebpfTracer) Collect(ch chan<- prometheus.Metric) {
	ebpfTelemetry := t.getEBPFTelemetry()
	if ebpfTelemetry == nil {
		return
	}
	delta := int64(ebpfTelemetry.Tcp_failed_connect) - EbpfTracerTelemetry.lastTcpFailedConnects.Load()
	EbpfTracerTelemetry.lastTcpFailedConnects.Store(int64(ebpfTelemetry.Tcp_failed_connect))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpFailedConnects, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_sent_miscounts) - EbpfTracerTelemetry.LastTcpSentMiscounts.Load()
	EbpfTracerTelemetry.LastTcpSentMiscounts.Store(int64(ebpfTelemetry.Tcp_sent_miscounts))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.TcpSentMiscounts, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Unbatched_tcp_close) - EbpfTracerTelemetry.lastUnbatchedTcpClose.Load()
	EbpfTracerTelemetry.lastUnbatchedTcpClose.Store(int64(ebpfTelemetry.Unbatched_tcp_close))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.unbatchedTcpClose, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Unbatched_udp_close) - EbpfTracerTelemetry.lastUnbatchedUdpClose.Load()
	EbpfTracerTelemetry.lastUnbatchedUdpClose.Store(int64(ebpfTelemetry.Unbatched_udp_close))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.unbatchedUdpClose, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_sends_processed) - EbpfTracerTelemetry.lastUdpSendsProcessed.Load()
	EbpfTracerTelemetry.lastUdpSendsProcessed.Store(int64(ebpfTelemetry.Udp_sends_processed))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.UdpSendsProcessed, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_sends_missed) - EbpfTracerTelemetry.lastUdpSendsMissed.Load()
	EbpfTracerTelemetry.lastUdpSendsMissed.Store(int64(ebpfTelemetry.Udp_sends_missed))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.UdpSendsMissed, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_dropped_conns) - EbpfTracerTelemetry.lastUdpDroppedConns.Load()
	EbpfTracerTelemetry.lastUdpDroppedConns.Store(int64(ebpfTelemetry.Udp_dropped_conns))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.UdpDroppedConns, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Double_flush_attempts_close) - EbpfTracerTelemetry.lastDoubleFlushAttemptsClose.Load()
	EbpfTracerTelemetry.lastDoubleFlushAttemptsClose.Store(int64(ebpfTelemetry.Double_flush_attempts_close))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.doubleFlushAttemptsClose, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Double_flush_attempts_done) - EbpfTracerTelemetry.lastDoubleFlushAttemptsDone.Load()
	EbpfTracerTelemetry.lastDoubleFlushAttemptsDone.Store(int64(ebpfTelemetry.Double_flush_attempts_done))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.doubleFlushAttemptsDone, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Unsupported_tcp_failures) - EbpfTracerTelemetry.lastUnsupportedTcpFailures.Load()
	EbpfTracerTelemetry.lastUnsupportedTcpFailures.Store(int64(ebpfTelemetry.Unsupported_tcp_failures))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.unsupportedTcpFailures, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_done_missing_pid) - EbpfTracerTelemetry.lastTcpDoneMissingPid.Load()
	EbpfTracerTelemetry.lastTcpDoneMissingPid.Store(int64(ebpfTelemetry.Tcp_done_missing_pid))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpDoneMissingPid, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_connect_failed_tuple) - EbpfTracerTelemetry.lastTcpConnectFailedTuple.Load()
	EbpfTracerTelemetry.lastTcpConnectFailedTuple.Store(int64(ebpfTelemetry.Tcp_connect_failed_tuple))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpConnectFailedTuple, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_done_failed_tuple) - EbpfTracerTelemetry.lastTcpDoneFailedTuple.Load()
	EbpfTracerTelemetry.lastTcpDoneFailedTuple.Store(int64(ebpfTelemetry.Tcp_done_failed_tuple))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpDoneFailedTuple, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_finish_connect_failed_tuple) - EbpfTracerTelemetry.lastTcpFinishConnectFailedTuple.Load()
	EbpfTracerTelemetry.lastTcpFinishConnectFailedTuple.Store(int64(ebpfTelemetry.Tcp_finish_connect_failed_tuple))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpFinishConnectFailedTuple, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_close_target_failures) - EbpfTracerTelemetry.lastTcpCloseTargetFailures.Load()
	EbpfTracerTelemetry.lastTcpCloseTargetFailures.Store(int64(ebpfTelemetry.Tcp_close_target_failures))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpCloseTargetFailures, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_done_connection_flush) - EbpfTracerTelemetry.lastTcpDoneConnectionFlush.Load()
	EbpfTracerTelemetry.lastTcpDoneConnectionFlush.Store(int64(ebpfTelemetry.Tcp_done_connection_flush))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpDoneConnectionFlush, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_close_connection_flush) - EbpfTracerTelemetry.lastTcpCloseConnectionFlush.Load()
	EbpfTracerTelemetry.lastTcpCloseConnectionFlush.Store(int64(ebpfTelemetry.Tcp_close_connection_flush))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpCloseConnectionFlush, prometheus.CounterValue, float64(delta))
}

// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
func (t *ebpfTracer) DumpMaps(w io.Writer, maps ...string) error {
	return t.m.DumpMaps(w, maps...)
}

// Type returns the type of the underlying ebpf tracer that is currently loaded
func (t *ebpfTracer) Type() TracerType {
	return t.ebpfTracerType
}

func (t *ebpfTracer) initializePortBindingMaps() error {
	tcpPorts, err := network.ReadListeningPorts(t.config.ProcRoot, network.TCP, t.config.CollectTCPv6Conns)
	if err != nil {
		return fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	}

	tcpPortMap, err := maps.GetMap[netebpf.PortBinding, uint32](t.m, probes.PortBindingsMap)
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

	udpPorts, err := network.ReadListeningPorts(t.config.ProcRoot, network.UDP, t.config.CollectUDPv6Conns)
	if err != nil {
		return fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	}

	udpPortMap, err := maps.GetMap[netebpf.PortBinding, uint32](t.m, probes.UDPPortBindingsMap)
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

func (t *ebpfTracer) getTCPRetransmits(tuple *netebpf.ConnTuple, seen map[netebpf.ConnTuple]struct{}) (uint32, bool) {
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
			EbpfTracerTelemetry.PidCollisions.Inc()
			retransmits = 0
		} else {
			seen[*tuple] = struct{}{}
		}
	}

	tuple.Pid = pid
	return retransmits, true
}

// getTCPStats reads tcp related stats for the given ConnTuple
func (t *ebpfTracer) getTCPStats(stats *netebpf.TCPStats, tuple *netebpf.ConnTuple) bool {
	if tuple.Type() != netebpf.TCP {
		return false
	}

	return t.tcpStats.Lookup(tuple, stats) == nil
}

// setupMapCleaner sets up a map cleaner for the tcp_ongoing_connect_pid map
func (t *ebpfTracer) setupMapCleaner(m *manager.Manager) {
	tcpOngoingConnectPidMap, _, err := m.GetMap(probes.TCPOngoingConnectPid)
	if err != nil {
		log.Errorf("error getting %v map: %s", probes.TCPOngoingConnectPid, err)
		return
	}

	tcpOngoingConnectPidCleaner, err := ddebpf.NewMapCleaner[netebpf.SkpConn, netebpf.PidTs](tcpOngoingConnectPidMap, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}
	tcpOngoingConnectPidCleaner.Clean(time.Minute*5, nil, nil, func(now int64, _ netebpf.SkpConn, val netebpf.PidTs) bool {
		ts := int64(val.Timestamp)
		expired := ts > 0 && now-ts > tcpOngoingConnectMapTTL
		if expired {
			EbpfTracerTelemetry.ongoingConnectPidCleaned.Inc()
		}
		return expired
	})

	t.ongoingConnectCleaner = tcpOngoingConnectPidCleaner
}

func populateConnStats(stats *network.ConnectionStats, t *netebpf.ConnTuple, s *netebpf.ConnStats, ch *cookieHasher) {
	*stats = network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Pid:    t.Pid,
		NetNS:  t.Netns,
		Source: t.SourceAddress(),
		Dest:   t.DestAddress(),
		SPort:  t.Sport,
		DPort:  t.Dport,
	},
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
		conn.Monotonic.TCPEstablished = tcpStats.State_transitions >> netebpf.Established & 1
		conn.Monotonic.TCPClosed = tcpStats.State_transitions >> netebpf.Close & 1
		conn.RTT = tcpStats.Rtt
		conn.RTTVar = tcpStats.Rtt_var
	}
}
