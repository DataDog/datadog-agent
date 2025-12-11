// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/mcp/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// JSON-RPC 2.0 constants
const (
	JSONRPCVersion = "2.0"

	// MCP method names
	MethodInitialize    = "initialize"
	MethodInitialized   = "notifications/initialized"
	MethodListTools     = "tools/list"
	MethodCallTool      = "tools/call"
	MethodPing          = "ping"
	MethodListResources = "resources/list"
	MethodReadResource  = "resources/read"
	MethodListPrompts   = "prompts/list"
	MethodGetPrompt     = "prompts/get"
	MethodComplete      = "completion/complete"
	MethodSetLogLevel   = "logging/setLevel"
	MethodCancelRequest = "$/cancelRequest"
	MethodProgress      = "$/progress"
)

// JSON-RPC error codes
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// MCP-specific error codes
	CodeToolNotFound     = -32001
	CodeToolExecutionErr = -32002
	CodeResourceNotFound = -32003
)

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"` // Can be string, number, or null for notifications
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
	ID      interface{}   `json:"id"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// MCP Protocol Types

// InitializeParams represents the parameters for the initialize request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

// InitializeResult represents the result of the initialize request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
}

// ClientCapabilities represents the capabilities of an MCP client.
type ClientCapabilities struct {
	Roots        *RootsCapability       `json:"roots,omitempty"`
	Sampling     *SamplingCapability    `json:"sampling,omitempty"`
	Experimental map[string]interface{} `json:"experimental,omitempty"`
}

// ServerCapabilities represents the capabilities of an MCP server.
type ServerCapabilities struct {
	Tools        *ToolsCapability       `json:"tools,omitempty"`
	Resources    *ResourcesCapability   `json:"resources,omitempty"`
	Prompts      *PromptsCapability     `json:"prompts,omitempty"`
	Logging      *LoggingCapability     `json:"logging,omitempty"`
	Experimental map[string]interface{} `json:"experimental,omitempty"`
}

// RootsCapability represents the roots capability.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability represents the sampling capability.
type SamplingCapability struct{}

// ToolsCapability represents the tools capability.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability represents the resources capability.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability represents the prompts capability.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability represents the logging capability.
type LoggingCapability struct{}

// Implementation identifies a client or server implementation.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"inputSchema"`
}

// ListToolsResult represents the result of tools/list.
type ListToolsResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// CallToolParams represents the parameters for tools/call.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// CallToolResult represents the result of tools/call.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem represents a content item in tool results.
type ContentItem struct {
	Type     string `json:"type"` // "text", "image", "resource"
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // Base64 for images
}

// PingResult represents the result of a ping request.
type PingResult struct{}

// ToolProvider provides access to registered tools.
type ToolProvider interface {
	// ListTools returns all available tools.
	ListTools() []Tool

	// CallTool executes a tool and returns the result.
	CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*types.ToolResponse, error)
}

// JSONRPCHandler handles JSON-RPC messages for the MCP protocol.
type JSONRPCHandler struct {
	toolProvider ToolProvider
	serverInfo   Implementation
	initialized  bool
	mu           sync.RWMutex
}

// NewJSONRPCHandler creates a new JSON-RPC handler.
func NewJSONRPCHandler(toolProvider ToolProvider, serverName, serverVersion string) *JSONRPCHandler {
	return &JSONRPCHandler{
		toolProvider: toolProvider,
		serverInfo: Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
	}
}

// HandleMessage processes a JSON-RPC message and returns the response.
// This implements the MessageHandler interface.
func (h *JSONRPCHandler) HandleMessage(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
	log.Debugf("MCP JSON-RPC: received message (%d bytes)", len(message))

	var req JSONRPCRequest
	if err := json.Unmarshal(message, &req); err != nil {
		log.Warnf("MCP JSON-RPC: parse error: %v", err)
		return h.errorResponse(nil, CodeParseError, "Parse error", err.Error())
	}

	log.Infof("MCP JSON-RPC: method=%s id=%v", req.Method, req.ID)

	// Validate JSON-RPC version
	if req.JSONRPC != JSONRPCVersion {
		return h.errorResponse(req.ID, CodeInvalidRequest, "Invalid Request", "Invalid JSON-RPC version")
	}

	// Handle the request based on method
	result, rpcErr := h.handleMethod(ctx, &req, connInfo)
	if rpcErr != nil {
		log.Warnf("MCP JSON-RPC: method=%s error: code=%d msg=%s", req.Method, rpcErr.Code, rpcErr.Message)
		return h.errorResponse(req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
	}

	log.Infof("MCP JSON-RPC: method=%s completed successfully", req.Method)

	// If this is a notification (no ID), don't send a response
	if req.ID == nil {
		return nil, nil
	}

	return h.successResponse(req.ID, result)
}

func (h *JSONRPCHandler) handleMethod(ctx context.Context, req *JSONRPCRequest, connInfo ConnectionInfo) (interface{}, *JSONRPCError) {
	switch req.Method {
	case MethodInitialize:
		return h.handleInitialize(ctx, req.Params)
	case MethodInitialized:
		return h.handleInitialized(ctx)
	case MethodPing:
		return h.handlePing(ctx)
	case MethodListTools:
		return h.handleListTools(ctx)
	case MethodCallTool:
		return h.handleCallTool(ctx, req.Params)
	case MethodListResources:
		return h.handleListResources(ctx)
	case MethodListPrompts:
		return h.handleListPrompts(ctx)
	default:
		return nil, &JSONRPCError{
			Code:    CodeMethodNotFound,
			Message: "Method not found",
			Data:    fmt.Sprintf("Unknown method: %s", req.Method),
		}
	}
}

func (h *JSONRPCHandler) handleInitialize(ctx context.Context, params json.RawMessage) (interface{}, *JSONRPCError) {
	var initParams InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return nil, &JSONRPCError{
			Code:    CodeInvalidParams,
			Message: "Invalid params",
			Data:    err.Error(),
		}
	}

	h.mu.Lock()
	h.initialized = true
	h.mu.Unlock()

	// Return server capabilities
	return &InitializeResult{
		ProtocolVersion: "2024-11-05", // MCP protocol version
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
			Logging: &LoggingCapability{},
		},
		ServerInfo: h.serverInfo,
	}, nil
}

func (h *JSONRPCHandler) handleInitialized(ctx context.Context) (interface{}, *JSONRPCError) {
	// This is a notification, no response needed
	return nil, nil
}

func (h *JSONRPCHandler) handlePing(ctx context.Context) (interface{}, *JSONRPCError) {
	return &PingResult{}, nil
}

func (h *JSONRPCHandler) handleListTools(ctx context.Context) (interface{}, *JSONRPCError) {
	h.mu.RLock()
	initialized := h.initialized
	h.mu.RUnlock()

	if !initialized {
		return nil, &JSONRPCError{
			Code:    CodeInvalidRequest,
			Message: "Server not initialized",
			Data:    "Call initialize first",
		}
	}

	tools := h.toolProvider.ListTools()
	return &ListToolsResult{
		Tools: tools,
	}, nil
}

func (h *JSONRPCHandler) handleCallTool(ctx context.Context, params json.RawMessage) (interface{}, *JSONRPCError) {
	h.mu.RLock()
	initialized := h.initialized
	h.mu.RUnlock()

	if !initialized {
		return nil, &JSONRPCError{
			Code:    CodeInvalidRequest,
			Message: "Server not initialized",
			Data:    "Call initialize first",
		}
	}

	var callParams CallToolParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, &JSONRPCError{
			Code:    CodeInvalidParams,
			Message: "Invalid params",
			Data:    err.Error(),
		}
	}

	log.Infof("MCP JSON-RPC: tools/call name=%s args=%v", callParams.Name, callParams.Arguments)

	// Call the tool
	resp, err := h.toolProvider.CallTool(ctx, callParams.Name, callParams.Arguments)
	if err != nil {
		log.Warnf("MCP JSON-RPC: tools/call name=%s error=%v", callParams.Name, err)
		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: fmt.Sprintf("Tool execution error: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	log.Infof("MCP JSON-RPC: tools/call name=%s completed", callParams.Name)

	// Convert tool response to MCP content
	content := h.convertToolResponseToContent(resp)
	return &CallToolResult{
		Content: content,
		IsError: resp.Error != "",
	}, nil
}

func (h *JSONRPCHandler) handleListResources(ctx context.Context) (interface{}, *JSONRPCError) {
	// Currently no resources are supported
	return map[string]interface{}{
		"resources": []interface{}{},
	}, nil
}

func (h *JSONRPCHandler) handleListPrompts(ctx context.Context) (interface{}, *JSONRPCError) {
	// Currently no prompts are supported
	return map[string]interface{}{
		"prompts": []interface{}{},
	}, nil
}

func (h *JSONRPCHandler) convertToolResponseToContent(resp *types.ToolResponse) []ContentItem {
	if resp.Error != "" {
		return []ContentItem{
			{
				Type: "text",
				Text: resp.Error,
			},
		}
	}

	// Marshal the result to JSON for text content
	resultJSON, err := json.MarshalIndent(resp.Result, "", "  ")
	if err != nil {
		return []ContentItem{
			{
				Type: "text",
				Text: fmt.Sprintf("%v", resp.Result),
			},
		}
	}

	return []ContentItem{
		{
			Type: "text",
			Text: string(resultJSON),
		},
	}
}

func (h *JSONRPCHandler) successResponse(id interface{}, result interface{}) ([]byte, error) {
	resp := JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  result,
		ID:      id,
	}
	return json.Marshal(resp)
}

func (h *JSONRPCHandler) errorResponse(id interface{}, code int, message string, data interface{}) ([]byte, error) {
	resp := JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
	return json.Marshal(resp)
}

// IsInitialized returns whether the handler has been initialized.
func (h *JSONRPCHandler) IsInitialized() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.initialized
}

// Reset resets the handler state (useful for testing or reconnection).
func (h *JSONRPCHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.initialized = false
}
