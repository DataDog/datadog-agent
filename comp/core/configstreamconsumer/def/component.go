// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumer implements a component that consumes config streams from the core agent.
//
// team: agent-configuration
package configstreamconsumer

import "context"

// Component is the config stream consumer component interface.
// Its sole purpose is to receive configuration from the core agent stream and write it
// into the local config.Component via the model.Writer provided at construction.
// Callers that need to read config or subscribe to changes should use config.Component directly.
type Component interface {
	// WaitReady blocks until the first config snapshot has been received and applied.
	// This ensures the consumer has a consistent config view before proceeding.
	WaitReady(ctx context.Context) error
}
