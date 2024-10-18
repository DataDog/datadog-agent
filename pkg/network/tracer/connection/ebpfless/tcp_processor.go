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

	// @stu doc
	hasSentPacket bool
	// localStartSeq is only used for debugging
	localStartSeq  uint32
	maxSeqSent     uint32
	hasAckedPacket bool
	lastAck        uint32

	// hasReceivedPacket is only used for debugging
	hasReceivedPacket bool
	// remoteStartSeq is only used for debugging
	remoteStartSeq uint32

	hasSynAcked bool

	hasLocalFin  bool
	hasRemoteFin bool
	localFinSeq  uint32
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

func logFailedConn(fmt string, params ...interface{}) {
	// @stu
	log.Debugf(fmt, params...)
}

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
		if tcp.ACK {
			isFinAck := st.hasRemoteFin && tcp.Ack == st.remoteFinSeq+1
			if !st.hasAckedPacket {
				st.hasAckedPacket = true
				st.lastAck = tcp.Ack
			} else if isSeqBefore(st.lastAck, tcp.Ack) {
				ackDiff := tcp.Ack - st.lastAck
				if isFinAck {
					ackDiff--
				}
				conn.Monotonic.RecvBytes += uint64(ackDiff)
				st.lastAck = tcp.Ack
			}
		}
	case unix.PACKET_HOST:
		conn.Monotonic.RecvPackets++
		if !st.hasReceivedPacket {
			st.hasReceivedPacket = true
			st.remoteStartSeq = tcp.Seq
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
			if pktType == unix.PACKET_OUTGOING {
				if st.hasSynAcked {
					// we are acking the remote's synack, mark as established
					st.tcpState = TcpStateEstablished
					conn.Monotonic.TCPEstablished++
				} else {
					log.Warnf("missed the SYNACK for tcpState=%d", st.tcpState)
				}
			}
		case TcpStateSynRecv:
			if pktType == unix.PACKET_HOST {
				if st.hasSynAcked {
					// remote has acked our synack, mark as established
					st.tcpState = TcpStateEstablished
					conn.Monotonic.TCPEstablished++
				} else {
					log.Warnf("missed the SYNACK for tcpState=%d", st.tcpState)
				}
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
					st.tcpState = TcpStateTimeWait
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
					st.tcpState = TcpStateClosed
					conn.Monotonic.TCPClosed++
				}
			}
		case TcpStateTimeWait:
		}
		// this branch is the only one that doesn't GOTO done - it needs to fall through
		// because there can be RSTACK and FINACK packets
	} // end regular ACK

	// RST flag
	if tcp.RST {
		switch st.tcpState {
		case TcpStateClosed:
			// retransmitted, ignore
		case TcpStateSynSent:
			fallthrough
		case TcpStateSynRecv:
			logFailedConn("RST on an attempted connection; Failure")
			conn.TCPFailures[uint32(syscall.ECONNREFUSED)]++
		case TcpStateEstablished:
			logFailedConn("RST on an established connection; Failure")
			conn.TCPFailures[uint32(syscall.ECONNRESET)]++
			// TODO what to do when RST happens in one of the closing states?
		default:
			logFailedConn("RST on a connection with tcpState=%d; Failure", st.tcpState)
			conn.TCPFailures[uint32(syscall.ECONNRESET)]++
		}

		if !isClosedState(st.tcpState) {
			conn.Monotonic.TCPClosed++
			st.tcpState = TcpStateClosed
		}
		goto done
	} // end RST

	// initial SYN (no ACK)
	if tcp.SYN && !tcp.ACK {
		switch st.tcpState {
		case TcpStateClosed:
			fallthrough
		case TcpStateTimeWait:
			if pktType == unix.PACKET_OUTGOING {
				st.tcpState = TcpStateSynSent
			} else {
				st.tcpState = TcpStateSynRecv
			}
		case TcpStateSynSent:
			if pktType == unix.PACKET_OUTGOING {
				// TODO retransmit
			} else {
				// simultaneous open
				st.tcpState = TcpStateSynRecv
			}
		case TcpStateSynRecv:
			// TODO retransmit
		default:
			log.Warnf("SYN on a connection with tcpState=%d", st.tcpState)
		}
		goto done
	} // end SYN (no ACK)

	// SYNACK
	if tcp.SYN && tcp.ACK {
		switch st.tcpState {
		case TcpStateClosed:
			// TODO we missed the initial SYN packet, what to do here?
			// for now assume things proceeded normally prior
			st.tcpState = TcpStateSynRecv
			fallthrough
		case TcpStateSynSent:
			fallthrough
		case TcpStateSynRecv:
			st.hasSynAcked = true
		case TcpStateEstablished:
			// TODO retransmit
		default:
			log.Warnf("SYNACK on a connection with tcpState=%d", st.tcpState)
		}
		goto done
	} // end SYNACK

	if tcp.FIN {
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
				// XXXXXXXXXXXXXXX
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
		goto done
	} // end FIN

	//if tcp.FIN || tcp.RST {
	//	if !st.closed {
	//		st.closed = true
	//		conn.Monotonic.TCPClosed++
	//		conn.Duration = time.Duration(time.Now().UnixNano() - int64(conn.Duration))
	//	}
	//	delete(t.conns, key)
	//	return nil
	//}
	//
	//if !tcp.SYN && !st.established {
	//	st.established = true
	//	conn.Monotonic.TCPEstablished++
	//}

done:

	log.TraceFunc(func() string {
		return fmt.Sprintf("ack_seq=%+v", st)
	})
	t.conns[conn.ConnectionTuple] = st
	return nil
}
