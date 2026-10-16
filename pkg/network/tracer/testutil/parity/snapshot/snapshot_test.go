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
	udp := &model.Connection{
		Laddr: &model.Addr{Ip: "10.0.0.1", Port: 53}, Raddr: &model.Addr{Ip: "10.0.0.3", Port: 5353},
		Type: model.ConnectionType_udp, LastBytesSent: 64, IntraHost: true,
	}
	totals := Sum(ToStats(&model.Connections{Conns: []*model.Connection{tcpConn(40000, 1), tcpConn(40001, 0), udp}}))

	assert.Equal(t, Totals{
		Records: 2, SentBytes: 200, RecvBytes: 4000, SentPackets: 6, RecvPackets: 8,
		TCPEstablished: 2, TCPClosed: 1, TCPFailures: 4,
	}, totals[Class{Type: network.TCP, Family: network.AFINET}])
	assert.Equal(t, Totals{Records: 1, SentBytes: 64}, totals[Class{Type: network.UDP, Family: network.AFINET, IntraHost: true}])
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
