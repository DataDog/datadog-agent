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

func (tc testCapture) reverse() testCapture { //nolint:unused // TODO
	ret := tc
	if tc.pktType == unix.PACKET_HOST {
		ret.pktType = unix.PACKET_OUTGOING
	} else {
		ret.pktType = unix.PACKET_HOST
	}
	if tc.ipv4 != nil {
		ipv4 := *tc.ipv4
		ipv4.SrcIP, ipv4.DstIP = ipv4.DstIP, ipv4.SrcIP
		ret.ipv4 = &ipv4
	}
	if tc.ipv6 != nil {
		ipv6 := *tc.ipv6
		ipv6.SrcIP, ipv6.DstIP = ipv6.DstIP, ipv6.SrcIP
		ret.ipv6 = &ipv6
	}
	tcp := *tc.tcp
	tcp.SrcPort, tcp.DstPort = tcp.DstPort, tcp.SrcPort
	ret.tcp = &tcp
	return ret
}
func reversePkts(tc []testCapture) []testCapture { //nolint:unused // TODO
	var ret []testCapture
	for _, t := range tc {
		ret = append(ret, t.reverse())
	}
	return ret
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
	t                           *testing.T
	tcp                         *TCPProcessor
	conn                        *network.ConnectionStats
	localSeqBase, remoteSeqBase uint32
}

const TCP_HEADER_SIZE = 20 //nolint:revive // TODO

func (fixture *tcpTestFixture) incoming(payloadLen uint16, relSeq, relAck uint32, flags uint8) testCapture {
	ipv4 := ipv4Packet(remoteIP, localhost, minIhl*4+TCP_HEADER_SIZE+payloadLen)
	seq := relSeq + fixture.localSeqBase
	ack := relAck + fixture.remoteSeqBase
	tcp := tcpPacket(defaultRemotePort, defaultLocalPort, seq, ack, flags)
	return testCapture{
		pktType: unix.PACKET_HOST,
		ipv4:    &ipv4,
		ipv6:    nil,
		tcp:     &tcp,
	}
}

func (fixture *tcpTestFixture) outgoing(payloadLen uint16, relSeq, relAck uint32, flags uint8) testCapture {
	ipv4 := ipv4Packet(localhost, remoteIP, minIhl*4+TCP_HEADER_SIZE+payloadLen)
	seq := relSeq + fixture.remoteSeqBase
	ack := relAck + fixture.localSeqBase
	tcp := tcpPacket(defaultLocalPort, defaultRemotePort, seq, ack, flags)
	return testCapture{
		pktType: unix.PACKET_OUTGOING,
		ipv4:    &ipv4,
		ipv6:    nil,
		tcp:     &tcp,
	}
}

func newTcpTestFixture(t *testing.T, localSeqBase, remoteSeqBase uint32) *tcpTestFixture { //nolint:revive // TODO
	return &tcpTestFixture{
		t:             t,
		tcp:           NewTCPProcessor(),
		conn:          nil,
		localSeqBase:  localSeqBase,
		remoteSeqBase: remoteSeqBase,
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

func testBasicHandshake(t *testing.T, f *tcpTestFixture) {

	basicHandshake := []testCapture{
		f.outgoing(0, 0, 0, SYN),
		f.incoming(0, 0, 1, SYN|ACK),
		// separate ack and first send of data
		f.outgoing(0, 1, 1, ACK),
		f.outgoing(123, 1, 1, ACK),
		// acknowledge data separately
		f.incoming(0, 1, 124, ACK),
		f.incoming(345, 1, 124, ACK),
		// remote FINs separately
		f.incoming(0, 346, 124, FIN|ACK),
		// local acknowledges data, (not the FIN)
		f.outgoing(0, 124, 346, ACK),
		// local acknowledges FIN and sends their own
		f.outgoing(0, 124, 347, FIN|ACK),
		// remote sends final ACK
		f.incoming(0, 347, 125, ACK),
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
		f := newTcpTestFixture(t, lowerSeq, higherSeq)
		testBasicHandshake(t, f)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		f := newTcpTestFixture(t, higherSeq, lowerSeq)
		testBasicHandshake(t, f)
	})
}

func testReversedBasicHandshake(t *testing.T, f *tcpTestFixture) {
	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		// separate ack and first send of data
		f.incoming(0, 1, 1, ACK),
		f.incoming(123, 1, 1, ACK),
		// acknowledge data separately
		f.outgoing(0, 1, 124, ACK),
		f.outgoing(345, 1, 124, ACK),
		// local FINs separately
		f.outgoing(0, 346, 124, FIN|ACK),
		// remote acknowledges data, (not the FIN)
		f.incoming(0, 124, 346, ACK),
		// remote acknowledges FIN and sends their own
		f.incoming(0, 124, 347, FIN|ACK),
		// local sends final ACK
		f.outgoing(0, 347, 125, ACK),
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
		f := newTcpTestFixture(t, lowerSeq, higherSeq)
		testReversedBasicHandshake(t, f)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		f := newTcpTestFixture(t, higherSeq, lowerSeq)
		testReversedBasicHandshake(t, f)
	})
}

func testCloseWaitState(t *testing.T, f *tcpTestFixture) {
	// test the CloseWait state, which is when the local client still has data left
	// to send during a passive close

	basicHandshake := []testCapture{
		f.outgoing(0, 0, 0, SYN),
		f.incoming(0, 0, 1, SYN|ACK),
		// local sends data right out the gate with ACK
		f.outgoing(123, 1, 1, ACK),
		// remote acknowledges and sends data back
		f.incoming(345, 1, 124, ACK),
		// remote FINs separately
		f.incoming(0, 346, 124, FIN|ACK),
		// local acknowledges FIN, but keeps sending data for a bit
		f.outgoing(100, 124, 347, ACK),
		// client finally FINACKs
		f.outgoing(42, 224, 347, FIN|ACK),
		// remote acknowledges data but not including the FIN
		f.incoming(0, 347, 224, ACK),
		// server sends final ACK
		f.incoming(0, 347, 224+42+1, ACK),
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
		f := newTcpTestFixture(t, lowerSeq, higherSeq)
		testCloseWaitState(t, f)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		f := newTcpTestFixture(t, higherSeq, lowerSeq)
		testCloseWaitState(t, f)
	})
}

func testFinWait2State(t *testing.T, f *tcpTestFixture) {
	// test the FinWait2 state, which is when the remote still has data left
	// to send during an active close

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		// separate ack and first send of data
		f.incoming(0, 1, 1, ACK),
		f.incoming(123, 1, 1, ACK),
		// acknowledge data separately
		f.outgoing(0, 1, 124, ACK),
		f.outgoing(345, 1, 124, ACK),
		// local FINs separately
		f.outgoing(0, 346, 124, FIN|ACK),
		// remote acknowledges the FIN but keeps sending data
		f.incoming(100, 124, 347, ACK),
		// local acknowledges this data
		f.outgoing(0, 347, 224, ACK),
		// remote sends their own FIN
		f.incoming(0, 224, 347, FIN|ACK),
		// local sends final ACK
		f.outgoing(0, 347, 225, ACK),
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
		f := newTcpTestFixture(t, lowerSeq, higherSeq)
		testFinWait2State(t, f)
	})

	t.Run("localSeq gt remoteSeq", func(t *testing.T) {
		f := newTcpTestFixture(t, higherSeq, lowerSeq)
		testFinWait2State(t, f)
	})
}

func TestImmediateFin(t *testing.T) {
	// originally captured from TestTCPConnsReported which closes connections right as it gets them

	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		f.incoming(0, 1, 1, ACK),
		// active close after sending no data
		f.outgoing(0, 1, 1, FIN|ACK),
		f.incoming(0, 1, 2, FIN|ACK),
		f.outgoing(0, 2, 2, ACK),
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
	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 0, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatClosed,
	}

	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Equal(t, f.conn.TCPFailures, map[uint16]uint32{
		uint16(syscall.ECONNREFUSED): 1,
	})

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
	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		f.outgoing(0, 0, 0, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatClosed,
	}

	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Equal(t, f.conn.TCPFailures, map[uint16]uint32{
		uint16(syscall.ECONNREFUSED): 1,
	})

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
	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		f.incoming(0, 1, 1, ACK),
		// handshake done, now blow up
		f.outgoing(0, 1, 1, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// reset
		ConnStatClosed,
	}

	f.runAgainstState(basicHandshake, expectedClientStates)

	require.Equal(t, f.conn.TCPFailures, map[uint16]uint32{
		uint16(syscall.ECONNRESET): 1,
	})

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
	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		f.incoming(0, 1, 1, ACK),
		// handshake done, now blow up
		f.outgoing(0, 1, 1, RST|ACK),
		f.outgoing(0, 1, 1, RST|ACK),
	}

	expectedClientStates := []ConnStatus{
		ConnStatAttempted,
		ConnStatAttempted,
		ConnStatEstablished,
		// reset
		ConnStatClosed,
		ConnStatClosed,
	}

	f.runAgainstState(basicHandshake, expectedClientStates)

	// should count as a single failure
	require.Equal(t, f.conn.TCPFailures, map[uint16]uint32{
		uint16(syscall.ECONNRESET): 1,
	})

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

	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		f.incoming(0, 1, 1, ACK),
		// active close after sending no data
		f.outgoing(0, 1, 1, FIN|ACK),
		f.incoming(0, 1, 2, FIN|ACK),
		f.outgoing(0, 2, 2, ACK),
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
	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		f.outgoing(0, 0, 1, SYN|ACK),
		f.incoming(0, 1, 1, ACK),
		// active close after sending no data
		f.outgoing(0, 1, 1, FIN|ACK),
		f.incoming(0, 1, 1, FIN|ACK),
		f.outgoing(0, 2, 2, ACK),
		f.incoming(0, 2, 2, ACK),
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
	f := newTcpTestFixture(t, lowerSeq, higherSeq)

	basicHandshake := []testCapture{
		f.incoming(0, 0, 0, SYN),
		// ACK the first SYN before even sending your own SYN
		f.outgoing(0, 0, 1, ACK),
		f.outgoing(0, 0, 1, SYN),
		f.incoming(0, 1, 1, ACK),
		// active close after sending no data
		f.outgoing(0, 1, 1, FIN|ACK),
		f.incoming(0, 1, 2, FIN|ACK),
		f.outgoing(0, 2, 2, ACK),
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
