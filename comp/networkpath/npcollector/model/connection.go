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

// NetworkPathConnection is the minimum information needed about a connection to schedule a network path test
type NetworkPathConnection struct {
	Source         netip.AddrPort
	Dest           netip.AddrPort
	TranslatedDest netip.AddrPort
	// SourceHostname is the Agent's own Datadog hostname. It is one of the four
	// inputs to the correlation key and is not derivable from the connection, so
	// callers must supply it. Empty when the Agent could not resolve it.
	SourceHostname    string
	SourceContainerID string
	Namespace         string
	Type              model.ConnectionType
	Direction         model.ConnectionDirection
	Family            model.ConnectionFamily
	Domain            string
	IntraHost         bool
	SystemProbeConn   bool

	// SentBytes and RecvBytes are traffic observed since the previous CNM
	// snapshot, not lifetime counters.
	SentBytes uint64
	RecvBytes uint64
}

// NetworkPath is the Agent's Network Path decision for one connection, returned
// so the caller can stamp it onto the CNM payload.
//
// HasTest true means policy approved a dynamic test for this connection. It does
// not mean a traceroute ran, succeeded, or was ingested. CorrelationKey names the
// test that decision scheduled; it is empty when the Agent could not resolve its
// own hostname, which is the one key input the connection does not carry.
type NetworkPath struct {
	HasTest        bool
	CorrelationKey string
}
