// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumer implements a component that consumes config streams from the core agent.
//
// team: agent-configuration
package configstreamconsumer

import (
	"context"
	"time"
)

// SessionIDProvider supplies the RAR session ID, typically after registration completes.
// When set, the consumer will call WaitSessionID at connect time instead of using Params.SessionID.
type SessionIDProvider interface {
	WaitSessionID(ctx context.Context) (string, error)
}

// Params defines the parameters for the configstreamconsumer component
type Params struct {
	// ClientName is the identity of this remote agent (e.g., "system-probe", "trace-agent")
	ClientName string
	// CoreAgentAddress is the address of the core agent IPC endpoint
	CoreAgentAddress string
	// SessionID is the RAR session ID for authorization. Required if SessionIDProvider is nil.
	SessionID string
	// SessionIDProvider supplies the session ID at connect time (e.g. from remote agent component).
	// When set, SessionID may be empty; the consumer will block on WaitSessionID before connecting.
	SessionIDProvider SessionIDProvider
	// ReadyTimeout is how long OnStart blocks waiting for the first config snapshot before
	// returning an error and aborting startup. Defaults to 60s when zero.
	ReadyTimeout time.Duration
}

// Component is the config stream consumer component interface.
// Its sole purpose is to receive configuration from the core agent stream and write it
// into the local config.Component via the model.Writer provided at construction.
// Callers that need to read config or subscribe to changes should use config.Component directly.
// Readiness is guaranteed by the FX lifecycle: start blocks until the first snapshot is received.
type Component interface{}
