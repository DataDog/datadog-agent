// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm) || darwin

package tracer

import (
	"net/netip"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	filter "github.com/DataDog/datadog-agent/pkg/network/tracer/networkfilter"
)

func convertToFilterable(conn *network.ConnectionStats) filter.FilterableConnection {
	return filter.FilterableConnection{
		Type:   marshal.FormatType(conn.Type),
		Source: netip.AddrPortFrom(conn.Source.Addr, conn.SPort),
		Dest:   netip.AddrPortFrom(conn.Dest.Addr, conn.DPort),
	}
}

// shouldSkipConnection returns whether or not the tracer should ignore a given connection:
//   - Local DNS (*:53) requests if configured (default: true)
func (t *Tracer) shouldSkipConnection(conn *network.ConnectionStats) bool {
	isDNSConnection := false
	for _, p := range t.config.DNSMonitoringPortList {
		if conn.DPort == uint16(p) || conn.SPort == uint16(p) {
			isDNSConnection = true
			break
		}
	}

	if !t.config.CollectLocalDNS && isDNSConnection && conn.Dest.IsLoopback() {
		return true
	} else if filter.IsExcludedConnection(t.sourceExcludes, t.destExcludes, convertToFilterable(conn)) {
		return true
	}
	return false
}
