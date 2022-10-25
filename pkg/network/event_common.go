// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// ConnectionType will be either TCP or UDP
type ConnectionType uint8

const (
	// TCP connection type
	TCP ConnectionType = 0

	// UDP connection type
	UDP ConnectionType = 1
)

func (c ConnectionType) String() string {
	if c == TCP {
		return "TCP"
	}
	return "UDP"
}

const (
	// AFINET represents v4 connections
	AFINET ConnectionFamily = 0

	// AFINET6 represents v6 connections
	AFINET6 ConnectionFamily = 1
)

// ConnectionFamily will be either v4 or v6
type ConnectionFamily uint8

func (c ConnectionFamily) String() string {
	if c == AFINET {
		return "v4"
	}
	return "v6"
}

// ConnectionDirection indicates if the connection is incoming to the host or outbound
type ConnectionDirection uint8

const (
	// INCOMING represents connections inbound to the host
	INCOMING ConnectionDirection = 1

	// OUTGOING represents outbound connections from the host
	OUTGOING ConnectionDirection = 2

	// LOCAL represents connections that don't leave the host
	LOCAL ConnectionDirection = 3

	// NONE represents connections that have no direction (udp, for example)
	NONE ConnectionDirection = 4
)

func (d ConnectionDirection) String() string {
	switch d {
	case OUTGOING:
		return "outgoing"
	case LOCAL:
		return "local"
	case NONE:
		return "none"
	default:
		return "incoming"
	}
}

// EphemeralPortType will be either EphemeralUnknown, EphemeralTrue, EphemeralFalse
type EphemeralPortType uint8

const (
	// EphemeralUnknown indicates inability to determine whether the port is in the ephemeral range or not
	EphemeralUnknown EphemeralPortType = 0

	// EphemeralTrue means the port has been detected to be in the configured ephemeral range
	EphemeralTrue EphemeralPortType = 1

	// EphemeralFalse means the port has been detected to not be in the configured ephemeral range
	EphemeralFalse EphemeralPortType = 2
)

func (e EphemeralPortType) String() string {
	switch e {
	case EphemeralTrue:
		return "ephemeral"
	case EphemeralFalse:
		return "not ephemeral"
	default:
		return "unspecified"
	}
}

// BufferedData encapsulates data whose underlying memory can be recycled
type BufferedData struct {
	Conns  []ConnectionStats
	buffer *clientBuffer
}

// Connections wraps a collection of ConnectionStats
type Connections struct {
	BufferedData
	DNS                         map[util.Address][]dns.Hostname
	ConnTelemetry               map[ConnTelemetryType]int64
	CompilationTelemetryByAsset map[string]RuntimeCompilationTelemetry
	KernelHeaderFetchResult     int32
	HTTP                        map[transaction.Key]*http.RequestStats
	DNSStats                    dns.StatsByKeyByNameByType
}

// ConnTelemetryType enumerates the connection telemetry gathered by the system-probe
// The string name of each telemetry type is the metric name which will be emitted
type ConnTelemetryType string

//revive:disable:exported
const (
	MonotonicKprobesTriggered       ConnTelemetryType = "kprobes_triggered"
	MonotonicKprobesMissed          ConnTelemetryType = "kprobes_missed"
	MonotonicClosedConnDropped      ConnTelemetryType = "closed_conn_dropped"
	MonotonicConnDropped            ConnTelemetryType = "conn_dropped"
	MonotonicConnsClosed            ConnTelemetryType = "conns_closed"
	MonotonicConntrackRegisters     ConnTelemetryType = "conntrack_registers"
	MonotonicDNSPacketsProcessed    ConnTelemetryType = "dns_packets_processed"
	MonotonicPerfLost               ConnTelemetryType = "perf_lost"
	MonotonicUDPSendsProcessed      ConnTelemetryType = "udp_sends_processed"
	MonotonicUDPSendsMissed         ConnTelemetryType = "udp_sends_missed"
	DNSStatsDropped                 ConnTelemetryType = "dns_stats_dropped"
	ConnsBpfMapSize                 ConnTelemetryType = "conns_bpf_map_size"
	ConntrackSamplingPercent        ConnTelemetryType = "conntrack_sampling_percent"
	NPMDriverFlowsMissedMaxExceeded ConnTelemetryType = "driver_flows_missed_max_exceeded"
	MonotonicDNSPacketsDropped      ConnTelemetryType = "dns_packets_dropped"
)

//revive:enable

var (
	// ConnTelemetryTypes lists all the possible (non-monotonic) telemetry which can be bundled
	// into the network connections payload
	ConnTelemetryTypes = []ConnTelemetryType{
		ConnsBpfMapSize,
		ConntrackSamplingPercent,
		DNSStatsDropped,
		NPMDriverFlowsMissedMaxExceeded,
	}

	// MonotonicConnTelemetryTypes lists all the possible monotonic telemetry which can be bundled
	// into the network connections payload
	MonotonicConnTelemetryTypes = []ConnTelemetryType{
		MonotonicKprobesTriggered,
		MonotonicKprobesMissed,
		MonotonicClosedConnDropped,
		MonotonicConnDropped,
		MonotonicConntrackRegisters,
		MonotonicDNSPacketsProcessed,
		MonotonicConnsClosed,
		MonotonicUDPSendsProcessed,
		MonotonicUDPSendsMissed,
		MonotonicDNSPacketsDropped,
		MonotonicPerfLost,
	}
)

// RuntimeCompilationTelemetry stores telemetry related to the runtime compilation of various assets
type RuntimeCompilationTelemetry struct {
	RuntimeCompilationEnabled  bool
	RuntimeCompilationResult   int32
	RuntimeCompilationDuration int64
}

// StatCounters represents all the per-connection stats we collect
type StatCounters struct {
	SentBytes   uint64
	RecvBytes   uint64
	SentPackets uint64
	RecvPackets uint64
	Retransmits uint32
	// TCPEstablished indicates whether the TCP connection was established
	// after system-probe initialization.
	// * A value of 0 means that this connection was established before system-probe was initialized;
	// * Value 1 represents a connection that was established after system-probe started;
	// * Values greater than 1 should be rare, but can occur when multiple connections
	//   are established with the same tuple between two agent checks;
	TCPEstablished uint32
	TCPClosed      uint32
}

// IsZero returns whether all the stat counter values are zeroes
func (s StatCounters) IsZero() bool {
	return s == StatCounters{}
}

// StatCountersByCookie stores StatCounters by unique cookie
type StatCountersByCookie []*struct {
	StatCounters
	Cookie uint32
}

// Get returns a StatCounters object for a cookie
func (s StatCountersByCookie) Get(cookie uint32) (StatCounters, bool) {
	for _, c := range s {
		if c.Cookie == cookie {
			return c.StatCounters, true
		}
	}

	return StatCounters{}, false
}

// Put adds or sets a StatCounters object for a cookie
func (s *StatCountersByCookie) Put(cookie uint32, sc StatCounters) {
	for _, c := range *s {
		if c.Cookie == cookie {
			c.StatCounters = sc
			return
		}
	}

	*s = append(*s, &struct {
		StatCounters
		Cookie uint32
	}{
		StatCounters: sc,
		Cookie:       cookie,
	})
}

// ConnectionStats stores statistics for a single connection.  Field order in the struct should be 8-byte aligned
type ConnectionStats struct {
	Source util.Address
	Dest   util.Address

	IPTranslation *IPTranslation
	Via           *Via

	// Monotonic stores a list of StatCounters
	// each identified by a unique "cookie"
	//
	// this is necessary because we use connection
	// info like src/dst address/port to uniquely
	// identify a connection and port reuse or
	// races in the ebpf code may occur that would
	// make conflicts in the stats per connection
	// impossible to resolve/detect
	//
	// the "cookie" is generated in the ebpf code
	// when we first create counters for a connection;
	// see the get_conn_stats() function
	Monotonic StatCountersByCookie

	Last StatCounters

	// Last time the stats for this connection were updated
	LastUpdateEpoch uint64

	RTT    uint32 // Stored in µs
	RTTVar uint32

	Pid   uint32
	NetNS uint32

	SPort            uint16
	DPort            uint16
	Type             ConnectionType
	Family           ConnectionFamily
	Direction        ConnectionDirection
	SPortIsEphemeral EphemeralPortType
	Tags             uint64

	IntraHost bool
	IsAssured bool
}

// Via has info about the routing decision for a flow
type Via struct {
	Subnet Subnet
}

// Subnet stores info about a subnet
type Subnet struct {
	Alias string
}

// IPTranslation can be associated with a connection to show the connection is NAT'd
type IPTranslation struct {
	ReplSrcIP   util.Address
	ReplDstIP   util.Address
	ReplSrcPort uint16
	ReplDstPort uint16
}

func (c ConnectionStats) String() string {
	return ConnectionSummary(&c, nil)
}

// IsExpired returns whether the connection is expired according to the provided time and timeout.
func (c ConnectionStats) IsExpired(now uint64, timeout uint64) bool {
	return c.LastUpdateEpoch+timeout <= now
}

// ByteKey returns a unique key for this connection represented as a byte slice
// It's as following:
//
//     4B      2B      2B     .5B     .5B      4/16B        4/16B   = 17/41B
//    32b     16b     16b      4b      4b     32/128b      32/128b
// |  PID  | SPORT | DPORT | Family | Type |  SrcAddr  |  DestAddr
func (c ConnectionStats) ByteKey(buf []byte) []byte {
	return generateConnectionKey(c, buf, false)
}

// ByteKeyNAT returns a unique key for this connection represented as a byte slice.
// The format is similar to the one emitted by `ByteKey` with the sole difference
// that the addresses used are translated.
// Currently this key is used only for the aggregation of ephemeral connections.
func (c ConnectionStats) ByteKeyNAT(buf []byte) []byte {
	return generateConnectionKey(c, buf, true)
}

// IsShortLived returns true when a connection went through its whole lifecycle
// between two connection checks
func (c ConnectionStats) IsShortLived() bool {
	return c.Last.TCPEstablished >= 1 && c.Last.TCPClosed >= 1
}

// MonotonicSum returns the sum of all the monotonic stats
func (c ConnectionStats) MonotonicSum() StatCounters {
	var stc StatCounters
	for _, st := range c.Monotonic {
		stc = stc.Add(st.StatCounters)
	}

	return stc
}

func (c ConnectionStats) clone() ConnectionStats {
	cl := c
	cl.Monotonic = make(StatCountersByCookie, 0, len(c.Monotonic))
	for _, s := range c.Monotonic {
		cl.Monotonic.Put(s.Cookie, s.StatCounters)
	}

	return cl
}

const keyFmt = "p:%d|src:%s:%d|dst:%s:%d|f:%d|t:%d"

// BeautifyKey returns a human readable byte key (used for debugging purposes)
// it should be in sync with ByteKey
// Note: This is only used in /debug/* endpoints
func BeautifyKey(key string) string {
	bytesToAddress := func(buf []byte) util.Address {
		if len(buf) == 4 {
			return util.V4AddressFromBytes(buf)
		}
		return util.V6AddressFromBytes(buf)
	}

	raw := []byte(key)

	// First 8 bytes are pid and ports
	h := binary.LittleEndian.Uint64(raw[:8])
	pid := h >> 32
	sport := (h >> 16) & 0xffff
	dport := h & 0xffff

	// Then we have the family, type
	family := (raw[8] >> 4) & 0xf
	typ := raw[8] & 0xf

	// Finally source addr, dest addr
	addrSize := 4
	if ConnectionFamily(family) == AFINET6 {
		addrSize = 16
	}

	source := bytesToAddress(raw[9 : 9+addrSize])
	dest := bytesToAddress(raw[9+addrSize : 9+2*addrSize])

	return fmt.Sprintf(keyFmt, pid, source, sport, dest, dport, family, typ)
}

// ConnectionSummary returns a string summarizing a connection
func ConnectionSummary(c *ConnectionStats, names map[util.Address][]dns.Hostname) string {
	str := fmt.Sprintf(
		"[%s%s] [PID: %d] [%v:%d ⇄ %v:%d] ",
		c.Type,
		c.Family,
		c.Pid,
		printAddress(c.Source, names[c.Source]),
		c.SPort,
		printAddress(c.Dest, names[c.Dest]),
		c.DPort,
	)
	if c.IPTranslation != nil {
		str += fmt.Sprintf(
			"xlated [%v:%d ⇄ %v:%d] ",
			c.IPTranslation.ReplSrcIP,
			c.IPTranslation.ReplSrcPort,
			c.IPTranslation.ReplDstIP,
			c.IPTranslation.ReplDstPort,
		)
	}

	var stc StatCounters
	cookies := make([]uint32, 0, len(c.Monotonic))
	for _, st := range c.Monotonic {
		stc = stc.Add(st.StatCounters)
		cookies = append(cookies, st.Cookie)
	}

	str += fmt.Sprintf("(%s) %s sent (+%s), %s received (+%s)",
		c.Direction,
		humanize.Bytes(stc.SentBytes), humanize.Bytes(c.Last.SentBytes),
		humanize.Bytes(stc.RecvBytes), humanize.Bytes(c.Last.RecvBytes),
	)

	if c.Type == TCP {
		str += fmt.Sprintf(
			", %d retransmits (+%d), RTT %s (± %s), %d established (+%d), %d closed (+%d)",
			stc.Retransmits, c.Last.Retransmits,
			time.Duration(c.RTT)*time.Microsecond,
			time.Duration(c.RTTVar)*time.Microsecond,
			stc.TCPEstablished, c.Last.TCPEstablished,
			stc.TCPClosed, c.Last.TCPClosed,
		)
	}

	str += fmt.Sprintf(", last update epoch: %d, cookies: %+v", c.LastUpdateEpoch, cookies)

	return str
}

func printAddress(address util.Address, names []dns.Hostname) string {
	if len(names) == 0 {
		return address.String()
	}

	var b strings.Builder
	b.WriteString(dns.ToString(names[0]))
	for _, s := range names[1:] {
		b.WriteString(",")
		b.WriteString(dns.ToString(s))
	}
	return b.String()
}

// HTTPKeyTuplesFromConn build the key for the http map based on whether the local or remote side is http.
func HTTPKeyTuplesFromConn(c ConnectionStats) [2]transaction.KeyTuple {
	// Retrieve translated addresses
	laddr, lport := GetNATLocalAddress(c)
	raddr, rport := GetNATRemoteAddress(c)

	// HTTP data is always indexed as (client, server), but we don't know which is the remote
	// and which is the local address. To account for this, we'll construct 2 possible
	// http keys and check for both of them in our http aggregations map.
	return [2]transaction.KeyTuple{
		transaction.NewKeyTuple(laddr, raddr, lport, rport),
		transaction.NewKeyTuple(raddr, laddr, rport, lport),
	}
}

func generateConnectionKey(c ConnectionStats, buf []byte, useNAT bool) []byte {
	laddr, sport := c.Source, c.SPort
	raddr, dport := c.Dest, c.DPort
	if useNAT {
		laddr, sport = GetNATLocalAddress(c)
		raddr, dport = GetNATRemoteAddress(c)
	}

	n := 0
	// Byte-packing to improve creation speed
	// PID (32 bits) + SPort (16 bits) + DPort (16 bits) = 64 bits
	p0 := uint64(c.Pid)<<32 | uint64(sport)<<16 | uint64(dport)
	binary.LittleEndian.PutUint64(buf[0:], p0)
	n += 8

	// Family (4 bits) + Type (4 bits) = 8 bits
	buf[n] = uint8(c.Family)<<4 | uint8(c.Type)
	n++

	n += laddr.WriteTo(buf[n:]) // 4 or 16 bytes
	n += raddr.WriteTo(buf[n:]) // 4 or 16 bytes
	return buf[:n]
}

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

// Add returns s+other
func (s StatCounters) Add(other StatCounters) StatCounters {
	return StatCounters{
		RecvBytes:      s.RecvBytes + other.RecvBytes,
		RecvPackets:    s.RecvPackets + other.RecvPackets,
		Retransmits:    s.Retransmits + other.Retransmits,
		SentBytes:      s.SentBytes + other.SentBytes,
		SentPackets:    s.SentPackets + other.SentPackets,
		TCPClosed:      s.TCPClosed + other.TCPClosed,
		TCPEstablished: s.TCPEstablished + other.TCPEstablished,
	}
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}

	return b
}

func maxUint32(a, b uint32) uint32 {
	if a > b {
		return a
	}

	return b
}

// Max returns max(s, other)
func (s StatCounters) Max(other StatCounters) StatCounters {
	return StatCounters{
		RecvBytes:      maxUint64(s.RecvBytes, other.RecvBytes),
		RecvPackets:    maxUint64(s.RecvPackets, other.RecvPackets),
		Retransmits:    maxUint32(s.Retransmits, other.Retransmits),
		SentBytes:      maxUint64(s.SentBytes, other.SentBytes),
		SentPackets:    maxUint64(s.SentPackets, other.SentPackets),
		TCPClosed:      maxUint32(s.TCPClosed, other.TCPClosed),
		TCPEstablished: maxUint32(s.TCPEstablished, other.TCPEstablished),
	}
}
