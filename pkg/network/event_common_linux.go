// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// Sub returns s-other.
//
// This implementation is different from the implementation on
// other platforms as packet counts are actually sampled from the kernel
// as uint32's, but stored in StatCounters as uint64's. To detect overflows
// in these counts correctly, a simple subtraction will not do, and they
// need to be treated differently (see below)
func (s StatCounters) Sub(other StatCounters) (sc StatCounters, underflow bool) {
	if s.Retransmits < other.Retransmits && s.Retransmits > 0 ||
		(s.TCPClosed < other.TCPClosed && s.TCPClosed > 0) ||
		(s.TCPEstablished < other.TCPEstablished && s.TCPEstablished > 0) ||
		isUnderflow(other.RecvBytes, s.RecvBytes, maxByteCountChange) ||
		isUnderflow(other.SentBytes, s.SentBytes, maxByteCountChange) {
		return sc, true
	}

	sc = StatCounters{
		RecvBytes: s.RecvBytes - other.RecvBytes,
		SentBytes: s.SentBytes - other.SentBytes,
	}

	// on linux, sent and recv packets are collected
	// as uint32's, but StatCounters stores them as uint64,
	// so we need to treat them as uint32 to detect underflows
	sc.RecvPackets = uint64(uint32(s.RecvPackets) - uint32(other.RecvPackets))
	sc.SentPackets = uint64(uint32(s.SentPackets) - uint32(other.SentPackets))
	if (s.RecvPackets < other.RecvPackets && sc.RecvPackets > maxPacketCountChange) ||
		(s.SentPackets < other.SentPackets && sc.SentPackets > maxPacketCountChange) {
		return StatCounters{}, true
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

// ConnectionKeysFromConnectionStats constructs connection key using the underlying raw connection stats object, which is produced by the tracer.
// Each ConnectionStats object contains both the source and destination addresses, as well as an IPTranslation object that stores the original addresses in the event that the connection is NAT'd.
// This function generates all relevant combinations of connection keys: [(source, dest), (dest, source), (NAT'd source, NAT'd dest), (NAT'd dest, NAT'd source)].
// This is necessary to handle all possible scenarios for connections originating from the USM module (i.e., whether they are NAT'd or not, and whether they use TLS).
func ConnectionKeysFromConnectionStats(connectionStats ConnectionStats) []types.ConnectionKey {

	// USM data is always indexed as (client, server), but we don't know which is the remote
	// and which is the local address. To account for this, we'll construct 2 possible
	// connection keys and check for both of them in the aggregations map.
	connectionKeys := []types.ConnectionKey{
		types.NewConnectionKey(connectionStats.Source, connectionStats.Dest, connectionStats.SPort, connectionStats.DPort),
		types.NewConnectionKey(connectionStats.Dest, connectionStats.Source, connectionStats.DPort, connectionStats.SPort),
	}

	// if IPTranslation is not nil, at least one of the sides has a translation, thus we need to add translated addresses.
	if connectionStats.IPTranslation != nil {
		localAddress, localPort := GetNATLocalAddress(connectionStats)
		remoteAddress, remotePort := GetNATRemoteAddress(connectionStats)
		connectionKeys = append(connectionKeys,
			types.NewConnectionKey(localAddress, remoteAddress, localPort, remotePort),
			types.NewConnectionKey(remoteAddress, localAddress, remotePort, localPort))
	}

	return connectionKeys
}
