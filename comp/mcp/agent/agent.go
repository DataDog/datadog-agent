// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/fx"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	mcpclient "github.com/DataDog/datadog-agent/comp/mcp/client"
	mcpconfig "github.com/DataDog/datadog-agent/comp/mcp/config"
)

// dependencies defines all components this AI agent needs
type dependencies struct {
	fx.In
	MCPConfig mcpconfig.Component
	Logger    log.Component
	MCPClient mcpclient.Component
}

// aiAgent is the internal implementation
type aiAgent struct {
	config              mcpconfig.Component
	logger              log.Component
	mcpClient           mcpclient.Component
	maxSteps            int // Maximum number of steps in the agentic loop
	claudeClient        *anthropic.Client
	conversationHistory []anthropic.MessageParam
}

// newAIAgent creates a new AI agent
func newAIAgent(deps dependencies) Component {
	mcpConf := deps.MCPConfig.Get()

	var claudeClient anthropic.Client
	if mcpConf.AnthropicAPIKey != "" {
		claudeClient = anthropic.NewClient(
			option.WithAPIKey(mcpConf.AnthropicAPIKey),
		)
	}

	return &aiAgent{
		config:              deps.MCPConfig,
		logger:              deps.Logger,
		mcpClient:           deps.MCPClient,
		maxSteps:            10, // Default max steps
		claudeClient:        &claudeClient,
		conversationHistory: []anthropic.MessageParam{},
	}
}

// Solve attempts to solve the given issue using the agentic loop
func (a *aiAgent) Solve(
	ctx context.Context,
	issue Issue,
) (
	*AgentResult,
	error,
) {
	a.logger.Infof(
		"AI Agent attempting to solve issue: %s",
		issue.Description,
	)

	result := &AgentResult{
		Success: false,
		Steps:   []AgentStep{},
	}

	// Ensure we're connected to the MCP server
	if !a.mcpClient.IsConnected() {
		a.logger.Info("Connecting to MCP server...")
		if err := a.mcpClient.Connect(ctx); err != nil {
			return nil, fmt.Errorf(
				"failed to connect to MCP server: %w",
				err,
			)
		}
	}

	// Get available tools
	tools, err := a.mcpClient.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to list tools: %w",
			err,
		)
	}

	a.logger.Infof(
		"Available tools: %d",
		len(tools),
	)

	// Run the agentic loop
	for step := 0; step < a.maxSteps; step++ {
		a.logger.Infof(
			"Agent step %d/%d",
			step+1,
			a.maxSteps,
		)

		// THINK: Analyze the current situation and decide what to do
		thinkStep := a.think(
			ctx,
			issue,
			result.Steps,
			tools,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"thinking: %w",
				err,
			)
		}
		result.Steps = append(
			result.Steps,
			thinkStep,
		)
		a.logger.Infof(
			"Think: %s",
			thinkStep.Content,
		)

		// Check if thinking resulted in an error
		if thinkStep.Type == "error" {
			a.logger.Errorf(
				"Thinking step failed: %s",
				thinkStep.Content,
			)
			result.FinalState = fmt.Sprintf(
				"Agent failed to think: %s",
				thinkStep.Content,
			)
			result.Error = fmt.Errorf(
				"%s",
				thinkStep.Content,
			)
			break
		}

		// Check if the agent thinks the problem is solved
		if a.isProblemSolved(thinkStep) {
			result.Success = true
			result.FinalState = "Problem resolved"
			a.logger.Info("Agent believes problem is solved")
			break
		}

		// ACT: Execute the planned action via MCP
		actStep, err := a.act(
			ctx,
			thinkStep,
		)
		if err != nil {
			a.logger.Errorf(
				"Action failed: %v",
				err,
			)
			result.Steps = append(
				result.Steps,
				AgentStep{
					Type: "error",
					Content: fmt.Sprintf(
						"Action failed: %v",
						err,
					),
				},
			)
			continue
		}
		result.Steps = append(
			result.Steps,
			actStep,
		)
		a.logger.Infof(
			"Act: Called %s",
			actStep.ToolCall.Name,
		)

		// OBSERVE: Observe the results of the action
		observeStep := a.observe(
			ctx,
			actStep,
		)
		result.Steps = append(
			result.Steps,
			observeStep,
		)
		a.logger.Infof(
			"Observe: %s",
			observeStep.Content,
		)
	}

	if !result.Success {
		result.FinalState = fmt.Sprintf(
			"Max steps (%d) reached without solving the issue",
			a.maxSteps,
		)
	}

	return result, nil
}

// act executes the action decided in the think step
func (a *aiAgent) act(
	ctx context.Context,
	thinkStep AgentStep,
) (
	AgentStep,
	error,
) {
	if thinkStep.ToolCall == nil {
		return AgentStep{}, fmt.Errorf("no tool call in think step")
	}

	toolCall := thinkStep.ToolCall

	// Call the MCP tool
	result, err := a.mcpClient.CallTool(
		ctx,
		toolCall.Name,
		toolCall.Parameters,
	)
	if err != nil {
		return AgentStep{}, fmt.Errorf(
			"tool call failed: %w",
			err,
		)
	}

	return AgentStep{
		Type: "act",
		Content: fmt.Sprintf(
			"Called tool: %s",
			toolCall.Name,
		),
		ToolCall:   toolCall,
		ToolResult: result,
	}, nil
}

// observe processes the results of the action
func (a *aiAgent) observe(
	ctx context.Context,
	actStep AgentStep,
) AgentStep {
	// Extract meaningful information from the tool result
	observation := fmt.Sprintf(
		"Tool %s returned result",
		actStep.ToolCall.Name,
	)

	if actStep.ToolResult != nil {
		// Parse the result to extract useful information
		if result, ok := actStep.ToolResult.(*mcp.CallToolResult); ok {
			if len(result.Content) > 0 {
				observation = fmt.Sprintf(
					"Tool %s succeeded: %v",
					actStep.ToolCall.Name,
					result.Content[0],
				)
			}
		}
	}

	return AgentStep{
		Type:       "observe",
		Content:    observation,
		ToolResult: actStep.ToolResult,
	}
}

// isProblemSolved checks if the agent believes the problem is solved
func (a *aiAgent) isProblemSolved(thinkStep AgentStep) bool {
	return thinkStep.ToolCall != nil && thinkStep.ToolCall.Name == "_solved"
}
