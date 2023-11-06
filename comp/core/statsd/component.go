// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statsd implements a component to get a statsd client.
package statsd

import (
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// Get a pre-configured statsd client
	Get(options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error)

	// GetForAddr returns a pre-configured statsd client that defaults to `addr` if no env var is set
	GetForAddr(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error)

	// GetForHostPort returns a pre-configured statsd client that defaults to `host:port` if no env var is set
	GetForHostPort(host string, port int, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error)
}
