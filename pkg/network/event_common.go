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

	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/dustin/go-humanize"
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
	DNS                         map[util.Address][]string
	ConnTelemetry               map[ConnTelemetryType]int64
	CompilationTelemetryByAsset map[string]RuntimeCompilationTelemetry
	HTTP                        map[http.Key]http.RequestStats
	DNSStats                    dns.StatsByKeyByNameByType
}

// ConnTelemetryType enumerates the connection telemetry gathered by the system-probe
// The string name of each telemetry type is the metric name which will be emitted
type ConnTelemetryType string

//revive:disable:exported
const (
	MonotonicKprobesTriggered          ConnTelemetryType = "kprobes_triggered"
	MonotonicKprobesMissed                               = "kprobes_missed"
	MonotonicConnsClosed                                 = "conns_closed"
	MonotonicConntrackRegisters                          = "conntrack_registers"
	MonotonicConntrackRegistersDropped                   = "conntrack_registers_dropped"
	MonotonicDNSPacketsProcessed                         = "dns_packets_processed"
	MonotonicUDPSendsProcessed                           = "udp_sends_processed"
	MonotonicUDPSendsMissed                              = "udp_sends_missed"
	DNSStatsDropped                                      = "dns_stats_dropped"
	ConnsBpfMapSize                                      = "conns_bpf_map_size"
	ConntrackSamplingPercent                             = "conntrack_sampling_percent"
	NPMDriverFlowsMissedMaxExceeded                      = "driver_flows_missed_max_exceeded"
	MonotonicDNSPacketsDropped                           = "dns_packets_dropped"
	HTTPRequestsDropped                                  = "http_requests_dropped"
	HTTPRequestsMissed                                   = "http_requests_missed"
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
		HTTPRequestsDropped,
		HTTPRequestsMissed,
	}

	// MonotonicConnTelemetryTypes lists all the possible monotonic telemetry which can be bundled
	// into the network connections payload
	MonotonicConnTelemetryTypes = []ConnTelemetryType{
		MonotonicKprobesTriggered,
		MonotonicKprobesMissed,
		MonotonicConntrackRegisters,
		MonotonicConntrackRegistersDropped,
		MonotonicDNSPacketsProcessed,
		MonotonicConnsClosed,
		MonotonicUDPSendsProcessed,
		MonotonicUDPSendsMissed,
		MonotonicDNSPacketsDropped,
	}
)

// RuntimeCompilationTelemetry stores telemetry related to the runtime compilation of various assets
type RuntimeCompilationTelemetry struct {
	RuntimeCompilationEnabled  bool
	RuntimeCompilationResult   int32
	KernelHeaderFetchResult    int32
	RuntimeCompilationDuration int64
}

// ConnectionStats stores statistics for a single connection.  Field order in the struct should be 8-byte aligned
type ConnectionStats struct {
	Source util.Address
	Dest   util.Address

	MonotonicSentBytes uint64
	LastSentBytes      uint64

	MonotonicRecvBytes uint64
	LastRecvBytes      uint64

	MonotonicSentPackets uint64
	LastSentPackets      uint64

	MonotonicRecvPackets uint64
	LastRecvPackets      uint64

	// Last time the stats for this connection were updated
	LastUpdateEpoch uint64

	MonotonicRetransmits uint32
	LastRetransmits      uint32

	RTT    uint32 // Stored in µs
	RTTVar uint32

	// MonotonicTCPEstablished indicates whether or not the TCP connection was established
	// after system-probe initialization.
	// * A value of 0 means that this connection was established before system-probe was initialized;
	// * Value 1 represents a connection that was established after system-probe started;
	// * Values greater than 1 should be rare, but can occur when multiple connections
	//   are established with the same tuple betweeen two agent checks;
	MonotonicTCPEstablished uint32
	LastTCPEstablished      uint32

	MonotonicTCPClosed uint32
	LastTCPClosed      uint32

	Pid   uint32
	NetNS uint32

	SPort            uint16
	DPort            uint16
	Type             ConnectionType
	Family           ConnectionFamily
	Direction        ConnectionDirection
	SPortIsEphemeral EphemeralPortType
	IPTranslation    *IPTranslation
	IntraHost        bool
	Via              *Via

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

// ByteKey returns a unique key for this connection represented as a byte array
// It's as following:
//
//     4B      2B      2B     .5B     .5B      4/16B        4/16B   = 17/41B
//    32b     16b     16b      4b      4b     32/128b      32/128b
// |  PID  | SPORT | DPORT | Family | Type |  SrcAddr  |  DestAddr
func (c ConnectionStats) ByteKey(buf []byte) ([]byte, error) {
	n := 0
	// Byte-packing to improve creation speed
	// PID (32 bits) + SPort (16 bits) + DPort (16 bits) = 64 bits
	p0 := uint64(c.Pid)<<32 | uint64(c.SPort)<<16 | uint64(c.DPort)
	binary.LittleEndian.PutUint64(buf[0:], p0)
	n += 8

	// Family (4 bits) + Type (4 bits) = 8 bits
	buf[n] = uint8(c.Family)<<4 | uint8(c.Type)
	n++

	n += c.Source.WriteTo(buf[n:]) // 4 or 16 bytes
	n += c.Dest.WriteTo(buf[n:])   // 4 or 16 bytes
	return buf[:n], nil
}

// IsShortLived returns true when a connection went through its whole lifecycle
// between two connection checks
func (c ConnectionStats) IsShortLived() bool {
	return c.LastTCPEstablished >= 1 && c.LastTCPClosed >= 1
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
func ConnectionSummary(c *ConnectionStats, names map[util.Address][]string) string {
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

	str += fmt.Sprintf("(%s) %s sent (+%s), %s received (+%s)",
		c.Direction,
		humanize.Bytes(c.MonotonicSentBytes), humanize.Bytes(c.LastSentBytes),
		humanize.Bytes(c.MonotonicRecvBytes), humanize.Bytes(c.LastRecvBytes),
	)

	if c.Type == TCP {
		str += fmt.Sprintf(
			", %d retransmits (+%d), RTT %s (± %s)",
			c.MonotonicRetransmits, c.LastRetransmits,
			time.Duration(c.RTT)*time.Microsecond,
			time.Duration(c.RTTVar)*time.Microsecond,
		)
	}

	return str
}

func printAddress(address util.Address, names []string) string {
	if len(names) == 0 {
		return address.String()
	}

	return strings.Join(names, ",")
}
