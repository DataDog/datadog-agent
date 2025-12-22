// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// think analyzes the situation and decides what action to take using Claude
func (a *aiAgent) think(
	ctx context.Context,
	issue Issue,
	history []AgentStep,
	tools []*mcp.Tool,
) AgentStep {
	// Require Claude - if not configured, return error
	if a.claudeClient == nil {
		a.logger.Error("Claude client not configured")
		return AgentStep{
			Type:    "error",
			Content: "AI agent requires Anthropic API key to be configured",
		}
	}

	// Build the prompt for Claude
	systemPrompt := a.buildSystemPrompt()
	userPrompt := a.buildUserPrompt(
		issue,
		history,
	)
	claudeTools := a.convertMCPToolsToClaudeTools(tools)

	a.logger.Debugf("Asking Claude to think about the problem")

	// Call Claude with tools
	mcpConf := a.config.Get()
	response, err := a.claudeClient.Messages.New(
		ctx,
		anthropic.MessageNewParams{
			Model:     anthropic.Model(mcpConf.AnthropicModel),
			MaxTokens: int64(mcpConf.MaxTokens),
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
			},
			System: []anthropic.TextBlockParam{
				{
					Text: systemPrompt,
					Type: "text",
				},
			},
			Tools: claudeTools,
		},
	)

	if err != nil {
		a.logger.Errorf(
			"Failed to get Claude response: %v",
			err,
		)
		return AgentStep{
			Type: "error",
			Content: fmt.Sprintf(
				"Failed to get Claude response: %v",
				err,
			),
		}
	}

	// Parse Claude's response
	if len(response.Content) == 0 {
		a.logger.Error("Claude returned empty response")
		return AgentStep{
			Type:    "error",
			Content: "Claude returned empty response",
		}
	}

	// Extract reasoning and tool calls from the response
	return a.parseClaudeResponse(response)
}

// buildSystemPrompt creates the system prompt for Claude
func (a *aiAgent) buildSystemPrompt() string {
	return `You are an expert-level Site Reliability Engineer (
SRE) resolving an issue on a system that is hosting the DataDog Agent.

Your expertise includes:
- Distributed systems and observability platforms
- Production debugging and incident response
- Metrics, logs, traces, and APM
- System performance tuning and optimization
- Root cause analysis and remediation
- Infrastructure monitoring best practices

Your approach to problem-solving:
1. Gather comprehensive information before taking action
2. Form clear hypotheses based on observed symptoms
3. Test hypotheses methodically using available tools
4. Implement targeted fixes when root cause is identified
5. Verify that changes resolve the issue without side effects

Use the available tools to diagnose and fix issues. Think carefully through each step, explain your reasoning clearly, and be systematic in your approach.`
}

// buildUserPrompt creates the user prompt for Claude
func (a *aiAgent) buildUserPrompt(
	issue Issue,
	history []AgentStep,
) string {
	var sb strings.Builder

	sb.WriteString("=== PROBLEM TO SOLVE ===\n")
	sb.WriteString(
		fmt.Sprintf(
			"Description: %s\n",
			issue.Description,
		),
	)
	sb.WriteString(
		fmt.Sprintf(
			"Severity: %s\n",
			issue.Severity,
		),
	)

	if len(issue.Metadata) > 0 {
		sb.WriteString("\nAdditional Context:\n")
		for key, value := range issue.Metadata {
			valueJSON, _ := json.Marshal(value)
			sb.WriteString(
				fmt.Sprintf(
					"- %s: %s\n",
					key,
					string(valueJSON),
				),
			)
		}
	}

	if len(history) > 0 {
		sb.WriteString("\n=== INVESTIGATION HISTORY ===\n")
		for i, step := range history {
			switch step.Type {
			case "think":
				sb.WriteString(
					fmt.Sprintf(
						"\n[Step %d - Your Previous Reasoning]\n%s\n",
						i+1,
						step.Content,
					),
				)
			case "act":
				sb.WriteString(
					fmt.Sprintf(
						"\n[Step %d - Action Taken]\n",
						i+1,
					),
				)
				if step.ToolCall != nil {
					params, _ := json.MarshalIndent(
						step.ToolCall.Parameters,
						"",
						"  ",
					)
					sb.WriteString(
						fmt.Sprintf(
							"Tool called: %s\n",
							step.ToolCall.Name,
						),
					)
					if len(step.ToolCall.Parameters) > 0 {
						sb.WriteString(
							fmt.Sprintf(
								"Parameters:\n%s\n",
								string(params),
							),
						)
					}
				}
			case "observe":
				sb.WriteString(
					fmt.Sprintf(
						"\n[Step %d - Observation]\n%s\n",
						i+1,
						step.Content,
					),
				)
			case "error":
				sb.WriteString(
					fmt.Sprintf(
						"\n[Step %d - Error]\n%s\n",
						i+1,
						step.Content,
					),
				)
			}
		}
	}

	sb.WriteString("\n=== YOUR TASK ===\n")
	if len(history) == 0 {
		sb.WriteString("Begin your investigation. What is the first step you should take?")
	} else {
		sb.WriteString("Based on what you've learned so far, what should you do next?")
	}

	return sb.String()
}

// convertMCPToolsToClaudeTools converts MCP tools to Claude's tool format
func (a *aiAgent) convertMCPToolsToClaudeTools(mcpTools []*mcp.Tool) []anthropic.ToolUnionParam {
	claudeTools := make(
		[]anthropic.ToolUnionParam,
		0,
		len(mcpTools)+1,
	)

	// Convert each MCP tool to Claude tool format
	for _, tool := range mcpTools {
		// Type assert the input schema to map[string]interface{}
		var schemaMap map[string]interface{}
		if tool.InputSchema != nil {
			if sm, ok := tool.InputSchema.(map[string]interface{}); ok {
				schemaMap = sm
			} else {
				a.logger.Warnf(
					"Skipping tool %s: invalid schema type %T",
					tool.Name,
					tool.InputSchema,
				)
				continue
			}
		}

		// Convert the input schema to Claude's format
		inputSchema := a.convertMCPSchemaToClaudeSchema(schemaMap)
		toolParam := anthropic.ToolParam{
			Name:        tool.Name,
			Description: anthropic.String(tool.Description),
			InputSchema: inputSchema,
		}
		claudeTools = append(
			claudeTools,
			anthropic.ToolUnionParam{
				OfTool: &toolParam,
			},
		)
	}

	// Add a special "_solved" tool for when the issue is resolved
	toolParam := anthropic.ToolParam{
		Name:        "_solved",
		Description: anthropic.String("Use this tool when you have confirmed that the issue is resolved. No parameters needed."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
	}
	claudeTools = append(
		claudeTools,
		anthropic.ToolUnionParam{
			OfTool: &toolParam,
		},
	)

	return claudeTools
}

// convertMCPSchemaToClaudeSchema converts MCP JSON Schema to Claude's ToolInputSchemaParam
func (a *aiAgent) convertMCPSchemaToClaudeSchema(mcpSchema map[string]interface{}) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{
		Type: "object",
	}

	if mcpSchema == nil {
		schema.Properties = map[string]interface{}{}
		return schema
	}

	// Extract properties from the schema
	if props, ok := mcpSchema["properties"]; ok {
		schema.Properties = props
	} else {
		schema.Properties = map[string]interface{}{}
	}

	// Extract required fields from the schema
	if req, ok := mcpSchema["required"]; ok {
		if reqArray, ok := req.([]interface{}); ok {
			required := make(
				[]string,
				0,
				len(reqArray),
			)
			for _, r := range reqArray {
				if rStr, ok := r.(string); ok {
					required = append(
						required,
						rStr,
					)
				}
			}
			schema.Required = required
		} else if reqArray, ok := req.([]string); ok {
			schema.Required = reqArray
		}
	}

	return schema
}

// parseClaudeResponse parses Claude's response and extracts the next step
func (a *aiAgent) parseClaudeResponse(response *anthropic.Message) AgentStep {
	var reasoning string
	var toolCall *ToolCall

	// Iterate through content blocks
	for _, block := range response.Content {
		switch block.Type {
		case "text":
			// This is Claude's reasoning/thinking
			reasoning += block.Text

		case "tool_use":
			// Claude wants to use a tool
			if toolCall == nil {
				// Parse the tool input
				var params map[string]interface{}
				if len(block.Input) > 0 {
					if err := json.Unmarshal(
						block.Input,
						&params,
					); err != nil {
						a.logger.Errorf(
							"Failed to parse tool input: %v",
							err,
						)
						params = map[string]interface{}{}
					}
				} else {
					params = map[string]interface{}{}
				}

				toolCall = &ToolCall{
					Name:       block.Name,
					Parameters: params,
				}

				a.logger.Infof(
					"Claude wants to call tool: %s",
					block.Name,
				)
			}
		}
	}

	// If no tool call was made, something went wrong
	if toolCall == nil {
		a.logger.Warn("Claude did not specify a tool call")
		return AgentStep{
			Type:    "error",
			Content: "Claude did not specify a tool call",
		}
	}

	if reasoning == "" {
		reasoning = fmt.Sprintf(
			"Calling tool: %s",
			toolCall.Name,
		)
	}

	return AgentStep{
		Type:     "think",
		Content:  strings.TrimSpace(reasoning),
		ToolCall: toolCall,
	}
}
