// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package network

import (
	"fmt"
	"math"
	"time"
	"unsafe"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls"
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

// convertMsToNs converts a 48-bit millisecond timestamp into a 64-bit nanosecond timestamp
func convertMsToNs(t netebpf.NetTimeMs) uint64 {
	var ms uint64
	for i := range 3 {
		ms <<= 16
		ms += uint64(t.Timestamp[i])
	}

	return ms * 1000
}

// FromConn populates relevant fields on ConnectionStats from the connection data
func (c *ConnectionStats) FromConn(ct *netebpf.Conn) {
	c.FromTupleAndStats(&ct.Tup, &ct.Conn_stats)
	c.FromTCPStats(&ct.Tcp_stats)
}

// FromTupleAndStats populates relevant fields on ConnectionStats from the arguments
func (c *ConnectionStats) FromTupleAndStats(t *netebpf.ConnTuple, s *netebpf.ConnStats) {
	timestampNs := convertMsToNs(s.Timestamp_ms)
	durationNs := convertMsToNs(s.Duration_ms)

	*c = ConnectionStats{ConnectionTuple: ConnectionTuple{
		Pid:    t.Pid,
		NetNS:  t.Netns,
		Source: t.SourceAddress(),
		Dest:   t.DestAddress(),
		SPort:  t.Sport,
		DPort:  t.Dport,
	},
		Monotonic: StatCounters{
			SentBytes:   s.Sent_bytes,
			RecvBytes:   s.Recv_bytes,
			SentPackets: uint64(s.Sent_packets),
			RecvPackets: uint64(s.Recv_packets),
		},
		LastUpdateEpoch: timestampNs,
		IsAssured:       s.IsAssured(),
		Cookie:          StatCookie(s.Cookie),
	}

	if durationNs <= uint64(math.MaxInt64) {
		c.Duration = time.Duration(durationNs) * time.Nanosecond
	}

	c.ProtocolStack = protocols.Stack{
		API:         protocols.API(s.Protocol_stack.Api),
		Application: protocols.Application(s.Protocol_stack.Application),
		Encryption:  protocols.Encryption(s.Protocol_stack.Encryption),
	}

	c.TLSTags = tls.Tags{
		ChosenVersion:   s.Tls_tags.Chosen_version,
		CipherSuite:     s.Tls_tags.Cipher_suite,
		OfferedVersions: s.Tls_tags.Offered_versions,
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

// FromTCPCongestionStats populates the TCP congestion fields on ConnectionStats.
// Gauge fields contain the max value seen over the polling interval; counter
// fields contain the latest (highest) monotonically-increasing value.
func (c *ConnectionStats) FromTCPCongestionStats(cs *netebpf.TCPCongestionStats) {
	if c.Type != TCP || cs == nil {
		return
	}

	c.TCPMaxPacketsOut = cs.Max_packets_out
	c.TCPMaxLostOut = cs.Max_lost_out
	c.TCPMaxSackedOut = cs.Max_sacked_out
	c.TCPDelivered = cs.Delivered
	c.TCPMaxRetransOut = cs.Max_retrans_out
	c.TCPDeliveredCE = cs.Delivered_ce
	c.TCPBytesRetrans = cs.Bytes_retrans
	c.TCPDSACKDups = cs.Dsack_dups
	c.TCPReordSeen = cs.Reord_seen
	c.TCPMinSndWnd = cs.Snd_wnd
	c.TCPMinRcvWnd = cs.Rcv_wnd
	c.TCPMaxCAState = cs.Max_ca_state
	c.TCPECNNegotiated = cs.Ecn_negotiated
}

// FromTCPRTORecoveryStats populates the RTO and fast-recovery event counter fields on ConnectionStats.
func (c *ConnectionStats) FromTCPRTORecoveryStats(rs *netebpf.TCPRTORecoveryStats) {
	if c.Type != TCP || rs == nil {
		return
	}

	c.TCPRTOCount = rs.Rto_count
	c.TCPRecoveryCount = rs.Recovery_count
	c.TCPProbe0Count = rs.Probe0_count
	c.TCPCwndAtLastRTO = rs.Cwnd_at_last_rto
	c.TCPSsthreshAtLastRTO = rs.Ssthresh_at_last_rto
	c.TCPSRTTAtLastRTOUs = rs.Srtt_at_last_rto
	c.TCPCwndAtLastRecovery = rs.Cwnd_at_last_recovery
	c.TCPSsthreshAtLastRecovery = rs.Ssthresh_at_last_recovery
	c.TCPSRTTAtLastRecoveryUs = rs.Srtt_at_last_recovery
	c.TCPMaxConsecRTOs = rs.Max_consecutive_rtos
}

// FromTCPStats populates relevant fields on ConnectionStats from the arguments
func (c *ConnectionStats) FromTCPStats(tcpStats *netebpf.TCPStats) {
	if c.Type != TCP || tcpStats == nil {
		return
	}

	c.Monotonic.Retransmits = tcpStats.Retransmits
	c.Monotonic.TCPEstablished = tcpStats.State_transitions >> netebpf.Established & 1
	c.Monotonic.TCPClosed = tcpStats.State_transitions >> netebpf.Close & 1
	c.RTT = tcpStats.Rtt
	c.RTTVar = tcpStats.Rtt_var
	if tcpStats.Failure_reason > 0 {
		c.TCPFailures = map[uint16]uint32{
			tcpStats.Failure_reason: 1,
		}
	}
}
