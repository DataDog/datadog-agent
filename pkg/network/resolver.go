// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"fmt"
	"net/netip"

	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network/slice"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LocalResolver resolves connection remote addresses
type LocalResolver struct {
	processEventsEnabled bool
}

// NewLocalResolver creates a new LocalResolver
func NewLocalResolver(processEventsEnabled bool) LocalResolver {
	return LocalResolver{
		processEventsEnabled: processEventsEnabled,
	}
}

// Resolve binds container IDs to the Raddr of connections
//
// An attempt is made to resolve as many local containers as possible.
//
// First, we go over all connections that have a container ID
// populated for their laddr, indexing them by (laddr, raddr, proto, netns).
// An additional entry is also inserted for a non-loopback laddr with netns = 0
// as well. Translated laddr and raddr are used throughout. Incoming
// connection dest ports (the ephemeral side) are also zero-ed before
// inserting into the index.
//
// Second, we go over the connections again, this time resolving
// the raddr container id using the lookup table we built previously.
// Translated addresses are used throughout. This time outgoing connection
// source ports are zero-ed when doing a lookup (since we zero-ed out
// the incoming dest ports in the index).
//
// Only connections that are local are resolved, i.e., for
// which `IntraHost` is set to true.
func (r LocalResolver) Resolve(conns slice.Chain[ConnectionStats]) {
	if !r.processEventsEnabled {
		return
	}

	type connKey struct {
		laddr, raddr netip.AddrPort
		proto        ConnectionType
		netns        uint32
	}

	ctrsByConn := make(map[connKey]*intern.Value, conns.Len()/2)
	conns.Iterate(func(_ int, conn *ConnectionStats) {
		if conn.ContainerID.Source == nil || len(conn.ContainerID.Source.Get().(string)) == 0 {
			return
		}

		source, dest := translatedAddrs(conn)
		if conn.Direction == INCOMING {
			dest = netip.AddrPortFrom(dest.Addr(), 0)
		}

		k := connKey{
			laddr: source,
			raddr: dest,
			proto: conn.Type,
			netns: conn.NetNS,
		}
		if conn.NetNS != 0 {
			ctrsByConn[k] = conn.ContainerID.Source
		}
		if !source.Addr().IsLoopback() {
			k.netns = 0
			ctrsByConn[k] = conn.ContainerID.Source
		}
	})

	log.TraceFunc(func() string { return fmt.Sprintf("ctrsByConn = %v", ctrsByConn) })

	if len(ctrsByConn) == 0 {
		return
	}

	// go over connections again using hashtable computed earlier to resolve dest
	conns.Iterate(func(_ int, conn *ConnectionStats) {
		if !conn.IntraHost {
			return
		}

		source, dest := translatedAddrs(conn)
		if conn.Direction == OUTGOING {
			source = netip.AddrPortFrom(source.Addr(), 0)
		}

		k := connKey{
			laddr: dest,
			raddr: source,
			proto: conn.Type,
			netns: conn.NetNS,
		}

		var cid *intern.Value
		if cid = ctrsByConn[k]; cid == nil {
			if !dest.Addr().IsLoopback() {
				k.netns = 0
				cid = ctrsByConn[k]
			}
		}

		if cid != nil {
			conn.ContainerID.Dest = cid
		}
	})
}

func translatedSource(conn *ConnectionStats) netip.AddrPort {
	if conn.IPTranslation != nil {
		return netip.AddrPortFrom(conn.IPTranslation.ReplDstIP.Addr, conn.IPTranslation.ReplDstPort)
	}

	return netip.AddrPortFrom(conn.Source.Addr, conn.SPort)
}

func translatedDest(conn *ConnectionStats) netip.AddrPort {
	if conn.IPTranslation != nil {
		return netip.AddrPortFrom(conn.IPTranslation.ReplSrcIP.Addr, conn.IPTranslation.ReplSrcPort)
	}

	return netip.AddrPortFrom(conn.Dest.Addr, conn.DPort)
}

func translatedAddrs(conn *ConnectionStats) (netip.AddrPort, netip.AddrPort) {
	return translatedSource(conn), translatedDest(conn)
}
