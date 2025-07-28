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
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"

	"golang.org/x/sys/unix"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var localhost = net.ParseIP("127.0.0.1")
var remoteIP = net.ParseIP("12.34.56.78")

const (
	minIhl            = 5
	defaultLocalPort  = 12345
	defaultRemotePort = 8080
	defaultNsID       = 123
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
	timestampNs uint64
	pktType     uint8
	ipv4        *layers.IPv4
	ipv6        *layers.IPv6
	tcp         *layers.TCP
}

// TODO can this be merged with the logic creating scratchConns in ebpfless tracer?
func makeTCPStates(synPkt testCapture) *network.ConnectionStats {
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
			Source:    util.AddressFromNetIP(srcIP),
			Dest:      util.AddressFromNetIP(dstIP),
			Pid:       0, // packet capture does not have PID information.
			NetNS:     defaultNsID,
			SPort:     uint16(synPkt.tcp.SrcPort),
			DPort:     uint16(synPkt.tcp.DstPort),
			Type:      network.TCP,
			Family:    family,
			Direction: direction,
		},
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
		timestampNs: 0, // timestampNs not populated except in tcp_processor_rtt_test
		pktType:     unix.PACKET_HOST,
		ipv4:        &ipv4,
		ipv6:        nil,
		tcp:         &tcp,
	}
}

func (pb packetBuilder) outgoing(payloadLen uint16, relSeq, relAck uint32, flags uint8) testCapture {
	ipv4 := ipv4Packet(localhost, remoteIP, minIhl*4+tcpHeaderSize+payloadLen)
	seq := relSeq + pb.remoteSeqBase
	ack := relAck + pb.localSeqBase
	tcp := tcpPacket(defaultLocalPort, defaultRemotePort, seq, ack, flags)
	return testCapture{
		timestampNs: 0, // timestampNs not populated except in tcp_processor_rtt_test
		pktType:     unix.PACKET_OUTGOING,
		ipv4:        &ipv4,
		ipv6:        nil,
		tcp:         &tcp,
	}
}

func newTCPTestFixture(t *testing.T) *tcpTestFixture {
	cfg := config.New()
	return &tcpTestFixture{
		t:    t,
		tcp:  NewTCPProcessor(cfg),
		conn: nil,
	}
}

func (fixture *tcpTestFixture) getConnectionState() *connectionState {
	tuple := MakeEbpflessTuple(fixture.conn.ConnectionTuple)
	conn, ok := fixture.tcp.getConn(tuple)
	if ok {
		return conn
	}
	return &connectionState{}
}

func (fixture *tcpTestFixture) runPkt(pkt testCapture) ProcessResult {
	if fixture.conn == nil {
		fixture.conn = makeTCPStates(pkt)
	}
	result, err := fixture.tcp.Process(fixture.conn, pkt.timestampNs, pkt.pktType, pkt.ipv4, pkt.ipv6, pkt.tcp)
	require.NoError(fixture.t, err)
	return result
}

func (fixture *tcpTestFixture) runPkts(packets []testCapture) {
	for _, pkt := range packets {
		fixture.runPkt(pkt)
	}
}

func (fixture *tcpTestFixture) runAgainstState(packets []testCapture, expected []connStatus) {
	require.Equal(fixture.t, len(packets), len(expected), "packet length didn't match expected states length")
	var expectedStrs []string
	var actualStrs []string

	for i, pkt := range packets {
		expectedStrs = append(expectedStrs, labelForState(expected[i]))

		fixture.runPkt(pkt)
		tcpState := fixture.getConnectionState().tcpState
		actualStrs = append(actualStrs, labelForState(tcpState))
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		// three-way handshake finishes here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		// passive close begins here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		// final FIN was ack'd
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

	require.Empty(t, f.tcp.pendingConns)
	require.Empty(t, f.tcp.establishedConns)
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		// three-way handshake finishes here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		// active close begins here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

	require.Empty(t, f.tcp.pendingConns)
	require.Empty(t, f.tcp.establishedConns)
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		// three-way handshake finishes here
		connStatEstablished,
		connStatEstablished,
		// passive close begins here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		// three-way handshake finishes here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		// active close begins here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		connStatEstablished,
		// active close begins here
		connStatEstablished,
		connStatEstablished,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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
		TCPClosed:      1,
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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
		TCPClosed:      1,
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		connStatEstablished,
		// reset
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

func TestProcessResult(t *testing.T) {
	// same as TestImmediateFin but checks ProcessResult
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

	processResults := []ProcessResult{
		ProcessResultNone,
		ProcessResultNone,
		ProcessResultStoreConn,
		ProcessResultStoreConn,
		ProcessResultStoreConn,
		ProcessResultCloseConn,
	}

	f := newTCPTestFixture(t)

	for i, pkt := range basicHandshake {
		require.Equal(t, processResults[i], f.runPkt(pkt), "packet #%d has the wrong ProcessResult", i)
	}
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		connStatEstablished,
		// active close begins here
		connStatEstablished,
		connStatEstablished,
		connStatEstablished,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		connStatAttempted,
		connStatEstablished,
		// active close begins here
		connStatEstablished,
		connStatEstablished,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
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

// TestOpenCloseConn checks whether IsClosed is set correctly in ConnectionStats
func TestOpenCloseConn(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)

	f := newTCPTestFixture(t)

	// send a SYN packet to kick things off
	f.runPkt(pb.incoming(0, 0, 0, SYN))
	require.False(t, f.conn.IsClosed)

	// finish up the connection handshake and close it
	remainingPkts := []testCapture{
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// active close after sending no data
		pb.outgoing(0, 1, 1, FIN|ACK),
		pb.incoming(0, 1, 2, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
	}
	f.runPkts(remainingPkts)
	// should be closed now
	require.True(t, f.conn.IsClosed)

	// open it up again, it should not be marked closed afterward
	f.runPkt(pb.incoming(0, 0, 0, SYN))
	require.False(t, f.conn.IsClosed)
}
func TestPreexistingConn(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)

	f := newTCPTestFixture(t)

	capture := []testCapture{
		// just sending data, no SYN
		pb.outgoing(1, 10, 10, ACK),
		pb.incoming(1, 10, 11, ACK),
		// active close after sending no data
		pb.outgoing(0, 11, 11, FIN|ACK),
		pb.incoming(0, 11, 12, FIN|ACK),
		pb.outgoing(0, 12, 12, ACK),
	}
	f.runPkts(capture)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      1,
		RecvBytes:      1,
		SentPackets:    3,
		RecvPackets:    2,
		Retransmits:    0,
		TCPEstablished: 0, // we missed when it established
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestPendingConnExpiry(t *testing.T) {
	now := uint64(time.Now().UnixNano())

	pb := newPacketBuilder(lowerSeq, higherSeq)
	pkt := pb.outgoing(0, 0, 0, SYN)
	pkt.timestampNs = now

	f := newTCPTestFixture(t)

	f.runPkt(pkt)
	require.Len(t, f.tcp.pendingConns, 1)

	// if no time has passed, should not remove the connection
	f.tcp.CleanupExpiredPendingConns(now)
	require.Len(t, f.tcp.pendingConns, 1)

	// if too much time has passed, should remove the connection
	tenSecNs := uint64((10 * time.Second).Nanoseconds())
	f.tcp.CleanupExpiredPendingConns(now + tenSecNs)
	require.Empty(t, f.tcp.pendingConns)
}

func TestTCPProcessorConnDirection(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)

	t.Run("outgoing", func(t *testing.T) {
		f := newTCPTestFixture(t)
		capture := []testCapture{
			pb.outgoing(0, 0, 0, SYN),
			pb.incoming(0, 0, 1, SYN|ACK),
			pb.outgoing(0, 1, 1, ACK),
		}
		f.runPkts(capture)

		require.Equal(t, network.OUTGOING, f.getConnectionState().connDirection)
	})
	t.Run("incoming", func(t *testing.T) {
		f := newTCPTestFixture(t)
		capture := []testCapture{
			pb.incoming(0, 0, 0, SYN),
			pb.outgoing(0, 0, 1, SYN|ACK),
			pb.incoming(0, 1, 1, ACK),
		}
		f.runPkts(capture)

		require.Equal(t, network.INCOMING, f.getConnectionState().connDirection)
	})
	t.Run("preexisting", func(t *testing.T) {
		f := newTCPTestFixture(t)
		capture := []testCapture{
			// just sending data, no SYN
			pb.outgoing(1, 10, 10, ACK),
			pb.incoming(1, 10, 11, ACK),
		}
		f.runPkts(capture)

		require.Equal(t, network.UNKNOWN, f.getConnectionState().connDirection)
	})
}
