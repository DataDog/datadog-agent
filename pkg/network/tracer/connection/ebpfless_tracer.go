// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && bpf) || darwin

package connection

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	syncutil "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	ebpfLessTelemetryPrefix = "network_tracer__ebpfless"

	// defaultReadErrorBackoff is the wait after the first packet source error. It doubles
	// after each consecutive error, up to defaultMaxReadErrorBackoff.
	defaultReadErrorBackoff    = 100 * time.Millisecond
	defaultMaxReadErrorBackoff = 5 * time.Second
	// defaultMaxConsecutiveReadErrors is the number of packet source errors in a row, with no
	// packet read in between, at which the reader stops. It waits before retrying after each
	// of the first 9, about 21s in total.
	defaultMaxConsecutiveReadErrors = 10
)

var (
	ebpfLessTracerTelemetry = struct {
		skippedPackets     telemetry.Counter
		droppedConnections telemetry.Counter
		packetSourceErrors telemetry.Counter
	}{
		telemetryimpl.GetCompatComponent().NewCounter(ebpfLessTelemetryPrefix, "skipped_packets", []string{"reason"}, "Counter measuring skipped packets"),
		telemetryimpl.GetCompatComponent().NewCounter(ebpfLessTelemetryPrefix, "dropped_connections", nil, "Counter measuring dropped connections"),
		telemetryimpl.GetCompatComponent().NewCounter(ebpfLessTelemetryPrefix, "packet_source_errors", nil, "Counter measuring errors returned by the packet source"),
	}

	// decodeErrorLimiter rate-limits the log emitted when a captured frame cannot be decoded
	decodeErrorLimiter = log.NewLogLimit(10, 10*time.Minute)
	// bigTCPLimiter rate-limits the log emitted when an IPv6 BIG TCP frame is skipped
	bigTCPLimiter = log.NewLogLimit(10, 10*time.Minute)
	// readErrorLimiter rate-limits the log emitted when the packet source returns an error
	readErrorLimiter = log.NewLogLimit(10, 10*time.Minute)
)

const (
	skipReasonDecodeError = "decode_error"
	// skipReasonIPv6BigTCP counts IPv6 BIG TCP frames, which gopacket cannot decode
	skipReasonIPv6BigTCP = "ipv6_big_tcp"
)

type ebpfLessTracer struct {
	m sync.Mutex

	config *config.Config

	packetSrc filter.PacketSource // Use interface to support both AFPacket and Libpcap
	// packetSrcBusy is needed because you can't close packetSrc while it's still visiting
	packetSrcBusy sync.WaitGroup
	exit          chan struct{}

	udp *udpProcessor
	tcp *ebpfless.TCPProcessor

	// connection maps
	conns        map[ebpfless.PCAPTuple]*network.ConnectionStats
	boundPorts   *ebpfless.BoundPorts
	cookieHasher *cookieHasher
	// dnsServerPorts is the set of ports to treat as DNS server ports
	// (from network_config.dns_monitoring_ports)
	dnsServerPorts map[uint16]struct{}

	connPool *syncutil.TypedPool[network.ConnectionStats]

	// retry policy for errors returned by packetSrc.VisitPackets
	readErrorBackoff         time.Duration
	maxReadErrorBackoff      time.Duration
	maxConsecutiveReadErrors int
}

// newEbpfLessTracer creates a new ebpfLessTracer instance
func newEbpfLessTracer(cfg *config.Config) (*ebpfLessTracer, error) {
	// Create platform-specific packet source
	packetSrc, err := createPacketSource(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating packet source: %w", err)
	}

	return newEbpfLessTracerWithPacketSource(cfg, packetSrc), nil
}

// newEbpfLessTracerWithPacketSource creates a new ebpfLessTracer instance that reads from packetSrc
func newEbpfLessTracerWithPacketSource(cfg *config.Config, packetSrc filter.PacketSource) *ebpfLessTracer {
	dnsServerPorts := make(map[uint16]struct{}, len(cfg.DNSMonitoringPortList))
	for _, port := range cfg.DNSMonitoringPortList {
		dnsServerPorts[uint16(port)] = struct{}{}
	}

	return &ebpfLessTracer{
		config:         cfg,
		packetSrc:      packetSrc,
		packetSrcBusy:  sync.WaitGroup{},
		exit:           make(chan struct{}),
		udp:            &udpProcessor{},
		tcp:            ebpfless.NewTCPProcessor(cfg),
		conns:          make(map[ebpfless.PCAPTuple]*network.ConnectionStats, cfg.MaxTrackedConnections),
		boundPorts:     ebpfless.NewBoundPorts(cfg),
		cookieHasher:   newCookieHasher(),
		dnsServerPorts: dnsServerPorts,
		connPool:       syncutil.NewDefaultTypedPool[network.ConnectionStats](),

		readErrorBackoff:         defaultReadErrorBackoff,
		maxReadErrorBackoff:      defaultMaxReadErrorBackoff,
		maxConsecutiveReadErrors: defaultMaxConsecutiveReadErrors,
	}
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
		var loopback layers.Loopback
		var ip4 layers.IPv4
		var ip6 layers.IPv6
		var tcp layers.TCP
		var udp layers.UDP
		decoded := make([]gopacket.LayerType, 0, 5)

		// Two parsers to handle mixed link types on Darwin (e.g. Ethernet for
		// en0 and BSD Loopback for utun* VPN interfaces). On Linux, every
		// packet is Ethernet via AF_PACKET so loopbackParser is never used.
		ethernetParser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &ip6, &tcp, &udp)
		ethernetParser.IgnoreUnsupported = true
		loopbackParser := gopacket.NewDecodingLayerParser(layers.LayerTypeLoopback, &loopback, &ip4, &ip6, &tcp, &udp)
		loopbackParser.IgnoreUnsupported = true

		readErrors := 0
		backoff := t.readErrorBackoff
		for {
			progressed := false
			err := t.packetSrc.VisitPackets(func(b []byte, info filter.PacketInfo, _ time.Time) error {
				progressed = true
				pktType := info.PacketType()

				// only process PacketHost and PacketOutgoing packets. Filter before
				// decoding, so that malformed frames we would ignore anyway (broadcast,
				// multicast, other hosts) are not counted as decode errors.
				if pktType != filter.PacketHost && pktType != filter.PacketOutgoing {
					ebpfLessTracerTelemetry.skippedPackets.Inc("unsupported_packet_type")
					return nil
				}

				var parser *gopacket.DecodingLayerParser
				var link gopacket.DecodingLayer
				if info.LinkLayerType() == layers.LayerTypeLoopback {
					parser = loopbackParser
					link = &loopback
				} else {
					parser = ethernetParser
					link = &eth
				}
				if err := parser.DecodeLayers(b, &decoded); err != nil {
					// A malformed or truncated frame must not stop packet capture, so
					// skip it and keep visiting. Skip it even if its link and network
					// layers decoded: processConnection would only count it as
					// not_tcp_udp, and it belongs in the decode_error count.
					// decoded holds only the layers decoded before the failure, so
					// the link layer is valid only when it is the one decoded.
					if len(decoded) == 1 && isIPv6BigTCP(link.NextLayerType(), link.LayerPayload()) {
						ebpfLessTracerTelemetry.skippedPackets.Inc(skipReasonIPv6BigTCP)
						if bigTCPLimiter.ShouldLog() {
							log.Warnf("skipping IPv6 BIG TCP packet (payload length 0 and no jumbo option) of %d bytes, which cannot be decoded; its bytes are not counted", info.OriginalLength())
						}
						return nil
					}
					ebpfLessTracerTelemetry.skippedPackets.Inc(skipReasonDecodeError)
					if decodeErrorLimiter.ShouldLog() {
						log.Warnf("skipping packet that could not be decoded: %s", err)
					}
					return nil
				}

				if err := t.processConnection(pktType, &ip4, &ip6, &udp, &tcp, decoded, closeCallback); err != nil {
					log.Warnf("could not process packet: %s", err)
				}

				return nil
			})

			// Properly synchronizes termination process
			select {
			case <-t.exit:
				return
			default:
			}

			if progressed {
				// Packets were read since the last error, so any error starts a new streak.
				// A pass that read nothing, such as an idle poll timeout on Linux, does not
				// end the streak.
				readErrors, backoff = 0, t.readErrorBackoff
			}
			if err == nil {
				continue
			}

			// The visitor never returns an error, so err comes from the packet source.
			ebpfLessTracerTelemetry.packetSourceErrors.Inc()
			if isFatalPacketSourceError(err) {
				log.Errorf("exiting visiting packets: %s", err)
				return
			}
			readErrors++
			if readErrors >= t.maxConsecutiveReadErrors {
				log.Errorf("exiting visiting packets after %d consecutive packet source errors: %s", readErrors, err)
				return
			}
			if readErrorLimiter.ShouldLog() {
				log.Warnf("error visiting packets, retrying in %s: %s", backoff, err)
			}
			select {
			case <-t.exit:
				return
			case <-time.After(backoff):
			}
			backoff = min(2*backoff, t.maxReadErrorBackoff)
		}
	}()

	return nil
}

// isFatalPacketSourceError reports whether err means the packet source can no longer be read,
// so retrying is pointless. Other errors are retried with a backoff.
func isFatalPacketSourceError(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EBADF) ||
		isPlatformFatalPacketSourceError(err)
}

// isIPv6BigTCP reports whether netLayer starts with an IPv6 header whose payload length is 0
// and that has no Hop-by-Hop header. Linux hands IPv6 BIG TCP packets (GSO/GRO packets larger
// than 64KiB) to packet sockets this way, and gopacket refuses to decode them. nextLayer is
// the network layer type the link layer announced, which must be IPv6.
func isIPv6BigTCP(nextLayer gopacket.LayerType, netLayer []byte) bool {
	const ipv6HeaderLen = 40
	return nextLayer == layers.LayerTypeIPv6 &&
		len(netLayer) >= ipv6HeaderLen &&
		netLayer[0]>>4 == 6 &&
		binary.BigEndian.Uint16(netLayer[4:6]) == 0 &&
		layers.IPProtocol(netLayer[6]) != layers.IPProtocolIPv6HopByHop
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
		conn.ConnectionTuple = ebpfless.MakeConnStatsTuple(tuple)

		// guess direction before calling the processors; for TCP, the processor
		// may also set direction from the SYN packet
		direction, err := guessConnectionDirection(conn, pktType, t.boundPorts, t.dnsServerPorts)
		if err != nil {
			t.putConn(conn)
			return err
		}
		conn.Direction = direction
	}

	var ts int64
	var err error
	if ts, err = nowNanoseconds(); err != nil {
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

	if pktType == filter.PacketHost {
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
func guessConnectionDirection(conn *network.ConnectionStats, pktType uint8, ports boundPortLookup, dnsServerPorts map[uint16]struct{}) (network.ConnectionDirection, error) {
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

	// for UDP, check if either port is a known DNS server port
	if conn.Type == network.UDP {
		_, srcIsDNS := dnsServerPorts[conn.SPort]
		_, dstIsDNS := dnsServerPorts[conn.DPort]
		if srcIsDNS && !dstIsDNS {
			return network.INCOMING, nil
		}
		if dstIsDNS && !srcIsDNS {
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
	case filter.PacketHost:
		return network.INCOMING, nil
	case filter.PacketOutgoing:
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
	ts, err := nowNanoseconds()
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

// Type returns the type of the underlying tracer that is currently loaded
func (t *ebpfLessTracer) Type() TracerType {
	if runtime.GOOS == "darwin" {
		return TracerTypeDarwin
	}
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
	case filter.PacketOutgoing:
		conn.Monotonic.SentPackets++
		conn.Monotonic.SentBytes += uint64(payloadLen)
	case filter.PacketHost:
		conn.Monotonic.RecvPackets++
		conn.Monotonic.RecvBytes += uint64(payloadLen)
	}

	return nil
}
