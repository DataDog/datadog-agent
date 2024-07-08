// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package testutil is test utilities for testing the network tracer
package testutil

import "net"

// TCPServer is a simple TCP server for use in tests
type TCPServer struct {
	address   string
	Network   string
	onMessage func(c net.Conn)
	ln        net.Listener
}

// NewTCPServer creates a TCPServer using the provided function for newly accepted connections.
// It defaults to listening on an ephemeral port on 127.0.0.1
func NewTCPServer(onMessage func(c net.Conn)) *TCPServer {
	return NewTCPServerOnAddress("127.0.0.1:0", onMessage)
}

// NewTCPServerOnAddress creates a TCPServer using the provided address.
func NewTCPServerOnAddress(addr string, onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

// Address returns the address of the server. This should be called after Run.
func (t *TCPServer) Address() string {
	return t.address
}

// Addr is the raw net.Addr of the listener
func (t *TCPServer) Addr() net.Addr {
	return t.ln.Addr()
}

// Run starts the TCP server
func (t *TCPServer) Run() error {
	networkType := "tcp"
	if t.Network != "" {
		networkType = t.Network
	}
	ln, err := net.Listen(networkType, t.address)
	if err != nil {
		return err
	}
	t.ln = ln
	t.address = ln.Addr().String()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go t.onMessage(conn)
		}
	}()

	return nil
}

// Shutdown stops the TCP server
func (t *TCPServer) Shutdown() {
	if t.ln != nil {
		_ = t.ln.Close()
		t.ln = nil
	}
}
