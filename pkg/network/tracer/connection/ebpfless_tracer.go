// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
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
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/failure"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// the segment length to read
	// mac header (with vlan) + ip header + tcp header
	segmentLen = 18 + 60 + 60

	ebpfLessTelemetryPrefix = "network_tracer__ebpfless"
)

var (
	ebpfLessTracerTelemetry = struct {
		skippedPackets telemetry.Counter
	}{
		telemetry.NewCounter(ebpfLessTelemetryPrefix, "skipped_packets", []string{"reason"}, "Counter measuring skipped packets"),
	}
)

type ebpfLessTracer struct {
	m sync.Mutex

	config *config.Config

	packetSrc   *filter.AFPacketSource
	exit        chan struct{}
	scratchConn *network.ConnectionStats

	udp *udpProcessor
	tcp *tcpProcessor

	// connection maps
	conns        map[network.ConnectionTuple]*network.ConnectionStats
	boundPorts   *ebpfless.BoundPorts
	cookieHasher *cookieHasher

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
		config:       cfg,
		packetSrc:    packetSrc,
		exit:         make(chan struct{}),
		scratchConn:  &network.ConnectionStats{},
		udp:          &udpProcessor{},
		tcp:          newTCPProcessor(),
		conns:        make(map[network.ConnectionTuple]*network.ConnectionStats, cfg.MaxTrackedConnections),
		boundPorts:   ebpfless.NewBoundPorts(cfg),
		cookieHasher: newCookieHasher(),
	}

	tr.ns, err = netns.Get()
	if err != nil {
		return nil, fmt.Errorf("error getting current net ns: %w", err)
	}

	return tr, nil
}

// Start begins collecting network connection data.
func (t *ebpfLessTracer) Start(_ func(*network.ConnectionStats)) error {
	if err := t.boundPorts.Start(); err != nil {
		return fmt.Errorf("could not update bound ports: %w", err)
	}

	go func() {
		var eth layers.Ethernet
		var ip4 layers.IPv4
		var ip6 layers.IPv6
		var tcp layers.TCP
		var udp layers.UDP
		decoded := make([]gopacket.LayerType, 0, 5)
		parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &ip6, &tcp, &udp)
		parser.IgnoreUnsupported = true
		for {
			err := t.packetSrc.VisitPackets(t.exit, func(b []byte, info filter.PacketInfo, _ time.Time) error {
				if err := parser.DecodeLayers(b, &decoded); err != nil {
					return fmt.Errorf("error decoding packet layers: %w", err)
				}

				pktType := info.(*filter.AFPacketInfo).PktType
				// only process PACKET_HOST and PACK_OUTGOING packets
				if pktType != unix.PACKET_HOST && pktType != unix.PACKET_OUTGOING {
					ebpfLessTracerTelemetry.skippedPackets.Inc("unsupported_packet_type")
					return nil
				}

				if err := t.processConnection(pktType, &ip4, &ip6, &udp, &tcp, decoded); err != nil {
					log.Warnf("could not process packet: %s", err)
				}

				return nil
			})

			if err != nil {
				log.Errorf("exiting packet loop: %s", err)
				return
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
) error {
	t.scratchConn.Source, t.scratchConn.Dest = util.Address{}, util.Address{}
	t.scratchConn.SPort, t.scratchConn.DPort = 0, 0
	var udpPresent, tcpPresent bool
	for _, layerType := range decoded {
		switch layerType {
		case layers.LayerTypeIPv4:
			t.scratchConn.Source = util.AddressFromNetIP(ip4.SrcIP)
			t.scratchConn.Dest = util.AddressFromNetIP(ip4.DstIP)
			t.scratchConn.Family = network.AFINET
		case layers.LayerTypeIPv6:
			t.scratchConn.Source = util.AddressFromNetIP(ip6.SrcIP)
			t.scratchConn.Dest = util.AddressFromNetIP(ip6.DstIP)
			t.scratchConn.Family = network.AFINET6
		case layers.LayerTypeTCP:
			t.scratchConn.SPort = uint16(tcp.SrcPort)
			t.scratchConn.DPort = uint16(tcp.DstPort)
			t.scratchConn.Type = network.TCP
			tcpPresent = true
		case layers.LayerTypeUDP:
			t.scratchConn.SPort = uint16(udp.SrcPort)
			t.scratchConn.DPort = uint16(udp.DstPort)
			t.scratchConn.Type = network.UDP
			udpPresent = true
		}
	}

	// check if have all the basic pieces
	if !udpPresent && !tcpPresent {
		log.Debugf("ignoring packet since its not udp or tcp")
		ebpfLessTracerTelemetry.skippedPackets.Inc("not_tcp_udp")
		return nil
	}

	flipSourceDest(t.scratchConn, pktType)
	t.determineConnectionDirection(t.scratchConn, pktType)

	t.m.Lock()
	defer t.m.Unlock()

	conn := t.conns[t.scratchConn.ConnectionTuple]
	if conn == nil {
		conn = &network.ConnectionStats{}
		*conn = *t.scratchConn
		t.cookieHasher.Hash(conn)
		conn.Duration = time.Duration(time.Now().UnixNano())
	}

	var err error
	switch conn.Type {
	case network.UDP:
		err = t.udp.process(conn, pktType, udp)
	case network.TCP:
		err = t.tcp.process(conn, pktType, ip4, ip6, tcp)
	default:
		err = fmt.Errorf("unsupported connection type %d", conn.Type)
	}

	if err != nil {
		return fmt.Errorf("error processing connection: %w", err)
	}

	if conn.Type == network.UDP || conn.Monotonic.TCPEstablished > 0 {
		var ts int64
		if ts, err = ddebpf.NowNanoseconds(); err != nil {
			return fmt.Errorf("error getting last updated timestamp for connection: %w", err)
		}
		conn.LastUpdateEpoch = uint64(ts)
		t.conns[t.scratchConn.ConnectionTuple] = conn
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("connection: %s", conn)
	})
	return nil
}

func flipSourceDest(conn *network.ConnectionStats, pktType uint8) {
	if pktType == unix.PACKET_HOST {
		conn.Dest, conn.Source = conn.Source, conn.Dest
		conn.DPort, conn.SPort = conn.SPort, conn.DPort
	}
}

func (t *ebpfLessTracer) determineConnectionDirection(conn *network.ConnectionStats, pktType uint8) {
	t.m.Lock()
	defer t.m.Unlock()

	ok := t.boundPorts.Find(conn.Type, conn.SPort)
	if ok {
		// incoming connection
		conn.Direction = network.INCOMING
		return
	}

	if conn.Type == network.TCP {
		switch pktType {
		case unix.PACKET_HOST:
			conn.Direction = network.INCOMING
		case unix.PACKET_OUTGOING:
			conn.Direction = network.OUTGOING
		}
	}
}

// Stop halts all network data collection.
func (t *ebpfLessTracer) Stop() {
	if t == nil {
		return
	}

	close(t.exit)
	t.ns.Close()
	t.boundPorts.Stop()
}

// GetConnections returns the list of currently active connections, using the buffer provided.
// The optional filter function is used to prevent unwanted connections from being returned and consuming resources.
func (t *ebpfLessTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	t.m.Lock()
	defer t.m.Unlock()

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

// FlushPending forces any closed connections waiting for batching to be processed immediately.
func (t *ebpfLessTracer) FlushPending() {}

// Remove deletes the connection from tracking state.
// It does not prevent the connection from re-appearing later, if additional traffic occurs.
func (t *ebpfLessTracer) Remove(conn *network.ConnectionStats) error {
	t.m.Lock()
	defer t.m.Unlock()

	delete(t.conns, conn.ConnectionTuple)
	return nil
}

// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
// An individual ebpfLessTracer implementation may choose which maps to expose via this function.
func (t *ebpfLessTracer) GetMap(string) (*ebpf.Map, error) { return nil, nil }

// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
func (t *ebpfLessTracer) DumpMaps(_ io.Writer, _ ...string) error {
	return fmt.Errorf("not implemented")
}

// Type returns the type of the underlying ebpf ebpfLessTracer that is currently loaded
func (t *ebpfLessTracer) Type() TracerType {
	return TracerTypeEbpfless
}

func (t *ebpfLessTracer) Pause() error {
	return fmt.Errorf("not implemented")
}

func (t *ebpfLessTracer) Resume() error {
	return fmt.Errorf("not implemented")
}

// Describe returns all descriptions of the collector
func (t *ebpfLessTracer) Describe(_ chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (t *ebpfLessTracer) Collect(_ chan<- prometheus.Metric) {}

// GetFailedConnections returns the underlying map used to store failed connections
func (t *ebpfLessTracer) GetFailedConnections() *failure.FailedConns { return nil }

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

type tcpProcessor struct {
	conns map[network.ConnectionTuple]struct {
		established bool
		closed      bool
	}
}

func newTCPProcessor() *tcpProcessor {
	return &tcpProcessor{
		conns: map[network.ConnectionTuple]struct {
			established bool
			closed      bool
		}{},
	}
}

func (t *tcpProcessor) process(conn *network.ConnectionStats, pktType uint8, ip4 *layers.IPv4, ip6 *layers.IPv6, tcp *layers.TCP) error {
	payloadLen, err := ebpfless.TCPPayloadLen(conn.Family, ip4, ip6, tcp)
	if err != nil {
		return err
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("tcp processor: pktType=%+v seq=%+v ack=%+v fin=%+v rst=%+v syn=%+v ack=%+v", pktType, tcp.Seq, tcp.Ack, tcp.FIN, tcp.RST, tcp.SYN, tcp.ACK)
	})
	c := t.conns[conn.ConnectionTuple]
	log.TraceFunc(func() string {
		return fmt.Sprintf("pre ack_seq=%+v", c)
	})
	switch pktType {
	case unix.PACKET_OUTGOING:
		conn.Monotonic.SentPackets++
		conn.Monotonic.SentBytes += uint64(payloadLen)
	case unix.PACKET_HOST:
		conn.Monotonic.RecvPackets++
		conn.Monotonic.RecvBytes += uint64(payloadLen)
	}

	if tcp.FIN || tcp.RST {
		if !c.closed {
			c.closed = true
			conn.Monotonic.TCPClosed++
			conn.Duration = time.Duration(time.Now().UnixNano() - int64(conn.Duration))
		}
		delete(t.conns, conn.ConnectionTuple)
		return nil
	}

	if !tcp.SYN && !c.established {
		c.established = true
		conn.Monotonic.TCPEstablished++
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("ack_seq=%+v", c)
	})
	t.conns[conn.ConnectionTuple] = c
	return nil
}
