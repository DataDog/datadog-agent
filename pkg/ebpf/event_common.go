package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

// ConnectionFamily will be either v4 or v6
type ConnectionFamily uint8

// ConnectionDirection indicates if the connection is incoming to the host or outbound
type ConnectionDirection uint8

const (
	// INCOMING represents connections inbound to the host
	INCOMING ConnectionDirection = 1

	// OUTGOING represents outbound connections from the host
	OUTGOING ConnectionDirection = 2

	// LOCAL represents connections that don't leave the host
	LOCAL ConnectionDirection = 3
)

func (d ConnectionDirection) String() string {
	switch d {
	case OUTGOING:
		return "outgoing"
	case LOCAL:
		return "local"
	default:
		return "incoming"
	}
}

const (
	// AFINET represents v4 connections
	AFINET ConnectionFamily = 0

	// AFINET6 represents v6 connections
	AFINET6 ConnectionFamily = 1
)

// Connections wraps a collection of ConnectionStats
//easyjson:json
type Connections struct {
	Conns []ConnectionStats `json:"connections"`
}

// ConnectionStats stores statistics for a single connection.  Field order in the struct should be 8-byte aligned
//easyjson:json
type ConnectionStats struct {
	// Source & Dest represented as a string to handle both IPv4 & IPv6
	Source string `json:"source"`
	Dest   string `json:"dest"`

	MonotonicSentBytes uint64 `json:"monotonic_sent_bytes"`
	LastSentBytes      uint64 `json:"last_sent_bytes"`

	MonotonicRecvBytes uint64 `json:"monotonic_recv_bytes"`
	LastRecvBytes      uint64 `json:"last_recv_bytes"`

	// Last time the stats for this connection were updated
	LastUpdateEpoch uint64 `json:"last_update_epoch"`

	MonotonicRetransmits uint32 `json:"monotonic_retransmits"`
	LastRetransmits      uint32 `json:"last_retransmits"`

	Pid   uint32 `json:"pid"`
	NetNS uint32 `json:"net_ns"`

	SPort     uint16              `json:"sport"`
	DPort     uint16              `json:"dport"`
	Type      ConnectionType      `json:"type"`
	Family    ConnectionFamily    `json:"family"`
	Direction ConnectionDirection `json:"direction"`
}

func (c ConnectionStats) String() string {
	return fmt.Sprintf(
		"[%s] [PID: %d] [%v:%d ⇄ %v:%d] (%s) %d bytes sent (+%d), %d bytes received (+%d), %d retransmits (+%d)",
		c.Type,
		c.Pid,
		c.Source,
		c.SPort,
		c.Dest,
		c.DPort,
		c.Direction,
		c.MonotonicSentBytes, c.LastSentBytes,
		c.MonotonicRecvBytes, c.LastRecvBytes,
		c.MonotonicRetransmits, c.LastRetransmits,
	)
}

// ByteKey returns a unique key for this connection represented as a byte array
func (c ConnectionStats) ByteKey(buffer *bytes.Buffer) ([]byte, error) {
	buffer.Reset()
	// Byte-packing to improve creation speed
	// PID (32 bits) + SPort (16 bits) + DPort (16 bits) = 64 bits
	p0 := uint64(c.Pid)<<32 | uint64(c.SPort)<<16 | uint64(c.DPort)

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], p0)

	if _, err := buffer.Write(buf[:]); err != nil {
		return nil, err
	}
	if _, err := buffer.WriteString(c.Source); err != nil {
		return nil, err
	}
	// Family (4 bits) + Type (4 bits) = 8 bits
	p1 := uint8(c.Family)<<4 | uint8(c.Type)
	if err := buffer.WriteByte(p1); err != nil {
		return nil, err
	}
	if _, err := buffer.WriteString(c.Dest); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
