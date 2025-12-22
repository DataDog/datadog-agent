// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client implements a component that provides an MCP (Model Context Protocol) client.
// The client can connect to an MCP server and call tools to interact with it.
package client

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the MCP client component interface.
type Component interface {
	// Connect connects to an MCP server at the specified socket path
	Connect(ctx context.Context) error

	// CallTool calls a tool on the connected MCP server
	CallTool(ctx context.Context, toolName string, params map[string]interface{}) (*mcp.CallToolResult, error)

	// ListTools lists all available tools on the connected MCP server
	ListTools(ctx context.Context) ([]*mcp.Tool, error)

	// Close closes the connection to the MCP server
	Close() error

	// IsConnected returns whether the client is connected to the server
	IsConnected() bool
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMCPClient))
}
