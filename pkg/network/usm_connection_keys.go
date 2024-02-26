// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
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
