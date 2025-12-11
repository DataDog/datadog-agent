// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"context"
	"fmt"
	"strings"
	"sync"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/mcp/tools"
	"github.com/DataDog/datadog-agent/pkg/mcp/transport"
	"github.com/DataDog/datadog-agent/pkg/mcp/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MCPServer handles MCP tool requests
type MCPServer struct {
	registry    *tools.Registry
	config      *Config
	transport   transport.Transport
	jsonrpc     *transport.JSONRPCHandler
	mu          sync.RWMutex
	started     bool
	cancel      context.CancelFunc
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(cfg pkgconfigmodel.Reader) (*MCPServer, error) {
	config, err := NewConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP config: %w", err)
	}

	return &MCPServer{
		registry: tools.NewRegistry(),
		config:   config,
	}, nil
}

// HandleToolCall processes incoming MCP tool requests
func (s *MCPServer) HandleToolCall(ctx context.Context, req *ToolRequest) (*ToolResponse, error) {
	handler, err := s.registry.GetHandler(req.ToolName)
	if err != nil {
		return &ToolResponse{
			ToolName:  req.ToolName,
			Error:     fmt.Sprintf("unknown tool: %s", req.ToolName),
			RequestID: req.RequestID,
		}, fmt.Errorf("unknown tool: %s", req.ToolName)
	}

	return handler.Handle(ctx, req)
}

// RegisterTool adds a new tool handler to the server
func (s *MCPServer) RegisterTool(name string, handler Handler) error {
	return s.registry.Register(name, handler)
}

// ListTools returns all registered tool names
func (s *MCPServer) ListTools() []string {
	return s.registry.List()
}

// Start initializes and starts the MCP server
func (s *MCPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		log.Info("MCP server: disabled, not starting transport")
		return nil
	}

	if s.started {
		return fmt.Errorf("MCP server already started")
	}

	// Parse the address to determine transport type
	address := s.config.Address
	log.Infof("MCP server: starting with address %s", address)

	var trans transport.Transport
	if strings.HasPrefix(address, "unix://") {
		socketPath := strings.TrimPrefix(address, "unix://")
		log.Infof("MCP server: creating Unix transport at %s", socketPath)
		trans = transport.NewUnixTransport(transport.UnixConfig{
			TransportConfig: transport.TransportConfig{
				MaxConnections: s.config.MaxConnections,
			},
			Path:           socketPath,
			RemoveExisting: true,
		})
	} else if strings.HasPrefix(address, "tcp://") {
		tcpAddr := strings.TrimPrefix(address, "tcp://")
		log.Infof("MCP server: creating TCP transport at %s", tcpAddr)
		var err error
		trans, err = transport.NewTCPTransport(transport.TCPConfig{
			TransportConfig: transport.TransportConfig{
				MaxConnections: s.config.MaxConnections,
			},
			Address: tcpAddr,
		})
		if err != nil {
			return fmt.Errorf("failed to create TCP transport: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported MCP address scheme: %s", address)
	}

	s.transport = trans

	// Create JSON-RPC handler that wraps our tool provider
	s.jsonrpc = transport.NewJSONRPCHandler(&mcpToolProvider{server: s}, "datadog-agent", "7.0.0")

	// Start transport in background with a fresh context.
	// We use context.Background() instead of the passed-in ctx because the lifecycle
	// start context is only valid during startup and gets cancelled once startup completes.
	// The transport should run until Stop() is explicitly called.
	transportCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go func() {
		log.Info("MCP server: starting transport...")
		err := trans.Start(transportCtx, s.jsonrpc)
		if err != nil {
			if err == context.Canceled {
				log.Info("MCP server: transport stopped (context cancelled - normal shutdown)")
			} else {
				log.Errorf("MCP server: transport error: %v", err)
			}
		} else {
			log.Info("MCP server: transport stopped (no error)")
		}
	}()

	s.started = true
	log.Info("MCP server: started successfully")
	return nil
}

// mcpToolProvider implements transport.ToolProvider interface
type mcpToolProvider struct {
	server *MCPServer
}

func (p *mcpToolProvider) ListTools() []transport.Tool {
	names := p.server.ListTools()
	tools := make([]transport.Tool, len(names))
	for i, name := range names {
		tools[i] = transport.Tool{
			Name:        name,
			Description: getToolDescription(name),
			InputSchema: getToolSchema(name),
		}
	}
	return tools
}

// getToolDescription returns the description for a tool
func getToolDescription(name string) string {
	switch name {
	case "GetProcessSnapshot":
		return "Get a snapshot of running processes on the host. Returns process info including PID, name, CPU%, and memory usage. Use 'compact: true' (default) for minimal output, 'compact: false' for full details. Use filters to reduce results."
	case "GetLogs":
		return "Collect and filter logs from the Datadog Agent logs pipeline. Captures logs for a configurable duration and returns matching entries. Use 'query' for regex filtering on message content. Filter by source, service, name, or type."
	case "ListLogSources":
		return "List all log sources currently being monitored by the Datadog Agent. Returns information about each source including file paths, status, bytes read, and configuration. Filter by source type or status."
	default:
		return fmt.Sprintf("Tool: %s", name)
	}
}

// getToolSchema returns the input schema for a tool
func getToolSchema(name string) map[string]interface{} {
	switch name {
	case "GetProcessSnapshot":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "integer"},
					"description": "Filter by specific PIDs",
				},
				"process_names": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Filter by process names (case-insensitive)",
				},
				"regex_filter": map[string]interface{}{
					"type":        "string",
					"description": "Regex to filter processes by name or command line",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max processes to return (default: 25, max: 1000)",
					"default":     25,
				},
				"sort_by": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"cpu", "memory", "pid", "name"},
					"description": "Sort processes by field (default: descending)",
				},
				"compact": map[string]interface{}{
					"type":        "boolean",
					"description": "Return minimal fields only (pid, name, cpu, mem). Default: true",
					"default":     true,
				},
				"max_cmd_length": map[string]interface{}{
					"type":        "integer",
					"description": "Truncate command lines to this length (default: 200)",
					"default":     200,
				},
				"include_stats": map[string]interface{}{
					"type":        "boolean",
					"description": "Include CPU/memory stats (default: true)",
					"default":     true,
				},
			},
		}
	case "GetLogs":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Regex pattern to filter log messages by content",
				},
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Filter by log source identifier",
				},
				"service": map[string]interface{}{
					"type":        "string",
					"description": "Filter by service name",
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Filter by integration/source name",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Filter by source type (e.g., 'file', 'docker', 'journald')",
				},
				"duration": map[string]interface{}{
					"type":        "integer",
					"description": "How long to collect logs in seconds (default: 10, max: 60)",
					"default":     10,
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max log messages to return (default: 100, max: 500)",
					"default":     100,
				},
			},
		}
	case "ListLogSources":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Filter by source type (e.g., 'file', 'docker', 'journald', 'tcp', 'udp')",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Filter by source status (e.g., 'OK', 'Error', 'Pending')",
				},
			},
		}
	default:
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
}

func (p *mcpToolProvider) CallTool(ctx context.Context, name string, args map[string]interface{}) (*types.ToolResponse, error) {
	req := &ToolRequest{
		ToolName:   name,
		Parameters: args,
	}
	resp, err := p.server.HandleToolCall(ctx, req)
	if err != nil {
		return nil, err
	}
	return &types.ToolResponse{
		ToolName:  resp.ToolName,
		Result:    resp.Result,
		Error:     resp.Error,
		RequestID: resp.RequestID,
	}, nil
}

// Stop shuts down the MCP server
func (s *MCPServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	// Cancel the context to signal transport to stop
	if s.cancel != nil {
		s.cancel()
	}

	// Stop the transport
	if s.transport != nil {
		if err := s.transport.Stop(context.Background()); err != nil {
			log.Warnf("MCP server: error stopping transport: %v", err)
		}
	}

	s.started = false
	log.Info("MCP server: stopped")
	return nil
}

// IsEnabled returns whether the MCP server is enabled
func (s *MCPServer) IsEnabled() bool {
	return s.config.Enabled
}
