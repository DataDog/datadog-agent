// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package mcp implements the MCP Server for the Cluster Agent.
package mcp

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/mcp/tools"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateMCPHandler creates and returns an HTTP handler for MCP endpoints
func CreateMCPHandler() http.Handler {
	// Create an MCP server.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "cluster-agent-mcp-server",
		Version: "0.0.1",
	}, nil)

	// Register MCP tools.
	registerMCPTools(server)

	// Create the streamable HTTP handler.
	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, nil)

	log.Infof("MCP server handler created")
	return handler
}

// registerMCPTools registers all MCP tools to the given server.
func registerMCPTools(server *mcp.Server) {
	// The get_leader tool provides information about the current Cluster Agent leader.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_leader",
		Description: "Get information about which Cluster Agent is currently the leader",
	}, tools.GetLeader)
}
