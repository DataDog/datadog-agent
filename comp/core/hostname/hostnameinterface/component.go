// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostnameinterface describes the interface for hostname methods
package hostnameinterface

import (
	"context"
)

// team: agent-runtimes

// Data contains hostname and the hostname provider
type Data struct {
	Hostname string
	Provider string
}

// Component is the type for hostname methods.
type Component interface {
	// Get returns the host name for the agent.
	Get(context.Context) (string, error)
	// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it.
	GetWithProvider(ctx context.Context) (Data, error)
	// GetSafe is Get(), but it returns 'unknown host' if anything goes wrong.
	GetSafe(context.Context) string
}
