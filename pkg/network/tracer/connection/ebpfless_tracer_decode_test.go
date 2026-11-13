// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build (linux && bpf) || darwin

package connection

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type fakePacketInfo struct {
	pktType uint8
	length  int
}

func (f *fakePacketInfo) PacketType() uint8                 { return f.pktType }
func (f *fakePacketInfo) LinkLayerType() gopacket.LayerType { return layers.LayerTypeEthernet }
func (f *fakePacketInfo) OriginalLength() int               { return f.length }
func (f *fakePacketInfo) CapturedLength() int               { return f.length }

// fakeItem is a packet to visit, or, when err is set, an error for VisitPackets to return,
// or, when idle is set, a nil return with no packet read, like an idle poll timeout on Linux
type fakeItem struct {
	data    []byte
	pktType uint8
	err     error
	idle    bool
}

// fakePacketSource is a filter.PacketSource that delivers the items written to it.
// It reports on visited once the callback has returned nil for a packet, and on
// stopped if the callback returned an error, which makes the tracer's reader exit.
type fakePacketSource struct {
	items     chan fakeItem
	visited   chan struct{}
	stopped   chan error
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakePacketSource(capacity int) *fakePacketSource {
	return &fakePacketSource{
		items:   make(chan fakeItem, capacity),
		visited: make(chan struct{}, capacity),
		stopped: make(chan error, 1),
		closed:  make(chan struct{}),
	}
}

func (f *fakePacketSource) VisitPackets(visit func(data []byte, info filter.PacketInfo, timestamp time.Time) error) error {
	for {
		select {
		case item := <-f.items:
			if item.err != nil {
				return item.err
			}
			if item.idle {
				return nil
			}
			info := &fakePacketInfo{pktType: item.pktType, length: len(item.data)}
			if err := visit(item.data, info, time.Now()); err != nil {
				f.stopped <- err
				return err
			}
			f.visited <- struct{}{}
		case <-f.closed:
			return nil
		}
	}
}

// waitVisited blocks until the callback has returned for n packets.
// It fails the test if the callback returned an error first.
func (f *fakePacketSource) waitVisited(t *testing.T, n int) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-f.visited:
		case err := <-f.stopped:
			t.Fatalf("packet reader stopped after %d of %d packets: %s", i, n, err)
		case <-timeout:
			t.Fatalf("packet reader visited only %d of %d packets", i, n)
		}
	}
}

func (f *fakePacketSource) LayerType() gopacket.LayerType { return layers.LayerTypeEthernet }

func (f *fakePacketSource) Close() {
	f.closeOnce.Do(func() { close(f.closed) })
}

type tcpFlags struct {
	syn, ack, fin, rst bool
}

// buildTCPFrame serializes an Ethernet/IPv4/TCP or Ethernet/IPv6/TCP frame from src to dst
func buildTCPFrame(t *testing.T, src, dst net.IP, sport, dport uint16, seq, ack uint32, flags tcpFlags, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC: net.HardwareAddr{0x02, 0, 0, 0, 0, 1},
		DstMAC: net.HardwareAddr{0x02, 0, 0, 0, 0, 2},
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(sport),
		DstPort: layers.TCPPort(dport),
		Seq:     seq,
		Ack:     ack,
		SYN:     flags.syn,
		ACK:     flags.ack,
		FIN:     flags.fin,
		RST:     flags.rst,
		Window:  65535,
	}
	var ipLayer gopacket.SerializableLayer
	if src.To4() != nil {
		eth.EthernetType = layers.EthernetTypeIPv4
		ip4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src.To4(), DstIP: dst.To4()}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip4))
		ipLayer = ip4
	} else {
		eth.EthernetType = layers.EthernetTypeIPv6
		ip6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip6))
		ipLayer = ip6
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ipLayer, tcp, gopacket.Payload(payload)))
	return buf.Bytes()
}

const (
	ethHeaderLen  = 14
	ipv4HeaderLen = 20
)

// unboundPort returns a port from 40000 up that the tracer's bound-ports lookup does not
// report as a listening TCP port, so that the tracer guesses the direction from the SYN
func unboundPort(t *testing.T, tr *ebpfLessTracer, after uint16) uint16 {
	t.Helper()
	for p := max(after+1, 40000); p < 60000; p++ {
		if !tr.boundPorts.Find(network.TCP, p) {
			return p
		}
	}
	t.Fatal("no unbound port found")
	return 0
}

// skippedCounts reads the skipped_packets counter for every reason the test checks
func skippedCounts() map[string]float64 {
	counts := map[string]float64{}
	for _, reason := range []string{skipReasonDecodeError, skipReasonIPv6BigTCP, "unsupported_packet_type", "not_tcp_udp"} {
		counts[reason] = ebpfLessTracerTelemetry.skippedPackets.WithValues(reason).Get()
	}
	return counts
}

func startFakeTracer(t *testing.T, src *fakePacketSource, configure func(*ebpfLessTracer)) (*ebpfLessTracer, <-chan struct{}) {
	t.Helper()
	tr := newEbpfLessTracerWithPacketSource(config.New(), src)
	if configure != nil {
		configure(tr)
	}
	require.NoError(t, tr.Start(func(*network.ConnectionStats) {}))
	t.Cleanup(tr.Stop)

	// readerDone is closed once the packet reader goroutine has exited
	readerDone := make(chan struct{})
	go func() {
		tr.packetSrcBusy.Wait()
		close(readerDone)
	}()
	return tr, readerDone
}

// sendHandshake writes a valid client SYN / SYN-ACK / ACK and a data segment, and
// returns the number of packets written
func sendHandshake(t *testing.T, src *fakePacketSource, clientIP, serverIP net.IP, clientPort, serverPort uint16, payload []byte) int {
	t.Helper()
	const clientISN, serverISN = 1000, 5000
	frames := []fakeItem{
		{data: buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN, 0, tcpFlags{syn: true}, nil), pktType: filter.PacketOutgoing},
		{data: buildTCPFrame(t, serverIP, clientIP, serverPort, clientPort, serverISN, clientISN+1, tcpFlags{syn: true, ack: true}, nil), pktType: filter.PacketHost},
		{data: buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN+1, serverISN+1, tcpFlags{ack: true}, nil), pktType: filter.PacketOutgoing},
		{data: buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN+1, serverISN+1, tcpFlags{ack: true}, payload), pktType: filter.PacketOutgoing},
	}
	for _, f := range frames {
		src.items <- f
	}
	return len(frames)
}

// requireConnection checks that the tracer tracks exactly the given outgoing TCP connection
func requireConnection(t *testing.T, tr *ebpfLessTracer, clientIP, serverIP net.IP, clientPort, serverPort uint16, sentBytes int) {
	t.Helper()
	buf := network.NewConnectionBuffer(4, 4)
	require.NoError(t, tr.GetConnections(buf, nil))
	conns := buf.Connections()
	require.Len(t, conns, 1)
	conn := conns[0]
	assert.Equal(t, util.AddressFromNetIP(clientIP), conn.Source)
	assert.Equal(t, util.AddressFromNetIP(serverIP), conn.Dest)
	assert.Equal(t, clientPort, conn.SPort)
	assert.Equal(t, serverPort, conn.DPort)
	assert.Equal(t, network.TCP, conn.Type)
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.Equal(t, uint64(sentBytes), conn.Monotonic.SentBytes)
}

// TestEbpfLessTracerSkipsUndecodableFrames checks that a frame gopacket cannot decode is
// skipped and counted under its own reason, without reaching the connection processing,
// and that the tracer keeps processing the frames that follow it.
func TestEbpfLessTracerSkipsUndecodableFrames(t *testing.T) {
	src := newFakePacketSource(32)
	tr, _ := startFakeTracer(t, src, nil)
	before := skippedCounts()

	clientIP := net.ParseIP("10.0.0.1").To4()
	serverIP := net.ParseIP("10.0.0.2").To4()
	clientPort := unboundPort(t, tr, 0)
	serverPort := unboundPort(t, tr, clientPort)
	payload := []byte("hello")

	sent := 0
	send := func(data []byte, pktType uint8) {
		src.items <- fakeItem{data: data, pktType: pktType}
		sent++
	}

	// A frame whose IPv4 header length is invalid. The Ethernet layer decodes,
	// then IPv4 decoding fails.
	badIHL := buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, 1000, 0, tcpFlags{syn: true}, nil)
	badIHL[ethHeaderLen] = 0x41 // version 4, IHL 1
	send(badIHL, filter.PacketOutgoing)

	// A segment whose TCP data offset points past the end of the frame. Ethernet
	// and IPv4 decode, then TCP decoding fails.
	badDataOffset := func() []byte {
		f := buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, 1001, 5001, tcpFlags{ack: true}, nil)
		f[ethHeaderLen+ipv4HeaderLen+12] = 0xf0 // data offset 15 (60 bytes), only 20 present
		return f
	}
	send(badDataOffset(), filter.PacketOutgoing)

	// The same undecodable frame, but broadcast: the packet type filter drops it
	// before decoding, so it is not a decode error.
	send(badDataOffset(), filter.PacketBroadcast)

	// An IPv6 BIG TCP frame: payload length 0 and no Hop-by-Hop jumbo option.
	bigTCP := func() []byte {
		f := buildTCPFrame(t, net.ParseIP("fd00::1"), net.ParseIP("fd00::2"), clientPort, serverPort, 1, 0, tcpFlags{ack: true}, payload)
		f[ethHeaderLen+4], f[ethHeaderLen+5] = 0, 0
		return f
	}
	send(bigTCP(), filter.PacketOutgoing)

	// The same bytes behind an IPv4 EtherType are a corrupted IPv4 frame, not BIG TCP.
	notBigTCP := bigTCP()
	binary.BigEndian.PutUint16(notBigTCP[12:14], uint16(layers.EthernetTypeIPv4))
	send(notBigTCP, filter.PacketOutgoing)

	// a valid handshake followed by a data segment, then another undecodable frame
	sent += sendHandshake(t, src, clientIP, serverIP, clientPort, serverPort, payload)
	send(badDataOffset(), filter.PacketOutgoing)

	// the callback processes each packet synchronously, so once it has returned
	// for every packet, the counters and the connection state are final
	src.waitVisited(t, sent)
	after := skippedCounts()
	assert.Equal(t, float64(4), after[skipReasonDecodeError]-before[skipReasonDecodeError], "undecodable frames were not counted as decode errors")
	assert.Equal(t, float64(1), after[skipReasonIPv6BigTCP]-before[skipReasonIPv6BigTCP], "IPv6 BIG TCP frame was not counted")
	assert.Equal(t, float64(1), after["unsupported_packet_type"]-before["unsupported_packet_type"], "broadcast frame was not filtered by packet type")
	// A reader that processed the skipped frames anyway would count them here,
	// because the partially decoded frames have no transport layer.
	assert.Zero(t, after["not_tcp_udp"]-before["not_tcp_udp"], "undecodable frames reached connection processing")

	requireConnection(t, tr, clientIP, serverIP, clientPort, serverPort, len(payload))
}

// TestEbpfLessTracerRetriesPacketSourceErrors checks that the reader retries after a
// transient packet source error, resets its error count once packets are read again,
// and stops at the limit of consecutive errors, even when read passes that read no
// packet come between them.
func TestEbpfLessTracerRetriesPacketSourceErrors(t *testing.T) {
	const maxErrors = 3
	src := newFakePacketSource(32)
	tr, readerDone := startFakeTracer(t, src, func(tr *ebpfLessTracer) {
		tr.readErrorBackoff = time.Millisecond
		tr.maxReadErrorBackoff = 2 * time.Millisecond
		tr.maxConsecutiveReadErrors = maxErrors
	})
	errorsBefore := ebpfLessTracerTelemetry.packetSourceErrors.WithValues().Get()

	clientIP := net.ParseIP("10.0.0.1").To4()
	serverIP := net.ParseIP("10.0.0.2").To4()
	clientPort := unboundPort(t, tr, 0)
	serverPort := unboundPort(t, tr, clientPort)
	payload := []byte("hello")

	transient := errors.New("packet poll failed")
	injectErrors := func(n int) {
		for i := 0; i < n; i++ {
			src.items <- fakeItem{err: fmt.Errorf("error %d: %w", i, transient)}
		}
	}

	// one error fewer than the limit, then packets: the reader must survive and read them
	injectErrors(maxErrors - 1)
	src.waitVisited(t, sendHandshake(t, src, clientIP, serverIP, clientPort, serverPort, payload))
	requireConnection(t, tr, clientIP, serverIP, clientPort, serverPort, len(payload))

	// reading packets reset the count, so the same number of errors is tolerated again
	injectErrors(maxErrors - 1)
	src.items <- fakeItem{data: buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, 1006, 5001, tcpFlags{ack: true}, payload), pktType: filter.PacketOutgoing}
	src.waitVisited(t, 1)
	requireConnection(t, tr, clientIP, serverIP, clientPort, serverPort, 2*len(payload))

	// The limit of errors in a row stops the reader. A pass that ends without error but
	// reads no packet, like an idle poll timeout, does not reset the count.
	for i := 0; i < maxErrors; i++ {
		if i > 0 {
			src.items <- fakeItem{idle: true}
		}
		injectErrors(1)
	}
	select {
	case <-readerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("packet reader did not stop after too many consecutive errors")
	}
	assert.Equal(t, float64(3*maxErrors-2), ebpfLessTracerTelemetry.packetSourceErrors.WithValues().Get()-errorsBefore)
}

// requireReaderStopsOnError checks that the reader stops at once, without retrying,
// when the packet source returns err.
func requireReaderStopsOnError(t *testing.T, err error) {
	t.Helper()
	src := newFakePacketSource(4)
	_, readerDone := startFakeTracer(t, src, func(tr *ebpfLessTracer) {
		// long enough that a retry would not exit in time
		tr.readErrorBackoff = time.Hour
	})
	src.items <- fakeItem{err: err}
	select {
	case <-readerDone:
	case <-time.After(10 * time.Second):
		t.Fatalf("packet reader did not stop after fatal packet source error %q", err)
	}
}

// TestEbpfLessTracerStopsOnFatalPacketSourceError checks that the reader stops at once
// when the packet source reports it can no longer be read.
func TestEbpfLessTracerStopsOnFatalPacketSourceError(t *testing.T) {
	for _, err := range []error{net.ErrClosed, os.ErrClosed, syscall.EBADF} {
		t.Run(err.Error(), func(t *testing.T) {
			requireReaderStopsOnError(t, fmt.Errorf("read: %w", err))
		})
	}
}
