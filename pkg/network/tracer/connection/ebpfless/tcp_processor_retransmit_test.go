// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// retransmitNth repeats the nth packet twice
func retransmitNth(packets []testCapture, n int) []testCapture {
	if packets[n].pktType != unix.PACKET_OUTGOING {
		panic("can only retransmit outgoing packets")
	}

	var traffic []testCapture
	traffic = append(traffic, packets[:n]...)
	traffic = append(traffic, packets[n])
	traffic = append(traffic, packets[n:]...)
	return traffic
}

// TestAllRetransmitsOutgoing runs through each possible retransmit on an outgoing connection
func TestAllRetransmitsOutgoing(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
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

	expectedStats := network.StatCounters{
		SentBytes:   123,
		RecvBytes:   345,
		SentPackets: 5 + 1,
		RecvPackets: 5,
		// one retransmit for each case
		Retransmits:    1,
		TCPEstablished: 1,
		TCPClosed:      1,
	}

	t.Run("retransmit SYN", func(t *testing.T) {
		traffic := retransmitNth(basicHandshake, 0)

		f := newTCPTestFixture(t)
		f.runPkts(traffic)

		require.Empty(t, f.conn.TCPFailures)
		require.Equal(t, expectedStats, f.conn.Monotonic)
	})

	t.Run("retransmit data", func(t *testing.T) {
		traffic := retransmitNth(basicHandshake, 3)

		f := newTCPTestFixture(t)
		f.runPkts(traffic)

		require.Empty(t, f.conn.TCPFailures)
		require.Equal(t, expectedStats, f.conn.Monotonic)
	})

	t.Run("retransmit FIN", func(t *testing.T) {
		traffic := retransmitNth(basicHandshake, 8)

		f := newTCPTestFixture(t)
		f.runPkts(traffic)

		require.Empty(t, f.conn.TCPFailures)
		require.Equal(t, expectedStats, f.conn.Monotonic)
	})
}

// TestAllRetransmitsIncoming runs through each possible retransmit on an incoming connection
func TestAllRetransmitsIncoming(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
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

	expectedStats := network.StatCounters{
		SentBytes:      345,
		RecvBytes:      123,
		SentPackets:    5 + 1,
		RecvPackets:    5,
		Retransmits:    1,
		TCPEstablished: 1,
		TCPClosed:      1,
	}

	t.Run("retransmit SYNACK", func(t *testing.T) {
		traffic := retransmitNth(basicHandshake, 1)

		f := newTCPTestFixture(t)
		f.runPkts(traffic)

		require.Empty(t, f.conn.TCPFailures)
		require.Equal(t, expectedStats, f.conn.Monotonic)
	})

	t.Run("retransmit data", func(t *testing.T) {
		traffic := retransmitNth(basicHandshake, 5)

		f := newTCPTestFixture(t)
		f.runPkts(traffic)

		require.Empty(t, f.conn.TCPFailures)
		require.Equal(t, expectedStats, f.conn.Monotonic)
	})

	t.Run("retransmit FIN", func(t *testing.T) {
		traffic := retransmitNth(basicHandshake, 6)

		f := newTCPTestFixture(t)
		f.runPkts(traffic)

		require.Empty(t, f.conn.TCPFailures)
		require.Equal(t, expectedStats, f.conn.Monotonic)
	})
}

// RST doesn't ever get retransmitted
func TestRstTwice(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// handshake done, now blow up
		pb.outgoing(0, 1, 1, RST|ACK),
		// second RST packet
		pb.outgoing(0, 1, 1, RST|ACK),
	}

	expectedClientStates := []connStatus{
		connStatAttempted,
		connStatAttempted,
		connStatEstablished,
		// reset
		connStatClosed,
		connStatClosed,
	}

	f := newTCPTestFixture(t)
	f.runAgainstState(basicHandshake, expectedClientStates)

	// should count as a single failure
	require.Equal(t, map[uint16]uint32{
		uint16(syscall.ECONNRESET): 1,
	}, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes: 0,
		RecvBytes: 0,
		// doesn't count the packet from the second reset because by that time, the connection is already closed
		// and additional packets no longer affect the outcome
		SentPackets: 2,
		RecvPackets: 2,
		// RSTs are not retransmits
		Retransmits:    0,
		TCPEstablished: 1,
		// should count as a single closed connection
		TCPClosed: 1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestKeepAlivePacketsArentRetransmits(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	basicHandshake := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// send a bunch of keepalive packets
		pb.incoming(0, 1, 1, ACK),
		pb.outgoing(0, 1, 1, ACK),
		pb.incoming(0, 1, 1, ACK),
		pb.outgoing(0, 1, 1, ACK),
		pb.incoming(0, 1, 1, ACK),
		// active close after sending no data
		pb.outgoing(0, 1, 1, FIN|ACK),
		pb.incoming(0, 1, 2, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
	}

	f := newTCPTestFixture(t)
	f.runPkts(basicHandshake)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:   0,
		RecvBytes:   0,
		SentPackets: 5,
		RecvPackets: 6,
		// no retransmits for keepalive
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

// TestRetransmitMultipleSegments tests retransmitting multiple segments as one
func TestRetransmitMultipleSegments(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	traffic := []testCapture{
		pb.incoming(0, 0, 0, SYN),
		pb.outgoing(0, 0, 1, SYN|ACK),
		pb.incoming(0, 1, 1, ACK),
		// send 3x 100 bytes
		pb.outgoing(100, 1, 1, ACK),
		pb.outgoing(100, 101, 1, ACK),
		pb.outgoing(100, 201, 1, FIN|ACK),
		// retransmit all three segments
		pb.outgoing(300, 1, 1, FIN|ACK),
		pb.incoming(0, 1, 302, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
	}

	f := newTCPTestFixture(t)
	f.runPkts(traffic)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:   300,
		RecvBytes:   0,
		SentPackets: 6,
		RecvPackets: 3,
		// one retransmit
		Retransmits:    1,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

// TestPartialOverlapRetransmit tests that when the kernel retransmits a segment
// that partially overlaps with previously-counted data but extends beyond the
// high-water mark.
func TestPartialOverlapRetransmit(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	traffic := []testCapture{
		pb.outgoing(0, 0, 0, SYN),
		pb.incoming(0, 0, 1, SYN|ACK),
		pb.outgoing(0, 1, 1, ACK),
		// send 100 bytes [1..100], maxSeqSent=101, SentBytes=100
		pb.outgoing(100, 1, 1, ACK),
		// partial retransmit [51..150], overlap=50, new=50, maxSeqSent=151, SentBytes=150
		pb.outgoing(100, 51, 1, ACK),
		// another partial retransmit [101..200], overlap=50, new=50, maxSeqSent=201, SentBytes=200
		pb.outgoing(100, 101, 1, ACK),
		// clean close
		pb.outgoing(0, 201, 1, FIN|ACK),
		pb.incoming(0, 1, 202, FIN|ACK),
		pb.outgoing(0, 2, 2, ACK),
	}

	f := newTCPTestFixture(t)
	f.runPkts(traffic)

	require.Empty(t, f.conn.TCPFailures)

	expectedStats := network.StatCounters{
		SentBytes:      200,
		RecvBytes:      0,
		SentPackets:    7,
		RecvPackets:    2,
		Retransmits:    0,
		TCPEstablished: 1,
		TCPClosed:      1,
	}
	require.Equal(t, expectedStats, f.conn.Monotonic)
}

func TestIncomingRetransmitAfterClose(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)

	// same as TestImmediateFin but adds an incoming retransmit at the end
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

	f.runAgainstState([]testCapture{
		// that outgoing ACK was lost, the client retransmits FIN
		pb.incoming(0, 1, 2, FIN|ACK),
		// we re-send the final ACK
		pb.outgoing(0, 2, 2, ACK),
	}, []connStatus{
		// it stays closed throughout
		connStatClosed,
		connStatClosed,
	})

	// ideally this would get counted as a retransmit, but the closed connection is already
	// sent off to the tracer at this point, so the stats can't change.
	// I think this situation is rare enough that it's not too important
	require.Empty(t, f.conn.TCPFailures)
	require.Equal(t, expectedStats, f.conn.Monotonic)
}
