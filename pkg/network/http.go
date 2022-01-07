// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/http"
)

// HTTPKeyFromConn build the key for the http map based on whether the local or remote side is http.
func HTTPKeyFromConn(c ConnectionStats) http.Key {
	// Retrieve translated addresses
	laddr, lport := GetNATLocalAddress(c)
	raddr, rport := GetNATRemoteAddress(c)

	// HTTP data is always indexed as (client, server), so we account for that when generating the
	// the lookup key using the port range heuristic.
	// In the rare cases where both ports are within the same range we ensure that sport < dport
	// to mimic the normalization heuristic done in the eBPF side (see `port_range.h`)
	if (IsEphemeralPort(int(lport)) && !IsEphemeralPort(int(rport))) ||
		(IsEphemeralPort(int(lport)) == IsEphemeralPort(int(rport)) && lport < rport) {
		return http.NewKey(laddr, raddr, lport, rport, "", http.MethodUnknown)
	}

	return http.NewKey(raddr, laddr, rport, lport, "", http.MethodUnknown)
}
