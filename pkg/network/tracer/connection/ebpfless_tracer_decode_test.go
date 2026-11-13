// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build (linux && bpf) || darwin

package connection

import (
	"net"
	"sync"
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

type fakePacket struct {
	data    []byte
	pktType uint8
}

// fakePacketSource is a filter.PacketSource that delivers the packets written to it.
// Like the real sources, VisitPackets returns nil when no packet arrives within a poll timeout.
type fakePacketSource struct {
	packets   chan fakePacket
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakePacketSource() *fakePacketSource {
	return &fakePacketSource{
		packets: make(chan fakePacket, 16),
		closed:  make(chan struct{}),
	}
}

func (f *fakePacketSource) VisitPackets(visit func(data []byte, info filter.PacketInfo, timestamp time.Time) error) error {
	for {
		select {
		case pkt := <-f.packets:
			info := &fakePacketInfo{pktType: pkt.pktType, length: len(pkt.data)}
			if err := visit(pkt.data, info, time.Now()); err != nil {
				return err
			}
		case <-time.After(10 * time.Millisecond):
			return nil
		case <-f.closed:
			return nil
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

// buildTCPFrame serializes an Ethernet/IPv4/TCP frame from src to dst
func buildTCPFrame(t *testing.T, src, dst net.IP, sport, dport uint16, seq, ack uint32, flags tcpFlags, payload []byte) []byte {
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 1},
		DstMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 2},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip4 := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    src,
		DstIP:    dst,
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
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip4))

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip4, tcp, gopacket.Payload(payload)))
	return buf.Bytes()
}

const (
	ethHeaderLen  = 14
	ipv4HeaderLen = 20
)

// TestEbpfLessTracerSkipsUndecodableFrames checks that a frame gopacket cannot decode is
// skipped and counted, and that the tracer keeps processing the frames that follow it.
func TestEbpfLessTracerSkipsUndecodableFrames(t *testing.T) {
	cfg := config.New()
	src := newFakePacketSource()
	tr := newEbpfLessTracerWithPacketSource(cfg, src)

	var closedMu sync.Mutex
	var closed []network.ConnectionStats
	require.NoError(t, tr.Start(func(c *network.ConnectionStats) {
		closedMu.Lock()
		defer closedMu.Unlock()
		closed = append(closed, *c)
	}))
	t.Cleanup(tr.Stop)

	skipped := ebpfLessTracerTelemetry.skippedPackets.WithValues("decode_error")
	skippedBefore := skipped.Get()

	clientIP := net.ParseIP("10.0.0.1").To4()
	serverIP := net.ParseIP("10.0.0.2").To4()
	const clientPort, serverPort = 40000, 40001
	const clientISN, serverISN = 1000, 5000
	payload := []byte("hello")

	send := func(data []byte, pktType uint8) {
		src.packets <- fakePacket{data: data, pktType: pktType}
	}

	// A frame whose IPv4 header length is invalid. The Ethernet layer decodes,
	// then IPv4 decoding fails.
	badIHL := buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN, 0, tcpFlags{syn: true}, nil)
	badIHL[ethHeaderLen] = 0x41 // version 4, IHL 1
	send(badIHL, filter.PacketOutgoing)

	// A RST for the same tuple whose TCP data offset points past the end of the
	// frame. gopacket fills in the TCP ports and flags before failing, so this
	// frame must be skipped as a whole and must not reset the connection.
	badRST := func() []byte {
		f := buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN+1, serverISN+1, tcpFlags{rst: true, ack: true}, nil)
		f[ethHeaderLen+ipv4HeaderLen+12] = 0xf0 // data offset 15 (60 bytes), only 20 present
		return f
	}
	send(badRST(), filter.PacketOutgoing)

	// a valid handshake followed by a data segment
	send(buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN, 0, tcpFlags{syn: true}, nil), filter.PacketOutgoing)
	send(buildTCPFrame(t, serverIP, clientIP, serverPort, clientPort, serverISN, clientISN+1, tcpFlags{syn: true, ack: true}, nil), filter.PacketHost)
	send(buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN+1, serverISN+1, tcpFlags{ack: true}, nil), filter.PacketOutgoing)
	send(buildTCPFrame(t, clientIP, serverIP, clientPort, serverPort, clientISN+1, serverISN+1, tcpFlags{ack: true}, payload), filter.PacketOutgoing)

	// another undecodable RST, after the connection is established
	send(badRST(), filter.PacketOutgoing)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, float64(3), skipped.Get()-skippedBefore)
	}, 5*time.Second, 10*time.Millisecond, "undecodable frames were not counted")

	expectedTuple := network.ConnectionTuple{
		Source: util.AddressFromNetIP(clientIP),
		Dest:   util.AddressFromNetIP(serverIP),
		SPort:  clientPort,
		DPort:  serverPort,
		Type:   network.TCP,
		Family: network.AFINET,
	}
	// the packets are visited in order, so by the time the last frame has been
	// counted the valid frames before it have been processed
	buf := network.NewConnectionBuffer(4, 4)
	require.NoError(t, tr.GetConnections(buf, nil))
	conns := buf.Connections()
	require.Len(t, conns, 1)
	conn := conns[0]
	assert.Equal(t, expectedTuple.Source, conn.Source)
	assert.Equal(t, expectedTuple.Dest, conn.Dest)
	assert.Equal(t, expectedTuple.SPort, conn.SPort)
	assert.Equal(t, expectedTuple.DPort, conn.DPort)
	assert.Equal(t, expectedTuple.Type, conn.Type)
	assert.Equal(t, expectedTuple.Family, conn.Family)
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.Equal(t, uint64(len(payload)), conn.Monotonic.SentBytes)
	assert.Empty(t, conn.TCPFailures, "undecodable RST must not be counted as a failure")

	closedMu.Lock()
	defer closedMu.Unlock()
	assert.Empty(t, closed, "undecodable RST must not close the connection")
}
