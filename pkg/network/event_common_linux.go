// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package network

// Sub returns s-other
func (s StatCounters) Sub(other StatCounters) (sc StatCounters, underflow bool) {
	if s.Retransmits < other.Retransmits && s.Retransmits > 0 {
		return sc, true
	}

	sc = StatCounters{
		RecvBytes:      s.RecvBytes - other.RecvBytes,
		SentBytes:      s.SentBytes - other.SentBytes,
		TCPEstablished: s.TCPEstablished - other.TCPEstablished,
		TCPClosed:      s.TCPClosed - other.TCPClosed,
	}

	// on linux, sent and recv packets are actually collected
	// as uint32's, but StatCounters stores them as uint64,
	// so we need to treat them as uint32 to detect overflows
	sc.RecvPackets = uint64(uint32(s.RecvPackets) - uint32(other.RecvPackets))
	sc.SentPackets = uint64(uint32(s.SentPackets) - uint32(other.SentPackets))

	if s.Retransmits > 0 {
		sc.Retransmits = s.Retransmits - other.Retransmits
	}

	return sc, false
}
