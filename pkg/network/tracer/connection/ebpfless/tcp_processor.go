// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type connectionState struct {
	tcpState ConnStatus

	// hasSentPacket is whether anything has been sent outgoing (aka whether maxSeqSent exists)
	hasSentPacket bool
	// maxSeqSent is the latest outgoing tcp.Seq if hasSentPacket==true
	maxSeqSent uint32

	// hasLocalAck is whether there have been outgoing ACK's
	hasLocalAck bool
	// lastLocalAck is the latest outgoing tcp.Ack if hasLocalAck
	lastLocalAck uint32
	// hasRemoteAck is whether there have been incoming ACK's
	hasRemoteAck bool
	// lastRemoteAck is the latest incoming tcp.Ack if hasRemoteAck
	lastRemoteAck uint32

	// localSynState is the status of the outgoing SYN handshake
	localSynState SynState
	// remoteSynState is the status of the incoming SYN handshake
	remoteSynState SynState

	// hasLocalFin is whether the outgoing side has FIN'd
	hasLocalFin bool
	// hasRemoteFin is whether the incoming side has FIN'd
	hasRemoteFin bool
	// localFinSeq is the tcp.Seq number for the outgoing FIN (including any payload length)
	localFinSeq uint32
	// remoteFinSeq is the tcp.Seq number for the incoming FIN (including any payload length)
	remoteFinSeq uint32
}

type TCPProcessor struct { //nolint:revive // TODO
	conns map[network.ConnectionTuple]connectionState
}

func NewTCPProcessor() *TCPProcessor { //nolint:revive // TODO
	return &TCPProcessor{
		conns: map[network.ConnectionTuple]connectionState{},
	}
}

func (t *TCPProcessor) updateSynFlag(conn *network.ConnectionStats, st *connectionState, pktType uint8, tcp *layers.TCP, payloadLen uint16) { //nolint:revive // TODO
	if tcp.RST {
		return
	}
	// progress the synStates based off this packet
	if pktType == unix.PACKET_OUTGOING {
		st.localSynState.update(tcp.SYN, tcp.ACK)
	} else {
		st.remoteSynState.update(tcp.SYN, tcp.ACK)
	}
	// if any SynState has progressed, move to attempted
	if st.tcpState == ConnStatClosed && (st.localSynState != SynStateNone || st.remoteSynState != SynStateNone) {
		st.tcpState = ConnStatAttempted
	}
	// if both synStates are ack'd, move to established
	if st.tcpState == ConnStatAttempted && st.localSynState == SynStateAcked && st.remoteSynState == SynStateAcked {
		st.tcpState = ConnStatEstablished
		conn.Monotonic.TCPEstablished++
	}
}

// updateTcpStats is designed to mirror the stat tracking in the windows driver's handleFlowProtocolTcp
// https://github.com/DataDog/datadog-windows-filter/blob/d7560d83eb627117521d631a4c05cd654a01987e/ddfilter/flow/flow_tcp.c#L91
func (t *TCPProcessor) updateTcpStats(conn *network.ConnectionStats, st *connectionState, pktType uint8, tcp *layers.TCP, payloadLen uint16) { //nolint:revive // TODO
	payloadSeq := tcp.Seq + uint32(payloadLen)

	if pktType == unix.PACKET_OUTGOING {
		conn.Monotonic.SentPackets++
		if !st.hasSentPacket || isSeqBefore(st.maxSeqSent, payloadSeq) {
			st.hasSentPacket = true
			conn.Monotonic.SentBytes += uint64(payloadLen)
			st.maxSeqSent = payloadSeq
		}

		ackOutdated := !st.hasLocalAck || isSeqBefore(st.lastLocalAck, tcp.Ack)
		if tcp.ACK && ackOutdated {
			// wait until data comes in via SynStateAcked
			if st.hasLocalAck && st.remoteSynState == SynStateAcked {
				ackDiff := tcp.Ack - st.lastLocalAck
				// if this is ack'ing a fin packet, there is an extra sequence number to cancel out
				isFinAck := st.hasRemoteFin && tcp.Ack == st.remoteFinSeq+1
				if isFinAck {
					ackDiff--
				}
				conn.Monotonic.RecvBytes += uint64(ackDiff)
			}

			st.hasLocalAck = true
			st.lastLocalAck = tcp.Ack
		}
	} else {
		conn.Monotonic.RecvPackets++

		ackOutdated := !st.hasRemoteAck || isSeqBefore(st.lastRemoteAck, tcp.Ack)
		if tcp.ACK && ackOutdated {
			st.hasRemoteAck = true
			st.lastRemoteAck = tcp.Ack
		}
	}
}

func (t *TCPProcessor) updateFinFlag(conn *network.ConnectionStats, st *connectionState, pktType uint8, tcp *layers.TCP, payloadLen uint16) {
	payloadSeq := tcp.Seq + uint32(payloadLen)
	// update FIN sequence numbers
	if tcp.FIN {
		if pktType == unix.PACKET_OUTGOING {
			st.hasLocalFin = true
			st.localFinSeq = payloadSeq
		} else {
			st.hasRemoteFin = true
			st.remoteFinSeq = payloadSeq
		}
	}

	// if both fins have been sent and ack'd, then mark the connection closed
	localFinIsAcked := st.hasLocalFin && isSeqBefore(st.localFinSeq, st.lastRemoteAck)
	remoteFinIsAcked := st.hasRemoteFin && isSeqBefore(st.remoteFinSeq, st.lastLocalAck)
	if st.tcpState == ConnStatEstablished && localFinIsAcked && remoteFinIsAcked {
		*st = connectionState{
			tcpState: ConnStatClosed,
		}
		conn.Monotonic.TCPClosed++
	}
}

func (t *TCPProcessor) updateRstFlag(conn *network.ConnectionStats, st *connectionState, pktType uint8, tcp *layers.TCP, payloadLen uint16) { //nolint:revive // TODO
	if !tcp.RST || st.tcpState == ConnStatClosed {
		return
	}

	reason := syscall.ECONNRESET
	if st.tcpState == ConnStatAttempted {
		reason = syscall.ECONNREFUSED
	}

	*st = connectionState{
		tcpState: ConnStatClosed,
	}
	conn.TCPFailures[uint16(reason)]++
	conn.Monotonic.TCPClosed++
}

// Process handles a TCP packet, calculating stats and keeping track of its state according to the
// TCP state machine.
func (t *TCPProcessor) Process(conn *network.ConnectionStats, pktType uint8, ip4 *layers.IPv4, ip6 *layers.IPv6, tcp *layers.TCP) error {
	if pktType != unix.PACKET_OUTGOING && pktType != unix.PACKET_HOST {
		return fmt.Errorf("TCPProcessor saw invalid pktType: %d", pktType)
	}
	payloadLen, err := TCPPayloadLen(conn.Family, ip4, ip6, tcp)
	if err != nil {
		return err
	}

	log.TraceFunc(func() string {
		return "tcp processor: " + debugPacketInfo(pktType, tcp, payloadLen)
	})

	// skip invalid packets we don't recognize:
	noFlagsCombo := !tcp.SYN && !tcp.FIN && !tcp.ACK && !tcp.RST
	if noFlagsCombo {
		// no flags at all (I think this can happen for expanding the TCP window sometimes?)
		statsTelemetry.missingTCPFlags.Inc()
		return nil
	}
	synFinCombo := tcp.SYN && tcp.FIN
	if synFinCombo {
		statsTelemetry.tcpSynAndFin.Inc()
		return nil
	}

	st := t.conns[conn.ConnectionTuple]

	t.updateSynFlag(conn, &st, pktType, tcp, payloadLen)
	t.updateTcpStats(conn, &st, pktType, tcp, payloadLen)
	t.updateFinFlag(conn, &st, pktType, tcp, payloadLen)
	t.updateRstFlag(conn, &st, pktType, tcp, payloadLen)

	t.conns[conn.ConnectionTuple] = st
	return nil
}
