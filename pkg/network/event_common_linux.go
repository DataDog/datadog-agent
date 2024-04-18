// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"fmt"
	"math"
	"time"
	"unsafe"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
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

// UnmarshalBinary converts a raw byte slice to a ConnectionStats object
func (c *ConnectionStats) UnmarshalBinary(data []byte) error {
	if len(data) < netebpf.SizeofConn {
		return fmt.Errorf("'Conn' binary data too small, received %d but expected %d bytes", len(data), netebpf.SizeofConn)
	}

	ct := (*netebpf.Conn)(unsafe.Pointer(&data[0]))
	c.FromConn(ct)
	return nil
}

// FromConn populates relevant fields on ConnectionStats from the connection data
func (c *ConnectionStats) FromConn(ct *netebpf.Conn) {
	c.FromTupleAndStats(&ct.Tup, &ct.Conn_stats)
	c.FromTCPStats(&ct.Tcp_stats, ct.Tcp_retransmits)
}

// FromTupleAndStats populates relevant fields on ConnectionStats from the arguments
func (c *ConnectionStats) FromTupleAndStats(t *netebpf.ConnTuple, s *netebpf.ConnStats) {
	*c = ConnectionStats{
		Pid:    t.Pid,
		NetNS:  t.Netns,
		Source: t.SourceAddress(),
		Dest:   t.DestAddress(),
		SPort:  t.Sport,
		DPort:  t.Dport,
		Monotonic: StatCounters{
			SentBytes:   s.Sent_bytes,
			RecvBytes:   s.Recv_bytes,
			SentPackets: uint64(s.Sent_packets),
			RecvPackets: uint64(s.Recv_packets),
		},
		LastUpdateEpoch: s.Timestamp,
		IsAssured:       s.IsAssured(),
		Cookie:          StatCookie(s.Cookie),
	}

	if s.Duration <= uint64(math.MaxInt64) {
		c.Duration = time.Duration(s.Duration) * time.Nanosecond
	}

	c.ProtocolStack = protocols.Stack{
		API:         protocols.API(s.Protocol_stack.Api),
		Application: protocols.Application(s.Protocol_stack.Application),
		Encryption:  protocols.Encryption(s.Protocol_stack.Encryption),
	}

	if t.Type() == netebpf.TCP {
		c.Type = TCP
	} else {
		c.Type = UDP
	}

	switch t.Family() {
	case netebpf.IPv4:
		c.Family = AFINET
	case netebpf.IPv6:
		c.Family = AFINET6
	}

	c.SPortIsEphemeral = IsPortInEphemeralRange(c.Family, c.Type, t.Sport)

	switch s.ConnectionDirection() {
	case netebpf.Incoming:
		c.Direction = INCOMING
	case netebpf.Outgoing:
		c.Direction = OUTGOING
	default:
		c.Direction = OUTGOING
	}
}

// FromTCPStats populates relevant fields on ConnectionStats from the arguments
func (c *ConnectionStats) FromTCPStats(tcpStats *netebpf.TCPStats, retransmits uint32) {
	if c.Type != TCP {
		return
	}

	c.Monotonic.Retransmits = retransmits
	if tcpStats != nil {
		c.Monotonic.TCPEstablished = uint32(tcpStats.State_transitions >> netebpf.Established & 1)
		c.Monotonic.TCPClosed = uint32(tcpStats.State_transitions >> netebpf.Close & 1)
		c.RTT = tcpStats.Rtt
		c.RTTVar = tcpStats.Rtt_var
	}
}
