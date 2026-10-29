// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/parity"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func tcpConn(sport int32, closed uint32) *model.Connection {
	return &model.Connection{
		Pid:                  42,
		Laddr:                &model.Addr{Ip: "10.0.0.1", Port: sport},
		Raddr:                &model.Addr{Ip: "10.0.0.2", Port: 80},
		Family:               model.ConnectionFamily_v4,
		Type:                 model.ConnectionType_tcp,
		IsLocalPortEphemeral: model.EphemeralPortState_ephemeralTrue,
		Direction:            model.ConnectionDirection_outgoing,
		NetNS:                7,
		LastBytesSent:        100,
		LastBytesReceived:    2000,
		LastPacketsSent:      3,
		LastPacketsReceived:  4,
		LastTcpEstablished:   1,
		LastTcpClosed:        closed,
		Rtt:                  250,
		Protocol:             &model.ProtocolStack{Stack: []model.ProtocolType{model.ProtocolType_protocolTLS, model.ProtocolType_protocolHTTP}},
		TcpFailuresByErrCode: map[uint32]uint32{32: 2},
	}
}

func TestToStats(t *testing.T) {
	stats := ToStats(&model.Connections{Conns: []*model.Connection{tcpConn(40000, 1), tcpConn(40001, 0)}})
	require.Len(t, stats, 2)

	c := stats[0]
	assert.Equal(t, network.ConnectionTuple{
		Source:    util.AddressFromString("10.0.0.1"),
		Dest:      util.AddressFromString("10.0.0.2"),
		Pid:       42,
		NetNS:     7,
		SPort:     40000,
		DPort:     80,
		Type:      network.TCP,
		Family:    network.AFINET,
		Direction: network.OUTGOING,
	}, c.ConnectionTuple)
	assert.Equal(t, uint64(100), c.Monotonic.SentBytes)
	assert.Equal(t, uint64(2000), c.Monotonic.RecvBytes)
	assert.Equal(t, uint16(1), c.Monotonic.TCPClosed)
	assert.Equal(t, network.EphemeralTrue, c.SPortIsEphemeral)
	assert.Equal(t, protocols.Stack{Encryption: protocols.TLS, Application: protocols.HTTP}, c.ProtocolStack)
	assert.Equal(t, map[uint16]uint32{32: 2}, c.TCPFailures)
	assert.True(t, c.IsClosed, "a TCP delta with a close is treated as closed")
	assert.False(t, stats[1].IsClosed)
}

func TestSnapshotsCompare(t *testing.T) {
	a := ToStats(&model.Connections{Conns: []*model.Connection{tcpConn(40000, 1), tcpConn(40001, 1)}})
	b := ToStats(&model.Connections{Conns: []*model.Connection{tcpConn(40000, 1)}})

	r := parity.Compare(a, b, parity.Options{})
	assert.Equal(t, 1, r.Matched)
	require.Len(t, r.Divergences, 1)
	assert.Equal(t, parity.OnlyA, r.Divergences[0].Kind)
	assert.Equal(t, uint16(40001), r.Divergences[0].Tuple.SPort)
}

func TestSum(t *testing.T) {
	tcp := tcpConn(40000, 1)
	tcp.IpTranslation = &model.IPTranslation{ReplSrcIP: "10.0.0.9"}
	udp := &model.Connection{
		Laddr: &model.Addr{Ip: "10.0.0.1", Port: 53}, Raddr: &model.Addr{Ip: "10.0.0.3", Port: 5353},
		Type: model.ConnectionType_udp, LastBytesSent: 64, IntraHost: true,
		DnsStatsByDomainOffsetByQueryType: map[int32]*model.DNSStatsByQueryType{0: {DnsStatsByQueryType: map[int32]*model.DNSStats{
			1: {DnsTimeouts: 1, DnsCountByRcode: map[uint32]uint32{0: 5, 3: 2}},
		}}},
	}
	totals := Sum(&model.Connections{Conns: []*model.Connection{tcp, tcpConn(40001, 0), udp}})

	tt := totals[Class{Type: model.ConnectionType_tcp, Family: model.ConnectionFamily_v4}]
	require.NotNil(t, tt)
	for field, want := range map[string]uint64{
		"records": 2, "sent_bytes": 200, "recv_bytes": 4000, "sent_packets": 6, "recv_packets": 8,
		"tcp_established": 2, "tcp_closed": 1, "tcp_failures": 4,
		"rtt_sum": 500, "rtt_count": 2, "nat_translated": 1,
	} {
		assert.Equal(t, want, tt.Counters[field], field)
	}
	assert.Equal(t, map[string]uint64{"32": 4}, tt.Breakdowns["tcp_failures_by_errno"])
	assert.Equal(t, map[string]uint64{"tls": 2, "http": 2}, tt.Breakdowns["conns_by_protocol"])

	ut := totals[Class{Type: model.ConnectionType_udp, Family: model.ConnectionFamily_v4, IntraHost: true}]
	require.NotNil(t, ut)
	assert.Equal(t, uint64(1), ut.Counters["records"])
	assert.Equal(t, uint64(5), ut.Counters["dns_successes"])
	assert.Equal(t, uint64(2), ut.Counters["dns_failures"])
	assert.Equal(t, uint64(1), ut.Counters["dns_timeouts"])
	assert.Equal(t, map[string]uint64{"0": 5, "3": 2}, ut.Breakdowns["dns_responses_by_rcode"])
}

func TestReadFileJSON(t *testing.T) {
	m := jsonpb.Marshaler{}
	s, err := m.MarshalToString(&model.Connections{Conns: []*model.Connection{tcpConn(40000, 1)}})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "conns.json")
	require.NoError(t, os.WriteFile(path, []byte(s), 0o600))

	conns, err := ReadFile(path)
	require.NoError(t, err)
	require.Len(t, conns.Conns, 1)
	assert.Equal(t, uint64(2000), conns.Conns[0].LastBytesReceived)
}
