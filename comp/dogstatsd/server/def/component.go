// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd server
package server

import (
	"time"
)

// team: agent-metric-pipelines

// Component is the component type.
type Component interface {
	// IsRunning returns true if the server is running
	IsRunning() bool

	// ServerlessFlush flushes all the data to the aggregator to them send it to the Datadog intake.
	ServerlessFlush(time.Duration)

	// SetExtraTags sets extra tags. All metrics sent to the DogstatsD will be tagged with them.
	SetExtraTags(tags []string)

	// UDPLocalAddr returns the local address of the UDP statsd listener, if enabled.
	UDPLocalAddr() string

	// SetBlocklist sets the blocklist to apply when parsing metrics from the DogStatsD listener.
	SetBlocklist([]string, bool)
}
