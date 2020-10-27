// +build linux_bpf

package ebpf

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

/*
#include "c/tracer-ebpf.h"
#include "c/tcp_states.h"
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

/* port_binding_t
__u32 pid;
__u32 net_ns;
__u16 port;
*/
type portBindingTuple C.port_binding_t

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

func ipPortFromAddr(addr net.Addr) (net.IP, int) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP, v.Port
	case *net.UDPAddr:
		return v.IP, v.Port
	}
	return nil, 0
}

func connTupleFromConn(conn net.Conn, pid uint32) (*ConnTuple, error) {
	saddr := conn.LocalAddr()
	shost, sport := ipPortFromAddr(saddr)

	daddr := conn.RemoteAddr()
	dhost, dport := ipPortFromAddr(daddr)

	ct := &ConnTuple{
		pid:   C.__u32(pid),
		sport: C.__u16(sport),
		dport: C.__u16(dport),
	}
	if sbytes := shost.To4(); sbytes != nil {
		dbytes := dhost.To4()
		ct.metadata |= C.CONN_V4
		ct.saddr_h = 0
		ct.saddr_l = C.__u64(binary.LittleEndian.Uint32(sbytes))
		ct.daddr_h = 0
		ct.daddr_l = C.__u64(binary.LittleEndian.Uint32(dbytes))
	} else if sbytes := shost.To16(); sbytes != nil {
		dbytes := dhost.To16()
		ct.metadata |= C.CONN_V6
		ct.saddr_h = C.__u64(binary.LittleEndian.Uint64(sbytes[:8]))
		ct.saddr_l = C.__u64(binary.LittleEndian.Uint64(sbytes[8:]))
		ct.daddr_h = C.__u64(binary.LittleEndian.Uint64(dbytes[:8]))
		ct.daddr_l = C.__u64(binary.LittleEndian.Uint64(dbytes[8:]))
	} else {
		return nil, fmt.Errorf("invalid source/dest address")
	}

	switch saddr.Network() {
	case "tcp", "tcp4", "tcp6":
		ct.metadata |= C.CONN_TYPE_TCP
	case "udp", "udp4", "udp6":
		ct.metadata |= C.CONN_TYPE_UDP
	}

	return ct, nil
}

func (t *ConnTuple) isTCP() bool {
	return connType(uint(t.metadata)) == network.TCP
}

func (t *ConnTuple) isIPv4() bool {
	return connFamily(uint(t.metadata)) == network.AFINET
}

func (t *ConnTuple) SourceAddress() util.Address {
	if t.isIPv4() {
		return util.V4Address(uint32(t.saddr_l))
	}
	return util.V6Address(uint64(t.saddr_l), uint64(t.saddr_h))
}

// SourceEndpoint returns the source address in the ip:port format (for example, "192.0.2.1:25", "[2001:db8::1]:80")
func (t *ConnTuple) SourceEndpoint() string {
	return net.JoinHostPort(t.SourceAddress().String(), strconv.Itoa(int(t.sport)))
}

func (t *ConnTuple) DestAddress() util.Address {
	if t.isIPv4() {
		return util.V4Address(uint32(t.daddr_l))
	}
	return util.V6Address(uint64(t.daddr_l), uint64(t.daddr_h))
}

// DestEndpoint returns the destination address in the ip:port format (for example, "192.0.2.1:25", "[2001:db8::1]:80")
func (t *ConnTuple) DestEndpoint() string {
	return net.JoinHostPort(t.DestAddress().String(), strconv.Itoa(int(t.dport)))
}

func (t *ConnTuple) String() string {
	m := uint(t.metadata)
	return fmt.Sprintf(
		"[%s%s] [PID: %d] [%s ⇄ %s] (ns: %d)",
		connType(m),
		connFamily(m),
		uint32(t.pid),
		t.SourceEndpoint(),
		t.DestEndpoint(),
		uint32(t.netns),
	)
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

	current := uintptr(unsafe.Pointer(b)) + uintptr(start)*C.sizeof_tcp_conn_t
	for i := start; i < end; i++ {
		ct := TCPConn(*(*C.tcp_conn_t)(unsafe.Pointer(current)))

		tup := ConnTuple(ct.tup)
		cst := ConnStatsWithTimestamp(ct.conn_stats)
		tst := TCPStats(ct.tcp_stats)

		buffer = append(buffer, connStats(&tup, &cst, &tst))
		current += C.sizeof_tcp_conn_t
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
		Pid:                     uint32(t.pid),
		Type:                    connType(metadata),
		Family:                  family,
		NetNS:                   uint32(t.netns),
		Source:                  source,
		Dest:                    dest,
		SPort:                   uint16(t.sport),
		DPort:                   uint16(t.dport),
		MonotonicSentBytes:      uint64(s.sent_bytes),
		MonotonicRecvBytes:      uint64(s.recv_bytes),
		MonotonicRetransmits:    uint32(tcpStats.retransmits),
		MonotonicTCPEstablished: uint32(tcpStats.state_transitions >> C.TCP_ESTABLISHED & 1),
		MonotonicTCPClosed:      uint32(tcpStats.state_transitions >> C.TCP_CLOSE & 1),
		RTT:                     uint32(tcpStats.rtt),
		RTTVar:                  uint32(tcpStats.rtt_var),
		LastUpdateEpoch:         uint64(s.timestamp),
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
