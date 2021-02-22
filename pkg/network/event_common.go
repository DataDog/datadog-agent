package network

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

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

// Connections wraps a collection of ConnectionStats
type Connections struct {
	DNS       map[util.Address][]string
	Conns     []ConnectionStats
	Telemetry *ConnectionsTelemetry
}

// ConnectionsTelemetry stores telemetry from the system probe
type ConnectionsTelemetry struct {
	MonotonicKprobesTriggered          int64
	MonotonicKprobesMissed             int64
	MonotonicConntrackRegisters        int64
	MonotonicConntrackRegistersDropped int64
	MonotonicDNSPacketsProcessed       int64
	MonotonicConnsClosed               int64
	ConnsBpfMapSize                    int64
	MonotonicUDPSendsProcessed         int64
	MonotonicUDPSendsMissed            int64
	ConntrackSamplingPercent           int64
}

// ConnectionStats stores statistics for a single connection.  Field order in the struct should be 8-byte aligned
type ConnectionStats struct {
	Source util.Address
	Dest   util.Address

	MonotonicSentBytes uint64
	LastSentBytes      uint64

	MonotonicRecvBytes uint64
	LastRecvBytes      uint64

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

	SPort                  uint16
	DPort                  uint16
	Type                   ConnectionType
	Family                 ConnectionFamily
	Direction              ConnectionDirection
	IPTranslation          *IPTranslation
	IntraHost              bool
	DNSSuccessfulResponses uint32
	DNSFailedResponses     uint32
	DNSTimeouts            uint32
	DNSSuccessLatencySum   uint64
	DNSFailureLatencySum   uint64
	DNSCountByRcode        map[uint32]uint32
	DNSStatsByDomain       map[string]DNSStats
	HTTPStatsByPath        map[string]http.RequestStats
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
		"[%s] [PID: %d] [%v:%d ⇄ %v:%d] (%s) %s sent (+%s), %s received (+%s)",
		c.Type,
		c.Pid,
		printAddress(c.Source, names[c.Source]),
		c.SPort,
		printAddress(c.Dest, names[c.Dest]),
		c.DPort,
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

// DNSKey is an identifier for a set of DNS connections
type DNSKey struct {
	serverIP   util.Address
	clientIP   util.Address
	clientPort uint16
	// ConnectionType will be either TCP or UDP
	protocol ConnectionType
}

// DNSStats holds statistics corresponding to a particular domain
type DNSStats struct {
	DNSTimeouts          uint32
	DNSSuccessLatencySum uint64
	DNSFailureLatencySum uint64
	DNSCountByRcode      map[uint32]uint32
}
