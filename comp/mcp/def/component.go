// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package mcp provides the Model Context Protocol (MCP) server component for the Datadog Agent.
// MCP enables AI assistants to interact with the Agent's monitoring capabilities through
// a standardized protocol.
package mcp

// team: agent-runtimes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/mcp/server"
)

// Component is the component type for the MCP server.
type Component interface {
	// IsEnabled returns whether the MCP server is enabled.
	IsEnabled() bool

	// ListTools returns all registered tool names.
	ListTools() []string

	// HandleToolCall processes an incoming MCP tool request.
	HandleToolCall(ctx context.Context, req *server.ToolRequest) (*server.ToolResponse, error)
}

// Params holds the configuration parameters for the MCP component.
type Params struct {
	// Enabled determines if the MCP server should be started.
	Enabled bool
}

// NewParams creates default Params for the MCP component.
func NewParams() Params {
	return Params{
		Enabled: true,
	}
}

// NewDisabledParams creates Params with the MCP server disabled.
func NewDisabledParams() Params {
	return Params{
		Enabled: false,
	}
}
