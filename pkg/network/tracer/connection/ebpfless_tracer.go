// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	syncutil "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	// the segment length to read
	// mac header (with vlan) + ip header + tcp header
	segmentLen = 18 + 60 + 60

	ebpfLessTelemetryPrefix = "network_tracer__ebpfless"
)

var (
	ebpfLessTracerTelemetry = struct {
		skippedPackets     telemetry.Counter
		droppedConnections telemetry.Counter
	}{
		telemetry.NewCounter(ebpfLessTelemetryPrefix, "skipped_packets", []string{"reason"}, "Counter measuring skipped packets"),
		telemetry.NewCounter(ebpfLessTelemetryPrefix, "dropped_connections", nil, "Counter measuring dropped connections"),
	}
)

type ebpfLessTracer struct {
	m sync.Mutex

	config *config.Config

	packetSrc *filter.AFPacketSource
	// packetSrcBusy is needed because you can't close packetSrc while it's still visiting
	packetSrcBusy sync.WaitGroup
	exit          chan struct{}

	udp *udpProcessor
	tcp *ebpfless.TCPProcessor

	// connection maps
	conns        map[ebpfless.PCAPTuple]*network.ConnectionStats
	boundPorts   *ebpfless.BoundPorts
	cookieHasher *cookieHasher

	connPool *syncutil.TypedPool[network.ConnectionStats]

	ns netns.NsHandle
}

// newEbpfLessTracer creates a new ebpfLessTracer instance
func newEbpfLessTracer(cfg *config.Config) (*ebpfLessTracer, error) {
	packetSrc, err := filter.NewAFPacketSource(
		8<<20, // 8 MB total space
		filter.OptSnapLen(segmentLen))
	if err != nil {
		return nil, fmt.Errorf("error creating packet source: %w", err)
	}

	tr := &ebpfLessTracer{
		config:        cfg,
		packetSrc:     packetSrc,
		packetSrcBusy: sync.WaitGroup{},
		exit:          make(chan struct{}),
		udp:           &udpProcessor{},
		tcp:           ebpfless.NewTCPProcessor(cfg),
		conns:         make(map[ebpfless.PCAPTuple]*network.ConnectionStats, cfg.MaxTrackedConnections),
		boundPorts:    ebpfless.NewBoundPorts(cfg),
		cookieHasher:  newCookieHasher(),
		connPool:      syncutil.NewDefaultTypedPool[network.ConnectionStats](),
	}

	tr.ns, err = netns.Get()
	if err != nil {
		return nil, fmt.Errorf("error getting current net ns: %w", err)
	}

	return tr, nil
}

// Start begins collecting network connection data.
func (t *ebpfLessTracer) Start(closeCallback func(*network.ConnectionStats)) error {
	if err := t.boundPorts.Start(); err != nil {
		return fmt.Errorf("could not update bound ports: %w", err)
	}

	t.packetSrcBusy.Add(1)
	go func() {
		defer func() {
			t.packetSrcBusy.Done()
		}()
		var eth layers.Ethernet
		var ip4 layers.IPv4
		var ip6 layers.IPv6
		var tcp layers.TCP
		var udp layers.UDP
		decoded := make([]gopacket.LayerType, 0, 5)
		parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &ip6, &tcp, &udp)
		parser.IgnoreUnsupported = true
		for {
			err := t.packetSrc.VisitPackets(func(b []byte, info filter.PacketInfo, _ time.Time) error {
				if err := parser.DecodeLayers(b, &decoded); err != nil {
					return fmt.Errorf("error decoding packet layers: %w", err)
				}

				pktType := info.(*filter.AFPacketInfo).PktType
				// only process PACKET_HOST and PACK_OUTGOING packets
				if pktType != unix.PACKET_HOST && pktType != unix.PACKET_OUTGOING {
					ebpfLessTracerTelemetry.skippedPackets.Inc("unsupported_packet_type")
					return nil
				}

				if err := t.processConnection(pktType, &ip4, &ip6, &udp, &tcp, decoded, closeCallback); err != nil {
					log.Warnf("could not process packet: %s", err)
				}

				return nil
			})

			if err != nil {
				log.Errorf("exiting visiting packets: %s", err)
				return
			}

			// Properly synchronizes termination process
			select {
			case <-t.exit:
				return
			default:
			}
		}
	}()

	return nil
}

func (t *ebpfLessTracer) processConnection(
	pktType uint8,
	ip4 *layers.IPv4,
	ip6 *layers.IPv6,
	udp *layers.UDP,
	tcp *layers.TCP,
	decoded []gopacket.LayerType,
	closeCallback func(*network.ConnectionStats),
) error {
	tuple, flags := buildTuple(pktType, ip4, ip6, udp, tcp, decoded)

	// check if we have all the basic pieces
	if !flags.udpPresent && !flags.tcpPresent {
		log.Debugf("ignoring packet since its not udp or tcp")
		ebpfLessTracerTelemetry.skippedPackets.Inc("not_tcp_udp")
		return nil
	}
	if !flags.ip4Present && !flags.ip6Present {
		return errors.New("expected to have an IP layer")
	}

	// don't trace families/protocols that are disabled by configuration
	switch tuple.Type {
	case network.UDP:
		if (flags.ip4Present && !t.config.CollectUDPv4Conns) || (flags.ip6Present && !t.config.CollectUDPv6Conns) {
			return nil
		}
	case network.TCP:
		if (flags.ip4Present && !t.config.CollectTCPv4Conns) || (flags.ip6Present && !t.config.CollectTCPv6Conns) {
			return nil
		}
	}

	t.m.Lock()
	defer t.m.Unlock()

	conn, ok := t.conns[tuple]
	isNewConn := !ok
	if isNewConn {
		conn = t.connPool.Get()
		// NOTE: this tuple does not have the connection direction set yet.
		// That will be set from determineConnectionDirection later
		conn.ConnectionTuple = ebpfless.MakeConnStatsTuple(tuple)
	}

	var ts int64
	var err error
	if ts, err = ddebpf.NowNanoseconds(); err != nil {
		return fmt.Errorf("error getting last updated timestamp for connection: %w", err)
	}
	conn.LastUpdateEpoch = uint64(ts)

	var result ebpfless.ProcessResult
	switch conn.Type {
	case network.UDP:
		result = ebpfless.ProcessResultStoreConn
		err = t.udp.process(conn, pktType, udp)
	case network.TCP:
		result, err = t.tcp.Process(conn, uint64(ts), pktType, ip4, ip6, tcp)
	default:
		err = fmt.Errorf("unsupported connection type %d", conn.Type)
	}

	if err != nil {
		return fmt.Errorf("error processing connection: %w", err)
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("connection: %s", conn)
	})

	if isNewConn && result.ShouldPersist() {
		conn.Duration = time.Duration(time.Now().UnixNano())
		direction, err := guessConnectionDirection(conn, pktType, t.boundPorts)
		if err != nil {
			return err
		}
		if direction == network.UNKNOWN {
			// silently drop connections whose direction can't be determined
			// (e.g. preexisting TCP connections where the SYN was missed)
			if conn.Type == network.TCP {
				t.tcp.RemoveConn(tuple)
			}
			t.putConn(conn)
			ebpfLessTracerTelemetry.droppedConnections.Inc()
			return nil
		}
		conn.Direction = direction

		// now that the direction is set, hash the connection
		t.cookieHasher.Hash(conn)
	}

	switch result {
	case ebpfless.ProcessResultNone:
		t.putConn(conn)
	case ebpfless.ProcessResultStoreConn:
		// if we fail to store this connection at any point, remove its TCP state tracking
		storeConnOk := false
		defer func() {
			if storeConnOk {
				return
			}
			if conn.Type == network.TCP {
				t.tcp.RemoveConn(tuple)
			}
			t.putConn(conn)
			ebpfLessTracerTelemetry.droppedConnections.Inc()
		}()

		maxTrackedConns := int(t.config.MaxTrackedConnections)
		storeConnOk = ebpfless.WriteMapWithSizeLimit(t.conns, tuple, conn, maxTrackedConns)
	case ebpfless.ProcessResultCloseConn:
		delete(t.conns, tuple)
		// do not call putConn after this, since the close handler owns it now
		closeCallback(conn)
	case ebpfless.ProcessResultMapFull:
		delete(t.conns, tuple)
		t.putConn(conn)
		ebpfLessTracerTelemetry.droppedConnections.Inc()
	}

	return nil
}

func (t *ebpfLessTracer) putConn(conn *network.ConnectionStats) {
	*conn = network.ConnectionStats{}
	t.connPool.Put(conn)
}

type packetFlags struct {
	ip4Present, ip6Present, udpPresent, tcpPresent bool
}

// buildTuple converts the packet capture layer info into an EbpflessTuple with flags that indicate which layers were present.
func buildTuple(pktType uint8, ip4 *layers.IPv4, ip6 *layers.IPv6, udp *layers.UDP, tcp *layers.TCP, decoded []gopacket.LayerType) (ebpfless.PCAPTuple, packetFlags) {
	var tuple ebpfless.PCAPTuple
	var flags packetFlags
	for _, layerType := range decoded {
		switch layerType {
		case layers.LayerTypeIPv4:
			tuple.Source = util.AddressFromNetIP(ip4.SrcIP)
			tuple.Dest = util.AddressFromNetIP(ip4.DstIP)
			tuple.Family = network.AFINET
			flags.ip4Present = true
		case layers.LayerTypeIPv6:
			tuple.Source = util.AddressFromNetIP(ip6.SrcIP)
			tuple.Dest = util.AddressFromNetIP(ip6.DstIP)
			tuple.Family = network.AFINET6
			flags.ip6Present = true
		case layers.LayerTypeTCP:
			tuple.SPort = uint16(tcp.SrcPort)
			tuple.DPort = uint16(tcp.DstPort)
			tuple.Type = network.TCP
			flags.tcpPresent = true
		case layers.LayerTypeUDP:
			tuple.SPort = uint16(udp.SrcPort)
			tuple.DPort = uint16(udp.DstPort)
			tuple.Type = network.UDP
			flags.udpPresent = true
		}
	}

	if pktType == unix.PACKET_HOST {
		tuple.Dest, tuple.Source = tuple.Source, tuple.Dest
		tuple.DPort, tuple.SPort = tuple.SPort, tuple.DPort
	}
	return tuple, flags
}

// boundPortLookup is an interface for finding bound ports
type boundPortLookup interface {
	Find(proto network.ConnectionType, port uint16) bool
}

// guessConnectionDirection attempts to guess the connection direction based off bound ports
func guessConnectionDirection(conn *network.ConnectionStats, pktType uint8, ports boundPortLookup) (network.ConnectionDirection, error) {
	// if we already have a direction, return that
	if conn.Direction != network.UNKNOWN {
		return conn.Direction, nil
	}

	ok := ports.Find(conn.Type, conn.SPort)
	if ok {
		// incoming connection
		return network.INCOMING, nil
	}
	// for local connections - the destination could be a bound port
	if conn.Dest.Addr.IsLoopback() {
		ok := ports.Find(conn.Type, conn.DPort)
		if ok {
			return network.OUTGOING, nil
		}
	}

	// system ports are always servers
	if conn.SPort < 1024 {
		return network.INCOMING, nil
	}
	if conn.DPort < 1024 {
		return network.OUTGOING, nil
	}

	// for TCP, don't guess direction from packet type; if we missed the SYN
	// then we can't reliably determine direction
	if conn.Type == network.TCP {
		return network.UNKNOWN, nil
	}

	switch pktType {
	case unix.PACKET_HOST:
		return network.INCOMING, nil
	case unix.PACKET_OUTGOING:
		return network.OUTGOING, nil
	default:
		return network.UNKNOWN, fmt.Errorf("unknown packet type %d", pktType)
	}
}

// Stop halts all network data collection.
func (t *ebpfLessTracer) Stop() {
	if t == nil {
		return
	}

	close(t.exit)
	// close the packet capture loop and wait for it to finish
	t.packetSrc.Close()
	t.packetSrcBusy.Wait()

	t.ns.Close()
	t.boundPorts.Stop()
}

// GetConnections returns the list of currently active connections, using the buffer provided.
// The optional filter function is used to prevent unwanted connections from being returned and consuming resources.
func (t *ebpfLessTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	t.m.Lock()
	defer t.m.Unlock()

	// use GetConnections to periodically cleanup pending connections
	err := t.cleanupPendingConns()
	if err != nil {
		return err
	}

	if len(t.conns) == 0 {
		return nil
	}

	log.Trace(t.conns)
	conns := make([]network.ConnectionStats, 0, len(t.conns))
	for _, c := range t.conns {
		if filter != nil && !filter(c) {
			continue
		}

		conns = append(conns, *c)
	}

	buffer.Append(conns)
	return nil
}

// cleanupPendingConns removes pending connections from the TCP tracer.
// For more information, refer to CleanupExpiredPendingConns
func (t *ebpfLessTracer) cleanupPendingConns() error {
	ts, err := ddebpf.NowNanoseconds()
	if err != nil {
		return fmt.Errorf("error getting last updated timestamp for connection: %w", err)
	}
	t.tcp.CleanupExpiredPendingConns(uint64(ts))
	return nil
}

// FlushPending forces any closed connections waiting for batching to be processed immediately.
func (t *ebpfLessTracer) FlushPending() {}

func (t *ebpfLessTracer) remove(conn *network.ConnectionStats) error {
	tuple := ebpfless.MakeEbpflessTuple(conn.ConnectionTuple)
	delete(t.conns, tuple)
	if conn.Type == network.TCP {
		t.tcp.RemoveConn(tuple)
	}
	return nil
}

// Remove deletes the connection from tracking state.
// It does not prevent the connection from re-appearing later, if additional traffic occurs.
func (t *ebpfLessTracer) Remove(conn *network.ConnectionStats) error {
	t.m.Lock()
	defer t.m.Unlock()

	return t.remove(conn)
}

// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
// An individual ebpfLessTracer implementation may choose which maps to expose via this function.
func (t *ebpfLessTracer) GetMap(string) (*ebpf.Map, error) { return nil, nil }

// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
func (t *ebpfLessTracer) DumpMaps(_ io.Writer, _ ...string) error {
	return errors.New("not implemented")
}

// Type returns the type of the underlying ebpf ebpfLessTracer that is currently loaded
func (t *ebpfLessTracer) Type() TracerType {
	return TracerTypeEbpfless
}

func (t *ebpfLessTracer) Pause() error {
	return errors.New("not implemented")
}

func (t *ebpfLessTracer) Resume() error {
	return errors.New("not implemented")
}

// Describe returns all descriptions of the collector
func (t *ebpfLessTracer) Describe(_ chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (t *ebpfLessTracer) Collect(_ chan<- prometheus.Metric) {}

var _ Tracer = &ebpfLessTracer{}

type udpProcessor struct {
}

func (u *udpProcessor) process(conn *network.ConnectionStats, pktType uint8, udp *layers.UDP) error {
	payloadLen, err := ebpfless.UDPPayloadLen(udp)
	if err != nil {
		return err
	}

	switch pktType {
	case unix.PACKET_OUTGOING:
		conn.Monotonic.SentPackets++
		conn.Monotonic.SentBytes += uint64(payloadLen)
	case unix.PACKET_HOST:
		conn.Monotonic.RecvPackets++
		conn.Monotonic.RecvBytes += uint64(payloadLen)
	}

	return nil
}
