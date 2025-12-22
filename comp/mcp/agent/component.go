// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements an AI agent that uses MCP to diagnose and fix issues.
// The agent runs an agentic loop: analyze problem -> plan actions -> execute via MCP -> observe results -> repeat
package agent

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Issue represents a problem that the AI agent should try to fix
type Issue struct {
	// Description is a human-readable description of the issue
	Description string

	// Severity indicates how critical the issue is
	Severity string

	// Metadata contains additional context about the issue
	Metadata map[string]interface{}
}

// AgentResult represents the outcome of the agent's attempt to fix an issue
type AgentResult struct {
	// Success indicates whether the issue was resolved
	Success bool

	// Steps contains a log of all steps the agent took
	Steps []AgentStep

	// FinalState describes the final state after the agent finished
	FinalState string

	// Error contains any error that occurred
	Error error
}

// AgentStep represents a single step in the agent's reasoning process
type AgentStep struct {
	// Type is the type of step (think, act, observe)
	Type string

	// Content is the content of the step
	Content string

	// ToolCall is the MCP tool call made (if Type is "act")
	ToolCall *ToolCall

	// ToolResult is the result from the MCP tool (if Type is "observe")
	ToolResult interface{}
}

// ToolCall represents a call to an MCP tool
type ToolCall struct {
	// Name is the name of the tool
	Name string

	// Parameters are the parameters to pass to the tool
	Parameters map[string]interface{}
}

// Component is the AI agent component interface.
type Component interface {
	// Solve attempts to solve the given issue using the agentic loop
	Solve(ctx context.Context, issue Issue) (*AgentResult, error)
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAIAgent))
}
