// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"fmt"
	"github.com/google/gopacket/layers"
	"golang.org/x/sys/unix"
	"strconv"
	"strings"
)

const tcpSeqMidpoint = 0x80000000

type tcpState uint8

const (
	TcpStateClosed tcpState = iota
	TcpStateSynSent
	TcpStateSynRecv
	TcpStateEstablished
	TcpStateFinWait1
	TcpStateFinWait2
	TcpStateCloseWait
	TcpStateClosing
	TcpStateLastAck
	TcpStateTimeWait
)

var TcpStateLabels = []string{
	"Closed",
	"SynSent",
	"SynRecv",
	"Established",
	"FinWait1",
	"FinWait2",
	"CloseWait",
	"Closing",
	"LastAck",
	"TimeWait",
}

func LabelForState(tcpState tcpState) string {
	idx := int(tcpState)
	if idx < len(TcpStateLabels) {
		return TcpStateLabels[idx]
	}
	return "BadState-" + strconv.Itoa(idx)
}

func isSeqBefore(prev, cur uint32) bool {
	// check for wraparound with unsigned subtraction
	diff := cur - prev
	// constrain the maximum difference to half the number space
	return diff > 0 && diff < tcpSeqMidpoint
}

func isClosedState(state tcpState) bool {
	return state == TcpStateClosed || state == TcpStateTimeWait
}

func debugPacketDir(pktType uint8) string {
	switch pktType {
	case unix.PACKET_HOST:
		return "Incoming"
	case unix.PACKET_OUTGOING:
		return "Outgoing"
	default:
		return "InvalidDir-" + strconv.Itoa(int(pktType))
	}
}

func debugTcpFlags(tcp *layers.TCP) string {
	var flags []string
	if tcp.RST {
		flags = append(flags, "RST")
	}
	if tcp.FIN {
		flags = append(flags, "FIN")
	}
	if tcp.SYN {
		flags = append(flags, "SYN")
	}
	if tcp.ACK {
		flags = append(flags, "ACK")
	}
	return strings.Join(flags, "|")
}

// getRelativeSeq is used for debugging to visualize
func getRelativeSeq(seq uint32, hasBase bool, base uint32) uint32 {
	if !hasBase {
		return seq
	}
	return seq - base
}

func debugPacketInfo(pktType uint8, tcp *layers.TCP, payloadLen uint16, st connectionState) string {
	hasStartSeq, startSeq := st.hasSentPacket, st.localStartSeq
	hasAckSeq, ackSeq := st.hasReceivedPacket, st.remoteStartSeq

	if pktType == unix.PACKET_HOST {
		hasStartSeq, hasAckSeq = hasAckSeq, hasStartSeq
		startSeq, ackSeq = ackSeq, startSeq
	}
	relativeSeq := getRelativeSeq(tcp.Seq, hasStartSeq, startSeq)
	relativeAck := getRelativeSeq(tcp.Ack, hasAckSeq, ackSeq)
	return fmt.Sprintf("pktType=%+v ports=(%+v, %+v) size=%d relSeq=%+v relAck=%+v flags=%s", debugPacketDir(pktType), uint16(tcp.SrcPort), uint16(tcp.DstPort), payloadLen, relativeSeq, relativeAck, debugTcpFlags(tcp))
}
