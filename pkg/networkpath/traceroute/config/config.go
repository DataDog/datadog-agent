// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package config is the configuration for the traceroute functionality
package config

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// Config specifies the configuration of an instance
// of Traceroute, on the system-probe side
type Config struct {
	// TODO: add common configuration
	// Destination Hostname
	DestHostname string
	// Destination Port number
	DestPort uint16
	// Destination service name
	DestinationService string
	// Source service name
	SourceService string
	// Source container ID
	SourceContainerID string
	// Max number of hops to try
	MaxTTL uint8
	// Timeout for each hop
	Timeout time.Duration
	// Protocol is the protocol to use
	// for traceroute, default is UDP
	Protocol payload.Protocol
	// TCPMethod is the method used to run a TCP traceroute.
	TCPMethod payload.TCPMethod
	// TCPSynParisTracerouteMode makes TCP SYN act like paris traceroute (fixed packet ID, randomized seq)
	TCPSynParisTracerouteMode bool
}
