// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// UnixTransport implements Transport for Unix domain socket communication.
// It supports multiple concurrent client connections.
type UnixTransport struct {
	config   UnixConfig
	listener net.Listener

	mu         sync.Mutex
	started    bool
	done       chan struct{}
	conns      map[string]*unixConn
	connsMu    sync.RWMutex
	connCount  int32
	nextConnID int64
}

// NewUnixTransport creates a new Unix socket transport.
func NewUnixTransport(config UnixConfig) *UnixTransport {
	// Apply defaults
	if config.MaxMessageSize == 0 {
		config.MaxMessageSize = DefaultConfig().MaxMessageSize
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = DefaultConfig().ReadTimeout
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = DefaultConfig().WriteTimeout
	}
	if config.MaxConnections == 0 {
		config.MaxConnections = DefaultConfig().MaxConnections
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = DefaultConfig().ShutdownTimeout
	}
	if config.Mode == 0 {
		config.Mode = 0666 // Default to world-readable/writable for MCP clients
	}

	return &UnixTransport{
		config: config,
		done:   make(chan struct{}),
		conns:  make(map[string]*unixConn),
	}
}

// Start begins listening on the Unix socket and processing messages.
// It blocks until the context is cancelled or an error occurs.
func (t *UnixTransport) Start(ctx context.Context, handler MessageHandler) error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("unix transport already started")
	}
	t.started = true
	t.mu.Unlock()

	// Ensure parent directory exists
	socketDir := filepath.Dir(t.config.Path)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory %s: %w", socketDir, err)
	}

	// Remove existing socket if configured
	if t.config.RemoveExisting {
		if err := os.Remove(t.config.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove existing socket: %w", err)
		}
	}

	// Create the Unix socket listener
	listener, err := net.Listen("unix", t.config.Path)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket %s: %w", t.config.Path, err)
	}
	t.listener = listener

	// Set socket permissions
	if err := os.Chmod(t.config.Path, os.FileMode(t.config.Mode)); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	log.Infof("MCP unix transport: listening on %s", t.config.Path)

	// Accept connections in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- t.acceptLoop(ctx, handler)
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		log.Infof("MCP unix transport: context cancelled, shutting down")
		return t.shutdown(ctx)
	case <-t.done:
		log.Infof("MCP unix transport: done channel closed, stopping")
		return nil
	case err := <-errCh:
		log.Infof("MCP unix transport: acceptLoop returned error: %v", err)
		return err
	}
}

// acceptLoop accepts and handles incoming connections.
func (t *UnixTransport) acceptLoop(ctx context.Context, handler MessageHandler) error {
	log.Info("MCP unix transport: acceptLoop started, waiting for connections")
	for {
		select {
		case <-ctx.Done():
			log.Info("MCP unix transport: acceptLoop context done")
			return ctx.Err()
		case <-t.done:
			log.Info("MCP unix transport: acceptLoop done channel closed")
			return nil
		default:
			// Set accept deadline to allow checking for shutdown
			if err := t.listener.(*net.UnixListener).SetDeadline(time.Now().Add(time.Second)); err != nil {
				log.Warnf("MCP unix transport: failed to set accept deadline: %v", err)
			}

			conn, err := t.listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				select {
				case <-t.done:
					log.Info("MCP unix transport: acceptLoop done during accept error")
					return nil
				default:
					log.Warnf("MCP unix transport: acceptLoop accept error: %v", err)
					return fmt.Errorf("accept error: %w", err)
				}
			}

			// Check connection limit
			currentConns := atomic.LoadInt32(&t.connCount)
			log.Infof("MCP unix transport: new connection from %s (current connections: %d)", conn.RemoteAddr(), currentConns)
			if t.config.MaxConnections > 0 && currentConns >= int32(t.config.MaxConnections) {
				log.Warnf("MCP unix transport: connection limit reached (%d), rejecting connection", t.config.MaxConnections)
				conn.Close()
				continue
			}

			go t.handleConnection(ctx, conn, handler)
		}
	}
}

// handleConnection processes a single client connection.
func (t *UnixTransport) handleConnection(ctx context.Context, netConn net.Conn, handler MessageHandler) {
	connID := fmt.Sprintf("unix-%d", atomic.AddInt64(&t.nextConnID, 1))
	atomic.AddInt32(&t.connCount, 1)
	defer atomic.AddInt32(&t.connCount, -1)

	connInfo := ConnectionInfo{
		ID:          connID,
		RemoteAddr:  netConn.RemoteAddr().String(),
		Transport:   TransportTypeUnix,
		ConnectedAt: time.Now(),
	}

	uc := &unixConn{
		Conn:     netConn,
		connInfo: connInfo,
	}

	// Track the connection
	t.connsMu.Lock()
	t.conns[connID] = uc
	t.connsMu.Unlock()

	defer func() {
		t.connsMu.Lock()
		delete(t.conns, connID)
		t.connsMu.Unlock()
		netConn.Close()
	}()

	// Notify connection handler if configured
	if t.config.ConnectionHandler != nil {
		if err := t.config.ConnectionHandler.OnConnect(ctx, uc); err != nil {
			log.Warnf("MCP unix transport: connection handler rejected connection %s: %v", connID, err)
			return
		}
		defer t.config.ConnectionHandler.OnDisconnect(ctx, uc)
	}

	log.Infof("MCP unix transport: accepted connection %s from %s", connID, netConn.RemoteAddr().String())

	// Create buffered reader for line-based reading
	reader := bufio.NewReader(netConn)

	messageCount := 0
	for {
		select {
		case <-ctx.Done():
			log.Infof("MCP unix transport: connection %s loop exiting (context done)", connID)
			return
		case <-t.done:
			log.Infof("MCP unix transport: connection %s loop exiting (done channel)", connID)
			return
		default:
			// Set read deadline
			if t.config.ReadTimeout > 0 {
				deadline := time.Now().Add(time.Duration(t.config.ReadTimeout) * time.Second)
				if err := netConn.SetReadDeadline(deadline); err != nil {
					log.Warnf("MCP unix transport: failed to set read deadline: %v", err)
				}
			}

			log.Debugf("MCP unix transport: connection %s waiting for next message (message #%d, timeout=%ds)", connID, messageCount+1, t.config.ReadTimeout)
			message, err := reader.ReadBytes('\n')
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Timeout is expected - just continue waiting for the next message
					// Clear the deadline before continuing to avoid issues
					netConn.SetReadDeadline(time.Time{})
					continue
				}
				// EOF or connection reset means the client disconnected
				if errors.Is(err, io.EOF) {
					log.Infof("MCP unix transport: connection %s closed (client disconnected after %d messages)", connID, messageCount)
				} else {
					log.Infof("MCP unix transport: connection %s read error: %v", connID, err)
				}
				return
			}

			// Trim the newline
			if len(message) > 0 && message[len(message)-1] == '\n' {
				message = message[:len(message)-1]
			}

			if len(message) == 0 {
				log.Debugf("MCP unix transport: connection %s received empty message, skipping", connID)
				continue
			}

			messageCount++
			log.Infof("MCP unix transport: connection %s received message #%d (%d bytes)", connID, messageCount, len(message))

			// Handle the message
			response, err := handler.HandleMessage(ctx, message, connInfo)
			if err != nil {
				log.Warnf("MCP unix transport: error handling message from %s: %v", connID, err)
				continue
			}

			log.Infof("MCP unix transport: connection %s message #%d handled, response=%d bytes", connID, messageCount, len(response))

			// Write response if there is one
			if response != nil {
				if err := t.writeResponse(netConn, response); err != nil {
					log.Warnf("MCP unix transport: error writing response to %s: %v", connID, err)
					return
				}
				log.Infof("MCP unix transport: connection %s message #%d response written", connID, messageCount)
			}
		}
	}
}

// writeResponse writes a JSON response followed by a newline.
func (t *UnixTransport) writeResponse(conn net.Conn, response []byte) error {
	if t.config.WriteTimeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(time.Duration(t.config.WriteTimeout) * time.Second)); err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	// Write response followed by newline
	if _, err := conn.Write(response); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}
	if _, err := conn.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// shutdown gracefully shuts down the transport.
func (t *UnixTransport) shutdown(ctx context.Context) error {
	log.Info("MCP unix transport: shutdown called")
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.started {
		log.Debug("MCP unix transport: shutdown called but not started")
		return nil
	}

	// Signal all goroutines to stop
	log.Debug("MCP unix transport: closing done channel")
	close(t.done)

	// Close the listener
	if t.listener != nil {
		t.listener.Close()
	}

	// Close all active connections
	t.connsMu.Lock()
	for _, conn := range t.conns {
		conn.Close()
	}
	t.connsMu.Unlock()

	// Remove the socket file
	if t.config.Path != "" {
		os.Remove(t.config.Path)
	}

	t.started = false
	return nil
}

// Stop gracefully shuts down the Unix transport.
func (t *UnixTransport) Stop(ctx context.Context) error {
	return t.shutdown(ctx)
}

// Type returns the transport type.
func (t *UnixTransport) Type() TransportType {
	return TransportTypeUnix
}

// Address returns the socket path.
func (t *UnixTransport) Address() string {
	return t.config.Path
}

// ActiveConnections returns the number of active connections.
func (t *UnixTransport) ActiveConnections() int {
	return int(atomic.LoadInt32(&t.connCount))
}

// unixConn wraps a net.Conn as a Connection.
type unixConn struct {
	net.Conn
	connInfo ConnectionInfo
}

func (c *unixConn) Info() ConnectionInfo {
	return c.connInfo
}
