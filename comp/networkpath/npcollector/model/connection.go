// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model is the data types for usage in the npcollector component interface
package model

import (
	"net/netip"

	model "github.com/DataDog/agent-payload/v5/process"
)

// TCPTimeoutErrno is the normalized ETIMEDOUT value emitted in CNM connection
// payloads by both Linux and Windows producers.
const TCPTimeoutErrno uint16 = 110

// SetBaselineSignals normalizes the CNM deltas used by baseline selection.
// Both direct and process-agent producers use this method so their ranking
// inputs remain equivalent.
func (c *NetworkPathConnection) SetBaselineSignals(timeoutCount, rtoCount, retransmits, sentBytes, recvBytes uint64) {
	c.BaselineDiagnostic = timeoutCount > 0 || rtoCount > 0 || retransmits > 0
	c.BaselineBytes = sentBytes + recvBytes
}

// NetworkPathConnection is the minimum information needed about a connection to schedule a network path test
type NetworkPathConnection struct {
	Source            netip.AddrPort
	Dest              netip.AddrPort
	TranslatedDest    netip.AddrPort
	SourceContainerID string
	Namespace         string
	Type              model.ConnectionType
	Direction         model.ConnectionDirection
	Family            model.ConnectionFamily
	Domain            string
	IntraHost         bool
	SystemProbeConn   bool

	BaselineDiagnostic bool
	BaselineBytes      uint64
}
