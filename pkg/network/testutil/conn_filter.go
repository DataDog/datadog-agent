// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"net"
	"net/netip"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// ConnectionFilterFunc is a function type which returns whether the provided connection matches the filter
type ConnectionFilterFunc func(c network.ConnectionStats) bool

// ByTuple matches connections when both source and destination address and port match
func ByTuple(l, r net.Addr) ConnectionFilterFunc {
	return func(c network.ConnectionStats) bool {
		return addrMatches(l, c.Source, c.SPort) && addrMatches(r, c.Dest, c.DPort)
	}
}

// BySourceAddress matches connections with the same source address and port
func BySourceAddress(a net.Addr) ConnectionFilterFunc {
	return func(c network.ConnectionStats) bool {
		return addrMatches(a, c.Source, c.SPort)
	}
}

// ByDestAddress matches connections with the same destination address and port
func ByDestAddress(a net.Addr) ConnectionFilterFunc {
	return func(c network.ConnectionStats) bool {
		return addrMatches(a, c.Dest, c.DPort)
	}
}

// ByType matches connections with the same connection type (TCP/UDP)
func ByType(ct network.ConnectionType) ConnectionFilterFunc {
	return func(c network.ConnectionStats) bool {
		return c.Type == ct
	}
}

// ByFamily matches connections with the same family (IPv4 / IPv6)
func ByFamily(f network.ConnectionFamily) ConnectionFilterFunc {
	return func(c network.ConnectionStats) bool {
		return c.Family == f
	}
}

// FirstConnection returns the first connection with matches all filters
func FirstConnection(c *network.Connections, filters ...ConnectionFilterFunc) *network.ConnectionStats {
	if result := FilterConnections(c, filters...); len(result) > 0 {
		return &result[0]
	}
	return nil
}

// FilterConnections returns connections which match all filters
func FilterConnections(c *network.Connections, filters ...ConnectionFilterFunc) []network.ConnectionStats {
	var results []network.ConnectionStats
ConnLoop:
	for _, conn := range c.Conns {
		for _, f := range filters {
			if !f(conn) {
				continue ConnLoop
			}
		}
		results = append(results, conn)
	}
	return results
}

func addrMatches(addr net.Addr, host util.Address, port uint16) bool {
	a := netip.MustParseAddrPort(addr.String())
	b := netip.AddrPortFrom(host.Addr, port)
	return a == b
}
