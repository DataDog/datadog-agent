// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build (linux && linux_bpf) || darwin

package ebpfless

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const ebpflessModuleName = "ebpfless_network_tracer"

// PCAPTuple represents a unique key for an ebpfless tracer connection.
// It represents a network.ConnectionTuple with only the fields that are available
// via packet capture: PID and Direction are zeroed out.
type PCAPTuple network.ConnectionTuple

func connDirectionFromPktType(pktType uint8) network.ConnectionDirection {
	switch pktType {
	case filter.PACKET_HOST:
		return network.INCOMING
	case filter.PACKET_OUTGOING:
		return network.OUTGOING
	default:
		return network.UNKNOWN
	}
}

// ProcessResult represents what the ebpfless tracer should do with ConnectionStats after processing a packet
type ProcessResult uint8

const (
	// ProcessResultNone - the updated ConnectionStats should NOT be stored in the connection map.
	// Usually, this is because the connection is not established yet.
	ProcessResultNone ProcessResult = iota
	// ProcessResultStoreConn - the updated ConnectionStats should be stored in the connection map.
	// This happens when the connection is established.
	ProcessResultStoreConn
	// ProcessResultCloseConn - this connection is done and its ConnectionStats should be passed
	// to the ebpfless tracer's closed connection handler.
	ProcessResultCloseConn
	// ProcessResultMapFull - this connection can't be tracked because the TCPProcessor's connection
	// map is full. This connection should be removed from the tracer as well.
	ProcessResultMapFull
)

// ShouldPersist returns whether this result has actionable data that can be persisted into the tracer
func (r ProcessResult) ShouldPersist() bool {
	return r == ProcessResultStoreConn || r == ProcessResultCloseConn
}

var statsTelemetry = struct {
	expiredPendingConns     telemetry.Counter
	droppedPendingConns     telemetry.Counter
	droppedEstablishedConns telemetry.Counter
	missedTCPHandshakes     telemetry.Counter
	missingTCPFlags         telemetry.Counter
	tcpSynAndFin            telemetry.Counter
	tcpRstAndSyn            telemetry.Counter
	tcpRstAndFin            telemetry.Counter
}{
	expiredPendingConns:     telemetry.NewCounter(ebpflessModuleName, "expired_pending_conns", nil, "Counter measuring the number of TCP connections which expired because it took too long to complete the handshake"),
	droppedPendingConns:     telemetry.NewCounter(ebpflessModuleName, "dropped_pending_conns", nil, "Counter measuring the number of TCP connections which were dropped during the handshake (because the map was full)"),
	droppedEstablishedConns: telemetry.NewCounter(ebpflessModuleName, "dropped_established_conns", nil, "Counter measuring the number of TCP connections which were dropped while established (because the map was full)"),
	missedTCPHandshakes:     telemetry.NewCounter(ebpflessModuleName, "missed_tcp_handshakes", nil, "Counter measuring the number of TCP connections where we missed the SYN handshake"),
	missingTCPFlags:         telemetry.NewCounter(ebpflessModuleName, "missing_tcp_flags", nil, "Counter measuring packets encountered with none of SYN, FIN, ACK, RST set"),
	tcpSynAndFin:            telemetry.NewCounter(ebpflessModuleName, "tcp_syn_and_fin", nil, "Counter measuring packets encountered with SYN+FIN together"),
	tcpRstAndSyn:            telemetry.NewCounter(ebpflessModuleName, "tcp_rst_and_syn", nil, "Counter measuring packets encountered with RST+SYN together"),
	tcpRstAndFin:            telemetry.NewCounter(ebpflessModuleName, "tcp_rst_and_fin", nil, "Counter measuring packets encountered with RST+FIN together"),
}

const tcpSeqMidpoint = 0x80000000

type connStatus uint8

const (
	connStatClosed connStatus = iota
	connStatAttempted
	connStatEstablished
)

var connStatusLabels = []string{
	"Closed",
	"Attempted",
	"Established",
}

type synState uint8

const (
	// synStateNone - Nothing seen yet (initial state)
	synStateNone synState = iota
	// synStateSent - We have seen the SYN but not its ACK
	synStateSent
	// synStateAcked - SYN is ACK'd for this side of the connection.
	// If both sides are synStateAcked, the connection is established.
	synStateAcked
	// synStateMissed is effectively the same as synStateAcked but represents
	// capturing a preexisting connection where we didn't get to see the SYN.
	synStateMissed
)

func (ss *synState) update(synFlag, ackFlag bool) {
	// for simplicity, this does not consider the sequence number of the SYNs and ACKs.
	// if these matter in the future, change this to store SYN seq numbers
	if *ss == synStateNone && synFlag {
		*ss = synStateSent
	}
	if *ss == synStateSent && ackFlag {
		*ss = synStateAcked
	}

	// this allows synStateMissed to recover via SYN in order to pass TestUnusualAckSyn
	if *ss == synStateMissed && synFlag {
		*ss = synStateAcked
	}
	// if we see ACK'd traffic but missed the SYN, assume the connection started before
	// the datadog-agent starts.
	if *ss == synStateNone && ackFlag {
		*ss = synStateMissed
	}
}
func (ss *synState) isSynAcked() bool {
	return *ss == synStateAcked || *ss == synStateMissed
}

func labelForState(tcpState connStatus) string {
	idx := int(tcpState)
	if idx < len(connStatusLabels) {
		return connStatusLabels[idx]
	}
	return "BadState-" + strconv.Itoa(idx)
}

func isSeqBefore(prev, cur uint32) bool {
	// check for wraparound with unsigned subtraction
	diff := cur - prev
	// constrain the maximum difference to half the number space
	return diff > 0 && diff < tcpSeqMidpoint
}
func isSeqBeforeEq(prev, cur uint32) bool {
	return prev == cur || isSeqBefore(prev, cur)
}

func debugPacketDir(pktType uint8) string {
	switch pktType {
	case filter.PACKET_HOST:
		return "Incoming"
	case filter.PACKET_OUTGOING:
		return "Outgoing"
	default:
		return "InvalidDir-" + strconv.Itoa(int(pktType))
	}
}

func debugTCPFlags(tcp *layers.TCP) string {
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

func debugPacketInfo(pktType uint8, tcp *layers.TCP, payloadLen uint16) string {
	return fmt.Sprintf("pktType=%+v ports=(%+v, %+v) size=%d seq=%+v ack=%+v flags=%s", debugPacketDir(pktType), uint16(tcp.SrcPort), uint16(tcp.DstPort), payloadLen, tcp.Seq, tcp.Ack, debugTCPFlags(tcp))
}
