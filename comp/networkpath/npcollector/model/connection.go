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
	Source            netip.AddrPort
	Dest              netip.AddrPort
	TranslatedDest    netip.AddrPort
	SourceContainerID string
	Type              model.ConnectionType
	Direction         model.ConnectionDirection
	Family            model.ConnectionFamily
	Domain            string
	IntraHost         bool
	SystemProbeConn   bool
}
