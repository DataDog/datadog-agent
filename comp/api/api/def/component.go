// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package api implements the internal Agent API which exposes endpoints such as config, flare or status
package def

import (
	"net"
	"net/http"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	ServerAddress() *net.TCPAddr
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// EndpointProvider is an interface to register api endpoints
type EndpointProvider interface {
	HandlerFunc() http.HandlerFunc

	Methods() []string
	Route() string
}
