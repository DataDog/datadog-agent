// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// this file is here because Windows has its own ConnectionKeysFromConnectionStats.
// however, putting this in `_linux.go` broke the mac build ?

// ConnectionKeysFromConnectionStats constructs connection key using the underlying raw connection stats object, which is produced by the tracer.
// Each ConnectionStats object contains both the source and destination addresses, as well as an IPTranslation object that stores the original addresses in the event that the connection is NAT'd.
// This function generates all relevant combinations of connection keys: [(source, dest), (dest, source), (NAT'd source, NAT'd dest), (NAT'd dest, NAT'd source)].
// This is necessary to handle all possible scenarios for connections originating from the USM module (i.e., whether they are NAT'd or not, and whether they use TLS).
func ConnectionKeysFromConnectionStats(connectionStats ConnectionStats) []types.ConnectionKey {
	hasTranslation := connectionStats.IPTranslation != nil
	connectionKeysCount := 2
	if hasTranslation {
		connectionKeysCount = 4
	}
	connectionKeys := make([]types.ConnectionKey, connectionKeysCount)
	// USM data is always indexed as (client, server), but we don't know which is the remote
	// and which is the local address. To account for this, we'll construct 2 possible
	// connection keys and check for both of them in the aggregations map.
	connectionKeys[0] = types.NewConnectionKey(connectionStats.Source, connectionStats.Dest, connectionStats.SPort, connectionStats.DPort)
	connectionKeys[1] = types.NewConnectionKey(connectionStats.Dest, connectionStats.Source, connectionStats.DPort, connectionStats.SPort)

	// if IPTranslation is not nil, at least one of the sides has a translation, thus we need to add translated addresses.
	if hasTranslation {
		localAddress, localPort := GetNATLocalAddress(connectionStats)
		remoteAddress, remotePort := GetNATRemoteAddress(connectionStats)
		connectionKeys[2] = types.NewConnectionKey(localAddress, remoteAddress, localPort, remotePort)
		connectionKeys[3] = types.NewConnectionKey(remoteAddress, localAddress, remotePort, localPort)
	}

	return connectionKeys
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

	shouldTraceLog := connectionStats.DPort == 7777 || connectionStats.SPort == 7777 || connectionStats.DPort == 7778 || connectionStats.SPort == 7778

	clientIP, clientPort = connectionStats.Source, connectionStats.SPort
	serverIP, serverPort = connectionStats.Dest, connectionStats.DPort

	hasNAT := connectionStats.IPTranslation != nil
	if hasNAT {
		clientIPNAT, clientPortNAT = GetNATLocalAddress(connectionStats)
		serverIPNAT, serverPortNAT = GetNATRemoteAddress(connectionStats)
	}

	if shouldTraceLog {
		log.Tracef("WithKey - clientIP: %s, clientPort: %d, serverIP: %s, serverPort: %d", clientIP, clientPort, serverIP, serverPort)
		log.Tracef("WithKey - hasNAT: %t, clientIPNAT: %s, clientPortNAT: %d, serverIPNAT: %s, serverPortNAT: %d", hasNAT, clientIPNAT, clientPortNAT, serverIPNAT, serverPortNAT)
	}

	// USM data is generally indexed as (client, server), so we do a
	// *best-effort* to determine the key tuple most likely to be the one
	// correct and minimize the numer of `f` calls
	if IsPortInEphemeralRange(connectionStats.Family, connectionStats.Type, clientPort) != EphemeralTrue {
		if shouldTraceLog {
			log.Tracef("WithKey - clientPort is not ephemeral, flipping IPs and ports")
		}
		// Flip IPs and ports
		clientIP, clientPort, serverIP, serverPort = serverIP, serverPort, clientIP, clientPort
		clientIPNAT, clientPortNAT, serverIPNAT, serverPortNAT = serverIPNAT, serverPortNAT, clientIPNAT, clientPortNAT
	}

	// Callback 1: NATed (client, server)
	if hasNAT && f(types.NewConnectionKey(clientIPNAT, serverIPNAT, clientPortNAT, serverPortNAT)) {
		return
	}

	//if hasNAT && f(types.NewConnectionKey(clientIP, serverIP, clientPort, serverPort)) {
	//	if shouldTraceLog {
	//		log.Tracef("WithKey - callback 1: NATed (client, server) returned true")
	//	}
	//	return
	//}

	// Callback 2: (client, server)
	if f(types.NewConnectionKey(clientIP, serverIP, clientPort, serverPort)) {
		if shouldTraceLog {
			log.Tracef("WithKey - callback 2: (client, server) returned true")
		}
		return
	}

	// Callback 3: NATed (server, client)
	if hasNAT && f(types.NewConnectionKey(serverIPNAT, clientIPNAT, serverPortNAT, clientPortNAT)) {
		if shouldTraceLog {
			log.Tracef("WithKey - callback 3: NATed (server, client) returned true")
		}
		return
	}

	if shouldTraceLog {
		log.Tracef("WithKey - callback 4: (server, client)")
	}
	// Callback 4: (server, client)
	f(types.NewConnectionKey(serverIP, clientIP, serverPort, clientPort))
}
