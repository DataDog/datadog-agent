// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)

package tracer

import (
	"github.com/DataDog/datadog-agent/pkg/network"
)

// shouldSkipConnection returns whether or not the tracer should ignore a given connection:
//   - Local DNS (*:53) requests if configured (default: true)
func (t *Tracer) shouldSkipConnection(conn *network.ConnectionStats) bool {
	isDNSConnection := conn.DPort == 53 || conn.SPort == 53
	if !t.config.CollectLocalDNS && isDNSConnection && conn.Dest.IsLoopback() {
		return true
	} else if network.IsExcludedConnection(t.sourceExcludes, t.destExcludes, conn) {
		return true
	}
	return false
}
