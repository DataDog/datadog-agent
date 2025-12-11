// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package transport provides transport layer implementations for the MCP server.
// It supports multiple transport types including stdio, Unix sockets, and TCP with TLS.
//
// The MCP (Model Context Protocol) uses JSON-RPC 2.0 over various transports.
// This package provides the transport abstraction layer that handles:
// - Connection management (accept, close, multiplex)
// - Message framing (JSON-RPC over newline-delimited JSON)
// - TLS termination for secure transports
// - Authentication hooks for access control
package transport

import (
	"context"
	"io"
	"time"
)

// TransportType identifies the transport implementation.
type TransportType string

const (
	// TransportTypeStdio uses stdin/stdout for communication (single client).
	TransportTypeStdio TransportType = "stdio"

	// TransportTypeUnix uses Unix domain sockets for local IPC.
	TransportTypeUnix TransportType = "unix"

	// TransportTypeTCP uses TCP sockets for network communication.
	TransportTypeTCP TransportType = "tcp"
)

// Transport defines the interface for MCP transport implementations.
// Each transport is responsible for handling the low-level communication
// between MCP clients and the server.
type Transport interface {
	// Start begins listening for connections and handling requests.
	// It blocks until the context is cancelled or an error occurs.
	// The handler is called for each incoming JSON-RPC message.
	Start(ctx context.Context, handler MessageHandler) error

	// Stop gracefully shuts down the transport, closing all connections.
	// It waits up to the configured timeout for active requests to complete.
	Stop(ctx context.Context) error

	// Type returns the transport type identifier.
	Type() TransportType

	// Address returns the address the transport is listening on.
	// For stdio, this returns "stdio". For sockets, it returns the socket path or address.
	Address() string
}

// MessageHandler processes incoming MCP messages and returns responses.
// This is the callback interface that transports use to delegate message
// processing to the MCP server.
type MessageHandler interface {
	// HandleMessage processes a raw JSON-RPC message and returns the response.
	// The implementation should handle all MCP protocol messages including
	// tool calls, list requests, and protocol negotiation.
	//
	// The connInfo parameter provides information about the connection that
	// received this message, which can be used for logging or authentication.
	HandleMessage(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error)
}

// MessageHandlerFunc is a function adapter for MessageHandler.
type MessageHandlerFunc func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error)

// HandleMessage implements MessageHandler.
func (f MessageHandlerFunc) HandleMessage(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
	return f(ctx, message, connInfo)
}

// ConnectionInfo provides metadata about a connection.
type ConnectionInfo struct {
	// ID is a unique identifier for this connection.
	ID string

	// RemoteAddr is the remote address of the connection (if applicable).
	// For Unix sockets, this is the socket path. For stdio, this is "stdio".
	RemoteAddr string

	// Transport is the type of transport this connection uses.
	Transport TransportType

	// ConnectedAt is the time the connection was established.
	ConnectedAt time.Time

	// Authenticated indicates whether the connection has been authenticated.
	Authenticated bool

	// AuthIdentity is the authenticated identity (e.g., client certificate CN).
	AuthIdentity string
}

// Connection represents a single client connection to the MCP server.
// It provides read/write capabilities and connection metadata.
type Connection interface {
	io.ReadWriteCloser

	// Info returns metadata about this connection.
	Info() ConnectionInfo

	// SetDeadline sets the read and write deadlines.
	SetDeadline(t time.Time) error

	// SetReadDeadline sets the read deadline.
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline sets the write deadline.
	SetWriteDeadline(t time.Time) error
}

// ConnectionHandler is called when connection lifecycle events occur.
// It allows for connection-level processing such as authentication and logging.
type ConnectionHandler interface {
	// OnConnect is called when a new connection is established.
	// Returning an error will reject the connection.
	OnConnect(ctx context.Context, conn Connection) error

	// OnDisconnect is called when a connection is closed.
	OnDisconnect(ctx context.Context, conn Connection)
}

// Authenticator validates client credentials for authenticated transports.
type Authenticator interface {
	// Authenticate validates the connection and returns the authenticated identity.
	// The credentials parameter contains transport-specific authentication data
	// (e.g., TLS client certificate, token, etc.).
	//
	// Returns the authenticated identity string and nil error on success.
	// Returns an empty string and error if authentication fails.
	Authenticate(ctx context.Context, conn Connection, credentials interface{}) (string, error)
}

// TransportConfig holds common configuration for all transports.
type TransportConfig struct {
	// MaxMessageSize is the maximum size of a single message in bytes.
	// Default: 10MB
	MaxMessageSize int

	// ReadTimeout is the timeout for read operations in seconds.
	// Default: 30 seconds
	ReadTimeout int

	// WriteTimeout is the timeout for write operations in seconds.
	// Default: 30 seconds
	WriteTimeout int

	// MaxConnections is the maximum number of concurrent connections (0 = unlimited).
	// Default: 100
	MaxConnections int

	// ShutdownTimeout is the timeout for graceful shutdown in seconds.
	// Default: 30 seconds
	ShutdownTimeout int

	// ConnectionHandler is called for connection lifecycle events.
	// Optional: if nil, no lifecycle callbacks are made.
	ConnectionHandler ConnectionHandler

	// Authenticator validates client credentials.
	// Optional: if nil, all connections are accepted.
	Authenticator Authenticator
}

// DefaultConfig returns a TransportConfig with sensible defaults.
func DefaultConfig() TransportConfig {
	return TransportConfig{
		MaxMessageSize:  10 * 1024 * 1024, // 10MB
		ReadTimeout:     30,               // 30 seconds
		WriteTimeout:    30,               // 30 seconds
		MaxConnections:  100,
		ShutdownTimeout: 30, // 30 seconds
	}
}

// TLSConfig holds TLS configuration for secure transports.
type TLSConfig struct {
	// Enabled indicates whether TLS is enabled.
	Enabled bool

	// CertFile is the path to the server certificate file.
	CertFile string

	// KeyFile is the path to the server private key file.
	KeyFile string

	// CAFile is the path to the CA certificate file for client authentication.
	// If set, client certificates will be verified against this CA.
	CAFile string

	// ClientAuth specifies the policy for client certificate authentication.
	// Values: "none", "request", "require", "verify", "require-and-verify"
	ClientAuth string

	// MinVersion is the minimum TLS version (e.g., "1.2", "1.3").
	MinVersion string
}

// UnixConfig holds configuration specific to Unix socket transports.
type UnixConfig struct {
	TransportConfig

	// Path is the path to the Unix socket file.
	Path string

	// Mode is the file mode for the socket file (e.g., 0600).
	Mode uint32

	// RemoveExisting indicates whether to remove an existing socket file.
	RemoveExisting bool
}

// TCPConfig holds configuration specific to TCP transports.
type TCPConfig struct {
	TransportConfig

	// Address is the address to listen on (e.g., "localhost:7890", ":7890").
	Address string

	// TLS is the TLS configuration for secure connections.
	TLS *TLSConfig
}

// StdioConfig holds configuration specific to stdio transports.
type StdioConfig struct {
	TransportConfig
}
