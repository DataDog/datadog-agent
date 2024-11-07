// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/layers"
	"golang.org/x/sys/unix"
	"syscall"
)

type connectionState struct {
	tcpState tcpState

	// hasSentPacket is whether anything has been sent outgoing (aka whether maxSeqSent exists)
	hasSentPacket bool
	// localStartSeq is the initial outgoing tcp.Seq - only used for debugging with relative SEQ's
	localStartSeq uint32
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

	// hasReceivedPacket is whether anything has been received - only used for debugging (to see if remoteStartSeq exists)
	hasReceivedPacket bool
	// remoteStartSeq is the initial incoming tcp.Seq - only used for debugging with relative SEQ's
	remoteStartSeq uint32

	// hasLocalSyn is whether there has been an outgoing SYN
	hasLocalSyn bool
	// localSynSeq is the tcp.Seq of the SYN if hasLocalSyn
	localSynSeq uint32
	// hasLocalSyn is whether there has been an incoming SYN
	hasRemoteSyn bool
	// remoteSynSeq is the tcp.Seq of the SYN if hasRemoteSyn
	remoteSynSeq uint32

	// hasLocalFin is whether the outgoing side has FIN'd
	hasLocalFin bool
	// hasRemoteFin is whether the incoming side has FIN'd
	hasRemoteFin bool
	// localFinSeq is the tcp.Seq number for the outgoing FIN (including any payload length)
	localFinSeq uint32
	// remoteFinSeq is the tcp.Seq number for the incoming FIN (including any payload length)
	remoteFinSeq uint32
}

type TCPProcessor struct {
	conns map[network.ConnectionTuple]connectionState
}

func NewTCPProcessor() *TCPProcessor {
	return &TCPProcessor{
		conns: map[network.ConnectionTuple]connectionState{},
	}
}

// Process handles a TCP packet, calculating stats and keeping track of its state according to the
// TCP state machine.
// https://users.cs.northwestern.edu/~agupta/cs340/project2/TCPIP_State_Transition_Diagram.pdf
func (t *TCPProcessor) Process(conn *network.ConnectionStats, pktType uint8, ip4 *layers.IPv4, ip6 *layers.IPv6, tcp *layers.TCP) error {
	payloadLen, err := TCPPayloadLen(conn.Family, ip4, ip6, tcp)
	if err != nil {
		return err
	}

	st := t.conns[conn.ConnectionTuple]
	log.TraceFunc(func() string {
		return fmt.Sprintf("pre ack_seq=%+v", st)
	})

	payloadSeq := tcp.Seq + uint32(payloadLen)

	switch pktType {
	case unix.PACKET_OUTGOING:
		conn.Monotonic.SentPackets++
		if !st.hasSentPacket {
			st.localStartSeq = tcp.Seq
		}
		if !st.hasSentPacket || isSeqBefore(st.maxSeqSent, payloadSeq) {
			st.hasSentPacket = true
			conn.Monotonic.SentBytes += uint64(payloadLen)
			st.maxSeqSent = payloadSeq
		} else {
			// TODO retransmit
		}

		ackOutdated := !st.hasLocalAck || isSeqBefore(st.lastLocalAck, tcp.Ack)
		if tcp.ACK && ackOutdated {
			// wait until data comes in via remoteSynIsAcked
			remoteSynIsAcked := st.hasRemoteSyn && st.hasLocalAck && isSeqBefore(st.remoteSynSeq, st.lastLocalAck)
			if remoteSynIsAcked {
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
	case unix.PACKET_HOST:
		conn.Monotonic.RecvPackets++
		if !st.hasReceivedPacket {
			st.hasReceivedPacket = true
			st.remoteStartSeq = tcp.Seq
		}

		ackOutdated := !st.hasRemoteAck || isSeqBefore(st.lastRemoteAck, tcp.Ack)
		if tcp.ACK && ackOutdated {
			st.hasRemoteAck = true
			st.lastRemoteAck = tcp.Ack
		}
	default:
		return fmt.Errorf("TCPProcessor saw invalid pktType: %d", pktType)
	}

	log.TraceFunc(func() string {
		packetInfo := debugPacketInfo(pktType, tcp, payloadLen, st)
		return "tcp processor: " + packetInfo
	})

	// skip invalid packets we don't recognize.
	synFinCombo := tcp.SYN && tcp.FIN
	noFlagsCombo := !tcp.SYN && !tcp.FIN && !tcp.ACK && !tcp.RST
	if synFinCombo || noFlagsCombo {
		// TODO remove this warning since probably these occur in the wild
		log.Warnf("invalid flags combo: SYN=%t FIN=%t ACK=%t RST=%t", tcp.SYN, tcp.FIN, tcp.ACK, tcp.RST)
		return nil
	}

	// regular ACK (no SYN)
	if !tcp.SYN && tcp.ACK {
		switch st.tcpState {
		case TcpStateClosed:
		// TODO missed the handshake?
		case TcpStateSynSent:
			fallthrough
		case TcpStateSynRecv:
			localSynIsAcked := st.hasLocalSyn && st.hasRemoteAck && isSeqBefore(st.localSynSeq, st.lastRemoteAck)
			remoteSynIsAcked := st.hasRemoteSyn && st.hasLocalAck && isSeqBefore(st.remoteSynSeq, st.lastLocalAck)
			if localSynIsAcked && remoteSynIsAcked {
				st.tcpState = TcpStateEstablished
				conn.Monotonic.TCPEstablished++
			}
		case TcpStateEstablished:
			// if the remote closed, and we have acknowledged, enter CloseWait (passive close)
			if st.hasRemoteFin && isSeqBefore(st.remoteFinSeq, tcp.Ack) {
				st.tcpState = TcpStateCloseWait
			}
		case TcpStateFinWait1:
			// active close: wait until we've received an ack of the fin we sent
			if pktType == unix.PACKET_HOST {
				if !st.hasLocalFin {
					return errors.New("entered FinWait1 without a localFin (shouldn't get here)")
				}
				if isSeqBefore(st.localFinSeq, tcp.Ack) {
					st.tcpState = TcpStateFinWait2
				}
			}
		case TcpStateFinWait2:
			// active close: wait until we send an ack of the fin received from the remote
			if pktType == unix.PACKET_OUTGOING {
				if st.hasRemoteFin && isSeqBefore(st.remoteFinSeq, tcp.Ack) {
					st = connectionState{
						tcpState: TcpStateTimeWait,
					}
					conn.Monotonic.TCPClosed++
				}
			}
		case TcpStateCloseWait:
			// nothing but counting up data bytes
		case TcpStateClosing:
			// same as LastAck - we wait until we receive an ACK of the fin
			fallthrough
		case TcpStateLastAck:
			// passive close: waiting until the remote acks the client's FIN
			if pktType == unix.PACKET_HOST {
				if !st.hasLocalFin {
					return fmt.Errorf("entered state=%d without a localFin (shouldn't get here)", st.tcpState)
				}
				if isSeqBefore(st.localFinSeq, tcp.Ack) {
					nextState := TcpStateClosed
					// in a simultaneous close, we go to TimeWait instead
					if st.tcpState == TcpStateClosing {
						nextState = TcpStateTimeWait
					}
					st = connectionState{
						tcpState: nextState,
					}
					conn.Monotonic.TCPClosed++
				}
			}
		case TcpStateTimeWait:
		}
	} // end tcp.ACK

	// All the remaining flags don't fall through, unlike tcp.ACK.
	// this is because there can be RSTACK and FINACK packets
	if tcp.RST {
		switch st.tcpState {
		case TcpStateClosed:
			// TODO retransmit
		case TcpStateTimeWait:
			// already closed, ignore
		case TcpStateSynSent:
			fallthrough
		case TcpStateSynRecv:
			conn.TCPFailures[uint32(syscall.ECONNREFUSED)]++
		case TcpStateEstablished:
			conn.TCPFailures[uint32(syscall.ECONNRESET)]++
		default:
			conn.TCPFailures[uint32(syscall.ECONNRESET)]++
		}

		if !isClosedState(st.tcpState) {
			conn.Monotonic.TCPClosed++
			st = connectionState{
				tcpState: TcpStateClosed,
			}
		}
		// end tcp.RST
	} else if tcp.SYN {
		// in the case of simultaneous open, this will overwrite (which is good)
		if pktType == unix.PACKET_OUTGOING {
			if !st.hasLocalSyn || isSeqBefore(st.localSynSeq, tcp.Seq) {
				st.hasLocalSyn = true
				st.localSynSeq = tcp.Seq
			} else {
				// TODO retransmit
			}
			if isClosedState(st.tcpState) {
				st.tcpState = TcpStateSynSent
			}

		} else {
			if !st.hasRemoteSyn || isSeqBefore(st.remoteSynSeq, tcp.Seq) {
				st.hasRemoteSyn = true
				st.remoteSynSeq = tcp.Seq
			} else {
				// TODO retransmit
			}
			if isClosedState(st.tcpState) {
				st.tcpState = TcpStateSynRecv
			} else if st.tcpState == TcpStateSynSent && !tcp.ACK {
				// simultaneous open
				st.tcpState = TcpStateSynRecv
			}
		}
		// end tcp.SYN
	} else if tcp.FIN {
		if pktType == unix.PACKET_OUTGOING {
			if !st.hasLocalFin {
				st.hasLocalFin = true
				st.localFinSeq = payloadSeq
			} else {
				// TODO retransmit
			}
		} else {
			if !st.hasRemoteFin {
				st.hasRemoteFin = true
				st.remoteFinSeq = payloadSeq
			} else {
				// TODO remote retransmit? do we care?
			}
		}
		switch st.tcpState {
		case TcpStateSynRecv:
			// apparently this is possible if the OS handles the connection via listen() but the server process isn't
			// calling accept(), or is out of FDs so accept() fails
			// https://stackoverflow.com/a/5245704
			fallthrough
		case TcpStateEstablished:
			if pktType == unix.PACKET_OUTGOING {
				// active close
				st.tcpState = TcpStateFinWait1
			} else {
				// passive close, no change here besides the above recording remoteFinSeq.
				// once remoteFinSeq is acked, it switches to TcpStateCloseWait
			}
		case TcpStateFinWait1:
			if pktType == unix.PACKET_HOST {
				// simultaneous close
				st.tcpState = TcpStateClosing
			}
			// nothing but counting FIN retransmits
		case TcpStateFinWait2:
		case TcpStateCloseWait:
			if pktType == unix.PACKET_OUTGOING {
				st.tcpState = TcpStateLastAck
			}
		case TcpStateClosing:
			// nothing but counting FIN retransmits
		case TcpStateLastAck:
			// nothing but counting FIN retransmits
		case TcpStateTimeWait:
			// nothing but counting FIN retransmits
		default:
			// if we get here, we missed traffic and can't know if this is an active or passive close.
			// TODO what to do?
			log.Warnf("saw initial FIN in tcpState=%d", st.tcpState)
		}
	} // end tcp.FIN

	log.TraceFunc(func() string {
		return fmt.Sprintf("ack_seq=%+v", st)
	})
	t.conns[conn.ConnectionTuple] = st
	return nil
}
