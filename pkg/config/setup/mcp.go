// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// MCP configuration paths.
const (
	MCPSection        = "mcp"
	MCPEnabled        = MCPSection + ".enabled"
	MCPServerSection  = MCPSection + ".server"
	MCPServerAddress  = MCPServerSection + ".address"
	MCPToolsSection   = MCPSection + ".tools"
	MCPProcessSection = MCPToolsSection + ".process"
	MCPLogsSection    = MCPToolsSection + ".logs"
)

// mcp configures the Model Context Protocol (MCP) server.
// MCP enables AI assistants to interact with agent telemetry through standardized tools.
func mcp(config pkgconfigmodel.Setup) {
	// MCP Server - Main toggle
	config.BindEnvAndSetDefault("mcp.enabled", false)

	// MCP Server - Network configuration
	config.BindEnvAndSetDefault("mcp.server.address", "unix:///var/run/datadog/mcp.sock")
	config.BindEnvAndSetDefault("mcp.server.max_request_size", 10485760) // 10MB
	config.BindEnvAndSetDefault("mcp.server.max_connections", 100)
	config.BindEnvAndSetDefault("mcp.server.request_timeout", "30s")

	// MCP Server - TLS configuration
	config.BindEnvAndSetDefault("mcp.server.tls.enabled", false)
	config.BindEnvAndSetDefault("mcp.server.tls.cert_file", "")
	config.BindEnvAndSetDefault("mcp.server.tls.key_file", "")
	config.BindEnvAndSetDefault("mcp.server.tls.ca_file", "")

	// MCP Tools - Process monitoring tool
	config.BindEnvAndSetDefault("mcp.tools.process.enabled", true)
	config.BindEnvAndSetDefault("mcp.tools.process.scrub_args", true)
	config.BindEnvAndSetDefault("mcp.tools.process.max_processes_per_request", 1000)
	config.BindEnvAndSetDefault("mcp.tools.process.include_container_metadata", true)

	// MCP Tools - Logs monitoring tool
	config.BindEnvAndSetDefault("mcp.tools.logs.enabled", true)
	config.BindEnvAndSetDefault("mcp.tools.logs.max_logs_per_request", 500)
	config.BindEnvAndSetDefault("mcp.tools.logs.max_duration", 60) // seconds
}
