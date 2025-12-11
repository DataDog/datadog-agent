// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import "context"

// ToolRequest represents an incoming MCP tool request
type ToolRequest struct {
	ToolName   string                 `json:"tool_name"`
	Parameters map[string]interface{} `json:"parameters"`
	RequestID  string                 `json:"request_id"`
}

// ToolResponse represents the response to an MCP tool request
type ToolResponse struct {
	ToolName  string      `json:"tool_name"`
	Result    interface{} `json:"result"`
	Error     string      `json:"error,omitempty"`
	RequestID string      `json:"request_id"`
}

// Handler processes MCP tool requests
type Handler interface {
	Handle(ctx context.Context, req *ToolRequest) (*ToolResponse, error)
}
