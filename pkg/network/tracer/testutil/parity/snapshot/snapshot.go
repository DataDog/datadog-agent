// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package snapshot reads connections from a running system-probe so that two
// tracers observing the same traffic (for example two agents on one node, one
// on the kprobe tracer and one on fentry) can be compared with parity.Compare.
//
// Each side is read through system-probe's /network_tracer/connections endpoint
// under its own client ID, so the process-agent's client is not disturbed. Both
// sides must be read with the same client ID at (nearly) the same moment, and
// more often than system-probe's client state expiry (2 minutes by default):
// a client that isn't read for longer is dropped, and its next read starts
// over with no closed connections.
package snapshot

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Client reads connections from one system-probe over its unix socket
type Client struct {
	socket string
	http   *http.Client
}

// NewClient returns a Client for the system-probe listening on socket
func NewClient(socket string) *Client {
	return &Client{
		socket: socket,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

// Fetch returns the connections system-probe has for clientID: the deltas
// since the previous Fetch with the same clientID, or everything currently
// active on the first call (which also registers the client).
func (c *Client) Fetch(ctx context.Context, clientID string) (*model.Connections, error) {
	u := "http://unix/network_tracer/connections?client_id=" + url.QueryEscape(clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", unmarshal.ContentTypeProtobuf)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c.socket, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", c.socket, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d: %s", c.socket, resp.StatusCode, body)
	}
	return unmarshal.GetUnmarshaler(resp.Header.Get("Content-Type")).Unmarshal(body)
}

// FetchPair reads both sides concurrently, so that their snapshots are taken
// as close together as possible
func FetchPair(ctx context.Context, a, b *Client, clientID string) (ca, cb *model.Connections, err error) {
	var wg sync.WaitGroup
	var errA, errB error
	wg.Add(2)
	go func() { defer wg.Done(); ca, errA = a.Fetch(ctx, clientID) }()
	go func() { defer wg.Done(); cb, errB = b.Fetch(ctx, clientID) }()
	wg.Wait()
	if errA != nil {
		return nil, nil, errA
	}
	if errB != nil {
		return nil, nil, errB
	}
	return ca, cb, nil
}

// ReadFile reads a saved /network_tracer/connections response, in either JSON
// or protobuf form
func ReadFile(path string) (*model.Connections, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	if len(body) > 0 && body[0] != '{' {
		ctype = unmarshal.ContentTypeProtobuf
	}
	return unmarshal.GetUnmarshaler(ctype).Unmarshal(body)
}

// ToStats converts a connections payload into ConnectionStats that
// parity.Compare accepts.
//
// The payload carries per-client deltas, so they are stored in Monotonic: the
// two sides are comparable as long as both were read with the same client ID
// at the same times. The payload has no closed flag, so a TCP connection is
// treated as closed when its delta includes a close.
func ToStats(conns *model.Connections) []network.ConnectionStats {
	out := make([]network.ConnectionStats, 0, len(conns.Conns))
	for _, c := range conns.Conns {
		s := network.ConnectionStats{
			ConnectionTuple: network.ConnectionTuple{
				Source:    util.AddressFromString(c.Laddr.GetIp()),
				Dest:      util.AddressFromString(c.Raddr.GetIp()),
				Pid:       uint32(c.Pid),
				NetNS:     c.NetNS,
				SPort:     uint16(c.Laddr.GetPort()),
				DPort:     uint16(c.Raddr.GetPort()),
				Type:      connType(c.Type),
				Family:    family(c.Family),
				Direction: direction(c.Direction),
			},
			Monotonic: network.StatCounters{
				SentBytes:        c.LastBytesSent,
				RecvBytes:        c.LastBytesReceived,
				SentPackets:      c.LastPacketsSent,
				RecvPackets:      c.LastPacketsReceived,
				Retransmits:      c.LastRetransmits,
				TCPEstablished:   uint16(c.LastTcpEstablished),
				TCPClosed:        uint16(c.LastTcpClosed),
				TCPRTOCount:      c.LastTcpRtoCount,
				TCPRecoveryCount: c.LastTcpRecoveryCount,
				TCPReordSeen:     c.LastTcpReordSeen,
				TCPRcvOOOPack:    c.LastTcpRcvOooPack,
				TCPDeliveredCE:   c.LastTcpDeliveredCe,
				TCPProbe0Count:   c.LastTcpProbe0Count,
			},
			RTT:              c.Rtt,
			RTTVar:           c.RttVar,
			IntraHost:        c.IntraHost,
			SPortIsEphemeral: ephemeral(c.IsLocalPortEphemeral),
			TCPECNNegotiated: c.TcpEcnNegotiated,
			ProtocolStack:    stack(c.Protocol),
		}
		s.IsClosed = s.Type == network.TCP && c.LastTcpClosed > 0
		if len(c.TcpFailuresByErrCode) > 0 {
			s.TCPFailures = make(map[uint16]uint32, len(c.TcpFailuresByErrCode))
			for errno, n := range c.TcpFailuresByErrCode {
				s.TCPFailures[uint16(errno)] = n
			}
		}
		out = append(out, s)
	}
	return out
}

func connType(t model.ConnectionType) network.ConnectionType {
	if t == model.ConnectionType_udp {
		return network.UDP
	}
	return network.TCP
}

func family(f model.ConnectionFamily) network.ConnectionFamily {
	if f == model.ConnectionFamily_v6 {
		return network.AFINET6
	}
	return network.AFINET
}

func direction(d model.ConnectionDirection) network.ConnectionDirection {
	switch d {
	case model.ConnectionDirection_incoming:
		return network.INCOMING
	case model.ConnectionDirection_outgoing:
		return network.OUTGOING
	case model.ConnectionDirection_local:
		return network.LOCAL
	case model.ConnectionDirection_none:
		return network.NONE
	default:
		return network.UNKNOWN
	}
}

func ephemeral(e model.EphemeralPortState) network.EphemeralPortType {
	switch e {
	case model.EphemeralPortState_ephemeralTrue:
		return network.EphemeralTrue
	case model.EphemeralPortState_ephemeralFalse:
		return network.EphemeralFalse
	default:
		return network.EphemeralUnknown
	}
}

// stack is the inverse of the encoder's FormatProtocolStack
func stack(p *model.ProtocolStack) protocols.Stack {
	var s protocols.Stack
	for _, t := range p.GetStack() {
		switch t {
		case model.ProtocolType_protocolTLS:
			s.Encryption = protocols.TLS
		case model.ProtocolType_protocolGRPC:
			s.API = protocols.GRPC
		case model.ProtocolType_protocolHTTP:
			s.Application = protocols.HTTP
		case model.ProtocolType_protocolHTTP2:
			s.Application = protocols.HTTP2
		case model.ProtocolType_protocolKafka:
			s.Application = protocols.Kafka
		case model.ProtocolType_protocolMongo:
			s.Application = protocols.Mongo
		case model.ProtocolType_protocolPostgres:
			s.Application = protocols.Postgres
		case model.ProtocolType_protocolAMQP:
			s.Application = protocols.AMQP
		case model.ProtocolType_protocolRedis:
			s.Application = protocols.Redis
		case model.ProtocolType_protocolMySQL:
			s.Application = protocols.MySQL
		}
	}
	return s
}

// Class groups connections for Totals
type Class struct {
	Type      network.ConnectionType
	Family    network.ConnectionFamily
	IntraHost bool
}

// Totals are the summed counters of one side's connections in one Class
type Totals struct {
	Records        uint64
	SentBytes      uint64
	RecvBytes      uint64
	SentPackets    uint64
	RecvPackets    uint64
	Retransmits    uint64
	TCPEstablished uint64
	TCPClosed      uint64
	TCPFailures    uint64
}

// Sum totals conns by Class. Unlike a per-connection comparison, totals are
// not sensitive to a connection being reported in different snapshots on the
// two sides, so they can be accumulated over a long run.
func Sum(conns []network.ConnectionStats) map[Class]Totals {
	out := make(map[Class]Totals)
	for i := range conns {
		c := &conns[i]
		k := Class{Type: c.Type, Family: c.Family, IntraHost: c.IntraHost}
		t := out[k]
		t.Records++
		t.SentBytes += c.Monotonic.SentBytes
		t.RecvBytes += c.Monotonic.RecvBytes
		t.SentPackets += c.Monotonic.SentPackets
		t.RecvPackets += c.Monotonic.RecvPackets
		t.Retransmits += uint64(c.Monotonic.Retransmits)
		t.TCPEstablished += uint64(c.Monotonic.TCPEstablished)
		t.TCPClosed += uint64(c.Monotonic.TCPClosed)
		for _, n := range c.TCPFailures {
			t.TCPFailures += uint64(n)
		}
		out[k] = t
	}
	return out
}
