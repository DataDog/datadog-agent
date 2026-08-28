// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	processutil "github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestDarwinTupleIndexMatchesOrientationAndRejectsAmbiguity(t *testing.T) {
	index := newDarwinTupleIndex()
	conn := testDarwinIndexedConnection(1, "192.0.2.10", 50000, "198.51.100.20", 443)
	index.add(conn)

	match := index.match(conn.ConnectionTuple)
	require.True(t, match.matched)
	require.True(t, match.orientationKnown)
	require.False(t, match.packetReversed)
	require.Equal(t, conn.Cookie, match.cookie)

	match = index.match(reverseDarwinTuple(conn.ConnectionTuple))
	require.True(t, match.matched)
	require.True(t, match.orientationKnown)
	require.True(t, match.packetReversed)

	second := testDarwinIndexedConnection(2, "192.0.2.10", 50000, "198.51.100.20", 443)
	index.add(second)
	match = index.match(conn.ConnectionTuple)
	require.False(t, match.matched)
	require.True(t, match.ambiguous)

	index.remove(second.Cookie)
	match = index.match(conn.ConnectionTuple)
	require.True(t, match.matched)
	require.Equal(t, conn.Cookie, match.cookie)
}

func TestDarwinTupleIndexNormalizesMappedAddresses(t *testing.T) {
	index := newDarwinTupleIndex()
	conn := testDarwinIndexedConnection(3, "192.0.2.10", 12345, "198.51.100.20", 80)
	index.add(conn)

	packet := conn.ConnectionTuple
	packet.Source.Addr = netip.MustParseAddr("::ffff:192.0.2.10")
	packet.Dest.Addr = netip.MustParseAddr("::ffff:198.51.100.20")

	match := index.match(packet)
	require.True(t, match.matched)
	require.Equal(t, conn.Cookie, match.cookie)
}

func TestDarwinTupleIndexMatchesExactOrientation(t *testing.T) {
	index := newDarwinTupleIndex()
	client := testDarwinIndexedConnection(4, "127.0.0.1", 50000, "127.0.0.1", 8080)
	server := testDarwinIndexedConnection(5, "127.0.0.1", 8080, "127.0.0.1", 50000)
	index.add(client)
	index.add(server)

	// Packet matching intentionally rejects a dual socket view as ambiguous.
	require.True(t, index.match(client.ConnectionTuple).ambiguous)

	match := index.matchExact(client.ConnectionTuple)
	require.True(t, match.matched)
	require.False(t, match.ambiguous)
	require.Equal(t, client.Cookie, match.cookie)

	match = index.matchExact(server.ConnectionTuple)
	require.True(t, match.matched)
	require.Equal(t, server.Cookie, match.cookie)

	duplicate := testDarwinIndexedConnection(6, "127.0.0.1", 8080, "127.0.0.1", 50000)
	index.add(duplicate)
	match = index.matchExact(server.ConnectionTuple)
	require.False(t, match.matched)
	require.True(t, match.ambiguous)

	index.remove(duplicate.Cookie)
	index.remove(server.Cookie)
	require.False(t, index.matchExact(server.ConnectionTuple).matched)
}

func testDarwinIndexedConnection(cookie uint64, source string, sport uint16, dest string, dport uint16) *network.ConnectionStats {
	return &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: processutil.Address{Addr: netip.MustParseAddr(source)},
			Dest:   processutil.Address{Addr: netip.MustParseAddr(dest)},
			SPort:  sport,
			DPort:  dport,
			Type:   network.TCP,
			Family: network.AFINET,
		},
		Cookie: cookie,
	}
}
