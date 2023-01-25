// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package network

// Sub returns s-other
func (s StatCounters) Sub(other StatCounters) (sc StatCounters, underflow bool) {
	if s.RecvBytes < other.RecvBytes ||
		s.RecvPackets < other.RecvPackets ||
		(s.Retransmits < other.Retransmits && s.Retransmits > 0) ||
		s.SentBytes < other.SentBytes ||
		s.SentPackets < other.SentPackets ||
		(s.TCPClosed < other.TCPClosed && s.TCPClosed > 0) ||
		(s.TCPEstablished < other.TCPEstablished && s.TCPEstablished > 0) {
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

	return sc, false
}
