// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"net"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var localhost net.IP = net.ParseIP("127.0.0.1")  //nolint:revive // TODO
var remoteIP net.IP = net.ParseIP("12.34.56.78") //nolint:revive // TODO

const (
	minIhl            = 5
	defaultLocalPort  = 12345
	defaultRemotePort = 8080
	defaultNsId       = 123 //nolint:revive // TODO
)

const (
	FIN = 0x01
	SYN = 0x02
	RST = 0x04
	ACK = 0x10
)

func ipv4Packet(src, dst net.IP, length uint16) layers.IPv4 {
	return layers.IPv4{
		Version:    4,
		IHL:        minIhl,
		TOS:        0,
		Length:     length,
		Id:         12345,
		Flags:      0x02,
		FragOffset: 0,
		TTL:        64,
		Protocol:   6,
		Checksum:   0xbeef,
		SrcIP:      src,
		DstIP:      dst,
	}
}

func tcpPacket(srcPort, dstPort uint16, seq, ack uint32, flags uint8) layers.TCP {
	return layers.TCP{
		DataOffset: 5,
		Window:     65535,
		SrcPort:    layers.TCPPort(srcPort),
		DstPort:    layers.TCPPort(dstPort),
		Seq:        seq,
		Ack:        ack,
		FIN:        flags&FIN != 0,
		SYN:        flags&SYN != 0,
		RST:        flags&RST != 0,
		ACK:        flags&ACK != 0,
	}
}

type testCapture struct {
	pktType uint8
	ipv4    *layers.IPv4
	ipv6    *layers.IPv6
	tcp     *layers.TCP
}

// TODO can this be merged with the logic creating scratchConns in ebpfless tracer?
func makeTcpStates(synPkt testCapture) *network.ConnectionStats { //nolint:revive // TODO
	var family network.ConnectionFamily
	var srcIP, dstIP net.IP
	if synPkt.ipv4 != nil && synPkt.ipv6 != nil {
		panic("testCapture shouldn't have both IP families")
	}
	if synPkt.ipv4 != nil {
		family = network.AFINET
		srcIP = synPkt.ipv4.SrcIP
		dstIP = synPkt.ipv4.DstIP
	} else if synPkt.ipv6 != nil {
		family = network.AFINET6
		srcIP = synPkt.ipv6.SrcIP
		dstIP = synPkt.ipv6.DstIP
	} else {
		panic("testCapture had no IP family")
	}
	var direction network.ConnectionDirection
	switch synPkt.pktType {
	case unix.PACKET_HOST:
		direction = network.INCOMING
	case unix.PACKET_OUTGOING:
		direction = network.OUTGOING
	default:
		panic("testCapture had unknown packet type")
	}
	return &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromNetIP(srcIP),
			Dest:   util.AddressFromNetIP(dstIP),
			Pid:    0, // @stu we can't know this right
			NetNS:  defaultNsId,
			SPort:  uint16(synPkt.tcp.SrcPort),
			DPort:  uint16(synPkt.tcp.DstPort),
			Type:   network.TCP,
			Family: family,
		},
		Direction:   direction,
		TCPFailures: make(map[uint16]uint32),
	}
}

type tcpTestFixture struct {
	t    *testing.T
	tcp  *TCPProcessor
	conn *network.ConnectionStats
}

type packetBuilder struct {
	localSeqBase, remoteSeqBase uint32
}

const tcpHeaderSize = 20

func newPacketBuilder(localSeqBase, remoteSeqBase uint32) packetBuilder {
	return packetBuilder{localSeqBase: localSeqBase, remoteSeqBase: remoteSeqBase}
}

func (pb packetBuilder) incoming(payloadLen uint16, relSeq, relAck uint32, flags uint8) testCapture {
	ipv4 := ipv4Packet(remoteIP, localhost, minIhl*4+tcpHeaderSize+payloadLen)
	seq := relSeq + pb.localSeqBase
	ack := relAck + pb.remoteSeqBase
	tcp := tcpPacket(defaultRemotePort, defaultLocalPort, seq, ack, flags)
	return testCapture{
		pktType: unix.PACKET_HOST,
		ipv4:    &ipv4,
		ipv6:    nil,
		tcp:     &tcp,
	}
}

func (pb packetBuilder) outgoing(payloadLen uint16, relSeq, relAck uint32, flags uint8) testCapture {
	ipv4 := ipv4Packet(localhost, remoteIP, minIhl*4+tcpHeaderSize+payloadLen)
	seq := relSeq + pb.remoteSeqBase
	ack := relAck + pb.localSeqBase
	tcp := tcpPacket(defaultLocalPort, defaultRemotePort, seq, ack, flags)
	return testCapture{
		pktType: unix.PACKET_OUTGOING,
		ipv4:    &ipv4,
		ipv6:    nil,
		tcp:     &tcp,
	}
}

func newTcpTestFixture(t *testing.T) *tcpTestFixture { //nolint:revive // TODO
	return &tcpTestFixture{
		t:    t,
		tcp:  NewTCPProcessor(),
		conn: nil,
	}
}

func (fixture *tcpTestFixture) runPkt(pkt testCapture) {
	if fixture.conn == nil {
		fixture.conn = makeTcpStates(pkt)
	}
	err := fixture.tcp.Process(fixture.conn, pkt.pktType, pkt.ipv4, pkt.ipv6, pkt.tcp)
	require.NoError(fixture.t, err)
}

func (fixture *tcpTestFixture) runPkts(packets []testCapture) { //nolint:unused // TODO
	for _, pkt := range packets {
		fixture.runPkt(pkt)
	}
}

func (fixture *tcpTestFixture) runAgainstState(packets []testCapture, expected []ConnStatus) {
	require.Equal(fixture.t, len(packets), len(expected), "packet length didn't match expected states length")
	var expectedStrs []string
	var actualStrs []string

	for i, pkt := range packets {
		expectedStrs = append(expectedStrs, LabelForState(expected[i]))

		fixture.runPkt(pkt)
		connTuple := fixture.conn.ConnectionTuple
		actual := fixture.tcp.conns[connTuple].tcpState
		actualStrs = append(actualStrs, LabelForState(actual))
	}
	require.Equal(fixture.t, expectedStrs, actualStrs)
}

func testBasicHandshake(t *testing.T, pb packetBuilder) {
	basicHandshake := []testCapture{
		pb.outgoing(0, 0, 0, SYN),
		pb.incoming(0, 0, 1, SYN|ACK),
		// separate ack and first send of data
		pb.outgoing(0, 1, 1, ACK),
		pb.outgoing(123, 1, 1, ACK),
		// acknowledge data separately
		pb.incoming(0, 1, 124, ACK),
		pb.incoming(345, 1, 124, ACK),
		// remote FINs separately
		pb.incoming(0, 346, 124, FIN|ACK),
		// local acknowledges data, (not the FIN)
		pb.outgoing(0, 124, 346, ACK),
		// local acknowledges FIN and sends their own
		pb.outgoing(0, 124, 347, FIN|ACK),
		// remote sends final ACK
		pb.incoming(0, 347, 125, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		// three-way handshake finishes here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		// passive close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		// final FIN was ack'd
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      123,
		RecvBytes:      345,
		SentPackets:    5,
		RecvPackets:    5,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}

	require.Equal(t, expectedStats, f.conn.Monotonic)
}

var lowerSeq uint32 = 2134452051
var higherSeq uint32 = 2973263073

func TestBasicHandshake(t *testing.T) {
	t.Run("localSeq lt remoteSeq", func(t *testing.T) {
		pb := newPacketBuilder(lowerSeq, higherSeq)
		testBasicHandshake(t, pb)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		pb := newPacketBuilder(higherSeq, lowerSeq)
		testBasicHandshake(t, pb)
	})
}

func testReversedBasicHandshake(t *testing.T, pb packetBuilder) {
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		// separate ack and first send of data
		pb.incoming(0, 1, 1, ACK),
		pb.incoming(123, 1, 1, ACK),
		// acknowledge data separately
		pb.outgoing(0, 1, 124, ACK),
		pb.outgoing(345, 1, 124, ACK),
		// local FINs separately
		pb.outgoing(0, 346, 124, FIN|ACK),
		// remote acknowledges data, (not the FIN)
		pb.incoming(0, 124, 346, ACK),
		// remote acknowledges FIN and sends their own
		pb.incoming(0, 124, 347, FIN|ACK),
		// local sends final ACK
		pb.outgoing(0, 347, 125, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		// three-way handshake finishes here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		// active close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      345,
		RecvBytes:      123,
		SentPackets:    5,
		RecvPackets:    5,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestReversedBasicHandshake(t *testing.T) {
	t.Run("localSeq lt remoteSeq", func(t *testing.T) {
		pb := newPacketBuilder(lowerSeq, higherSeq)
		testReversedBasicHandshake(t, pb)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		pb := newPacketBuilder(higherSeq, lowerSeq)
		testReversedBasicHandshake(t, pb)
	})
}

func testCloseWaitState(t *testing.T, pb packetBuilder) {
	// test the CloseWait state, which is when the local client still has data left
	// to send during a passive close

	basicHandshake := []testCapture{
		pb.outgoing(0, 0, 0, SYN),
		pb.incoming(0, 0, 1, SYN|ACK),
		// local sends data right out the gate with ACK
		pb.outgoing(123, 1, 1, ACK),
		// remote acknowledges and sends data back
		pb.incoming(345, 1, 124, ACK),
		// remote FINs separately
		pb.incoming(0, 346, 124, FIN|ACK),
		// local acknowledges FIN, but keeps sending data for a bit
		pb.outgoing(100, 124, 347, ACK),
		// client finally FINACKs
		pb.outgoing(42, 224, 347, FIN|ACK),
		// remote acknowledges data but not including the FIN
		pb.incoming(0, 347, 224, ACK),
		// server sends final ACK
		pb.incoming(0, 347, 224+42+1, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		// three-way handshake finishes here
		ConnStatEstablished,
		ConnStatEstablished,
		// passive close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      123 + 100 + 42,
		RecvBytes:      345,
		SentPackets:    4,
		RecvPackets:    5,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestCloseWaitState(t *testing.T) {
	t.Run("localSeq lt remoteSeq", func(t *testing.T) {
		pb := newPacketBuilder(lowerSeq, higherSeq)
		testCloseWaitState(t, pb)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		f := newPacketBuilder(higherSeq, lowerSeq)
		testCloseWaitState(t, f)
	})
}

func testFinWait2State(t *testing.T, pb packetBuilder) {
	// test the FinWait2 state, which is when the remote still has data left
	// to send during an active close

	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		// separate ack and first send of data
		pb.incoming(0, 1, 1, ACK),
		pb.incoming(123, 1, 1, ACK),
		// acknowledge data separately
		pb.outgoing(0, 1, 124, ACK),
		pb.outgoing(345, 1, 124, ACK),
		// local FINs separately
		pb.outgoing(0, 346, 124, FIN|ACK),
		// remote acknowledges the FIN but keeps sending data
		pb.incoming(100, 124, 347, ACK),
		// local acknowledges this data
		pb.outgoing(0, 347, 224, ACK),
		// remote sends their own FIN
		pb.incoming(0, 224, 347, FIN|ACK),
		// local sends final ACK
		pb.outgoing(0, 347, 225, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		// three-way handshake finishes here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		// active close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      345,
		RecvBytes:      223,
		SentPackets:    6,
		RecvPackets:    5,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestFinWait2State(t *testing.T) {
	t.Run("localSeq lt remoteSeq", func(t *testing.T) {
		pb := newPacketBuilder(lowerSeq, higherSeq)
		testFinWait2State(t, pb)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		pb := newPacketBuilder(higherSeq, lowerSeq)
		testFinWait2State(t, pb)
	})
}

func TestImmediateFin(t *testing.T) {
	// originally captured from TestTCPConnsReported which closes connections right as it gets them

	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// active close after sending no data
		pb.outgoing(0, 1, 1, FIN|ACK),
		pb.incoming(0, 1, 2, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// active close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    3,
		RecvPackets:    3,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestConnRefusedSyn(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 0, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Equal(t, map[uint16]uint32{
		uint16(syscall.ECONNREFUSED): 1,
	}, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    1,
		RecvPackets:    1,
		Retransmits:    0,
		TCPEstablished: 0,
		TCPClosed:      0,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestConnRefusedSynAck(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.outgoing(0, 0, 0, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Equal(t, map[uint16]uint32{
		uint16(syscall.ECONNREFUSED): 1,
	}, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    2,
		RecvPackets:    1,
		Retransmits:    0,
		TCPEstablished: 0,
		TCPClosed:      0,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestConnReset(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// handshake done, now blow up
		pb.outgoing(0, 1, 1, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// reset
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Equal(t, map[uint16]uint32{
		uint16(syscall.ECONNRESET): 1,
	}, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    2,
		RecvPackets:    2,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestRstRetransmit(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// handshake done, now blow up
		pb.outgoing(0, 1, 1, RST|ACK),
		pb.outgoing(0, 1, 1, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// reset
		ConnStatClosed,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	// should count as a single failure
	require.Equal(t, map[uint16]uint32{
		uint16(syscall.ECONNRESET): 1,
	}, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    3,
		RecvPackets:    2,
		Retransmits:    0,
		TCPEstablished: 1,
		// should count as a single closed connection
		TCPClosed: 1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestConnectTwice(t *testing.T) {
	// same as TestImmediateFin but everything happens twice

	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// active close after sending no data
		pb.outgoing(0, 1, 1, FIN|ACK),
		pb.incoming(0, 1, 2, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// active close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	state := f.tcp.conns[f.conn.ConnectionTuple]
	// make sure the TCP state was erased after the connection was closed
	require.Equal(t, connectionState{
		tcpState: ConnStatClosed,
	}, state)

	// second connection here
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    3 * 2,
		RecvPackets:    3 * 2,
		Retransmits:    0,
		TCPEstablished: 1 * 2,
		TCPClosed:      1 * 2,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestSimultaneousClose(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// active close after sending no data
		pb.outgoing(0, 1, 1, FIN|ACK),
		pb.incoming(0, 1, 1, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
		pb.incoming(0, 2, 2, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// active close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    3,
		RecvPackets:    4,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestUnusualAckSyn(t *testing.T) {
	// according to zeek, some unusual clients such as ftp.microsoft.com do the ACK and SYN separately
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		// ACK the first SYN before even sending your own SYN
		pb.outgoing(0, 0, 1, ACK),
		pb.outgoing(0, 0, 1, SYN),
		pb.incoming(0, 1, 1, ACK),
		// active close after sending no data
		pb.outgoing(0, 1, 1, FIN|ACK),
		pb.incoming(0, 1, 2, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// active close begins here
		ConnStatEstablished,
		ConnStatEstablished,
		ConnStatClosed,
	}

	f := newTcpTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      0,
		RecvBytes:      0,
		SentPackets:    4,
		RecvPackets:    3,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}
