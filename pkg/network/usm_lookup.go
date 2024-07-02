// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// USMLookup determines the strategy for associating a given connection to USM
// In the context of Linux we may perform up to 4 lookups as described below
func USMLookup[K comparable, V any](c ConnectionStats, data map[types.ConnectionKey]*USMConnectionData[K, V]) *USMConnectionData[K, V] {
	var connectionData *USMConnectionData[K, V]

	// WithKey will attempt 4 lookups in total
	// 1) (A, B)
	// 2) (B, A)
	// 3) (translated(A), translated(B))
	// 3) (translated(B), translated(A))
	// The callback API is used to avoid allocating a slice of all pre-computed keys
	WithKey(c, func(key types.ConnectionKey) (stopIteration bool) {
		val, ok := data[key]
		if !ok {
			return false
		}

		connectionData = val
		return true
	})

	return connectionData
}

// WithKey calls `f` *up to* 4 times (or until the callback returns a `true`)
// with all possible connection keys. The generated keys are:
// 1) (src, dst)
// 2) (dst, src)
// 3) (src, dst) NAT
// 4) (dst, src) NAT
// In addition to that, we do a best-effort to call `f` in the order that most
// likely to succeed early (see comment below)
func WithKey(connectionStats ConnectionStats, f func(key types.ConnectionKey) (stop bool)) {
	var (
		clientIP, serverIP, clientIPNAT, serverIPNAT         util.Address
		clientPort, serverPort, clientPortNAT, serverPortNAT uint16
	)

	clientIP, clientPort = connectionStats.Source, connectionStats.SPort
	serverIP, serverPort = connectionStats.Dest, connectionStats.DPort

	hasNAT := connectionStats.IPTranslation != nil
	if hasNAT {
		clientIPNAT, clientPortNAT = GetNATLocalAddress(connectionStats)
		serverIPNAT, serverPortNAT = GetNATRemoteAddress(connectionStats)
	}

	// USM data is generally indexed as (client, server), so we do a
	// *best-effort* to determine the key tuple most likely to be the one
	// correct and minimize the numer of `f` calls
	if IsPortInEphemeralRange(connectionStats.Family, connectionStats.Type, clientPort) != EphemeralTrue {
		// Flip IPs and ports
		clientIP, clientPort, serverIP, serverPort = serverIP, serverPort, clientIP, clientPort
		clientIPNAT, clientPortNAT, serverIPNAT, serverPortNAT = serverIPNAT, serverPortNAT, clientIPNAT, clientPortNAT
	}

	// Callback 1: NATed (client, server)
	if hasNAT && f(types.NewConnectionKey(clientIPNAT, serverIPNAT, clientPortNAT, serverPortNAT)) {
		return
	}

	// Callback 2: (client, server)
	if f(types.NewConnectionKey(clientIP, serverIP, clientPort, serverPort)) {
		return
	}

	// Callback 3: NATed (server, client)
	if hasNAT && f(types.NewConnectionKey(serverIPNAT, clientIPNAT, serverPortNAT, clientPortNAT)) {
		return
	}

	// Callback 4: (server, client)
	f(types.NewConnectionKey(serverIP, clientIP, serverPort, clientPort))
}
