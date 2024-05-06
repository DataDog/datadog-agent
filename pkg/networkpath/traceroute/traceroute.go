// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceroute adds traceroute functionality to the agent
package traceroute

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/network"
)

type (
	// Config specifies the configuration of an instance
	// of Traceroute
	Config struct {
		// TODO: add common configuration
		// Destination Hostname
		DestHostname string
		// Destination Port number
		DestPort uint16
		// Max number of hops to try
		MaxTTL uint8
		// TODO: do we want to expose this?
		TimeoutMs uint
	}

	// Traceroute defines an interface for running
	// traceroutes for the Network Path integration
	Traceroute interface {
		Run(context.Context) (NetworkPath, error)
	}

	// NetworkPathHop encapsulates the data for a single
	// hop within a path
	NetworkPathHop struct {
		TTL       int     `json:"ttl"`
		IPAddress string  `json:"ip_address"`
		Hostname  string  `json:"hostname"`
		RTT       float64 `json:"rtt"`
		Success   bool    `json:"success"`
	}

	// NetworkPathSource encapsulates information
	// about the source of a path
	NetworkPathSource struct {
		Hostname  string       `json:"hostname"`
		Via       *network.Via `json:"via"`
		NetworkID string       `json:"network_id"` // Today this will be a VPC ID since we only resolve AWS resources
	}

	// NetworkPathDestination encapsulates information
	// about the destination of a path
	NetworkPathDestination struct {
		Hostname  string `json:"hostname"`
		IPAddress string `json:"ip_address"`
	}

	// NetworkPath encapsulates data that defines a
	// path between two hosts as mapped by the agent
	NetworkPath struct {
		Timestamp   int64                  `json:"timestamp"`
		Namespace   string                 `json:"namespace"` // namespace used to resolve NDM resources
		PathID      string                 `json:"path_id"`
		Source      NetworkPathSource      `json:"source"`
		Destination NetworkPathDestination `json:"destination"`
		Hops        []NetworkPathHop       `json:"hops"`
		Tags        []string               `json:"tags"`
	}
)
