// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceroute adds traceroute functionality to the agent
package traceroute

type (
	Config struct {
		// TODO: add common configuration
		DestHostname string
	}

	Traceroute interface {
		Run() (NetworkPath, error)
	}

	NetworkPathHop struct {
		TTL       int     `json:"ttl"`
		IpAddress string  `json:"ip_address"`
		Hostname  string  `json:"hostname"`
		RTT       float64 `json:"rtt"`
		Success   bool    `json:"success"`
	}

	NetworkPathSource struct {
		Hostname string `json:"hostname"`
	}

	NetworkPathDestination struct {
		Hostname  string `json:"hostname"`
		IpAddress string `json:"ip_address"`
	}

	NetworkPath struct {
		Timestamp   int64                  `json:"timestamp"`
		PathId      string                 `json:"path_id"`
		Source      NetworkPathSource      `json:"source"`
		Destination NetworkPathDestination `json:"destination"`
		Hops        []NetworkPathHop       `json:"hops"`
	}
)
