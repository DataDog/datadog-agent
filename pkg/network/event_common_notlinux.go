// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

package network

// Sub returns s-other
//
// Note: there is separate implementation for this function on Linux;
// see event_common_linux.go in this folder
func (s StatCounters) Sub(other StatCounters) (sc StatCounters, underflow bool) {
	if (s.Retransmits < other.Retransmits && s.Retransmits > 0) ||
		(s.TCPClosed < other.TCPClosed && s.TCPClosed > 0) ||
		(s.TCPEstablished < other.TCPEstablished && s.TCPEstablished > 0) ||
		(s.TCPRTOCount < other.TCPRTOCount && s.TCPRTOCount > 0) ||
		(s.TCPRecoveryCount < other.TCPRecoveryCount && s.TCPRecoveryCount > 0) ||
		(s.TCPProbe0Count < other.TCPProbe0Count && s.TCPProbe0Count > 0) ||
		(s.TCPDeliveredCE < other.TCPDeliveredCE && s.TCPDeliveredCE > 0) ||
		(s.TCPReordSeen < other.TCPReordSeen && s.TCPReordSeen > 0) ||
		(s.TCPRcvOOOPack < other.TCPRcvOOOPack && s.TCPRcvOOOPack > 0) ||
		isUnderflow(other.RecvBytes, s.RecvBytes, maxByteCountChange) ||
		isUnderflow(other.SentBytes, s.SentBytes, maxByteCountChange) ||
		isUnderflow(other.RecvPackets, s.RecvPackets, maxPacketCountChange) ||
		isUnderflow(other.SentPackets, s.SentPackets, maxPacketCountChange) {
		return sc, true
	}

	sc = StatCounters{
		RecvBytes:   s.RecvBytes - other.RecvBytes,
		RecvPackets: s.RecvPackets - other.RecvPackets,
		SentBytes:   s.SentBytes - other.SentBytes,
		SentPackets: s.SentPackets - other.SentPackets,
	}

	if s.Retransmits > 0 {
		sc.Retransmits = s.Retransmits - other.Retransmits
	}
	if s.TCPEstablished > 0 {
		sc.TCPEstablished = s.TCPEstablished - other.TCPEstablished
	}
	if s.TCPClosed > 0 {
		sc.TCPClosed = s.TCPClosed - other.TCPClosed
	}
	if s.TCPRTOCount > 0 {
		sc.TCPRTOCount = s.TCPRTOCount - other.TCPRTOCount
	}
	if s.TCPRecoveryCount > 0 {
		sc.TCPRecoveryCount = s.TCPRecoveryCount - other.TCPRecoveryCount
	}
	if s.TCPProbe0Count > 0 {
		sc.TCPProbe0Count = s.TCPProbe0Count - other.TCPProbe0Count
	}
	if s.TCPDeliveredCE > 0 {
		sc.TCPDeliveredCE = s.TCPDeliveredCE - other.TCPDeliveredCE
	}
	if s.TCPReordSeen > 0 {
		sc.TCPReordSeen = s.TCPReordSeen - other.TCPReordSeen
	}
	if s.TCPRcvOOOPack > 0 {
		sc.TCPRcvOOOPack = s.TCPRcvOOOPack - other.TCPRcvOOOPack
	}

	return sc, false
}
