// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
)

// MCPConfig holds the MCP server configuration
type MCPConfig struct {
	// Enabled indicates whether the MCP server is enabled
	Enabled bool

	// SocketPath is the Unix domain socket path for the MCP server
	SocketPath string

	// BufferSize is the size of the read buffer
	BufferSize int

	// LogLevel specific to MCP server operations
	LogLevel string
}

type dependencies struct {
	fx.In
	Config coreconfig.Component
}

type mcpConfig struct {
	config *MCPConfig
}

// newConfig creates a new MCP configuration from the agent's config
func newConfig(deps dependencies) Component {
	cfg := &MCPConfig{
		Enabled:    deps.Config.GetBool("mcp_server.enabled"),
		SocketPath: deps.Config.GetString("mcp_server.socket_path"),
		BufferSize: deps.Config.GetInt("mcp_server.buffer_size"),
		LogLevel:   deps.Config.GetString("mcp_server.log_level"),
	}

	// Set defaults if not configured
	if cfg.SocketPath == "" {
		cfg.SocketPath = "/tmp/datadog-agent-mcp.sock"
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 4096
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	return &mcpConfig{
		config: cfg,
	}
}

// Get returns the MCP server configuration
func (c *mcpConfig) Get() *MCPConfig {
	return c.config
}
