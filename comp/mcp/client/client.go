// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/fx"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	mcpconfig "github.com/DataDog/datadog-agent/comp/mcp/config"
)

// dependencies defines all components this client needs
type dependencies struct {
	fx.In
	Config mcpconfig.Component
	Logger log.Component
}

// mcpClient is the internal implementation
type mcpClient struct {
	config  mcpconfig.Component
	logger  log.Component
	client  *mcp.Client
	session *mcp.ClientSession
	conn    net.Conn
	mu      sync.RWMutex
}

// newMCPClient creates a new MCP client
func newMCPClient(deps dependencies) Component {
	mcpConf := deps.Config.Get()

	// Check if MCP is enabled
	if !mcpConf.Enabled {
		deps.Logger.Info("MCP client is disabled")
		return &mcpClient{
			config: deps.Config,
			logger: deps.Logger,
		}
	}

	// Create MCP client
	client := mcp.NewClient(
		&mcp.Implementation{
			Name:    "Datadog Agent MCP Client",
			Version: "v1.0.0",
		},
		nil,
	)

	return &mcpClient{
		config: deps.Config,
		logger: deps.Logger,
		client: client,
	}
}

// Connect connects to an MCP server at the configured socket path
func (c *mcpClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return fmt.Errorf("MCP client is disabled")
	}

	if c.session != nil {
		return fmt.Errorf("already connected to MCP server")
	}

	mcpConf := c.config.Get()
	socketPath := mcpConf.SocketPath

	c.logger.Infof(
		"Connecting to MCP server at %s",
		socketPath,
	)

	// Dial Unix socket
	conn, err := net.Dial(
		"unix",
		socketPath,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to connect to Unix socket: %w",
			err,
		)
	}

	c.conn = conn

	// Create transport
	transport := &clientTransport{
		conn: conn,
	}

	// Connect to the server
	session, err := c.client.Connect(
		ctx,
		transport,
		nil,
	)
	if err != nil {
		conn.Close()
		c.conn = nil
		return fmt.Errorf(
			"failed to connect to MCP server: %w",
			err,
		)
	}

	c.session = session
	c.logger.Infof("Successfully connected to MCP server")

	return nil
}

// CallTool calls a tool on the connected MCP server
func (c *mcpClient) CallTool(
	ctx context.Context,
	toolName string,
	params map[string]interface{},
) (
	*mcp.CallToolResult,
	error,
) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.session == nil {
		return nil, fmt.Errorf("not connected to MCP server")
	}

	c.logger.Debugf(
		"Calling tool: %s with params: %v",
		toolName,
		params,
	)

	result, err := c.session.CallTool(
		ctx,
		&mcp.CallToolParams{
			Name:      toolName,
			Arguments: params,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to call tool %s: %w",
			toolName,
			err,
		)
	}

	return result, nil
}

// ListTools lists all available tools on the connected MCP server
func (c *mcpClient) ListTools(ctx context.Context) (
	[]*mcp.Tool,
	error,
) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.session == nil {
		return nil, fmt.Errorf("not connected to MCP server")
	}

	c.logger.Debug("Listing available tools")

	result, err := c.session.ListTools(
		ctx,
		&mcp.ListToolsParams{},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to list tools: %w",
			err,
		)
	}

	return result.Tools, nil
}

// Close closes the connection to the MCP server
func (c *mcpClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session == nil {
		return nil
	}

	c.logger.Info("Closing MCP client connection")

	if err := c.session.Close(); err != nil {
		c.logger.Warnf(
			"Error closing MCP session: %v",
			err,
		)
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.logger.Warnf(
				"Error closing connection: %v",
				err,
			)
		}
		c.conn = nil
	}

	c.session = nil
	c.logger.Info("MCP client connection closed")

	return nil
}

// IsConnected returns whether the client is connected to the server
func (c *mcpClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.session != nil
}

// clientTransport implements mcp.Transport using a net.Conn
type clientTransport struct {
	conn net.Conn
}

// Connect implements mcp.Transport
func (t *clientTransport) Connect(ctx context.Context) (
	mcp.Connection,
	error,
) {
	return &clientConnection{
		conn:   t.conn,
		reader: bufio.NewReader(t.conn),
		writer: bufio.NewWriter(t.conn),
	}, nil
}

// clientConnection implements mcp.Connection
type clientConnection struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex
}

// Read implements mcp.Connection.Read
func (c *clientConnection) Read(ctx context.Context) (
	jsonrpc.Message,
	error,
) {
	// Read a JSON-RPC message (newline-delimited)
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	// Parse the JSON-RPC message
	var msg jsonrpc.Message
	if err := json.Unmarshal(
		line,
		&msg,
	); err != nil {
		return nil, fmt.Errorf(
			"failed to parse JSON-RPC message: %w",
			err,
		)
	}

	return msg, nil
}

// Write implements mcp.Connection.Write
func (c *clientConnection) Write(
	ctx context.Context,
	msg jsonrpc.Message,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Serialize the JSON-RPC message
	data, err := json.Marshal(msg)
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
func (c *clientConnection) Close() error {
	return c.conn.Close()
}

// SessionID implements mcp.Connection.SessionID
func (c *clientConnection) SessionID() string {
	return c.conn.LocalAddr().String()
}
