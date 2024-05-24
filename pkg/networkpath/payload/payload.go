// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package payload contains Network Path payload
package payload

import "github.com/DataDog/datadog-agent/pkg/network"

// Protocol defines supported network protocols
type Protocol string

const (
	// ProtocolTCP is the TCP protocol.
	ProtocolTCP Protocol = "TCP"
	// ProtocolUDP is the UDP protocol.
	ProtocolUDP Protocol = "UDP"
)

// NetworkPathHop encapsulates the data for a single
// hop within a path
type NetworkPathHop struct {
	TTL       int     `json:"ttl"`
	IPAddress string  `json:"ip_address"`
	Hostname  string  `json:"hostname,omitempty"`
	RTT       float64 `json:"rtt,omitempty"`
	Success   bool    `json:"success"`
}

// NetworkPathSource encapsulates information
// about the source of a path
type NetworkPathSource struct {
	Hostname  string       `json:"hostname"`
	Via       *network.Via `json:"via,omitempty"`
	NetworkID string       `json:"network_id,omitempty"` // Today this will be a VPC ID since we only resolve AWS resources
	Service   string       `json:"service,omitempty"`
}

// NetworkPathDestination encapsulates information
// about the destination of a path
type NetworkPathDestination struct {
	Hostname  string `json:"hostname"`
	IPAddress string `json:"ip_address"`
	Port      uint16 `json:"port"`
	Service   string `json:"service,omitempty"`
}

// NetworkPath encapsulates data that defines a
// path between two hosts as mapped by the agent
type NetworkPath struct {
	Timestamp   int64                  `json:"timestamp"`
	Namespace   string                 `json:"namespace"` // namespace used to resolve NDM resources
	PathID      string                 `json:"path_id"`
	Protocol    Protocol               `json:"protocol"`
	Source      NetworkPathSource      `json:"source"`
	Destination NetworkPathDestination `json:"destination"`
	Hops        []NetworkPathHop       `json:"hops"`
	Tags        []string               `json:"tags,omitempty"`
}
