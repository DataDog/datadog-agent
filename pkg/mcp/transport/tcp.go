// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transport

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TCPTransport implements Transport for TCP socket communication with optional TLS.
// It supports multiple concurrent client connections and TLS encryption.
type TCPTransport struct {
	config    TCPConfig
	listener  net.Listener
	tlsConfig *tls.Config

	mu         sync.Mutex
	started    bool
	done       chan struct{}
	conns      map[string]*tcpConn
	connsMu    sync.RWMutex
	connCount  int32
	nextConnID int64
}

// NewTCPTransport creates a new TCP transport.
func NewTCPTransport(config TCPConfig) (*TCPTransport, error) {
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

	t := &TCPTransport{
		config: config,
		done:   make(chan struct{}),
		conns:  make(map[string]*tcpConn),
	}

	// Configure TLS if enabled
	if config.TLS != nil && config.TLS.Enabled {
		tlsConfig, err := buildTLSConfig(config.TLS)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		t.tlsConfig = tlsConfig
	}

	return t, nil
}

// buildTLSConfig creates a tls.Config from the TLSConfig.
func buildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil, fmt.Errorf("TLS enabled but cert_file or key_file not specified")
	}

	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Configure minimum TLS version
	switch cfg.MinVersion {
	case "1.3":
		tlsConfig.MinVersion = tls.VersionTLS13
	case "1.2", "":
		tlsConfig.MinVersion = tls.VersionTLS12
	case "1.1":
		tlsConfig.MinVersion = tls.VersionTLS11
	case "1.0":
		tlsConfig.MinVersion = tls.VersionTLS10
	default:
		return nil, fmt.Errorf("invalid TLS version: %s", cfg.MinVersion)
	}

	// Configure client authentication
	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.ClientCAs = caCertPool
	}

	// Set client auth policy
	switch cfg.ClientAuth {
	case "none", "":
		tlsConfig.ClientAuth = tls.NoClientCert
	case "request":
		tlsConfig.ClientAuth = tls.RequestClientCert
	case "require":
		tlsConfig.ClientAuth = tls.RequireAnyClientCert
	case "verify":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case "require-and-verify":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	default:
		return nil, fmt.Errorf("invalid client auth policy: %s", cfg.ClientAuth)
	}

	return tlsConfig, nil
}

// Start begins listening on the TCP socket and processing messages.
// It blocks until the context is cancelled or an error occurs.
func (t *TCPTransport) Start(ctx context.Context, handler MessageHandler) error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("TCP transport already started")
	}
	t.started = true
	t.mu.Unlock()

	// Create the TCP listener
	var listener net.Listener
	var err error

	if t.tlsConfig != nil {
		listener, err = tls.Listen("tcp", t.config.Address, t.tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to create TLS listener on %s: %w", t.config.Address, err)
		}
		log.Infof("MCP TCP transport: listening on %s with TLS", t.config.Address)
	} else {
		listener, err = net.Listen("tcp", t.config.Address)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", t.config.Address, err)
		}
		log.Infof("MCP TCP transport: listening on %s", t.config.Address)
	}
	t.listener = listener

	// Accept connections in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- t.acceptLoop(ctx, handler)
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return t.shutdown(ctx)
	case <-t.done:
		return nil
	case err := <-errCh:
		return err
	}
}

// acceptLoop accepts and handles incoming connections.
func (t *TCPTransport) acceptLoop(ctx context.Context, handler MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.done:
			return nil
		default:
			// Set accept deadline to allow checking for shutdown
			if tcpListener, ok := t.listener.(*net.TCPListener); ok {
				if err := tcpListener.SetDeadline(time.Now().Add(time.Second)); err != nil {
					log.Warnf("MCP TCP transport: failed to set accept deadline: %v", err)
				}
			}

			conn, err := t.listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				select {
				case <-t.done:
					return nil
				default:
					return fmt.Errorf("accept error: %w", err)
				}
			}

			// Check connection limit
			if t.config.MaxConnections > 0 && atomic.LoadInt32(&t.connCount) >= int32(t.config.MaxConnections) {
				log.Warnf("MCP TCP transport: connection limit reached (%d), rejecting connection from %s",
					t.config.MaxConnections, conn.RemoteAddr())
				conn.Close()
				continue
			}

			go t.handleConnection(ctx, conn, handler)
		}
	}
}

// handleConnection processes a single client connection.
func (t *TCPTransport) handleConnection(ctx context.Context, netConn net.Conn, handler MessageHandler) {
	connID := fmt.Sprintf("tcp-%d", atomic.AddInt64(&t.nextConnID, 1))
	atomic.AddInt32(&t.connCount, 1)
	defer atomic.AddInt32(&t.connCount, -1)

	connInfo := ConnectionInfo{
		ID:          connID,
		RemoteAddr:  netConn.RemoteAddr().String(),
		Transport:   TransportTypeTCP,
		ConnectedAt: time.Now(),
	}

	// Extract authentication info from TLS connection
	if tlsConn, ok := netConn.(*tls.Conn); ok {
		// Perform handshake to get connection state
		if err := tlsConn.Handshake(); err != nil {
			log.Warnf("MCP TCP transport: TLS handshake failed for %s: %v", connID, err)
			netConn.Close()
			return
		}

		state := tlsConn.ConnectionState()
		if len(state.PeerCertificates) > 0 {
			connInfo.Authenticated = true
			connInfo.AuthIdentity = state.PeerCertificates[0].Subject.CommonName
		}
	}

	tc := &tcpConn{
		Conn:     netConn,
		connInfo: connInfo,
	}

	// Track the connection
	t.connsMu.Lock()
	t.conns[connID] = tc
	t.connsMu.Unlock()

	defer func() {
		t.connsMu.Lock()
		delete(t.conns, connID)
		t.connsMu.Unlock()
		netConn.Close()
	}()

	// Notify connection handler if configured
	if t.config.ConnectionHandler != nil {
		if err := t.config.ConnectionHandler.OnConnect(ctx, tc); err != nil {
			log.Warnf("MCP TCP transport: connection handler rejected connection %s: %v", connID, err)
			return
		}
		defer t.config.ConnectionHandler.OnDisconnect(ctx, tc)
	}

	// Authenticate if authenticator is configured
	if t.config.Authenticator != nil {
		identity, err := t.config.Authenticator.Authenticate(ctx, tc, nil)
		if err != nil {
			log.Warnf("MCP TCP transport: authentication failed for %s: %v", connID, err)
			return
		}
		connInfo.Authenticated = true
		connInfo.AuthIdentity = identity
	}

	log.Debugf("MCP TCP transport: accepted connection %s from %s", connID, netConn.RemoteAddr())

	// Create buffered reader for line-based reading
	scanner := bufio.NewScanner(netConn)
	scanner.Buffer(make([]byte, t.config.MaxMessageSize), t.config.MaxMessageSize)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		default:
			// Set read deadline
			if t.config.ReadTimeout > 0 {
				if err := netConn.SetReadDeadline(time.Now().Add(time.Duration(t.config.ReadTimeout) * time.Second)); err != nil {
					log.Warnf("MCP TCP transport: failed to set read deadline: %v", err)
				}
			}

			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						continue
					}
					log.Debugf("MCP TCP transport: connection %s read error: %v", connID, err)
				}
				return
			}

			message := scanner.Bytes()
			if len(message) == 0 {
				continue
			}

			// Handle the message
			response, err := handler.HandleMessage(ctx, message, connInfo)
			if err != nil {
				log.Warnf("MCP TCP transport: error handling message from %s: %v", connID, err)
				continue
			}

			// Write response if there is one
			if response != nil {
				if err := t.writeResponse(netConn, response); err != nil {
					log.Warnf("MCP TCP transport: error writing response to %s: %v", connID, err)
					return
				}
			}
		}
	}
}

// writeResponse writes a JSON response followed by a newline.
func (t *TCPTransport) writeResponse(conn net.Conn, response []byte) error {
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
func (t *TCPTransport) shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.started {
		return nil
	}

	// Signal all goroutines to stop
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

	t.started = false
	return nil
}

// Stop gracefully shuts down the TCP transport.
func (t *TCPTransport) Stop(ctx context.Context) error {
	return t.shutdown(ctx)
}

// Type returns the transport type.
func (t *TCPTransport) Type() TransportType {
	return TransportTypeTCP
}

// Address returns the TCP address the transport is listening on.
func (t *TCPTransport) Address() string {
	if t.listener != nil {
		return t.listener.Addr().String()
	}
	return t.config.Address
}

// ActiveConnections returns the number of active connections.
func (t *TCPTransport) ActiveConnections() int {
	return int(atomic.LoadInt32(&t.connCount))
}

// IsTLSEnabled returns whether TLS is enabled for this transport.
func (t *TCPTransport) IsTLSEnabled() bool {
	return t.tlsConfig != nil
}

// tcpConn wraps a net.Conn as a Connection.
type tcpConn struct {
	net.Conn
	connInfo ConnectionInfo
}

func (c *tcpConn) Info() ConnectionInfo {
	return c.connInfo
}
