// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceroute adds traceroute functionality to the agent
package traceroute

type (
	// Config specifies the configuration of an instance
	// of Traceroute
	Config struct {
		// TODO: add common configuration
		DestHostname string
		DestPort     uint16
		MaxTTL       uint8
		TimeoutMs    uint
	}

	// Traceroute defines an interface for running
	// traceroutes for the Network Path integration
	Traceroute interface {
		Run() (NetworkPath, error)
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
		Hostname string `json:"hostname"`
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
		PathID      string                 `json:"path_id"`
		Source      NetworkPathSource      `json:"source"`
		Destination NetworkPathDestination `json:"destination"`
		Hops        []NetworkPathHop       `json:"hops"`
		Tags        []string               `json:"tags"`
	}
)
