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
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"

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
