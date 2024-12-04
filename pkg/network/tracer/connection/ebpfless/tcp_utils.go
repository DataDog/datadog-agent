// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const ebpflessModuleName = "ebpfless_network_tracer"

var statsTelemetry = struct {
	missedTCPConnections telemetry.Counter
	missingTCPFlags      telemetry.Counter
	tcpSynAndFin         telemetry.Counter
	tcpRstAndSyn         telemetry.Counter
	tcpRstAndFin         telemetry.Counter
}{
	telemetry.NewCounter(ebpflessModuleName, "missed_tcp_connections", []string{}, "Counter measuring the number of TCP connections where we missed the SYN handshake"),
	telemetry.NewCounter(ebpflessModuleName, "missing_tcp_flags", []string{}, "Counter measuring packets encountered with none of SYN, FIN, ACK, RST set"),
	telemetry.NewCounter(ebpflessModuleName, "tcp_syn_and_fin", []string{}, "Counter measuring packets encountered with SYN+FIN together"),
	telemetry.NewCounter(ebpflessModuleName, "tcp_rst_and_syn", []string{}, "Counter measuring packets encountered with RST+SYN together"),
	telemetry.NewCounter(ebpflessModuleName, "tcp_rst_and_fin", []string{}, "Counter measuring packets encountered with RST+FIN together"),
}

const tcpSeqMidpoint = 0x80000000

type ConnStatus uint8 //nolint:revive // TODO

const (
	ConnStatClosed      ConnStatus = iota //nolint:revive // TODO
	ConnStatAttempted                     //nolint:revive // TODO
	ConnStatEstablished                   //nolint:revive // TODO
)

var connStatusLabels = []string{
	"Closed",
	"Attempted",
	"Established",
}

type SynState uint8 //nolint:revive // TODO

const (
	SynStateNone  SynState = iota //nolint:revive // TODO
	SynStateSent                  //nolint:revive // TODO
	SynStateAcked                 //nolint:revive // TODO
)

func (ss *SynState) update(synFlag, ackFlag bool) {
	// for simplicity, this does not consider the sequence number of the SYNs and ACKs.
	// if these matter in the future, change this to store SYN seq numbers
	if *ss == SynStateNone && synFlag {
		*ss = SynStateSent
	}
	if *ss == SynStateSent && ackFlag {
		*ss = SynStateAcked
	}
	// if we see ACK'd traffic but missed the SYN, assume the connection started before
	// the datadog-agent starts.
	if *ss == SynStateNone && ackFlag {
		statsTelemetry.missedTCPConnections.Inc()
		*ss = SynStateAcked
	}
}

func LabelForState(tcpState ConnStatus) string { //nolint:revive // TODO
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
	case unix.PACKET_HOST:
		return "Incoming"
	case unix.PACKET_OUTGOING:
		return "Outgoing"
	default:
		return "InvalidDir-" + strconv.Itoa(int(pktType))
	}
}

func debugTcpFlags(tcp *layers.TCP) string { //nolint:revive // TODO
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
	return fmt.Sprintf("pktType=%+v ports=(%+v, %+v) size=%d seq=%+v ack=%+v flags=%s", debugPacketDir(pktType), uint16(tcp.SrcPort), uint16(tcp.DstPort), payloadLen, tcp.Seq, tcp.Ack, debugTcpFlags(tcp))
}
