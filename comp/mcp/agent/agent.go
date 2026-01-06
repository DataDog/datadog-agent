// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
		maxSteps:            30, // Default max steps
		claudeClient:        &claudeClient,
		conversationHistory: []anthropic.MessageParam{},
	}
}

// GenerateConversationID generates a unique 8-character conversation identifier
func GenerateConversationID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Solve attempts to solve the given issue using the agentic loop
func (a *aiAgent) Solve(
	ctx context.Context,
	issue Issue,
) (
	*AgentResult,
	error,
) {
	// Use conversation ID from issue metadata if available, otherwise generate a new one
	var conversationID string
	if id, ok := issue.Metadata["conversation_id"].(string); ok && id != "" {
		conversationID = id
	} else {
		conversationID = GenerateConversationID()
	}

	a.logger.Infof(
		"[MCP AI Agent][%s] Attempting to solve issue: %s",
		conversationID,
		issue.Description,
	)

	result := &AgentResult{
		Success:    false,
		Steps:      []AgentStep{},
		Iterations: 0,
	}

	// Ensure we're connected to the MCP server
	if !a.mcpClient.IsConnected() {
		a.logger.Infof("[MCP AI Agent][%s] Connecting to MCP server...", conversationID)
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
		"[MCP AI Agent][%s] Available tools: %d",
		conversationID,
		len(tools),
	)

	// Run the agentic loop
	for step := 0; step < a.maxSteps; step++ {
		// Check if context is cancelled (e.g., Ctrl+C pressed)
		select {
		case <-ctx.Done():
			a.logger.Infof(
				"[MCP AI Agent][%s] Context cancelled, stopping investigation",
				conversationID,
			)
			result.FinalState = "Investigation stopped: context cancelled"
			result.Error = ctx.Err()
			return result, ctx.Err()
		default:
			// Continue with normal operation
		}

		a.logger.Infof(
			"[MCP AI Agent][%s] Step %d/%d",
			conversationID,
			step+1,
			a.maxSteps,
		)

		// THINK: Analyze the current situation and decide what to do
		thinkStep := a.think(
			ctx,
			issue,
			result.Steps,
			tools,
			step,
			a.maxSteps,
			conversationID,
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
			"[MCP AI Agent][%s] Think: %s",
			conversationID,
			thinkStep.Content,
		)

		// Check if thinking resulted in an error
		if thinkStep.Type == "error" {
			a.logger.Errorf(
				"[MCP AI Agent][%s] Thinking step failed: %s",
				conversationID,
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
			result.Iterations++
			break
		}

		// Check if the agent thinks the problem is solved
		if a.isProblemSolved(thinkStep) {
			result.Success = true
			result.FinalState = "Problem resolved"
			a.logger.Infof("[MCP AI Agent][%s] Problem is solved", conversationID)
			result.Iterations++
			break
		}

		// If no tool call was specified, Claude is just reasoning
		// Continue to the next iteration without acting
		if thinkStep.ToolCall == nil {
			a.logger.Debugf(
				"[MCP AI Agent][%s] No tool call, continuing to next iteration",
				conversationID,
			)
			result.Iterations++
			continue
		}

		// ACT: Execute the planned action via MCP
		actStep, err := a.act(
			ctx,
			thinkStep,
		)
		if err != nil {
			a.logger.Errorf(
				"[MCP AI Agent][%s] Action failed: %v",
				conversationID,
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
			result.Iterations++
			continue
		}
		result.Steps = append(
			result.Steps,
			actStep,
		)
		a.logger.Debugf(
			"[MCP AI Agent][%s] Act: Called %s",
			conversationID,
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
		a.logger.Debugf(
			"[MCP AI Agent][%s] Observe: %s",
			conversationID,
			observeStep.Content,
		)

		// Increment iteration counter after completing think-act-observe cycle
		result.Iterations++
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
