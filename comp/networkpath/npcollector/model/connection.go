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
func (c *NetworkPathConnection) SetBaselineSignals(timeoutCount, rtoCount, retransmits, rttVar, sentBytes, recvBytes uint64) {
	c.TCPTimeout = timeoutCount > 0
	c.TCPRTO = rtoCount > 0
	c.Retransmits = retransmits
	c.RTTVar = rttVar
	c.Bytes = sentBytes + recvBytes
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
	TCPTimeout        bool
	TCPRTO            bool
	Retransmits       uint64
	RTTVar            uint64
	Bytes             uint64
}
