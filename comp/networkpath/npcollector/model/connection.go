// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model is the data types for usage in the npcollector component interface
package model

import (
	"net/netip"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// OriginType identifies how a path test was triggered.
type OriginType string

const (
	// OriginAgentTraffic is the default/zero-value: the path test was triggered
	// by traffic observed on the agent host (CNM / NPM behavior).
	OriginAgentTraffic OriginType = ""
	// OriginNetworkDevice means the path test was triggered by NetFlow records
	// exported by a network device (NDM behavior).
	OriginNetworkDevice OriginType = "network_device"
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
	// Origin identifies the source of this connection record.
	// Zero value (OriginAgentTraffic) preserves existing CNM behavior.
	Origin OriginType
	// Protocol, when set, overrides the protocol derived from Type.
	// Used by the NDM converter to express ICMP without extending the
	// vendored ConnectionType protobuf.
	Protocol payload.Protocol
	// DestIP is the originally-observed destination IP, preserved separately
	// from Dest so it survives the pipeline even when Hostname becomes a domain.
	DestIP netip.Addr
	// Namespaces is the set of NetFlow namespaces that observed this destination.
	// Empty for CNM-origin connections.
	Namespaces []string
	// ExporterAddrs is the set of NetFlow exporter device IPs that observed this destination.
	// Empty for CNM-origin connections.
	ExporterAddrs []netip.Addr
}
