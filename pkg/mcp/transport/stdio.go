// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transport

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StdioTransport implements Transport for stdin/stdout communication.
// This is useful for running the MCP server as a subprocess where the
// parent process communicates via pipes.
type StdioTransport struct {
	config StdioConfig
	reader io.Reader
	writer io.Writer

	mu      sync.Mutex
	started bool
	done    chan struct{}
}

// NewStdioTransport creates a new stdio transport using os.Stdin and os.Stdout.
func NewStdioTransport(config StdioConfig) *StdioTransport {
	return NewStdioTransportWithIO(config, os.Stdin, os.Stdout)
}

// NewStdioTransportWithIO creates a new stdio transport with custom reader/writer.
// This is useful for testing.
func NewStdioTransportWithIO(config StdioConfig, reader io.Reader, writer io.Writer) *StdioTransport {
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

	return &StdioTransport{
		config: config,
		reader: reader,
		writer: writer,
		done:   make(chan struct{}),
	}
}

// Start begins reading from stdin and processing messages.
// It blocks until the context is cancelled or EOF is reached.
func (t *StdioTransport) Start(ctx context.Context, handler MessageHandler) error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("stdio transport already started")
	}
	t.started = true
	t.mu.Unlock()

	connInfo := ConnectionInfo{
		ID:          "stdio",
		RemoteAddr:  "stdio",
		Transport:   TransportTypeStdio,
		ConnectedAt: time.Now(),
	}

	// Notify connection handler if configured
	if t.config.ConnectionHandler != nil {
		conn := &stdioConnection{
			reader:   t.reader,
			writer:   t.writer,
			connInfo: connInfo,
		}
		if err := t.config.ConnectionHandler.OnConnect(ctx, conn); err != nil {
			return fmt.Errorf("connection handler rejected stdio connection: %w", err)
		}
		defer t.config.ConnectionHandler.OnDisconnect(ctx, conn)
	}

	// Create a buffered reader for line-based reading
	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, t.config.MaxMessageSize), t.config.MaxMessageSize)

	// Process messages until context cancellation or EOF
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.done:
			return nil
		default:
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("error reading from stdin: %w", err)
				}
				// EOF reached
				return nil
			}

			message := scanner.Bytes()
			if len(message) == 0 {
				continue
			}

			// Handle the message
			response, err := handler.HandleMessage(ctx, message, connInfo)
			if err != nil {
				log.Warnf("MCP stdio transport: error handling message: %v", err)
				continue
			}

			// Write response if there is one (notifications don't have responses)
			if response != nil {
				if err := t.writeResponse(response); err != nil {
					log.Warnf("MCP stdio transport: error writing response: %v", err)
				}
			}
		}
	}
}

// writeResponse writes a JSON response followed by a newline.
func (t *StdioTransport) writeResponse(response []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Write response followed by newline
	if _, err := t.writer.Write(response); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}
	if _, err := t.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush if possible
	if flusher, ok := t.writer.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return fmt.Errorf("failed to flush: %w", err)
		}
	}

	return nil
}

// Stop shuts down the stdio transport.
func (t *StdioTransport) Stop(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.started {
		return nil
	}

	close(t.done)
	t.started = false
	return nil
}

// Type returns the transport type.
func (t *StdioTransport) Type() TransportType {
	return TransportTypeStdio
}

// Address returns "stdio" for this transport.
func (t *StdioTransport) Address() string {
	return "stdio"
}

// stdioConnection wraps stdin/stdout as a Connection.
type stdioConnection struct {
	reader   io.Reader
	writer   io.Writer
	connInfo ConnectionInfo
}

func (c *stdioConnection) Read(p []byte) (n int, err error) {
	return c.reader.Read(p)
}

func (c *stdioConnection) Write(p []byte) (n int, err error) {
	return c.writer.Write(p)
}

func (c *stdioConnection) Close() error {
	// Can't close stdin/stdout
	return nil
}

func (c *stdioConnection) Info() ConnectionInfo {
	return c.connInfo
}

func (c *stdioConnection) SetDeadline(_ time.Time) error {
	// Deadlines not supported for stdio
	return nil
}

func (c *stdioConnection) SetReadDeadline(_ time.Time) error {
	return nil
}

func (c *stdioConnection) SetWriteDeadline(_ time.Time) error {
	return nil
}
