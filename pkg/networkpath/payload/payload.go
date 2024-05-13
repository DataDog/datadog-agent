// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package payload contains Network Path payload
package payload

import "github.com/DataDog/datadog-agent/pkg/network"

// NetworkPathHop encapsulates the data for a single
// hop within a path
type NetworkPathHop struct {
	TTL       int     `json:"ttl"`
	IPAddress string  `json:"ip_address"`
	Hostname  string  `json:"hostname"`
	RTT       float64 `json:"rtt"`
	Success   bool    `json:"success"`
}

// NetworkPathSource encapsulates information
// about the source of a path
type NetworkPathSource struct {
	Hostname  string       `json:"hostname"`
	Via       *network.Via `json:"via"`
	NetworkID string       `json:"network_id"` // Today this will be a VPC ID since we only resolve AWS resources
}

// NetworkPathDestination encapsulates information
// about the destination of a path
type NetworkPathDestination struct {
	Hostname  string `json:"hostname"`
	IPAddress string `json:"ip_address"`
	Port      uint16 `json:"port"`
}

// NetworkPath encapsulates data that defines a
// path between two hosts as mapped by the agent
type NetworkPath struct {
	Timestamp   int64                  `json:"timestamp"`
	Namespace   string                 `json:"namespace"` // namespace used to resolve NDM resources
	PathID      string                 `json:"path_id"`
	Source      NetworkPathSource      `json:"source"`
	Destination NetworkPathDestination `json:"destination"`
	Hops        []NetworkPathHop       `json:"hops"`
	Tags        []string               `json:"tags"`
}
