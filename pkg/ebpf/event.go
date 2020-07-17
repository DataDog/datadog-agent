// +build linux_bpf

package ebpf

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

/*
#include "c/tracer-ebpf.h"
*/
import "C"

/* tcp_conn_t
conn_tuple_t tup;
conn_stats_ts_t conn_stats;
tcp_stats_t tcp_stats;
*/
type TCPConn C.tcp_conn_t

/* conn_tuple_t
__u64 saddr_h;
__u64 saddr_l;
__u64 daddr_h;
__u64 daddr_l;
__u16 sport;
__u16 dport;
__u32 netns;
__u32 pid;
__u32 metadata;
*/
type ConnTuple C.conn_tuple_t

/* batch_t
tcp_conn_t c0;
tcp_conn_t c1;
tcp_conn_t c2;
tcp_conn_t c3;
tcp_conn_t c4;
__u16 pos;
__u16 cpu;
*/
type batch C.batch_t

func (t *ConnTuple) copy() *ConnTuple {
	return &ConnTuple{
		pid:      t.pid,
		saddr_h:  t.saddr_h,
		saddr_l:  t.saddr_l,
		daddr_h:  t.daddr_h,
		daddr_l:  t.daddr_l,
		sport:    t.sport,
		dport:    t.dport,
		netns:    t.netns,
		metadata: t.metadata,
	}
}

func (t *ConnTuple) isTCP() bool {
	return connType(uint(t.metadata)) == network.TCP
}

/* conn_stats_ts_t
__u64 sent_bytes;
__u64 recv_bytes;
__u64 timestamp;
*/
type ConnStatsWithTimestamp C.conn_stats_ts_t

/* tcp_stats_t
__u32 retransmits;
__u32 rtt;
__u32 rtt_var;
*/
type TCPStats C.tcp_stats_t

/*
__u32 tcp_sent_miscounts;
*/
type kernelTelemetry C.telemetry_t

const TCPCloseBatchSize = int(C.TCP_CLOSED_BATCH_SIZE)

func (cs *ConnStatsWithTimestamp) isExpired(latestTime uint64, timeout uint64) bool {
	return latestTime > timeout+uint64(cs.timestamp)
}

func toBatch(data []byte) *batch {
	return (*batch)(unsafe.Pointer(&data[0]))
}

// ExtractBatchInto extract network.ConnectionStats objects from the given `batch` into the supplied `buffer`.
// The `start` (inclusive) and `end` (exclusive) arguments represent the offsets of the connections we're interested in.
func ExtractBatchInto(buffer []network.ConnectionStats, b *batch, start, end int) []network.ConnectionStats {
	if start >= end || end > TCPCloseBatchSize {
		return nil
	}

	var (
		connSize = unsafe.Sizeof(C.tcp_conn_t{})
		current  = uintptr(unsafe.Pointer(b)) + uintptr(start)*connSize
	)

	for i := start; i < end; i++ {
		ct := TCPConn(*(*C.tcp_conn_t)(unsafe.Pointer(current)))

		tup := ConnTuple(ct.tup)
		cst := ConnStatsWithTimestamp(ct.conn_stats)
		tst := TCPStats(ct.tcp_stats)

		buffer = append(buffer, connStats(&tup, &cst, &tst))
		current += connSize
	}

	return buffer
}

func connStats(t *ConnTuple, s *ConnStatsWithTimestamp, tcpStats *TCPStats) network.ConnectionStats {
	metadata := uint(t.metadata)
	family := connFamily(metadata)

	var source, dest util.Address
	if family == network.AFINET {
		source = util.V4Address(uint32(t.saddr_l))
		dest = util.V4Address(uint32(t.daddr_l))
	} else {
		source = util.V6Address(uint64(t.saddr_l), uint64(t.saddr_h))
		dest = util.V6Address(uint64(t.daddr_l), uint64(t.daddr_h))
	}

	return network.ConnectionStats{
		Pid:                  uint32(t.pid),
		Type:                 connType(metadata),
		Family:               family,
		NetNS:                uint32(t.netns),
		Source:               source,
		Dest:                 dest,
		SPort:                uint16(t.sport),
		DPort:                uint16(t.dport),
		MonotonicSentBytes:   uint64(s.sent_bytes),
		MonotonicRecvBytes:   uint64(s.recv_bytes),
		MonotonicRetransmits: uint32(tcpStats.retransmits),
		RTT:                  uint32(tcpStats.rtt),
		RTTVar:               uint32(tcpStats.rtt_var),
		LastUpdateEpoch:      uint64(s.timestamp),
	}
}

func connType(m uint) network.ConnectionType {
	// First bit of metadata indicates if the connection is TCP or UDP
	if m&C.CONN_TYPE_TCP == 0 {
		return network.UDP
	}
	return network.TCP
}

func connFamily(m uint) network.ConnectionFamily {
	// Second bit of metadata indicates if the connection is IPv6 or IPv4
	if m&C.CONN_V6 == 0 {
		return network.AFINET
	}

	return network.AFINET6
}

func isPortClosed(state uint8) bool {
	return state == C.PORT_CLOSED
}
