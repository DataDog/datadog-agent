// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// interfaceTagState is a network.State used only by the interface-tag tests.
// It returns the active slice from GetDelta and records a closed connection by value.
type interfaceTagState struct {
	closed      network.ConnectionStats
	snapshotted bool
}

// GetDelta returns the active connections unchanged.
func (m *interfaceTagState) GetDelta(_ string, _ uint64, active []network.ConnectionStats, _ dns.StatsByKeyByNameByType, _ map[protocols.ProtocolType]interface{}) network.Delta {
	return network.Delta{Conns: active}
}

// GetTelemetryDelta returns no telemetry.
func (m *interfaceTagState) GetTelemetryDelta(_ string, _ map[network.ConnTelemetryType]int64) map[network.ConnTelemetryType]int64 {
	return nil
}

// RegisterClient does nothing.
func (m *interfaceTagState) RegisterClient(_ string) {}

// RemoveClient does nothing.
func (m *interfaceTagState) RemoveClient(_ string) {}

// RemoveExpiredClients does nothing.
func (m *interfaceTagState) RemoveExpiredClients(_ time.Time) {}

// RemoveConnections does nothing.
func (m *interfaceTagState) RemoveConnections(_ []*network.ConnectionStats) {}

// StoreClosedConnection records a copy of conn.
func (m *interfaceTagState) StoreClosedConnection(conn *network.ConnectionStats) {
	if conn != nil {
		m.closed = *conn
		m.snapshotted = true
	}
}

// GetStats returns no statistics.
func (m *interfaceTagState) GetStats() map[string]interface{} { return nil }

// DumpState returns no state.
func (m *interfaceTagState) DumpState(_ string) map[string]interface{} { return nil }

// hasInterfaceTag reports whether tags contains the interned name.
func hasInterfaceTag(tags []*intern.Value, name string) bool {
	want := intern.GetByString(name)
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

// TestDarwinTracer_GetActiveConnections_TagsInterface checks that an active row
// gains interface tags and the source row does not.
func TestDarwinTracer_GetActiveConnections_TagsInterface(t *testing.T) {
	conn := &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.8.0.2"),
			Dest:   util.AddressFromString("1.1.1.1"),
			SPort:  12345,
			DPort:  443,
			Pid:    4242,
		},
		LastUpdateEpoch: uint64(time.Now().UnixNano()),
	}
	keep := intern.GetByString("keep-me")
	spare := intern.GetByString("spare")
	conn.Tags = []*intern.Value{keep, spare}
	conn.Tags = conn.Tags[:1]
	tr := newTestTracer(
		&mockConnTracer{connsToReturn: []*network.ConnectionStats{conn}},
		&interfaceTagState{},
	)
	utun := darwinInterface{name: "utun3", typ: "tunnel", index: 22}
	tr.interfaceTagger = &interfaceTagger{
		byAddr: map[netip.Addr]darwinInterface{
			netip.MustParseAddr("10.8.0.2"): utun,
		},
		byName:  map[string]darwinInterface{"utun3": utun},
		byIndex: map[int]darwinInterface{22: utun},
	}

	conns, _, err := tr.GetActiveConnections("c1")
	require.NoError(t, err)
	require.Len(t, conns.Conns, 1)
	assert.True(t, hasInterfaceTag(conns.Conns[0].Tags, "keep-me"))
	assert.True(t, hasInterfaceTag(conns.Conns[0].Tags, "interface_name:utun3"))
	assert.True(t, hasInterfaceTag(conns.Conns[0].Tags, "interface_type:tunnel"))
	assert.True(t, hasInterfaceTag(conn.Tags, "keep-me"))
	assert.False(t, hasInterfaceTag(conn.Tags, "interface_name:utun3"))
	assert.Same(t, spare, conn.Tags[:2][1])
}

// TestDarwinTracer_StoreClosedConnection_TagsInterface checks that a closed row
// is tagged before it is stored.
func TestDarwinTracer_StoreClosedConnection_TagsInterface(t *testing.T) {
	state := &interfaceTagState{}
	tr := newTestTracer(&mockConnTracer{}, state)
	en0 := darwinInterface{name: "en0", typ: "ethernet_csmacd", index: 14}
	tr.interfaceTagger = &interfaceTagger{
		byAddr: map[netip.Addr]darwinInterface{
			netip.MustParseAddr("10.0.0.8"): en0,
		},
		byIndex: map[int]darwinInterface{14: en0},
	}
	conn := &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.0.0.8"),
			Dest:   util.AddressFromString("10.0.0.9"),
		},
	}

	tr.storeClosedConnection(conn)

	require.True(t, state.snapshotted)
	assert.True(t, hasInterfaceTag(state.closed.Tags, "interface_name:en0"))
	assert.True(t, hasInterfaceTag(state.closed.Tags, "interface_type:ethernet_csmacd"))
}

// TestDarwinTracer_GetActiveConnections_SkipsUnknownInterface checks that a
// wildcard row with no peer stays untagged.
func TestDarwinTracer_GetActiveConnections_SkipsUnknownInterface(t *testing.T) {
	conn := &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("0.0.0.0"),
			Dest:   util.AddressFromString("0.0.0.0"),
			SPort:  9,
			DPort:  9,
		},
		LastUpdateEpoch: uint64(time.Now().UnixNano()),
	}
	tr := newTestTracer(
		&mockConnTracer{connsToReturn: []*network.ConnectionStats{conn}},
		&interfaceTagState{},
	)
	tr.interfaceTagger = &interfaceTagger{}

	conns, _, err := tr.GetActiveConnections("c1")
	require.NoError(t, err)
	require.Len(t, conns.Conns, 1)
	assert.Empty(t, conns.Conns[0].Tags)
}

// TestAppendInterfaceTagsIsIdempotent checks that tagging the same connection twice does not duplicate tags.
func TestAppendInterfaceTagsIsIdempotent(t *testing.T) {
	conn := &network.ConnectionStats{}
	appendInterfaceTags(conn, "utun3", "tunnel")
	appendInterfaceTags(conn, "utun3", "tunnel")
	require.Len(t, conn.Tags, 2)
	assert.True(t, hasInterfaceTag(conn.Tags, "interface_name:utun3"))
	assert.True(t, hasInterfaceTag(conn.Tags, "interface_type:tunnel"))
}
