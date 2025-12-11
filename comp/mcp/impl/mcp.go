// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package mcpimpl implements the MCP (Model Context Protocol) server component.
package mcpimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	mcpcomp "github.com/DataDog/datadog-agent/comp/mcp/def"
	"github.com/DataDog/datadog-agent/pkg/mcp/server"
	"github.com/DataDog/datadog-agent/pkg/mcp/tools/logs"
	"github.com/DataDog/datadog-agent/pkg/mcp/tools/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the MCP component.
type Requires struct {
	Lc        compdef.Lifecycle
	Config    config.Component
	Params    mcpcomp.Params
	LogsAgent option.Option[logsAgent.Component]
}

// Provides defines what the MCP component provides.
type Provides struct {
	Comp mcpcomp.Component
}

// mcpComponent implements the MCP component.
type mcpComponent struct {
	config         config.Component
	server         *server.MCPServer
	processHandler *process.ProcessHandler
	logsHandler    *logs.LogsHandler
	sourcesHandler *logs.SourcesHandler
	telemetry      *mcpTelemetry
	mu             sync.RWMutex
	started        bool
	enabled        bool
}

// NewComponent creates a new MCP component.
func NewComponent(reqs Requires) (Provides, error) {
	log.Infof("MCP component: NewComponent called")

	// Check if MCP is enabled in config
	enabled := reqs.Config.GetBool("mcp.enabled")
	log.Infof("MCP component: mcp.enabled from config = %v", enabled)
	log.Infof("MCP component: Params.Enabled = %v", reqs.Params.Enabled)

	// If params explicitly disable MCP, override config
	if !reqs.Params.Enabled {
		log.Infof("MCP component: Params.Enabled is false, disabling MCP")
		enabled = false
	}

	comp := &mcpComponent{
		config:    reqs.Config,
		enabled:   enabled,
		telemetry: newMCPTelemetry(),
	}

	log.Infof("MCP component: final enabled state = %v", enabled)

	if enabled {
		// Create the MCP server
		mcpServer, err := server.NewMCPServer(reqs.Config)
		if err != nil {
			return Provides{}, fmt.Errorf("failed to create MCP server: %w", err)
		}
		comp.server = mcpServer

		// Create and register the process tool handler
		processHandler, err := process.NewProcessHandler(reqs.Config)
		if err != nil {
			return Provides{}, fmt.Errorf("failed to create process handler: %w", err)
		}
		comp.processHandler = processHandler

		// Register the process tool
		if err := mcpServer.RegisterTool("GetProcessSnapshot", comp.wrapHandlerWithTelemetry(processHandler)); err != nil {
			return Provides{}, fmt.Errorf("failed to register process tool: %w", err)
		}

		// Create and register the logs tools if logs agent is available
		if logsAgentComp, ok := reqs.LogsAgent.Get(); ok {
			// Register GetLogs tool
			logsHandler, err := logs.NewLogsHandler(logsAgentComp, reqs.Config)
			if err != nil {
				log.Warnf("Failed to create logs handler: %v", err)
			} else {
				comp.logsHandler = logsHandler
				if err := mcpServer.RegisterTool("GetLogs", comp.wrapHandlerWithTelemetry(logsHandler)); err != nil {
					log.Warnf("Failed to register GetLogs tool: %v", err)
				} else {
					log.Info("MCP GetLogs tool registered successfully")
				}
			}

			// Register ListLogSources tool
			sourcesHandler, err := logs.NewSourcesHandler(logsAgentComp, reqs.Config)
			if err != nil {
				log.Warnf("Failed to create sources handler: %v", err)
			} else {
				comp.sourcesHandler = sourcesHandler
				if err := mcpServer.RegisterTool("ListLogSources", comp.wrapHandlerWithTelemetry(sourcesHandler)); err != nil {
					log.Warnf("Failed to register ListLogSources tool: %v", err)
				} else {
					log.Info("MCP ListLogSources tool registered successfully")
				}
			}
		} else {
			log.Info("Logs agent not available, MCP logs tools will not be registered")
		}

		log.Infof("MCP component initialized with tools: %v", mcpServer.ListTools())
	}

	// Register lifecycle hooks
	reqs.Lc.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})

	return Provides{Comp: comp}, nil
}

// start initializes the MCP server when the agent starts.
func (m *mcpComponent) start(ctx context.Context) error {
	log.Infof("MCP component: start() called, enabled=%v", m.enabled)
	if !m.enabled {
		log.Info("MCP server is disabled, skipping start")
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("MCP server already started")
	}

	if err := m.server.Start(ctx); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	m.started = true
	log.Info("MCP server started successfully")
	return nil
}

// stop shuts down the MCP server when the agent stops.
func (m *mcpComponent) stop(_ context.Context) error {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	// Close handlers first
	if m.processHandler != nil {
		m.processHandler.Close()
	}
	if m.logsHandler != nil {
		m.logsHandler.Close()
	}
	if m.sourcesHandler != nil {
		m.sourcesHandler.Close()
	}

	if err := m.server.Stop(); err != nil {
		return fmt.Errorf("failed to stop MCP server: %w", err)
	}

	m.started = false
	log.Info("MCP server stopped successfully")
	return nil
}

// IsEnabled returns whether the MCP server is enabled.
func (m *mcpComponent) IsEnabled() bool {
	return m.enabled
}

// ListTools returns all registered tool names.
func (m *mcpComponent) ListTools() []string {
	if m.server == nil {
		return nil
	}
	return m.server.ListTools()
}

// HandleToolCall processes an incoming MCP tool request.
func (m *mcpComponent) HandleToolCall(ctx context.Context, req *server.ToolRequest) (*server.ToolResponse, error) {
	if !m.enabled {
		return nil, fmt.Errorf("MCP server is disabled")
	}

	m.mu.RLock()
	if !m.started {
		m.mu.RUnlock()
		return nil, fmt.Errorf("MCP server is not started")
	}
	m.mu.RUnlock()

	return m.server.HandleToolCall(ctx, req)
}

// wrapHandlerWithTelemetry wraps a handler with telemetry tracking.
func (m *mcpComponent) wrapHandlerWithTelemetry(handler server.Handler) server.Handler {
	return &telemetryHandler{
		wrapped:   handler,
		telemetry: m.telemetry,
	}
}

// telemetryHandler wraps a handler and records telemetry.
type telemetryHandler struct {
	wrapped   server.Handler
	telemetry *mcpTelemetry
}

// Handle processes the request and records telemetry.
func (h *telemetryHandler) Handle(ctx context.Context, req *server.ToolRequest) (*server.ToolResponse, error) {
	h.telemetry.IncrementRequestCount(req.ToolName)
	start := time.Now()

	// Log the incoming request with parameters
	log.Infof("MCP tool request: tool=%s request_id=%s params=%v", req.ToolName, req.RequestID, req.Parameters)

	resp, err := h.wrapped.Handle(ctx, req)

	duration := time.Since(start)
	h.telemetry.RecordLatency(req.ToolName, duration)

	if err != nil {
		h.telemetry.IncrementErrorCount(req.ToolName)
		log.Warnf("MCP tool error: tool=%s request_id=%s duration=%v error=%v", req.ToolName, req.RequestID, duration, err)
	} else {
		log.Infof("MCP tool completed: tool=%s request_id=%s duration=%v", req.ToolName, req.RequestID, duration)
	}

	return resp, err
}
