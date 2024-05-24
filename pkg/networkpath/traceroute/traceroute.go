// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceroute adds traceroute functionality to the agent
package traceroute

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
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
		Run(context.Context) (payload.NetworkPath, error)
	}
)
