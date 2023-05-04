// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

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
func ConnectionKeysFromConnectionStats(c ConnectionStats) []types.ConnectionKey {

	// Retrieve translated addresses
	laddr, lport := GetNATLocalAddress(c)
	raddr, rport := GetNATRemoteAddress(c)

	// HTTP data is always indexed as (client, server), but we don't know which is the remote
	// and which is the local address. To account for this, we'll construct 2 possible
	// http keys and check for both of them in our http aggregations map.
	return []types.ConnectionKey{
		types.NewConnectionKey(laddr, raddr, lport, rport),
		types.NewConnectionKey(raddr, laddr, rport, lport),
	}
}
