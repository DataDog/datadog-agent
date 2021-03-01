// +build linux_bpf

package tracer

import (
	"fmt"
	"net"
	"strconv"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

/*
#include "../ebpf/c/tracer.h"
#include "../ebpf/c/tcp_states.h"
*/
import "C"

/* conn_t
conn_tuple_t tup;
conn_stats_ts_t conn_stats;
tcp_stats_t tcp_stats;
*/
type Conn C.conn_t

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
conn_t c0;
conn_t c1;
conn_t c2;
conn_t c3;
conn_t c4;
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

func connTupleFromConn(conn net.Conn, pid uint32, netns uint32) (*ConnTuple, error) {
	saddr := conn.LocalAddr()
	shost, sport := ipPortFromAddr(saddr)

	daddr := conn.RemoteAddr()
	dhost, dport := ipPortFromAddr(daddr)

	ct := &ConnTuple{
		netns: C.__u32(netns),
		pid:   C.__u32(pid),
		sport: C.__u16(sport),
		dport: C.__u16(dport),
	}
	if sbytes := shost.To4(); sbytes != nil {
		dbytes := dhost.To4()
		ct.metadata |= C.CONN_V4
		ct.saddr_h = 0
		ct.saddr_l = C.__u64(nativeEndian.Uint32(sbytes))
		ct.daddr_h = 0
		ct.daddr_l = C.__u64(nativeEndian.Uint32(dbytes))
	} else if sbytes := shost.To16(); sbytes != nil {
		dbytes := dhost.To16()
		ct.metadata |= C.CONN_V6
		ct.saddr_h = C.__u64(nativeEndian.Uint64(sbytes[:8]))
		ct.saddr_l = C.__u64(nativeEndian.Uint64(sbytes[8:]))
		ct.daddr_h = C.__u64(nativeEndian.Uint64(dbytes[:8]))
		ct.daddr_l = C.__u64(nativeEndian.Uint64(dbytes[8:]))
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

func newConnTuple(pid int, netns uint64, saddr, daddr util.Address, sport, dport uint16, proto network.ConnectionType) *ConnTuple {
	ct := &ConnTuple{
		pid:   C.__u32(pid),
		netns: C.__u32(netns),
		sport: C.__u16(sport),
		dport: C.__u16(dport),
	}
	sbytes := saddr.Bytes()
	dbytes := daddr.Bytes()
	if len(sbytes) == 4 {
		ct.metadata |= C.CONN_V4
		ct.saddr_h = 0
		ct.saddr_l = C.__u64(nativeEndian.Uint32(sbytes))
		ct.daddr_h = 0
		ct.daddr_l = C.__u64(nativeEndian.Uint32(dbytes))
	} else if len(sbytes) == 16 {
		ct.metadata |= C.CONN_V6
		ct.saddr_h = C.__u64(nativeEndian.Uint64(sbytes[:8]))
		ct.saddr_l = C.__u64(nativeEndian.Uint64(sbytes[8:]))
		ct.daddr_h = C.__u64(nativeEndian.Uint64(dbytes[:8]))
		ct.daddr_l = C.__u64(nativeEndian.Uint64(dbytes[8:]))
	} else {
		return nil
	}

	switch proto {
	case network.TCP:
		ct.metadata |= C.CONN_TYPE_TCP
	case network.UDP:
		ct.metadata |= C.CONN_TYPE_UDP
	}

	return ct
}

func (t *ConnTuple) isTCP() bool {
	return connType(uint(t.metadata)) == network.TCP
}

func (t *ConnTuple) isUDP() bool {
	return connType(uint(t.metadata)) == network.UDP
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

func (t *ConnTuple) SourcePort() uint16 {
	return uint16(t.sport)
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

func (t *ConnTuple) DestPort() uint16 {
	return uint16(t.dport)
}

func (t *ConnTuple) Pid() uint32 {
	return uint32(t.pid)
}

func (t *ConnTuple) NetNS() uint64 {
	return uint64(t.netns)
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
__u32 flags;
__u8  direction;
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

func (cs *ConnStatsWithTimestamp) isExpired(latestTime uint64, timeout uint64) bool {
	return latestTime > timeout+uint64(cs.timestamp)
}

func (cs *ConnStatsWithTimestamp) isAssured() bool {
	return uint(cs.flags)&C.CONN_ASSURED > 0
}

func toBatch(data []byte) *batch {
	return (*batch)(unsafe.Pointer(&data[0]))
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

	stats := network.ConnectionStats{
		Pid:                uint32(t.pid),
		Type:               connType(metadata),
		Direction:          connDirection(uint8(s.direction)),
		Family:             family,
		NetNS:              uint32(t.netns),
		Source:             source,
		Dest:               dest,
		SPort:              uint16(t.sport),
		DPort:              uint16(t.dport),
		MonotonicSentBytes: uint64(s.sent_bytes),
		MonotonicRecvBytes: uint64(s.recv_bytes),
		LastUpdateEpoch:    uint64(s.timestamp),
	}

	if connType(metadata) == network.TCP {
		stats.MonotonicRetransmits = uint32(tcpStats.retransmits)
		stats.MonotonicTCPEstablished = uint32(tcpStats.state_transitions >> C.TCP_ESTABLISHED & 1)
		stats.MonotonicTCPClosed = uint32(tcpStats.state_transitions >> C.TCP_CLOSE & 1)
		stats.RTT = uint32(tcpStats.rtt)
		stats.RTTVar = uint32(tcpStats.rtt_var)
	}

	return stats
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

func connDirection(m uint8) network.ConnectionDirection {
	switch m {
	case C.CONN_DIRECTION_INCOMING:
		return network.INCOMING
	case C.CONN_DIRECTION_OUTGOING:
		return network.OUTGOING
	default:
		return network.OUTGOING
	}
}
