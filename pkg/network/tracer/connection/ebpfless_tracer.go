// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package connection

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cilium/ebpf"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/ebpfless"
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
	ErrEbpflessNotEnabled = errors.New("ebpf-less tracer not enabled")
)

type ebpfLessTracer struct {
	config *config.Config

	packetSrc *filter.AFPacketSource
	exit      chan struct{}
	keyBuf    []byte
	keyConn   network.ConnectionStats

	// connection maps
	tcpInProgress map[string]*network.ConnectionStats
	conns         map[string]*network.ConnectionStats
	boundPorts    *ebpfless.BoundPorts

	ns netns.NsHandle

	telemetry struct {
		skippedPackets telemetry.Counter
	}
}

func NewEbpfLessTracer(cfg *config.Config) (*ebpfLessTracer, error) {
	if !cfg.EnableEbpflessTracer {
		return nil, ErrEbpflessNotEnabled
	}

	packetSrc, err := filter.NewPacketSource(8, filter.OptSnapLen(segmentLen))
	if err != nil {
		return nil, fmt.Errorf("error creating packet source: %w", err)
	}

	tr := &ebpfLessTracer{
		config:        cfg,
		packetSrc:     packetSrc,
		exit:          make(chan struct{}),
		keyBuf:        make([]byte, network.ConnectionByteKeyMaxLen),
		tcpInProgress: make(map[string]*network.ConnectionStats, cfg.MaxTrackedConnections),
		conns:         make(map[string]*network.ConnectionStats, cfg.MaxTrackedConnections),
		boundPorts:    ebpfless.NewBoundPorts(cfg),
	}

	tr.ns, err = netns.Get()
	if err != nil {
		return nil, fmt.Errorf("error getting current net ns: %w", err)
	}

	tr.telemetry.skippedPackets = telemetry.NewCounter(ebpfLessTelemetryPrefix, "skipped_packets", []string{"reason"}, "Counter measuring skipped packets")

	return tr, nil
}

// Start begins collecting network connection data.
func (t *ebpfLessTracer) Start(func([]network.ConnectionStats)) error {
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
			err := t.packetSrc.VisitPackets(t.exit, func(b []byte, pktType uint8, ts time.Time) error {
				if err := parser.DecodeLayers(b, &decoded); err != nil {
					return fmt.Errorf("error decoding packet layers: %w", err)
				}

				// only process PACKET_HOST and PACK_OUTGOING packets
				if pktType != unix.PACKET_HOST && pktType != unix.PACKET_OUTGOING {
					t.telemetry.skippedPackets.Inc("unsupported_packet_type")
					return nil
				}

				if err := t.processConnection(pktType, ip4, ip6, udp, tcp, decoded); err != nil {
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
	ip4 layers.IPv4,
	ip6 layers.IPv6,
	udp layers.UDP,
	tcp layers.TCP,
	decoded []gopacket.LayerType,
) error {
	t.keyConn.Source, t.keyConn.Dest = util.Address{}, util.Address{}
	t.keyConn.SPort, t.keyConn.DPort = 0, 0
	var udpPresent, tcpPresent bool
	for _, layerType := range decoded {
		switch layerType {
		case layers.LayerTypeIPv4:
			t.keyConn.Source = util.AddressFromNetIP(ip4.SrcIP)
			t.keyConn.Dest = util.AddressFromNetIP(ip4.DstIP)
			t.keyConn.Family = network.AFINET
		case layers.LayerTypeIPv6:
			t.keyConn.Source = util.AddressFromNetIP(ip6.SrcIP)
			t.keyConn.Dest = util.AddressFromNetIP(ip6.DstIP)
			t.keyConn.Family = network.AFINET6
		case layers.LayerTypeTCP:
			t.keyConn.SPort = uint16(tcp.SrcPort)
			t.keyConn.DPort = uint16(tcp.DstPort)
			t.keyConn.Type = network.TCP
			tcpPresent = true
		case layers.LayerTypeUDP:
			t.keyConn.SPort = uint16(udp.SrcPort)
			t.keyConn.DPort = uint16(udp.DstPort)
			t.keyConn.Type = network.UDP
			udpPresent = true
		}
	}

	// check if have all the basic pieces
	if !udpPresent && !tcpPresent {
		log.Debugf("ignoring packet since its not udp or tcp")
		t.telemetry.skippedPackets.Inc("not_tcp_udp")
		return nil
	}

	if !t.keyConn.Source.IsValid() ||
		!t.keyConn.Dest.IsValid() ||
		t.keyConn.SPort == 0 ||
		t.keyConn.DPort == 0 {
		return fmt.Errorf("missing dest/source ip/port in packet conn=%+v", t.keyConn)
	}

	flipSourceDest(&t.keyConn, pktType)

	var err error
	switch t.keyConn.Type {
	case network.UDP:
		err = udpConnection(&t.keyConn, pktType, udp)
	case network.TCP:
		var processed bool
		if processed, err = t.tcpConnection(&t.keyConn, pktType, ip4, ip6, tcp); !processed {
			return nil
		}
	}

	if err != nil {
		return fmt.Errorf("error processing connection: %w", err)
	}

	log.Debugf("connection: %s", conn)
	return nil
}

func flipSourceDest(conn *network.ConnectionStats, pktType uint8) {
	if pktType == unix.PACKET_HOST {
		conn.Dest, conn.Source = conn.Source, conn.Dest
		conn.DPort, conn.SPort = conn.SPort, conn.DPort
	}
}

func udpConnection(conn *network.ConnectionStats, pktType uint8, udp layers.UDP) error {
	if udp.Length == 0 {
		return fmt.Errorf("udp packet with length 0")
	}

	updateConnectionStats(conn, pktType, udp.Length-8)
	return nil
}

func ipv6PayloadLen(ip6 layers.IPv6) (uint16, error) {
	if ip6.NextHeader == layers.IPProtocolUDP || ip6.NextHeader == layers.IPProtocolTCP {
		return ip6.Length, nil
	}

	var ipExt layers.IPv6ExtensionSkipper
	parser := gopacket.NewDecodingLayerParser(gopacket.LayerTypePayload, &ipExt)
	decoded := make([]gopacket.LayerType, 0, 1)
	l := ip6.Length
	payload := ip6.Payload
	for len(payload) > 0 {
		err := parser.DecodeLayers(payload, &decoded)
		if err != nil {
			return 0, fmt.Errorf("error decoding with ipv6 extension skipper: %w", err)
		}

		if len(decoded) == 0 {
			return l, nil
		}

		l -= uint16(len(ipExt.Contents))
		if ipExt.NextHeader == layers.IPProtocolTCP || ipExt.NextHeader == layers.IPProtocolUDP {
			break
		}

		payload = ipExt.Payload
	}

	return l, nil
}

func (t *ebpfLessTracer) tcpConnection(conn *network.ConnectionStats, pktType uint8, ip4 layers.IPv4, ip6 layers.IPv6, tcp layers.TCP) (processed bool, err error) {
	if tcp.SYN && !tcp.ACK {
		t.tcpInProgress[string(conn.ByteKey(t.keyBuf))] = conn
		return
	}

	if tcp.SYN || tcp.FIN {
		return false, nil
	}

	var payloadLen uint16
	switch conn.Family {
	case network.AFINET:
		payloadLen = ip4.Length - uint16(ip4.IHL)*4 - uint16(tcp.DataOffset)*4
	case network.AFINET6:
		if ip6.Length == 0 {
			return true, fmt.Errorf("not processing ipv6 jumbogram")
		}

		l, err := ipv6PayloadLen(ip6)
		if err != nil {
			return true, err
		}

		payloadLen = l - uint16(tcp.DataOffset)*4
	}

	updateConnectionStats(conn, pktType, payloadLen)

	return true, nil
}

func updateConnectionStats(conn *network.ConnectionStats, pktType uint8, payloadLen uint16) {
	switch pktType {
	case unix.PACKET_OUTGOING:
		conn.Monotonic.SentPackets++
		conn.Monotonic.SentBytes += uint64(payloadLen)
	case unix.PACKET_HOST:
		conn.Monotonic.RecvPackets++
		conn.Monotonic.RecvBytes += uint64(payloadLen)
	}
}

func (t *ebpfLessTracer) determineConnectionDirection(conn *network.ConnectionStats, pktType uint8, isSyn, isAck bool) {
	ok := t.boundPorts.Find(conn.Type, conn.SPort)
	if ok {
		// incoming connection
		conn.Direction = network.INCOMING
		return
	}

	switch conn.Type {
	case network.TCP:
		if !isSyn || isAck {
			return
		}

		// isSyn && !isAck
		switch pktType {
		case unix.PACKET_HOST:
			conn.Direction = network.INCOMING
		case unix.PACKET_OUTGOING:
			// new outgoing connection
			conn.Direction = network.OUTGOING
		}

		key := string(conn.ByteKey(t.keyBuf))
		t.tcpInProgress[key] = conn
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
	return nil
}

// FlushPending forces any closed connections waiting for batching to be processed immediately.
func (t *ebpfLessTracer) FlushPending() {}

// Remove deletes the connection from tracking state.
// It does not prevent the connection from re-appearing later, if additional traffic occurs.
func (t *ebpfLessTracer) Remove(conn *network.ConnectionStats) error { return nil }

// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
// An individual ebpfLessTracer implementation may choose which maps to expose via this function.
func (t *ebpfLessTracer) GetMap(string) *ebpf.Map { return nil }

// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
func (t *ebpfLessTracer) DumpMaps(w io.Writer, maps ...string) error { return nil }

// Type returns the type of the underlying ebpf ebpfLessTracer that is currently loaded
func (t *ebpfLessTracer) Type() TracerType {
	return TracerTypeEpfless
}

func (t *ebpfLessTracer) Pause() error {
	return nil
}

func (t *ebpfLessTracer) Resume() error {
	return nil
}

// Describe returns all descriptions of the collector
func (t *ebpfLessTracer) Describe(descs chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (t *ebpfLessTracer) Collect(metrics chan<- prometheus.Metric) {}

var _ Tracer = &ebpfLessTracer{}
