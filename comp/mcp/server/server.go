// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	mcpconfig "github.com/DataDog/datadog-agent/comp/mcp/config"
)

// dependencies defines all components this server needs
type dependencies struct {
	fx.In
	Lc            fx.Lifecycle
	Config        coreconfig.Component
	MCPConfig     mcpconfig.Component
	Logger        log.Component
	Demultiplexer demultiplexer.Component
}

// provides defines what this component provides
type provides struct {
	fx.Out

	Comp           Component
	StatusProvider status.InformationProvider
}

// mcpServer is the internal implementation
type mcpServer struct {
	config        mcpconfig.Component
	logger        log.Component
	demultiplexer demultiplexer.Component
	listener      net.Listener
	stopChan      chan struct{}
	wg            sync.WaitGroup
	running       atomic.Bool
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	clientCount   atomic.Int32
}

// Parameter structs for MCP tools

// listDirectoryParams represents parameters for list_directory tool
type listDirectoryParams struct {
	Path string `json:"path"`
}

// readFileParams represents parameters for read_file tool
type readFileParams struct {
	Path string `json:"path"`
}

// tailFileParams represents parameters for tail_file tool
type tailFileParams struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}

// checkFileStatsParams represents parameters for check_file_stats tool
type checkFileStatsParams struct {
	Path string `json:"path"`
}

// findFilesParams represents parameters for find_files tool
type findFilesParams struct {
	Path       string `json:"path"`
	Pattern    string `json:"pattern"`
	MaxResults int    `json:"max_results"`
}

// testTCPConnectionParams represents parameters for test_tcp_connection tool
type testTCPConnectionParams struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Timeout int    `json:"timeout"`
}

// grepLogParams represents parameters for grep_log tool
type grepLogParams struct {
	Path          string `json:"path"`
	Pattern       string `json:"pattern"`
	CaseSensitive bool   `json:"case_sensitive"`
	ContextLines  int    `json:"context_lines"`
	MaxMatches    int    `json:"max_matches"`
}

// getProcessStatsParams represents parameters for get_process_stats tool
type getProcessStatsParams struct {
	PID         int    `json:"pid"`
	ProcessName string `json:"process_name"`
}

// resolveHostnameParams represents parameters for resolve_hostname tool
type resolveHostnameParams struct {
	Hostname string `json:"hostname"`
}

// checkListeningPortsParams represents parameters for check_listening_ports tool
type checkListeningPortsParams struct {
	Protocol string `json:"protocol"` // "tcp", "udp", "all"
	Port     int    `json:"port"`     // filter by port (optional)
}

// newMCPServer creates and initializes the MCP server
func newMCPServer(deps dependencies) (
	provides,
	error,
) {
	mcpConf := deps.MCPConfig.Get()

	// Check if MCP is enabled
	if !mcpConf.Enabled {
		deps.Logger.Info("MCP server is disabled")
		return provides{
			Comp:           &mcpServer{},
			StatusProvider: status.NewInformationProvider(nil),
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	server := &mcpServer{
		config:        deps.MCPConfig,
		logger:        deps.Logger,
		demultiplexer: deps.Demultiplexer,
		stopChan:      make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Register lifecycle hooks
	deps.Lc.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				return server.Start()
			},
			OnStop: func(ctx context.Context) error {
				return server.Stop()
			},
		},
	)

	return provides{
		Comp:           server,
		StatusProvider: status.NewInformationProvider(mcpStatusProvider{server}),
	}, nil
}

// Start starts the MCP server on a Unix domain socket
func (s *mcpServer) Start() error {
	if s.running.Load() {
		return fmt.Errorf("MCP server already running")
	}

	mcpConf := s.config.Get()
	socketPath := mcpConf.SocketPath

	// Remove existing socket file if it exists
	if err := os.RemoveAll(socketPath); err != nil {
		return fmt.Errorf(
			"removing existing socket: %w",
			err,
		)
	}

	// Create Unix domain socket listener
	listener, err := net.Listen(
		"unix",
		socketPath,
	)
	if err != nil {
		return fmt.Errorf(
			"creating Unix socket listener: %w",
			err,
		)
	}

	s.listener = listener
	s.running.Store(true)

	s.logger.Infof(
		"MCP server listening on Unix socket: %s",
		socketPath,
	)

	// Start accepting connections in a goroutine
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// acceptLoop accepts incoming connections and handles them
func (s *mcpServer) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopChan:
			s.logger.Debug("MCP server accept loop stopping")
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.stopChan:
					// Server is shutting down, this is expected
					return
				default:
					s.logger.Errorf(
						"Error accepting MCP connection: %v",
						err,
					)
					continue
				}
			}

			// Handle each client in a separate goroutine
			s.wg.Add(1)
			go s.handleClient(conn)
		}
	}
}

// handleClient handles a single MCP client connection
func (s *mcpServer) handleClient(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	clientID := s.clientCount.Add(1)
	s.logger.Infof(
		"MCP client #%d connected",
		clientID,
	)
	defer s.logger.Infof(
		"MCP client #%d disconnected",
		clientID,
	)

	// Create a new MCP server instance for this client
	mcpSrv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "Datadog Agent MCP",
			Version: "v1.0.0",
		},
		nil,
	)

	// Register tools for this client
	s.registerTools(
		mcpSrv,
		clientID,
	)

	// Create a custom transport using the connection
	transport := &connTransport{
		conn: conn,
	}

	// Run the MCP server for this client
	if err := mcpSrv.Run(
		s.ctx,
		transport,
	); err != nil {
		s.logger.Errorf(
			"MCP client #%d error: %v",
			clientID,
			err,
		)
	}
}

// registerTools registers all MCP tools for a client
func (s *mcpServer) registerTools(
	mcpSrv *mcp.Server,
	clientID int32,
) {
	// Register check_cpu_usage tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "check_cpu_usage",
			Description: "Check current CPU usage on the system",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleCheckCPUUsage(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register check_memory_usage tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "check_memory_usage",
			Description: "Check current memory usage on the system",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleCheckMemoryUsage(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register check_disk_usage tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "check_disk_usage",
			Description: "Check disk usage for a specific path",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to check disk usage for (default: /)",
					},
				},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleCheckDiskUsage(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register check_process tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "check_process",
			Description: "Check if a specific process is running",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"process_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the process to check",
					},
				},
				"required": []string{"process_name"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleCheckProcess(
				ctx,
				req,
				clientID,
			)
		},
	)

	// File System Diagnostic Tools

	// Register list_directory tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "list_directory",
			Description: "List files and directories at a specified path with details (name, size, permissions, modification time)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to list (default: /var/log/datadog)",
					},
				},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleListDirectory(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register read_file tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "read_file",
			Description: "Read the contents of a text file (limited to 1MB for safety)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to read",
					},
				},
				"required": []string{"path"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleReadFile(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register tail_file tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "tail_file",
			Description: "Read the last N lines of a file (useful for log files)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to tail",
					},
					"lines": map[string]interface{}{
						"type":        "number",
						"description": "Number of lines to read from the end (default: 50, max: 1000)",
					},
				},
				"required": []string{"path"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleTailFile(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register check_file_stats tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "check_file_stats",
			Description: "Get file metadata including size, permissions, timestamps, and owner",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file or directory",
					},
				},
				"required": []string{"path"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleCheckFileStats(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register find_files tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "find_files",
			Description: "Search for files matching a pattern in a directory (recursive)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Starting directory path for search",
					},
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Filename pattern to match (supports * and ? wildcards)",
					},
					"max_results": map[string]interface{}{
						"type":        "number",
						"description": "Maximum number of results to return (default: 100, max: 500)",
					},
				},
				"required": []string{"path", "pattern"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleFindFiles(
				ctx,
				req,
				clientID,
			)
		},
	)

	// SRE Diagnostic Tools

	// Register test_tcp_connection tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "test_tcp_connection",
			Description: "Test TCP connectivity to a remote host and port",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"host": map[string]interface{}{
						"type":        "string",
						"description": "Target hostname or IP address",
					},
					"port": map[string]interface{}{
						"type":        "number",
						"description": "Target port number",
					},
					"timeout": map[string]interface{}{
						"type":        "number",
						"description": "Connection timeout in seconds (default: 5, max: 30)",
					},
				},
				"required": []string{"host", "port"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleTestTCPConnection(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register grep_log tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "grep_log",
			Description: "Search for patterns in log files with context lines",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to log file to search",
					},
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Regular expression pattern to search for",
					},
					"case_sensitive": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether search should be case sensitive (default: false)",
					},
					"context_lines": map[string]interface{}{
						"type":        "number",
						"description": "Number of lines to show before and after each match (default: 2, max: 10)",
					},
					"max_matches": map[string]interface{}{
						"type":        "number",
						"description": "Maximum number of matches to return (default: 100, max: 500)",
					},
				},
				"required": []string{"path", "pattern"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleGrepLog(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register get_process_stats tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "get_process_stats",
			Description: "Get detailed resource usage statistics for a process (cross-platform: Linux and macOS)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pid": map[string]interface{}{
						"type":        "number",
						"description": "Process ID (PID) to get stats for",
					},
					"process_name": map[string]interface{}{
						"type":        "string",
						"description": "Process name (if PID not provided, will find first matching process)",
					},
				},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleGetProcessStats(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register resolve_hostname tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "resolve_hostname",
			Description: "Resolve a hostname to IP addresses (DNS lookup)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"hostname": map[string]interface{}{
						"type":        "string",
						"description": "Hostname to resolve (e.g., api.datadog.com)",
					},
				},
				"required": []string{"hostname"},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleResolveHostname(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register get_system_overview tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "get_system_overview",
			Description: "Get comprehensive system information including OS, uptime, load averages, memory, disk usage, and failed services",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleGetSystemOverview(
				ctx,
				req,
				clientID,
			)
		},
	)

	// Register check_listening_ports tool
	mcpSrv.AddTool(
		&mcp.Tool{
			Name:        "check_listening_ports",
			Description: "List all TCP/UDP ports in LISTEN state with associated processes",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"protocol": map[string]interface{}{
						"type":        "string",
						"description": "Protocol to filter (tcp, udp, all). Default: all",
						"enum":        []string{"tcp", "udp", "all"},
					},
					"port": map[string]interface{}{
						"type":        "number",
						"description": "Optional port number to filter",
					},
				},
			},
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
		) (
			*mcp.CallToolResult,
			error,
		) {
			return s.handleCheckListeningPorts(
				ctx,
				req,
				clientID,
			)
		},
	)
}

// Stop stops the MCP server
func (s *mcpServer) Stop() error {
	if !s.running.Load() {
		return nil
	}

	s.logger.Info("Stopping MCP server")
	close(s.stopChan)
	s.running.Store(false)

	// Close the listener to stop accepting new connections
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			s.logger.Warnf(
				"Error closing MCP listener: %v",
				err,
			)
		}
	}

	// Cancel context to stop all client handlers
	s.cancel()

	// Wait for all goroutines to finish
	s.wg.Wait()

	// Remove socket file
	socketPath := s.config.Get().SocketPath
	if err := os.RemoveAll(socketPath); err != nil {
		s.logger.Warnf(
			"Error removing socket file: %v",
			err,
		)
	}

	s.logger.Info("MCP server stopped")
	return nil
}

// handleGetAgentStatus returns the agent status
func (s *mcpServer) handleGetAgentStatus(
	ctx context.Context,
	params map[string]interface{},
	clientID int32,
) (
	interface{},
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: get_agent_status called",
		clientID,
	)

	// Return basic status information
	statusInfo := map[string]interface{}{
		"running":        s.running.Load(),
		"version":        "7.x.x",
		"active_clients": s.clientCount.Load(),
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf(
					"Datadog Agent Status: %v",
					statusInfo,
				),
			},
		},
	}, nil
}

// handleSendMetric sends a metric to Datadog
func (s *mcpServer) handleSendMetric(
	ctx context.Context,
	params map[string]interface{},
	clientID int32,
) (
	interface{},
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: send_metric called",
		clientID,
	)

	// Extract parameters
	metricName, ok := params["metric_name"].(string)
	if !ok {
		return nil, fmt.Errorf("metric_name is required and must be a string")
	}

	value, ok := params["value"].(float64)
	if !ok {
		return nil, fmt.Errorf("value is required and must be a number")
	}

	// Extract tags if provided
	var tags []string
	if tagsRaw, ok := params["tags"].([]interface{}); ok {
		for _, tag := range tagsRaw {
			if tagStr, ok := tag.(string); ok {
				tags = append(
					tags,
					tagStr,
				)
			}
		}
	}

	// Get the default sender
	sender, err := s.demultiplexer.GetDefaultSender()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get sender: %w",
			err,
		)
	}

	// Send the metric as a gauge
	sender.Gauge(
		metricName,
		value,
		"",
		tags,
	)

	s.logger.Infof(
		"MCP client #%d: Sent metric %s=%f with tags %v",
		clientID,
		metricName,
		value,
		tags,
	)

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf(
					"Metric %s sent successfully with value %f",
					metricName,
					value,
				),
			},
		},
	}, nil
}

// handleCheckCPUUsage checks current CPU usage
func (s *mcpServer) handleCheckCPUUsage(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: check_cpu_usage called",
		clientID,
	)

	// Get CPU count
	numCPU := runtime.NumCPU()

	// Read /proc/loadavg for load average (Linux-specific)
	loadavg := "N/A"
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		loadavg = string(data)
	}

	result := fmt.Sprintf(
		"CPU Info:\n- Number of CPUs: %d\n- Load Average: %s",
		numCPU,
		loadavg,
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleCheckMemoryUsage checks current memory usage
func (s *mcpServer) handleCheckMemoryUsage(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: check_memory_usage called",
		clientID,
	)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	result := fmt.Sprintf(
		"Memory Usage:\n- Allocated: %d MB\n- Total Allocated: %d MB\n- System: %d MB\n- GC Count: %d",
		m.Alloc/1024/1024,
		m.TotalAlloc/1024/1024,
		m.Sys/1024/1024,
		m.NumGC,
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleCheckDiskUsage checks disk usage for a path
func (s *mcpServer) handleCheckDiskUsage(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: check_disk_usage called",
		clientID,
	)

	// Parse arguments
	var params map[string]interface{}
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(
			req.Params.Arguments,
			&params,
		); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf(
							"Failed to parse arguments: %v",
							err,
						),
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Get path parameter, default to "/"
	path := "/"
	if p, ok := params["path"].(string); ok && p != "" {
		path = p
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(
		path,
		&stat,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to get disk usage for %s: %v",
						path,
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	// Calculate usage
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	usedPercent := float64(used) / float64(total) * 100

	result := fmt.Sprintf(
		"Disk Usage for %s:\n- Total: %d GB\n- Used: %d GB\n- Free: %d GB\n- Usage: %.2f%%",
		path,
		total/1024/1024/1024,
		used/1024/1024/1024,
		free/1024/1024/1024,
		usedPercent,
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleCheckProcess checks if a process is running
func (s *mcpServer) handleCheckProcess(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: check_process called",
		clientID,
	)

	// Parse arguments
	var params map[string]interface{}
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to parse arguments: %v",
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	processName, ok := params["process_name"].(string)
	if !ok {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "process_name is required and must be a string",
				},
			},
			IsError: true,
		}, nil
	}

	// Use pgrep to check for the process
	cmd := exec.CommandContext(
		ctx,
		"pgrep",
		"-x",
		processName,
	)
	output, err := cmd.Output()

	var result string
	if err != nil {
		result = fmt.Sprintf(
			"Process '%s' is NOT running",
			processName,
		)
	} else {
		pids := string(output)
		result = fmt.Sprintf(
			"Process '%s' is running with PIDs:\n%s",
			processName,
			pids,
		)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleListDirectory lists files and directories at a path
func (s *mcpServer) handleListDirectory(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"[MCP Server] Client #%d: list_directory called",
		clientID,
	)

	// Parse arguments into struct
	var params listDirectoryParams
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(
			req.Params.Arguments,
			&params,
		); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf(
							"Failed to parse arguments: %v",
							err,
						),
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Get path parameter, default to /var/log/datadog
	path := params.Path
	if path == "" {
		path = "/var/log/datadog"
	}

	// Read directory
	entries, err := os.ReadDir(path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to read directory %s: %v",
						path,
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	// Build output
	var result string
	result = fmt.Sprintf("Directory listing for %s:\n\n", path)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			result += fmt.Sprintf("%-40s ERROR: %v\n", entry.Name(), err)
			continue
		}

		// Format: type, permissions, size, modified time, name
		fileType := "-"
		if entry.IsDir() {
			fileType = "d"
		}

		result += fmt.Sprintf(
			"%s %s %10d %s  %s\n",
			fileType,
			info.Mode().String(),
			info.Size(),
			info.ModTime().Format("2006-01-02 15:04:05"),
			entry.Name(),
		)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleReadFile reads the contents of a file
func (s *mcpServer) handleReadFile(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: read_file called",
		clientID,
	)

	// Parse arguments into struct
	var params readFileParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to parse arguments: %v",
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	if params.Path == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "path is required",
				},
			},
			IsError: true,
		}, nil
	}

	// Check file size first (limit to 1MB)
	fileInfo, err := os.Stat(params.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to stat file %s: %v",
						params.Path,
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	const maxFileSize = 1024 * 1024 // 1MB
	if fileInfo.Size() > maxFileSize {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"File %s is too large (%d bytes). Maximum size is %d bytes. Use tail_file instead.",
						params.Path,
						fileInfo.Size(),
						maxFileSize,
					),
				},
			},
			IsError: true,
		}, nil
	}

	// Read the file
	content, err := os.ReadFile(params.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to read file %s: %v",
						params.Path,
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	result := fmt.Sprintf(
		"Contents of %s (%d bytes):\n\n%s",
		params.Path,
		len(content),
		string(content),
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleTailFile reads the last N lines of a file
func (s *mcpServer) handleTailFile(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: tail_file called",
		clientID,
	)

	// Parse arguments into struct
	var params tailFileParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to parse arguments: %v",
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	if params.Path == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "path is required",
				},
			},
			IsError: true,
		}, nil
	}

	// Get number of lines, default to 50, max 1000
	lines := params.Lines
	if lines == 0 {
		lines = 50
	}
	if lines > 1000 {
		lines = 1000
	}
	if lines < 1 {
		lines = 1
	}

	// Open the file
	file, err := os.Open(params.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to open file %s: %v",
						params.Path,
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}
	defer file.Close()

	// Read the file line by line, keeping only the last N lines
	scanner := bufio.NewScanner(file)
	var lineBuffer []string

	for scanner.Scan() {
		lineBuffer = append(lineBuffer, scanner.Text())
		if len(lineBuffer) > lines {
			lineBuffer = lineBuffer[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Error reading file %s: %v",
						params.Path,
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	result := fmt.Sprintf(
		"Last %d lines of %s:\n\n",
		len(lineBuffer),
		params.Path,
	)

	// Join all lines
	for _, line := range lineBuffer {
		result += line + "\n"
	}

	if len(lineBuffer) == 0 {
		result = fmt.Sprintf("File %s is empty", params.Path)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleCheckFileStats gets file metadata
func (s *mcpServer) handleCheckFileStats(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: check_file_stats called",
		clientID,
	)

	// Parse arguments into struct
	var params checkFileStatsParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to parse arguments: %v",
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	if params.Path == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "path is required",
				},
			},
			IsError: true,
		}, nil
	}

	// Get file info
	fileInfo, err := os.Stat(params.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to stat file %s: %v",
						params.Path,
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	// Determine file type
	fileType := "file"
	if fileInfo.IsDir() {
		fileType = "directory"
	} else if fileInfo.Mode()&os.ModeSymlink != 0 {
		fileType = "symlink"
	}

	// Get owner information (Unix-specific)
	var ownerInfo string
	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		ownerInfo = fmt.Sprintf(
			"UID: %d, GID: %d",
			stat.Uid,
			stat.Gid,
		)
	} else {
		ownerInfo = "N/A"
	}

	result := fmt.Sprintf(
		"File statistics for %s:\n\n"+
			"Type: %s\n"+
			"Size: %d bytes\n"+
			"Permissions: %s\n"+
			"Owner: %s\n"+
			"Modified: %s\n",
		params.Path,
		fileType,
		fileInfo.Size(),
		fileInfo.Mode().String(),
		ownerInfo,
		fileInfo.ModTime().Format("2006-01-02 15:04:05"),
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleFindFiles searches for files matching a pattern
func (s *mcpServer) handleFindFiles(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: find_files called",
		clientID,
	)

	// Parse arguments into struct
	var params findFilesParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to parse arguments: %v",
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	if params.Path == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "path is required",
				},
			},
			IsError: true,
		}, nil
	}

	if params.Pattern == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "pattern is required",
				},
			},
			IsError: true,
		}, nil
	}

	// Get max results, default to 100, max 500
	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = 100
	}
	if maxResults > 500 {
		maxResults = 500
	}
	if maxResults < 1 {
		maxResults = 1
	}

	// Use find command to search for files
	cmd := exec.CommandContext(
		ctx,
		"find",
		params.Path,
		"-name",
		params.Pattern,
		"-type",
		"f",
	)

	output, err := cmd.Output()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to search for files: %v",
						err,
					),
				},
			},
			IsError: true,
		}, nil
	}

	// Parse results
	lines := []string{}
	scanner := bufio.NewScanner(io.Reader(bytes.NewReader(output)))
	for scanner.Scan() && len(lines) < maxResults {
		lines = append(lines, scanner.Text())
	}

	result := fmt.Sprintf(
		"Found %d files matching '%s' in %s",
		len(lines),
		params.Pattern,
		params.Path,
	)

	if len(lines) >= maxResults {
		result += fmt.Sprintf(" (limited to %d results)", maxResults)
	}

	result += ":\n\n"

	for _, line := range lines {
		result += line + "\n"
	}

	if len(lines) == 0 {
		result = fmt.Sprintf(
			"No files found matching '%s' in %s",
			params.Pattern,
			params.Path,
		)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// SRE Diagnostic Tool Handlers

// handleTestTCPConnection tests TCP connectivity to a remote host
func (s *mcpServer) handleTestTCPConnection(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: test_tcp_connection called",
		clientID,
	)

	// Parse parameters
	var params testTCPConnectionParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Failed to parse arguments: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Validate required parameters
	if params.Host == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "host is required",
				},
			},
			IsError: true,
		}, nil
	}

	if params.Port <= 0 || params.Port > 65535 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "port must be between 1 and 65535",
				},
			},
			IsError: true,
		}, nil
	}

	// Apply safety limits for timeout
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 5 // default 5 seconds
	}
	if timeout > 30 {
		timeout = 30 // max 30 seconds
	}
	if timeout < 1 {
		timeout = 1 // min 1 second
	}

	// Test TCP connection
	address := fmt.Sprintf("%s:%d", params.Host, params.Port)
	start := time.Now()

	conn, err := net.DialTimeout(
		"tcp",
		address,
		time.Duration(timeout)*time.Second,
	)

	latency := time.Since(start)

	if err != nil {
		// Connection failed
		result := fmt.Sprintf(
			"TCP Connection Test: %s\nStatus: FAILED\nError: %v\nAttempted at: %s\nTimeout: %d seconds",
			address,
			err,
			start.Format(time.RFC3339),
			timeout,
		)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: result,
				},
			},
		}, nil
	}

	// Connection successful, close immediately
	conn.Close()

	result := fmt.Sprintf(
		"TCP Connection Test: %s\nStatus: SUCCESS\nLatency: %dms\nConnected at: %s",
		address,
		latency.Milliseconds(),
		start.Format(time.RFC3339),
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// handleGrepLog searches for patterns in log files
func (s *mcpServer) handleGrepLog(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: grep_log called",
		clientID,
	)

	// Parse parameters
	var params grepLogParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Failed to parse arguments: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Validate required parameters
	if params.Path == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "path is required",
				},
			},
			IsError: true,
		}, nil
	}

	if params.Pattern == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "pattern is required",
				},
			},
			IsError: true,
		}, nil
	}

	// Apply safety limits
	contextLines := params.ContextLines
	if contextLines == 0 {
		contextLines = 2 // default
	}
	if contextLines > 10 {
		contextLines = 10 // max
	}
	if contextLines < 0 {
		contextLines = 0
	}

	maxMatches := params.MaxMatches
	if maxMatches == 0 {
		maxMatches = 100 // default
	}
	if maxMatches > 500 {
		maxMatches = 500 // max
	}
	if maxMatches < 1 {
		maxMatches = 1
	}

	// Check file size (max 100MB)
	fileInfo, err := os.Stat(params.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Failed to stat file %s: %v", params.Path, err),
				},
			},
			IsError: true,
		}, nil
	}

	const maxFileSize = 100 * 1024 * 1024 // 100MB
	if fileInfo.Size() > maxFileSize {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf(
						"File %s is too large (%d bytes). Maximum size is %d bytes (100MB).",
						params.Path,
						fileInfo.Size(),
						maxFileSize,
					),
				},
			},
			IsError: true,
		}, nil
	}

	// Compile regex
	var regex *regexp.Regexp
	if params.CaseSensitive {
		regex, err = regexp.Compile(params.Pattern)
	} else {
		regex, err = regexp.Compile("(?i)" + params.Pattern)
	}
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Invalid regex pattern: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Read and search file
	file, err := os.Open(params.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Failed to open file %s: %v", params.Path, err),
				},
			},
			IsError: true,
		}, nil
	}
	defer file.Close()

	type match struct {
		lineNum int
		line    string
		before  []string
		after   []string
	}

	var matches []match
	var beforeBuffer []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	lastMatchLine := -1
	afterLinesNeeded := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// If we're collecting "after" lines for a previous match
		if afterLinesNeeded > 0 {
			if len(matches) > 0 {
				matches[len(matches)-1].after = append(matches[len(matches)-1].after, line)
			}
			afterLinesNeeded--
		}

		// Check if this line matches
		if regex.MatchString(line) {
			// Stop if we've reached max matches
			if len(matches) >= maxMatches {
				break
			}

			m := match{
				lineNum: lineNum,
				line:    line,
				before:  make([]string, len(beforeBuffer)),
				after:   []string{},
			}
			copy(m.before, beforeBuffer)
			matches = append(matches, m)
			lastMatchLine = lineNum
			afterLinesNeeded = contextLines
		}

		// Maintain before buffer (ring buffer)
		if lineNum > lastMatchLine+contextLines {
			beforeBuffer = append(beforeBuffer, line)
			if len(beforeBuffer) > contextLines {
				beforeBuffer = beforeBuffer[1:]
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Error reading file: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Format results
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Grep Results: %s\n", params.Path))
	result.WriteString(fmt.Sprintf("Pattern: %q\n", params.Pattern))
	result.WriteString(fmt.Sprintf("Matches: %d", len(matches)))
	if len(matches) >= maxMatches {
		result.WriteString(fmt.Sprintf(" (limited to %d)", maxMatches))
	}
	result.WriteString("\n\n")

	for i, m := range matches {
		result.WriteString(fmt.Sprintf("--- Match %d (line %d) ---\n", i+1, m.lineNum))

		// Before context
		for j, beforeLine := range m.before {
			result.WriteString(fmt.Sprintf("%d: %s\n", m.lineNum-len(m.before)+j, beforeLine))
		}

		// Matching line
		result.WriteString(fmt.Sprintf("%d: %s\n", m.lineNum, m.line))

		// After context
		for j, afterLine := range m.after {
			result.WriteString(fmt.Sprintf("%d: %s\n", m.lineNum+j+1, afterLine))
		}

		result.WriteString("\n")
	}

	if len(matches) == 0 {
		result.WriteString("No matches found.\n")
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result.String(),
			},
		},
	}, nil
}

// handleGetProcessStats gets detailed process statistics (cross-platform)
func (s *mcpServer) handleGetProcessStats(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: get_process_stats called",
		clientID,
	)

	// Parse parameters
	var params getProcessStatsParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Failed to parse arguments: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Need either PID or ProcessName
	if params.PID == 0 && params.ProcessName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "Either pid or process_name is required",
				},
			},
			IsError: true,
		}, nil
	}

	// If ProcessName provided but not PID, find PID using pgrep
	pid := params.PID
	if pid == 0 && params.ProcessName != "" {
		cmd := exec.CommandContext(ctx, "pgrep", "-n", params.ProcessName)
		output, err := cmd.Output()
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("Process '%s' not found", params.ProcessName),
					},
				},
				IsError: true,
			}, nil
		}
		pidStr := strings.TrimSpace(string(output))
		pid, err = strconv.Atoi(pidStr)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("Failed to parse PID: %v", err),
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Platform-specific implementation
	var result string
	var err error

	switch runtime.GOOS {
	case "linux":
		result, err = s.getProcessStatsLinux(pid)
	case "darwin":
		result, err = s.getProcessStatsDarwin(pid)
	default:
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Platform %s not supported", runtime.GOOS),
				},
			},
			IsError: true,
		}, nil
	}

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Failed to get process stats: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: result,
			},
		},
	}, nil
}

// getProcessStatsLinux gets process stats on Linux
func (s *mcpServer) getProcessStatsLinux(pid int) (string, error) {
	var warnings []string
	var result strings.Builder

	result.WriteString(fmt.Sprintf("Process Stats: PID %d\n", pid))

	// Get command line
	cmdlineData, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("⚠️  Could not read command line: %v", err))
		result.WriteString("Command: [Permission Denied]\n")
	} else {
		cmdline := strings.ReplaceAll(string(cmdlineData), "\x00", " ")
		cmdline = strings.TrimSpace(cmdline)
		if cmdline == "" {
			cmdline = "[kernel thread]"
		}
		result.WriteString(fmt.Sprintf("Command: %s\n", cmdline))
	}

	// Get memory stats from /proc/[pid]/status
	statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("⚠️  Could not read status: %v", err))
		result.WriteString("Memory: [Permission Denied]\n")
	} else {
		statusLines := strings.Split(string(statusData), "\n")
		var vmRSS, vmSize string
		for _, line := range statusLines {
			if strings.HasPrefix(line, "VmRSS:") {
				vmRSS = strings.TrimSpace(strings.TrimPrefix(line, "VmRSS:"))
			} else if strings.HasPrefix(line, "VmSize:") {
				vmSize = strings.TrimSpace(strings.TrimPrefix(line, "VmSize:"))
			}
		}
		if vmRSS != "" || vmSize != "" {
			result.WriteString(fmt.Sprintf("Memory: RSS: %s, VSZ: %s\n", vmRSS, vmSize))
		}
	}

	// Get state and CPU from /proc/[pid]/stat
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("⚠️  Could not read stat: %v", err))
		result.WriteString("State: [Permission Denied]\n")
	} else {
		statFields := strings.Fields(string(statData))
		if len(statFields) > 2 {
			state := statFields[2]
			stateMap := map[string]string{
				"R": "Running",
				"S": "Sleeping",
				"D": "Waiting (uninterruptible)",
				"Z": "Zombie",
				"T": "Stopped",
			}
			stateStr, ok := stateMap[state]
			if !ok {
				stateStr = state
			}
			result.WriteString(fmt.Sprintf("State: %s\n", stateStr))
		}
	}

	// Add warnings if any
	if len(warnings) > 0 {
		result.WriteString("\nWarnings:\n")
		for _, w := range warnings {
			result.WriteString(w + "\n")
		}
		result.WriteString("\nNote: Some information unavailable due to permissions. Run agent with elevated privileges for complete data.\n")
	}

	return result.String(), nil
}

// getProcessStatsDarwin gets process stats on macOS
func (s *mcpServer) getProcessStatsDarwin(pid int) (string, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "pid=,ppid=,%cpu=,%mem=,vsz=,rss=,state=,lstart=,comm=")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("process not found or error running ps: %v", err)
	}

	fields := strings.Fields(string(output))
	if len(fields) < 9 {
		return "", fmt.Errorf("unexpected ps output format")
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Process Stats: PID %s\n", fields[0]))
	result.WriteString(fmt.Sprintf("Parent PID: %s\n", fields[1]))
	result.WriteString(fmt.Sprintf("CPU: %s%%\n", fields[2]))
	result.WriteString(fmt.Sprintf("Memory: %s%% (VSZ: %s KB, RSS: %s KB)\n", fields[3], fields[4], fields[5]))
	result.WriteString(fmt.Sprintf("State: %s\n", fields[6]))

	// Parse start time (fields[7] onwards until command)
	// lstart format: "Fri Jan  5 14:23:15 2026"
	startIdx := 7
	var startTime strings.Builder
	for i := startIdx; i < len(fields)-1; i++ {
		if i > startIdx {
			startTime.WriteString(" ")
		}
		startTime.WriteString(fields[i])
	}
	result.WriteString(fmt.Sprintf("Started: %s\n", startTime.String()))

	// Command is the last field
	result.WriteString(fmt.Sprintf("Command: %s\n", fields[len(fields)-1]))

	return result.String(), nil
}

// handleResolveHostname handles the resolve_hostname tool call
func (s *mcpServer) handleResolveHostname(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: resolve_hostname called",
		clientID,
	)

	// Parse parameters
	var params resolveHostnameParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf(
					"Failed to parse arguments: %v",
					err,
				),
			}},
			IsError: true,
		}, nil
	}

	// Validate parameters
	if params.Hostname == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: "hostname parameter is required",
			}},
			IsError: true,
		}, nil
	}

	// Create a context with 10 second timeout
	ctxWithTimeout, cancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)
	defer cancel()

	// Perform DNS lookup with timing
	start := time.Now()
	ips, err := net.DefaultResolver.LookupIP(
		ctxWithTimeout,
		"ip",
		params.Hostname,
	)
	latency := time.Since(start)

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf(
					"DNS Resolution: %s\nStatus: FAILED\nError: %v\nAttempted at: %s\n",
					params.Hostname,
					err,
					time.Now().UTC().Format(time.RFC3339),
				),
			}},
			IsError: false, // Not an error - just failed resolution
		}, nil
	}

	// Build result
	var result strings.Builder
	result.WriteString(
		fmt.Sprintf("DNS Resolution: %s\n", params.Hostname),
	)
	result.WriteString("Status: SUCCESS\n")
	result.WriteString("Resolved IPs:\n")

	// Limit to 100 IPs per safety requirements
	maxIPs := 100
	if len(ips) > maxIPs {
		result.WriteString(
			fmt.Sprintf("  (showing first %d of %d IPs)\n", maxIPs, len(ips)),
		)
		ips = ips[:maxIPs]
	}

	// Categorize and display IPs
	ipv4Count := 0
	ipv6Count := 0
	for _, ip := range ips {
		if ip.To4() != nil {
			result.WriteString(fmt.Sprintf("  - %s (IPv4)\n", ip.String()))
			ipv4Count++
		} else {
			result.WriteString(fmt.Sprintf("  - %s (IPv6)\n", ip.String()))
			ipv6Count++
		}
	}

	result.WriteString(
		fmt.Sprintf("\nResolution time: %dms\n", latency.Milliseconds()),
	)
	result.WriteString(
		fmt.Sprintf("Total: %d IPv4, %d IPv6\n", ipv4Count, ipv6Count),
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: result.String(),
		}},
	}, nil
}

// handleGetSystemOverview handles the get_system_overview tool call
func (s *mcpServer) handleGetSystemOverview(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: get_system_overview called",
		clientID,
	)

	// Create a context with 5 second timeout
	ctxWithTimeout, cancel := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer cancel()

	// Get platform-specific system overview
	var overview string
	var err error

	switch runtime.GOOS {
	case "linux":
		overview, err = s.getSystemOverviewLinux(ctxWithTimeout)
	case "darwin":
		overview, err = s.getSystemOverviewDarwin(ctxWithTimeout)
	default:
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf(
					"Platform %s not supported",
					runtime.GOOS,
				),
			}},
			IsError: true,
		}, nil
	}

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf(
					"Failed to get system overview: %v",
					err,
				),
			}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: overview,
		}},
	}, nil
}

// getSystemOverviewLinux gets system overview on Linux
func (s *mcpServer) getSystemOverviewLinux(
	ctx context.Context,
) (
	string,
	error,
) {
	var result strings.Builder
	var warnings []string
	var cmd *exec.Cmd

	result.WriteString("System Overview:\n")

	// Get OS info from /etc/os-release
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osName := strings.Trim(
					strings.TrimPrefix(line, "PRETTY_NAME="),
					"\"",
				)
				result.WriteString(fmt.Sprintf("  OS: %s\n", osName))
				break
			}
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not read OS info: %v", err),
		)
		result.WriteString("  OS: Unknown\n")
	}

	// Get kernel version using uname command (cross-platform)
	cmd = exec.CommandContext(ctx, "uname", "-r")
	if output, err := cmd.Output(); err == nil {
		kernel := strings.TrimSpace(string(output))
		result.WriteString(fmt.Sprintf("  Kernel: %s\n", kernel))
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not get kernel info: %v", err),
		)
	}

	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		result.WriteString(fmt.Sprintf("  Hostname: %s\n", hostname))
	}

	// Get uptime from /proc/uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if uptimeSec, err := strconv.ParseFloat(fields[0], 64); err == nil {
				days := int(uptimeSec / 86400)
				hours := int((uptimeSec - float64(days*86400)) / 3600)
				minutes := int((uptimeSec - float64(days*86400) - float64(hours*3600)) / 60)
				result.WriteString(
					fmt.Sprintf("  Uptime: %d days, %dh %dm\n", days, hours, minutes),
				)
			}
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not read uptime: %v", err),
		)
	}

	// Get load averages from /proc/loadavg
	result.WriteString("\n  Load Averages:\n")
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			result.WriteString(fmt.Sprintf("    1 min:  %s\n", fields[0]))
			result.WriteString(fmt.Sprintf("    5 min:  %s\n", fields[1]))
			result.WriteString(fmt.Sprintf("    15 min: %s\n", fields[2]))
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not read load average: %v", err),
		)
		result.WriteString("    (unavailable)\n")
	}

	// Get CPU count from /proc/cpuinfo
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cpuCount := strings.Count(string(data), "processor\t:")
		result.WriteString(fmt.Sprintf("\n  CPUs: %d cores\n", cpuCount))
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not read CPU info: %v", err),
		)
	}

	// Get memory info from /proc/meminfo
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		var memTotal, memAvailable int64
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					memTotal, _ = strconv.ParseInt(fields[1], 10, 64)
				}
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					memAvailable, _ = strconv.ParseInt(fields[1], 10, 64)
				}
			}
		}
		if memTotal > 0 {
			memTotalGB := float64(memTotal) / 1024 / 1024
			memAvailGB := float64(memAvailable) / 1024 / 1024
			memUsedPercent := float64(memTotal-memAvailable) / float64(memTotal) * 100
			result.WriteString(
				fmt.Sprintf(
					"  Memory: %.1f GB available / %.1f GB total (%.0f%% used)\n",
					memAvailGB,
					memTotalGB,
					memUsedPercent,
				),
			)
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not read memory info: %v", err),
		)
	}

	// Get disk usage for root filesystem using df command
	cmd = exec.CommandContext(ctx, "df", "-h", "/")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 6 {
				result.WriteString(
					fmt.Sprintf(
						"  Disk (/): %s available / %s total (%s used)\n",
						fields[3],
						fields[1],
						fields[4],
					),
				)
			}
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not get disk usage: %v", err),
		)
	}

	// Get failed systemd services
	result.WriteString("\n  Failed Services: ")
	cmd = exec.CommandContext(ctx, "systemctl", "list-units", "--state=failed", "--no-pager", "--no-legend")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		failedCount := 0
		if len(lines) > 0 && lines[0] != "" {
			failedCount = len(lines)
			result.WriteString(fmt.Sprintf("%d\n", failedCount))
			for i, line := range lines {
				if i >= 5 {
					result.WriteString(fmt.Sprintf("    (and %d more...)\n", len(lines)-5))
					break
				}
				fields := strings.Fields(line)
				if len(fields) > 0 {
					result.WriteString(fmt.Sprintf("    - %s\n", fields[0]))
				}
			}
		} else {
			result.WriteString("0 (all services healthy)\n")
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not check failed services: %v", err),
		)
		result.WriteString("(unavailable)\n")
	}

	// Add warnings if any
	if len(warnings) > 0 {
		result.WriteString("\nWarnings:\n")
		for _, w := range warnings {
			result.WriteString(fmt.Sprintf("  ⚠️  %s\n", w))
		}
	}

	return result.String(), nil
}

// getSystemOverviewDarwin gets system overview on macOS
func (s *mcpServer) getSystemOverviewDarwin(
	ctx context.Context,
) (
	string,
	error,
) {
	var result strings.Builder
	var warnings []string

	result.WriteString("System Overview:\n")

	// Get OS version using sw_vers
	cmd := exec.CommandContext(ctx, "sw_vers", "-productName")
	if output, err := cmd.Output(); err == nil {
		osName := strings.TrimSpace(string(output))
		cmd = exec.CommandContext(ctx, "sw_vers", "-productVersion")
		if versionOutput, err := cmd.Output(); err == nil {
			osVersion := strings.TrimSpace(string(versionOutput))
			result.WriteString(fmt.Sprintf("  OS: %s %s\n", osName, osVersion))
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not get OS version: %v", err),
		)
		result.WriteString("  OS: macOS (version unknown)\n")
	}

	// Get kernel version using uname
	cmd = exec.CommandContext(ctx, "uname", "-r")
	if output, err := cmd.Output(); err == nil {
		kernel := strings.TrimSpace(string(output))
		result.WriteString(fmt.Sprintf("  Kernel: Darwin %s\n", kernel))
	}

	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		result.WriteString(fmt.Sprintf("  Hostname: %s\n", hostname))
	}

	// Get uptime using uptime command
	cmd = exec.CommandContext(ctx, "uptime")
	if output, err := cmd.Output(); err == nil {
		uptimeStr := strings.TrimSpace(string(output))
		// Parse uptime output (format: "up X days, HH:MM")
		if idx := strings.Index(uptimeStr, "up "); idx >= 0 {
			uptimeStr = uptimeStr[idx+3:]
			if idx := strings.Index(uptimeStr, ","); idx >= 0 {
				if idx2 := strings.Index(uptimeStr[idx:], "user"); idx2 >= 0 {
					uptimeStr = strings.TrimSpace(uptimeStr[:idx+idx2])
				}
			}
			result.WriteString(fmt.Sprintf("  Uptime: %s\n", uptimeStr))
		}

		// Extract load averages (at the end after "load averages:")
		if idx := strings.Index(string(output), "load average"); idx >= 0 {
			loadStr := strings.TrimSpace(string(output)[idx:])
			if idx := strings.Index(loadStr, ":"); idx >= 0 {
				loads := strings.TrimSpace(loadStr[idx+1:])
				loadFields := strings.Split(loads, " ")
				if len(loadFields) >= 3 {
					result.WriteString("\n  Load Averages:\n")
					result.WriteString(fmt.Sprintf("    1 min:  %s\n", strings.TrimSpace(loadFields[0])))
					result.WriteString(fmt.Sprintf("    5 min:  %s\n", strings.TrimSpace(loadFields[1])))
					result.WriteString(fmt.Sprintf("    15 min: %s\n", strings.TrimSpace(loadFields[2])))
				}
			}
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not get uptime: %v", err),
		)
	}

	// Get CPU count using sysctl
	cmd = exec.CommandContext(ctx, "sysctl", "-n", "hw.ncpu")
	if output, err := cmd.Output(); err == nil {
		cpuCount := strings.TrimSpace(string(output))
		result.WriteString(fmt.Sprintf("\n  CPUs: %s cores\n", cpuCount))
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not get CPU count: %v", err),
		)
	}

	// Get memory info using vm_stat
	cmd = exec.CommandContext(ctx, "vm_stat")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		var pagesFree, pagesInactive int64
		pageSize := int64(4096) // default page size

		// Try to get actual page size
		cmd = exec.CommandContext(ctx, "sysctl", "-n", "hw.pagesize")
		if psOutput, err := cmd.Output(); err == nil {
			if ps, err := strconv.ParseInt(strings.TrimSpace(string(psOutput)), 10, 64); err == nil {
				pageSize = ps
			}
		}

		for _, line := range lines {
			if strings.HasPrefix(line, "Pages free:") {
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					val := strings.TrimSuffix(fields[2], ".")
					pagesFree, _ = strconv.ParseInt(val, 10, 64)
				}
			} else if strings.HasPrefix(line, "Pages inactive:") {
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					val := strings.TrimSuffix(fields[2], ".")
					pagesInactive, _ = strconv.ParseInt(val, 10, 64)
				}
			}
		}

		// Get total memory using sysctl
		cmd = exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize")
		if output, err := cmd.Output(); err == nil {
			if memTotal, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil {
				memTotalGB := float64(memTotal) / 1024 / 1024 / 1024
				memAvailBytes := (pagesFree + pagesInactive) * pageSize
				memAvailGB := float64(memAvailBytes) / 1024 / 1024 / 1024
				memUsedPercent := float64(memTotal-memAvailBytes) / float64(memTotal) * 100
				result.WriteString(
					fmt.Sprintf(
						"  Memory: %.1f GB available / %.1f GB total (%.0f%% used)\n",
						memAvailGB,
						memTotalGB,
						memUsedPercent,
					),
				)
			}
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not get memory info: %v", err),
		)
	}

	// Get disk usage for root filesystem using df command
	cmd = exec.CommandContext(ctx, "df", "-h", "/")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 6 {
				result.WriteString(
					fmt.Sprintf(
						"  Disk (/): %s available / %s total (%s used)\n",
						fields[3],
						fields[1],
						fields[4],
					),
				)
			}
		}
	} else {
		warnings = append(
			warnings,
			fmt.Sprintf("Could not get disk usage: %v", err),
		)
	}

	// Note: macOS doesn't have systemd
	result.WriteString("\n  Failed Services: (systemd not available on macOS)\n")

	// Add warnings if any
	if len(warnings) > 0 {
		result.WriteString("\nWarnings:\n")
		for _, w := range warnings {
			result.WriteString(fmt.Sprintf("  ⚠️  %s\n", w))
		}
	}

	return result.String(), nil
}

// handleCheckListeningPorts handles the check_listening_ports tool call
func (s *mcpServer) handleCheckListeningPorts(
	ctx context.Context,
	req *mcp.CallToolRequest,
	clientID int32,
) (
	*mcp.CallToolResult,
	error,
) {
	s.logger.Debugf(
		"MCP client #%d: check_listening_ports called",
		clientID,
	)

	// Parse parameters
	var params checkListeningPortsParams
	if err := json.Unmarshal(
		req.Params.Arguments,
		&params,
	); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf(
					"Failed to parse arguments: %v",
					err,
				),
			}},
			IsError: true,
		}, nil
	}

	// Set default protocol
	if params.Protocol == "" {
		params.Protocol = "all"
	}

	// Validate protocol
	if params.Protocol != "tcp" && params.Protocol != "udp" && params.Protocol != "all" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: "protocol must be 'tcp', 'udp', or 'all'",
			}},
			IsError: true,
		}, nil
	}

	// Create a context with 10 second timeout
	ctxWithTimeout, cancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)
	defer cancel()

	// Use lsof for cross-platform compatibility
	// Build lsof command based on protocol
	var lsofArgs []string
	lsofArgs = append(lsofArgs, "-i") // Internet connections
	lsofArgs = append(lsofArgs, "-P") // Don't resolve port names
	lsofArgs = append(lsofArgs, "-n") // Don't resolve hostnames

	// Add protocol-specific filters
	if params.Protocol == "tcp" {
		lsofArgs = append(lsofArgs, "-iTCP")
		lsofArgs = append(lsofArgs, "-sTCP:LISTEN")
	} else if params.Protocol == "udp" {
		// UDP doesn't have LISTEN state, just show all UDP
		lsofArgs = append(lsofArgs, "-iUDP")
	} else {
		// For "all", we need to run lsof twice
		// First get TCP LISTEN ports
		lsofArgs = append(lsofArgs, "-iTCP")
		lsofArgs = append(lsofArgs, "-sTCP:LISTEN")
	}

	// Execute lsof
	cmd := exec.CommandContext(ctxWithTimeout, "lsof", lsofArgs...)
	output, err := cmd.Output()

	var result strings.Builder
	result.WriteString("Listening Ports:\n")

	if err != nil {
		// Check if it's just no results (exit code 1)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			result.WriteString("  (no listening ports found)\n")
		} else {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{
					Text: fmt.Sprintf(
						"Failed to execute lsof: %v\nNote: This tool requires lsof to be installed",
						err,
					),
				}},
				IsError: true,
			}, nil
		}
	} else {
		// Parse lsof output
		lines := strings.Split(string(output), "\n")
		count := 0
		maxPorts := 1000

		// Skip header line
		for i, line := range lines {
			if i == 0 {
				continue // Skip header
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) < 9 {
				continue
			}

			process := fields[0]
			pid := fields[1]
			protocol := fields[7]
			address := fields[8]

			// Parse port from address (format: *:PORT or IP:PORT)
			portStr := ""
			if idx := strings.LastIndex(address, ":"); idx >= 0 {
				portStr = address[idx+1:]
			}

			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue // Skip if we can't parse port
			}

			// Apply port filter if specified
			if params.Port > 0 && port != params.Port {
				continue
			}

			// Format: Port | Protocol | Process | PID | Address
			result.WriteString(
				fmt.Sprintf(
					"  Port %-6d | %-5s | %-20s | PID %-8s | %s\n",
					port,
					protocol,
					process,
					pid,
					address,
				),
			)

			count++
			if count >= maxPorts {
				result.WriteString(
					fmt.Sprintf(
						"\n(showing first %d ports, %d more not displayed)\n",
						maxPorts,
						len(lines)-i-1,
					),
				)
				break
			}
		}

		if count == 0 && params.Port > 0 {
			result.WriteString(
				fmt.Sprintf(
					"  (no listening ports found for port %d)\n",
					params.Port,
				),
			)
		}

		result.WriteString(fmt.Sprintf("\nTotal: %d listening port(s)\n", count))
	}

	// If protocol is "all", also get UDP ports
	if params.Protocol == "all" && err == nil {
		udpArgs := []string{"-i", "-P", "-n", "-iUDP"}
		cmd = exec.CommandContext(ctxWithTimeout, "lsof", udpArgs...)
		udpOutput, udpErr := cmd.Output()

		if udpErr == nil {
			lines := strings.Split(string(udpOutput), "\n")
			udpCount := 0

			result.WriteString("\nUDP Ports:\n")
			for i, line := range lines {
				if i == 0 {
					continue // Skip header
				}
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				fields := strings.Fields(line)
				if len(fields) < 9 {
					continue
				}

				process := fields[0]
				pid := fields[1]
				protocol := fields[7]
				address := fields[8]

				// Parse port
				portStr := ""
				if idx := strings.LastIndex(address, ":"); idx >= 0 {
					portStr = address[idx+1:]
				}

				port, err := strconv.Atoi(portStr)
				if err != nil {
					continue
				}

				// Apply port filter if specified
				if params.Port > 0 && port != params.Port {
					continue
				}

				result.WriteString(
					fmt.Sprintf(
						"  Port %-6d | %-5s | %-20s | PID %-8s | %s\n",
						port,
						protocol,
						process,
						pid,
						address,
					),
				)

				udpCount++
				if udpCount >= 100 {
					result.WriteString("  (showing first 100 UDP ports)\n")
					break
				}
			}

			result.WriteString(fmt.Sprintf("\nTotal UDP: %d port(s)\n", udpCount))
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: result.String(),
		}},
	}, nil
}

// connTransport implements mcp.Transport using a net.Conn
type connTransport struct {
	conn net.Conn
}

// Connect implements mcp.Transport
func (t *connTransport) Connect(ctx context.Context) (
	mcp.Connection,
	error,
) {
	return &connConnection{
		conn:   t.conn,
		reader: bufio.NewReader(t.conn),
		writer: bufio.NewWriter(t.conn),
	}, nil
}

// connConnection implements mcp.Connection
type connConnection struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex
}

// Read implements mcp.Connection.Read
func (c *connConnection) Read(ctx context.Context) (
	jsonrpc.Message,
	error,
) {
	// Read a JSON-RPC message (newline-delimited)
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	// Parse the JSON-RPC 2.0 message using the SDK's decoder
	msg, err := jsonrpc.DecodeMessage(line)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse JSON-RPC message: %w",
			err,
		)
	}

	return msg, nil
}

// Write implements mcp.Connection.Write
func (c *connConnection) Write(
	ctx context.Context,
	msg jsonrpc.Message,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Serialize the JSON-RPC 2.0 message using the SDK's encoder
	data, err := jsonrpc.EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal JSON-RPC message: %w",
			err,
		)
	}

	// Write the message with newline delimiter
	if _, err := c.writer.Write(data); err != nil {
		return err
	}
	if _, err := c.writer.Write([]byte("\n")); err != nil {
		return err
	}

	return c.writer.Flush()
}

// Close implements mcp.Connection.Close
func (c *connConnection) Close() error {
	return c.conn.Close()
}

// SessionID implements mcp.Connection.SessionID
func (c *connConnection) SessionID() string {
	return c.conn.RemoteAddr().String()
}

// mcpStatusProvider implements status.Provider for the status page
type mcpStatusProvider struct {
	server *mcpServer
}

func (p mcpStatusProvider) Name() string {
	return "MCP Server"
}

func (p mcpStatusProvider) Section() string {
	return "MCP"
}

func (p mcpStatusProvider) JSON(
	_ bool,
	stats map[string]interface{},
) error {
	stats["enabled"] = p.server.config.Get().Enabled
	stats["running"] = p.server.running.Load()
	stats["socket_path"] = p.server.config.Get().SocketPath
	stats["active_clients"] = p.server.clientCount.Load()
	return nil
}

func (p mcpStatusProvider) Text(
	_ bool,
	buffer io.Writer,
) error {
	if p.server.running.Load() {
		fmt.Fprintf(
			buffer,
			"MCP Server: running on %s (%d clients)",
			p.server.config.Get().SocketPath,
			p.server.clientCount.Load(),
		)
	} else {
		buffer.Write([]byte("MCP Server: stopped"))
	}
	return nil
}

func (p mcpStatusProvider) HTML(
	_ bool,
	buffer io.Writer,
) error {
	if p.server.running.Load() {
		fmt.Fprintf(
			buffer,
			"<p>MCP Server: running on %s (%d clients)</p>",
			p.server.config.Get().SocketPath,
			p.server.clientCount.Load(),
		)
	} else {
		buffer.Write([]byte("<p>MCP Server: stopped</p>"))
	}
	return nil
}
