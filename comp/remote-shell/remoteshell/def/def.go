// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package def

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Component is the component type.
type Component interface {
	// Start starts the remote shell component
	Start()
	// Stop stops the remote shell component
	Stop()
	// GetActiveSessions returns a map of active shell sessions
	GetActiveSessions() map[string]Session
}

// Session represents an active remote shell session
type Session interface {
	// GetID returns the session ID
	GetID() string
	// GetPodName returns the pod name
	GetPodName() string
	// GetNamespace returns the namespace
	GetNamespace() string
	// GetContainer returns the container name
	GetContainer() string
	// GetWebsocketURI returns the websocket URI
	GetWebsocketURI() string
	// GetExpiresAt returns the expiration timestamp
	GetExpiresAt() int64
	// GetMetadata returns the session metadata
	GetMetadata() map[string]string
	// Write writes data to the shell session
	Write(data []byte) error
	// Read reads data from the shell session
	Read() ([]byte, error)
	// Close closes the shell session
	Close() error
}

// Params contains the parameters needed to create the remote shell component
type Params struct {
	// Config is the remote shell configuration
	Config state.RemoteShellConfig
	// Context is the context for the component
	Context context.Context
	// Cancel is the cancel function for the context
	Cancel context.CancelFunc
}
