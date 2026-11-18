// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package rcx509client defines the Agent's Remote Config x509 client component.
package rcx509client

import (
	"context"
	"net/http"
)

// team: remote-config

// State describes the lifecycle state of the x509 client component.
type State string

const (
	// StateDisabled means the x509 client is disabled by configuration.
	StateDisabled State = "disabled"
	// StateInitialized means the client was constructed but has not started.
	StateInitialized State = "initialized"
	// StateRunning means the client's connection-management loop is running.
	StateRunning State = "running"
	// StateStopping means the client is shutting down.
	StateStopping State = "stopping"
	// StateStopped means the client shut down cleanly.
	StateStopped State = "stopped"
	// StateFailed means construction or the connection-management loop failed.
	StateFailed State = "failed"
)

// Component owns the lifecycle of the Agent's Remote Config x509 client.
type Component interface {
	State() State
}

// Client is the lifecycle surface required from the public rcx509 client.
type Client interface {
	Start() error
	Close() error
}

// ExecuteHandler handles the Remote Queries Execute URI. It is registered only
// when the Agent has a handler that carries its own trusted authorization; the
// component adds none and treats the debug ping as no authorization at all.
type ExecuteHandler func(ctx context.Context, correlationID uint64, payload []byte) ([]byte, error)

// ClientConfig contains the Agent-owned settings used to construct an x509
// client. APIKey must never be logged.
type ClientConfig struct {
	URL              string
	AppName          string
	Version          string
	APIKey           string
	HTTPClient       *http.Client
	DebugPingEnabled bool
	// ExecuteHandler, if set, is registered for the Remote Queries Execute URI.
	// A nil handler leaves the URI unregistered.
	ExecuteHandler ExecuteHandler
}

// ClientFactory constructs an x509 client.
type ClientFactory func(ClientConfig) (Client, error)
