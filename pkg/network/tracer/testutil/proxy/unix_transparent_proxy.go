// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package proxy provides a unix transparent proxy server that can be used for testing.
package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultDialTimeout      = 15 * time.Second
	connectionRetries       = 30
	connectionRetryInterval = 100 * time.Millisecond
)

// UnixTransparentProxyServer is a proxy server that listens on a unix socket, and forwards all incoming and outgoing traffic
// to a remote address.
type UnixTransparentProxyServer struct {
	// unixPath is the path to the unix socket to listen on.
	unixPath string
	// remoteAddr is the address to forward all traffic to.
	remoteAddr string
	// useTLS indicates whether the proxy should use TLS to connect to the remote address.
	useTLS bool
	// isReady is a flag indicating whether the server is ready to accept connections.
	isReady atomic.Bool
	// wg is a wait group used to wait for the server to stop.
	wg sync.WaitGroup
	// ln is the listener used by the server.
	ln net.Listener
}

// NewUnixTransparentProxyServer returns a new instance of a UnixTransparentProxyServer.
func NewUnixTransparentProxyServer(unixPath, remoteAddr string, useTLS bool) *UnixTransparentProxyServer {
	return &UnixTransparentProxyServer{
		unixPath:   unixPath,
		remoteAddr: remoteAddr,
		useTLS:     useTLS,
	}
}

// Run starts the proxy server.
func (p *UnixTransparentProxyServer) Run() error {
	// Clear the old socket if it exists.
	if err := p.clearOldSocket(); err != nil {
		return err
	}

	ln, err := net.Listen("unix", p.unixPath)
	if err != nil {
		return err
	}
	p.ln = ln
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.isReady.Store(true)

		for {
			unixSocketConn, err := ln.Accept()
			if err != nil {
				// We can get this error when the listener is closed, during shutdown.
				if !errors.Is(err, net.ErrClosed) {
					log.Errorf("failed accepting connection: %s", err)
				}
				return
			}
			go p.handleConnection(unixSocketConn)
		}
	}()

	return nil
}

// Stop stops the proxy server.
func (p *UnixTransparentProxyServer) Stop() {
	defer func() { _ = p.clearOldSocket() }()

	_ = p.ln.Close()
	p.wg.Wait()
}

// WaitUntilServerIsReady blocks until the server is ready to accept connections.
func (p *UnixTransparentProxyServer) WaitUntilServerIsReady() {
	for !p.isReady.Load() {
		time.Sleep(10 * time.Millisecond)
	}
}

// WaitForConnectionReady blocks until the server is ready to accept connections.
// meant for external servers.
func WaitForConnectionReady(unixSocket string) error {
	for i := 0; i < connectionRetries; i++ {
		c, err := net.DialTimeout("unix", unixSocket, time.Second)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(connectionRetryInterval)
	}

	return fmt.Errorf("could not connect %q after %d retries (after %v)", unixSocket, connectionRetries, connectionRetryInterval*connectionRetries)
}

// handleConnection handles a new connection, by forwarding all traffic to the remote address.
func (p *UnixTransparentProxyServer) handleConnection(unixSocketConn net.Conn) {
	defer unixSocketConn.Close()

	var remoteConn net.Conn
	var err error
	if p.useTLS {
		timedContext, cancel := context.WithTimeout(context.Background(), defaultDialTimeout)
		dialer := &tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}
		remoteConn, err = dialer.DialContext(timedContext, "tcp", p.remoteAddr)
		cancel()
	} else {
		remoteConn, err = net.DialTimeout("tcp", p.remoteAddr, defaultDialTimeout)
	}
	if err != nil {
		log.Errorf("failed to dial remote: %s", err)
		return
	}
	defer remoteConn.Close()

	var streamWait sync.WaitGroup
	streamWait.Add(2)

	streamConn := func(dst io.Writer, src io.Reader) {
		defer streamWait.Done()
		_, _ = io.Copy(dst, src)
	}

	go streamConn(remoteConn, unixSocketConn)
	go streamConn(unixSocketConn, remoteConn)

	streamWait.Wait()
}

// clearOldSocket clears the old socket if it exists.
func (p *UnixTransparentProxyServer) clearOldSocket() error {
	pipe, err := os.Stat(p.unixPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	mode := pipe.Mode()
	if os.ModeSocket&mode != 0 { // is a socket
		return os.Remove(p.unixPath)
	}
	return fmt.Errorf("%q exists but it is not a socket", p.unixPath)
}
