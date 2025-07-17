// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

// Package connection provides tracing for connections
package connection

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/fentry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/encoding"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	connTracerModuleName = "network_tracer__ebpf"
)

var tcpOngoingConnectMapTTL = 30 * time.Minute.Nanoseconds()
var tlsTagsMapTTL = 3 * time.Minute.Nanoseconds()

// EbpfTracerTelemetry holds telemetry from the EBPF tracer
var EbpfTracerTelemetry = struct {
	connections       telemetry.Gauge
	tcpSentMiscounts  *prometheus.Desc
	unbatchedTCPClose *prometheus.Desc
	unbatchedUDPClose *prometheus.Desc
	udpSendsProcessed *prometheus.Desc
	udpSendsMissed    *prometheus.Desc
	udpDroppedConns   *prometheus.Desc
	// tcpDoneMissingPid is a counter measuring the number of TCP connections with a PID mismatch between tcp_connect and tcp_done
	tcpDoneMissingPid           *prometheus.Desc
	tcpConnectFailedTuple       *prometheus.Desc
	tcpDoneFailedTuple          *prometheus.Desc
	tcpFinishConnectFailedTuple *prometheus.Desc
	tcpCloseTargetFailures      *prometheus.Desc
	tcpDoneConnectionFlush      *prometheus.Desc
	tcpCloseConnectionFlush     *prometheus.Desc
	tcpFailedConnections        telemetry.Counter
	tcpSynRetransmit            *prometheus.Desc
	ongoingConnectPidCleaned    telemetry.Counter
	PidCollisions               *telemetry.StatCounterWrapper
	iterationDups               telemetry.Counter
	iterationAborts             telemetry.Counter

	LastTCPSentMiscounts  *atomic.Int64
	lastUnbatchedTCPClose *atomic.Int64
	lastUnbatchedUDPClose *atomic.Int64
	lastUDPSendsProcessed *atomic.Int64
	lastUDPSendsMissed    *atomic.Int64
	lastUDPDroppedConns   *atomic.Int64
	// lastTCPDoneMissingPid is a counter measuring the diff between the last two values of tcpDoneMissingPid
	lastTCPDoneMissingPid           *atomic.Int64
	lastTCPConnectFailedTuple       *atomic.Int64
	lastTCPDoneFailedTuple          *atomic.Int64
	lastTCPFinishConnectFailedTuple *atomic.Int64
	lastTCPCloseTargetFailures      *atomic.Int64
	lastTCPDoneConnectionFlush      *atomic.Int64
	lastTCPCloseConnectionFlush     *atomic.Int64
	lastTCPSynRetransmit            *atomic.Int64
}{
	telemetry.NewGauge(connTracerModuleName, "connections", []string{"ip_proto", "family"}, "Gauge measuring the number of active connections in the EBPF map"),
	prometheus.NewDesc(connTracerModuleName+"__tcp_sent_miscounts", "Counter measuring the number of miscounted tcp sends in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__unbatched_tcp_close", "Counter measuring the number of missed TCP close events in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__unbatched_udp_close", "Counter measuring the number of missed UDP close events in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_sends_processed", "Counter measuring the number of processed UDP sends in EBPF", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_sends_missed", "Counter measuring failures to process UDP sends in EBPF", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__udp_dropped_conns", "Counter measuring the number of dropped UDP connections in the EBPF map", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_done_missing_pid", "Counter measuring the number of TCP connections with a missing PID in tcp_done", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_connect_failed_tuple", "Counter measuring the number of failed TCP connections due to tuple collisions", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_done_failed_tuple", "Counter measuring the number of failed TCP connections due to tuple collisions", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_finish_connect_failed_tuple", "Counter measuring the number of failed TCP connections due to tuple collisions", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_close_target_failures", "Counter measuring the number of failed TCP connections in tcp_close", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_done_connection_flush", "Counter measuring the number of connection flushes performed in tcp_done", nil, nil),
	prometheus.NewDesc(connTracerModuleName+"__tcp_close_connection_flush", "Counter measuring the number of connection flushes performed in tcp_close", nil, nil),
	telemetry.NewCounter(connTracerModuleName, "tcp_failed_connections", []string{"errno"}, "Gauge measuring the number of unsupported failed TCP connections"),
	prometheus.NewDesc(connTracerModuleName+"__tcp_syn_retransmit", "Counter measuring the number of tcp retransmits of syn packets", nil, nil),
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
}

type ebpfTracer struct {
	m *ddebpf.Manager

	conns                   *maps.GenericMap[netebpf.ConnTuple, netebpf.ConnStats]
	tcpStats                *maps.GenericMap[netebpf.ConnTuple, netebpf.TCPStats]
	tcpRetransmits          *maps.GenericMap[netebpf.ConnTuple, uint32]
	ebpfTelemetryMap        *maps.GenericMap[uint32, netebpf.Telemetry]
	tcpFailuresTelemetryMap *maps.GenericMap[int32, uint64]
	config                  *config.Config

	// tcp_close events
	closeConsumer *tcpCloseConsumer

	// periodically clean the ongoing connection pid map
	ongoingConnectCleaner *ddebpf.MapCleaner[netebpf.SkpConn, netebpf.PidTs]
	// periodically clean the enhanced TLS tags map
	TLSTagsCleaner *ddebpf.MapCleaner[netebpf.ConnTuple, netebpf.TLSTagsWrapper]

	removeTuple *netebpf.ConnTuple

	closeTracer func()
	stopOnce    sync.Once

	ebpfTracerType TracerType

	ch *cookieHasher

	lastTCPFailureTelemetry map[int32]uint64
}

// NewTracer creates a new tracer
func newEbpfTracer(config *config.Config, _ telemetryComponent.Component) (Tracer, error) {
	if _, err := tracefs.Root(); err != nil {
		return nil, fmt.Errorf("eBPF based network tracer unsupported: %s", err)
	}

	mgrOptions := manager.Options{
		RemoveRlimit: true,
		MapSpecEditors: map[string]manager.MapSpecEditor{
			probes.ConnMap:                           {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.TCPStatsMap:                       {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.TCPRetransmitsMap:                 {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.PortBindingsMap:                   {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.UDPPortBindingsMap:                {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.ConnectionProtocolMap:             {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.EnhancedTLSTagsMap:                {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.ConnectionTupleToSocketSKBConnMap: {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.TCPOngoingConnectPid:              {MaxEntries: config.MaxTrackedConnections, EditorFlag: manager.EditMaxEntries},
			probes.TCPRecvMsgArgsMap:                 {MaxEntries: config.MaxTrackedConnections / 32, EditorFlag: manager.EditMaxEntries},
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

	connPool := ddsync.NewDefaultTypedPool[network.ConnectionStats]()
	var extractor *batchExtractor

	util.AddBoolConst(&mgrOptions, "batching_enabled", config.CustomBatchingEnabled)
	if config.CustomBatchingEnabled {
		numCPUs, err := ebpf.PossibleCPU()
		if err != nil {
			return nil, fmt.Errorf("could not determine number of CPUs: %w", err)
		}
		extractor = newBatchExtractor(numCPUs)
		mgrOptions.MapSpecEditors[probes.ConnCloseBatchMap] = manager.MapSpecEditor{
			MaxEntries: uint32(numCPUs),
			EditorFlag: manager.EditMaxEntries,
		}
	}

	tr := &ebpfTracer{
		removeTuple:             &netebpf.ConnTuple{},
		ch:                      newCookieHasher(),
		lastTCPFailureTelemetry: make(map[int32]uint64),
	}

	connCloseEventHandler, err := initClosedConnEventHandler(config, tr.closedPerfCallback, connPool, extractor)
	if err != nil {
		return nil, err
	}

	var m *ddebpf.Manager
	var tracerType = TracerTypeFentry
	var closeTracerFn func()
	m, closeTracerFn, err = fentry.LoadTracer(config, mgrOptions, connCloseEventHandler)
	if err != nil && !errors.Is(err, fentry.ErrorDisabled) {
		// failed to load fentry tracer
		return nil, err
	}

	if err != nil {
		// load the kprobe tracer
		log.Info("loading kprobe-based tracer")
		var kprobeTracerType kprobe.TracerType
		m, closeTracerFn, kprobeTracerType, err = kprobe.LoadTracer(config, mgrOptions, connCloseEventHandler)
		if err != nil {
			return nil, err
		}
		tracerType = TracerType(kprobeTracerType)
	}
	m.DumpHandler = dumpMapsHandler
	ddebpf.AddNameMappings(m.Manager, "npm_tracer")

	var flusher perf.Flusher = connCloseEventHandler
	if config.CustomBatchingEnabled {
		flusher, err = newConnBatchManager(m.Manager, extractor, connPool, tr.closedPerfCallback)
		if err != nil {
			return nil, err
		}
	}
	tr.closeConsumer = newTCPCloseConsumer(flusher, connPool)

	// Failed connections are not supported on prebuilt
	if tracerType == TracerTypeKProbePrebuilt {
		if config.TCPFailedConnectionsEnabled {
			log.Warn("Failed TCP connections are not supported with the prebuilt kprobe tracer. Disabling.")
		}
		config.TCPFailedConnectionsEnabled = false
	}

	tr.m = m
	tr.config = config
	tr.closeTracer = closeTracerFn
	tr.ebpfTracerType = tracerType

	tr.setupMapCleaners(m.Manager)

	tr.conns, err = maps.GetMap[netebpf.ConnTuple, netebpf.ConnStats](m.Manager, probes.ConnMap)
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.ConnMap, err)
	}

	tr.tcpStats, err = maps.GetMap[netebpf.ConnTuple, netebpf.TCPStats](m.Manager, probes.TCPStatsMap)
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.TCPStatsMap, err)
	}

	if tr.tcpRetransmits, err = maps.GetMap[netebpf.ConnTuple, uint32](m.Manager, probes.TCPRetransmitsMap); err != nil {
		tr.Stop()
		return nil, fmt.Errorf("error retrieving the bpf %s map: %s", probes.TCPRetransmitsMap, err)
	}

	tr.ebpfTelemetryMap, err = maps.GetMap[uint32, netebpf.Telemetry](m.Manager, probes.TelemetryMap)
	if err != nil {
		log.Warnf("error retrieving telemetry map: %s", err)
	}

	tr.tcpFailuresTelemetryMap, err = maps.GetMap[int32, uint64](m.Manager, probes.TCPFailureTelemetry)
	if err != nil {
		log.Warnf("error retrieving tcp failure telemetry map: %s", err)
	}

	return tr, nil
}

func initClosedConnEventHandler(config *config.Config, closedCallback func(*network.ConnectionStats), pool ddsync.Pool[network.ConnectionStats], extractor *batchExtractor) (*perf.EventHandler, error) {
	connHasher := newCookieHasher()
	singleConnHandler := encoding.BinaryUnmarshalCallback(pool.Get, func(b *network.ConnectionStats, err error) {
		if err != nil {
			if b != nil {
				pool.Put(b)
			}
			log.Debug(err.Error())
			return
		}
		if b != nil {
			connHasher.Hash(b)
		}
		closedCallback(b)
	})

	handler := singleConnHandler
	perfMode := perf.WakeupEvents(config.ClosedBufferWakeupCount)
	// multiply by number of connections with in-buffer batching to have same effective size as with custom batching
	chanSize := config.ClosedChannelSize * config.ClosedBufferWakeupCount
	if config.CustomBatchingEnabled {
		perfMode = perf.Watermark(1)
		chanSize = config.ClosedChannelSize
		handler = func(buf []byte) {
			l := len(buf)
			switch {
			case l >= netebpf.SizeofBatch:
				b := netebpf.ToBatch(buf)
				for rc := extractor.NextConnection(b); rc != nil; rc = extractor.NextConnection(b) {
					c := pool.Get()
					c.FromConn(rc)
					connHasher.Hash(c)

					closedCallback(c)
				}
			case l >= netebpf.SizeofConn:
				singleConnHandler(buf)
			case l == 0:
				singleConnHandler(nil)
			default:
				log.Debugf("unexpected %q binary data of size %d bytes", probes.ConnCloseEventMap, l)
			}
		}
	}

	perfBufferSize := util.ComputeDefaultClosedConnPerfBufferSize()
	mode := perf.UsePerfBuffers(perfBufferSize, chanSize, perfMode)
	if config.RingBufferSupportedNPM() {
		mode = perf.UpgradePerfBuffers(perfBufferSize, chanSize, perfMode, util.ComputeDefaultClosedConnRingBufferSize())
	}

	return perf.NewEventHandler(probes.ConnCloseEventMap, handler, mode,
		perf.SendTelemetry(config.InternalTelemetryEnabled),
		perf.RingBufferEnabledConstantName("ringbuffers_enabled"),
		perf.RingBufferWakeupSize("ringbuffer_wakeup_size", uint64(config.ClosedBufferWakeupCount*(netebpf.SizeofConn+unix.BPF_RINGBUF_HDR_SZ))))
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

func (t *ebpfTracer) closedPerfCallback(c *network.ConnectionStats) {
	t.closeConsumer.Callback(c)
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

	t.closeConsumer.Start(callback)

	if err := t.m.Start(); err != nil {
		t.closeConsumer.Stop()
		return fmt.Errorf("could not start ebpf manager: %s", err)
	}

	ddebpf.AddProbeFDMappings(t.m.Manager)

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

func (t *ebpfTracer) Stop() {
	t.stopOnce.Do(func() {
		ddebpf.RemoveNameMappings(t.m.Manager)
		ebpftelemetry.UnregisterTelemetry(t.m.Manager)
		_ = t.m.Stop(manager.CleanAll)
		t.closeConsumer.Stop()
		t.ongoingConnectCleaner.Stop()
		t.TLSTagsCleaner.Stop()
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
	entries := t.conns.IterateWithBatchSize(1000)
	for entries.Next(key, stats) {
		if cookie, exists := connsByTuple[*key]; exists && cookie == stats.Cookie {
			// already seen the connection in current batch processing,
			// due to race between the iterator and bpf_map_delete
			EbpfTracerTelemetry.iterationDups.Inc()
			continue
		}

		conn.FromTupleAndStats(key, stats)
		t.ch.Hash(conn)
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
			conn.FromTCPStats(tcp)
		}
		if retrans, ok := t.getTCPRetransmits(key, seen); ok && conn.Type == network.TCP {
			conn.Monotonic.Retransmits = retrans
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
	if t.ebpfTelemetryMap == nil {
		return nil
	}

	var zero uint32
	tm := &netebpf.Telemetry{}
	if err := t.ebpfTelemetryMap.Lookup(&zero, tm); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("error retrieving the telemetry struct: %s", err)
		}
		return nil
	}
	return tm
}

func (t *ebpfTracer) getTCPFailureTelemetry() map[int32]uint64 {
	if t.tcpFailuresTelemetryMap == nil {
		return nil
	}

	it := t.tcpFailuresTelemetryMap.IterateWithBatchSize(100)
	var key int32
	var val uint64
	result := make(map[int32]uint64)

	for it.Next(&key, &val) {
		if err := it.Err(); err != nil {
			log.Warnf("error retrieving tcp failure telemetry map: %s", err)
			return nil
		}

		result[key] = val - t.lastTCPFailureTelemetry[key]
		t.lastTCPFailureTelemetry[key] = val
	}
	return result
}

// Describe returns all descriptions of the collector
func (t *ebpfTracer) Describe(ch chan<- *prometheus.Desc) {
	ch <- EbpfTracerTelemetry.tcpSentMiscounts
	ch <- EbpfTracerTelemetry.unbatchedTCPClose
	ch <- EbpfTracerTelemetry.unbatchedUDPClose
	ch <- EbpfTracerTelemetry.udpSendsProcessed
	ch <- EbpfTracerTelemetry.udpSendsMissed
	ch <- EbpfTracerTelemetry.udpDroppedConns
	ch <- EbpfTracerTelemetry.tcpDoneMissingPid
	ch <- EbpfTracerTelemetry.tcpConnectFailedTuple
	ch <- EbpfTracerTelemetry.tcpDoneFailedTuple
	ch <- EbpfTracerTelemetry.tcpFinishConnectFailedTuple
	ch <- EbpfTracerTelemetry.tcpCloseTargetFailures
	ch <- EbpfTracerTelemetry.tcpDoneConnectionFlush
	ch <- EbpfTracerTelemetry.tcpCloseConnectionFlush
	ch <- EbpfTracerTelemetry.tcpSynRetransmit
}

// Collect returns the current state of all metrics of the collector
func (t *ebpfTracer) Collect(ch chan<- prometheus.Metric) {
	ebpfTelemetry := t.getEBPFTelemetry()
	if ebpfTelemetry == nil {
		return
	}
	delta := int64(ebpfTelemetry.Tcp_sent_miscounts) - EbpfTracerTelemetry.LastTCPSentMiscounts.Load()
	EbpfTracerTelemetry.LastTCPSentMiscounts.Store(int64(ebpfTelemetry.Tcp_sent_miscounts))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpSentMiscounts, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Unbatched_tcp_close) - EbpfTracerTelemetry.lastUnbatchedTCPClose.Load()
	EbpfTracerTelemetry.lastUnbatchedTCPClose.Store(int64(ebpfTelemetry.Unbatched_tcp_close))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.unbatchedTCPClose, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Unbatched_udp_close) - EbpfTracerTelemetry.lastUnbatchedUDPClose.Load()
	EbpfTracerTelemetry.lastUnbatchedUDPClose.Store(int64(ebpfTelemetry.Unbatched_udp_close))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.unbatchedUDPClose, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_sends_processed) - EbpfTracerTelemetry.lastUDPSendsProcessed.Load()
	EbpfTracerTelemetry.lastUDPSendsProcessed.Store(int64(ebpfTelemetry.Udp_sends_processed))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.udpSendsProcessed, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_sends_missed) - EbpfTracerTelemetry.lastUDPSendsMissed.Load()
	EbpfTracerTelemetry.lastUDPSendsMissed.Store(int64(ebpfTelemetry.Udp_sends_missed))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.udpSendsMissed, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Udp_dropped_conns) - EbpfTracerTelemetry.lastUDPDroppedConns.Load()
	EbpfTracerTelemetry.lastUDPDroppedConns.Store(int64(ebpfTelemetry.Udp_dropped_conns))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.udpDroppedConns, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_done_missing_pid) - EbpfTracerTelemetry.lastTCPDoneMissingPid.Load()
	EbpfTracerTelemetry.lastTCPDoneMissingPid.Store(int64(ebpfTelemetry.Tcp_done_missing_pid))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpDoneMissingPid, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_connect_failed_tuple) - EbpfTracerTelemetry.lastTCPConnectFailedTuple.Load()
	EbpfTracerTelemetry.lastTCPConnectFailedTuple.Store(int64(ebpfTelemetry.Tcp_connect_failed_tuple))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpConnectFailedTuple, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_done_failed_tuple) - EbpfTracerTelemetry.lastTCPDoneFailedTuple.Load()
	EbpfTracerTelemetry.lastTCPDoneFailedTuple.Store(int64(ebpfTelemetry.Tcp_done_failed_tuple))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpDoneFailedTuple, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_finish_connect_failed_tuple) - EbpfTracerTelemetry.lastTCPFinishConnectFailedTuple.Load()
	EbpfTracerTelemetry.lastTCPFinishConnectFailedTuple.Store(int64(ebpfTelemetry.Tcp_finish_connect_failed_tuple))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpFinishConnectFailedTuple, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_close_target_failures) - EbpfTracerTelemetry.lastTCPCloseTargetFailures.Load()
	EbpfTracerTelemetry.lastTCPCloseTargetFailures.Store(int64(ebpfTelemetry.Tcp_close_target_failures))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpCloseTargetFailures, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_done_connection_flush) - EbpfTracerTelemetry.lastTCPDoneConnectionFlush.Load()
	EbpfTracerTelemetry.lastTCPDoneConnectionFlush.Store(int64(ebpfTelemetry.Tcp_done_connection_flush))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpDoneConnectionFlush, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_close_connection_flush) - EbpfTracerTelemetry.lastTCPCloseConnectionFlush.Load()
	EbpfTracerTelemetry.lastTCPCloseConnectionFlush.Store(int64(ebpfTelemetry.Tcp_close_connection_flush))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpCloseConnectionFlush, prometheus.CounterValue, float64(delta))

	delta = int64(ebpfTelemetry.Tcp_syn_retransmit) - EbpfTracerTelemetry.lastTCPSynRetransmit.Load()
	EbpfTracerTelemetry.lastTCPSynRetransmit.Store(int64(ebpfTelemetry.Tcp_syn_retransmit))
	ch <- prometheus.MustNewConstMetric(EbpfTracerTelemetry.tcpSynRetransmit, prometheus.CounterValue, float64(delta))

	// Collect the TCP failure telemetry
	for k, v := range t.getTCPFailureTelemetry() {
		EbpfTracerTelemetry.tcpFailedConnections.Add(float64(v), fmt.Sprintf("%d", k))
	}
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

	tcpPortMap, err := maps.GetMap[netebpf.PortBinding, uint32](t.m.Manager, probes.PortBindingsMap)
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

	udpPortMap, err := maps.GetMap[netebpf.PortBinding, uint32](t.m.Manager, probes.UDPPortBindingsMap)
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

// setupMapCleaners sets up the map cleaners for the eBPF maps
func (t *ebpfTracer) setupMapCleaners(m *manager.Manager) {
	t.setupOngoingConnectMapCleaner(m)
	t.setupTLSTagsMapCleaner(m)
}

// setupOngoingConnectMapCleaner sets up a map cleaner for the tcp_ongoing_connect_pid map
func (t *ebpfTracer) setupOngoingConnectMapCleaner(m *manager.Manager) {
	tcpOngoingConnectPidMap, _, err := m.GetMap(probes.TCPOngoingConnectPid)
	if err != nil {
		log.Errorf("error getting %v map: %s", probes.TCPOngoingConnectPid, err)
		return
	}

	tcpOngoingConnectPidCleaner, err := ddebpf.NewMapCleaner[netebpf.SkpConn, netebpf.PidTs](tcpOngoingConnectPidMap, 1024, probes.TCPOngoingConnectPid, "npm_tracer")
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}
	tcpOngoingConnectPidCleaner.Start(time.Minute*5, nil, nil, func(now int64, _ netebpf.SkpConn, val netebpf.PidTs) bool {
		ts := int64(val.Timestamp)
		expired := ts > 0 && now-ts > tcpOngoingConnectMapTTL
		if expired {
			EbpfTracerTelemetry.ongoingConnectPidCleaned.Inc()
		}
		return expired
	})

	t.ongoingConnectCleaner = tcpOngoingConnectPidCleaner
}

// setupTLSTagsMapCleaner sets up a map cleaner for the tls_enhanced_tags map
func (t *ebpfTracer) setupTLSTagsMapCleaner(m *manager.Manager) {
	TLSTagsMap, _, err := m.GetMap(probes.EnhancedTLSTagsMap)
	if err != nil {
		log.Errorf("error getting %v map: %s", probes.EnhancedTLSTagsMap, err)
		return
	}

	TLSTagsMapCleaner, err := ddebpf.NewMapCleaner[netebpf.ConnTuple, netebpf.TLSTagsWrapper](TLSTagsMap, 1024, probes.EnhancedTLSTagsMap, "npm_tracer")
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}
	// slight jitter to avoid all maps being cleaned at the same time
	TLSTagsMapCleaner.Start(time.Second*70, nil, nil, func(now int64, _ netebpf.ConnTuple, val netebpf.TLSTagsWrapper) bool {
		ts := int64(val.Updated)
		return ts > 0 && now-ts > tlsTagsMapTTL
	})

	t.TLSTagsCleaner = TLSTagsMapCleaner
}
